package storage

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// MemoryPool manages memory allocation for cache stores with pressure detection
// This implementation provides O(1) operations and thread-safe memory management
type MemoryPool struct {
	name         string
	maxSize      int64             // Maximum memory this pool can allocate
	currentUsage int64             // Current memory usage (atomic for thread safety)
	allocations  map[uintptr]int64 // Track allocations for proper cleanup
	mutex        sync.RWMutex      // Protect allocations map

	// Memory pressure thresholds
	warningThreshold  float64 // 0.85 - Start getting nervous
	criticalThreshold float64 // 0.90 - Begin aggressive cleanup
	panicThreshold    float64 // 0.95 - Emergency eviction mode

	// Statistics and monitoring
	totalAllocations   int64     // Total number of allocations made
	totalDeallocations int64     // Total number of deallocations made
	allocationFailures int64     // Number of failed allocations
	lastCleanup        time.Time // Last time pressure-based cleanup occurred

	// Memory pressure callbacks
	onWarningPressure  func(float64) // Called when pressure exceeds warning threshold
	onCriticalPressure func(float64) // Called when pressure exceeds critical threshold
	onPanicPressure    func(float64) // Called when pressure exceeds panic threshold
}

// NewMemoryPool creates a new memory pool with the specified maximum size
func NewMemoryPool(name string, maxSize int64) *MemoryPool {
	pool := &MemoryPool{
		name:              name,
		maxSize:           maxSize,
		currentUsage:      0,
		allocations:       make(map[uintptr]int64),
		warningThreshold:  0.85,
		criticalThreshold: 0.90,
		panicThreshold:    0.95,
		lastCleanup:       time.Now(),
	}

	// Set default pressure handlers
	pool.onWarningPressure = pool.defaultWarningHandler
	pool.onCriticalPressure = pool.defaultCriticalHandler
	pool.onPanicPressure = pool.defaultPanicHandler

	return pool
}

// Allocate requests memory from the pool - MUST be O(1)
func (mp *MemoryPool) Allocate(size int64) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid allocation size: %d", size)
	}

	// Check if allocation would exceed maximum
	currentUsage := atomic.LoadInt64(&mp.currentUsage)
	if currentUsage+size > mp.maxSize {
		atomic.AddInt64(&mp.allocationFailures, 1)
		return nil, fmt.Errorf("allocation would exceed pool limit: %d + %d > %d",
			currentUsage, size, mp.maxSize)
	}

	// Allocate memory
	data := make([]byte, size)
	if data == nil {
		atomic.AddInt64(&mp.allocationFailures, 1)
		return nil, fmt.Errorf("failed to allocate %d bytes", size)
	}

	// Track the allocation
	ptr := uintptr(unsafe.Pointer(&data[0]))
	mp.mutex.Lock()
	mp.allocations[ptr] = size
	mp.mutex.Unlock()

	// Update usage atomically
	newUsage := atomic.AddInt64(&mp.currentUsage, size)
	atomic.AddInt64(&mp.totalAllocations, 1)

	// Check memory pressure and trigger callbacks if needed
	mp.checkMemoryPressure(float64(newUsage) / float64(mp.maxSize))

	return data, nil
}

// Free releases memory back to the pool - MUST be O(1)
func (mp *MemoryPool) Free(ptr []byte) error {
	if len(ptr) == 0 {
		return fmt.Errorf("cannot free nil or empty slice")
	}

	// Find the allocation size
	ptrKey := uintptr(unsafe.Pointer(&ptr[0]))
	mp.mutex.Lock()
	size, exists := mp.allocations[ptrKey]
	if !exists {
		mp.mutex.Unlock()
		return fmt.Errorf("attempt to free untracked memory")
	}
	delete(mp.allocations, ptrKey)
	mp.mutex.Unlock()

	// Update usage atomically
	atomic.AddInt64(&mp.currentUsage, -size)
	atomic.AddInt64(&mp.totalDeallocations, 1)

	return nil
}

// CurrentUsage returns current memory usage - O(1)
func (mp *MemoryPool) CurrentUsage() int64 {
	return atomic.LoadInt64(&mp.currentUsage)
}

// MaxSize returns maximum pool size - O(1)
func (mp *MemoryPool) MaxSize() int64 {
	return mp.maxSize
}

// AvailableSpace returns available memory - O(1)
func (mp *MemoryPool) AvailableSpace() int64 {
	return mp.maxSize - atomic.LoadInt64(&mp.currentUsage)
}

// MemoryPressure calculates current memory pressure (0.0 to 1.0) - O(1)
func (mp *MemoryPool) MemoryPressure() float64 {
	return float64(atomic.LoadInt64(&mp.currentUsage)) / float64(mp.maxSize)
}

// checkMemoryPressure evaluates current pressure and triggers appropriate callbacks
func (mp *MemoryPool) checkMemoryPressure(pressure float64) {
	if pressure >= mp.panicThreshold && mp.onPanicPressure != nil {
		go mp.onPanicPressure(pressure) // Async to avoid blocking allocation
	} else if pressure >= mp.criticalThreshold && mp.onCriticalPressure != nil {
		go mp.onCriticalPressure(pressure) // Async to avoid blocking allocation
	} else if pressure >= mp.warningThreshold && mp.onWarningPressure != nil {
		go mp.onWarningPressure(pressure) // Async to avoid blocking allocation
	}
}

// SetPressureThresholds allows customization of pressure detection levels
func (mp *MemoryPool) SetPressureThresholds(warning, critical, panic float64) error {
	if warning < 0 || warning > 1 || critical < 0 || critical > 1 || panic < 0 || panic > 1 {
		return fmt.Errorf("thresholds must be between 0.0 and 1.0")
	}
	if warning >= critical || critical >= panic {
		return fmt.Errorf("thresholds must be ordered: warning < critical < panic")
	}

	mp.warningThreshold = warning
	mp.criticalThreshold = critical
	mp.panicThreshold = panic
	return nil
}

// SetPressureHandlers allows customization of pressure response callbacks
func (mp *MemoryPool) SetPressureHandlers(
	onWarning, onCritical, onPanic func(float64)) {
	mp.onWarningPressure = onWarning
	mp.onCriticalPressure = onCritical
	mp.onPanicPressure = onPanic
}

// GetStats returns comprehensive statistics about the memory pool
func (mp *MemoryPool) GetStats() map[string]interface{} {
	mp.mutex.RLock()
	activeAllocations := len(mp.allocations)
	mp.mutex.RUnlock()

	currentUsage := atomic.LoadInt64(&mp.currentUsage)
	pressure := float64(currentUsage) / float64(mp.maxSize)

	return map[string]interface{}{
		"name":                mp.name,
		"max_size":            mp.maxSize,
		"current_usage":       currentUsage,
		"available_space":     mp.maxSize - currentUsage,
		"memory_pressure":     pressure,
		"active_allocations":  activeAllocations,
		"total_allocations":   atomic.LoadInt64(&mp.totalAllocations),
		"total_deallocations": atomic.LoadInt64(&mp.totalDeallocations),
		"allocation_failures": atomic.LoadInt64(&mp.allocationFailures),
		"warning_threshold":   mp.warningThreshold,
		"critical_threshold":  mp.criticalThreshold,
		"panic_threshold":     mp.panicThreshold,
		"last_cleanup":        mp.lastCleanup,
	}
}

// Resize changes the maximum size of the memory pool
func (mp *MemoryPool) Resize(newMaxSize int64) error {
	if newMaxSize <= 0 {
		return fmt.Errorf("invalid pool size: %d", newMaxSize)
	}

	currentUsage := atomic.LoadInt64(&mp.currentUsage)
	if newMaxSize < currentUsage {
		return fmt.Errorf("cannot resize below current usage: %d < %d",
			newMaxSize, currentUsage)
	}

	mp.maxSize = newMaxSize
	return nil
}

// Cleanup forces cleanup of tracked allocations and updates statistics
func (mp *MemoryPool) Cleanup() {
	mp.mutex.Lock()
	defer mp.mutex.Unlock()

	// Note: In a real implementation, we might scan for leaked allocations here
	// For now, we just update the cleanup timestamp
	mp.lastCleanup = time.Now()
}

// Default pressure handlers - can be overridden by users
func (mp *MemoryPool) defaultWarningHandler(pressure float64) {
	// Log warning level memory pressure
	fmt.Printf("⚠️  Memory pool '%s' at warning pressure: %.1f%%\n",
		mp.name, pressure*100)
}

func (mp *MemoryPool) defaultCriticalHandler(pressure float64) {
	// Log critical level memory pressure
	fmt.Printf("🔥 Memory pool '%s' at critical pressure: %.1f%% - aggressive cleanup needed\n",
		mp.name, pressure*100)
}

func (mp *MemoryPool) defaultPanicHandler(pressure float64) {
	// Log panic level memory pressure
	fmt.Printf("💥 Memory pool '%s' at panic pressure: %.1f%% - emergency eviction!\n",
		mp.name, pressure*100)
}

// Name returns the name of this memory pool
func (mp *MemoryPool) Name() string {
	return mp.name
}
