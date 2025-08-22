package storage

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryPool_BasicOperations(t *testing.T) {
	pool := NewMemoryPool("test-pool", 1024)
	
	// Test initial state
	if pool.CurrentUsage() != 0 {
		t.Errorf("Expected initial usage to be 0, got %d", pool.CurrentUsage())
	}
	
	if pool.MaxSize() != 1024 {
		t.Errorf("Expected max size to be 1024, got %d", pool.MaxSize())
	}
	
	if pool.AvailableSpace() != 1024 {
		t.Errorf("Expected available space to be 1024, got %d", pool.AvailableSpace())
	}
	
	if pool.MemoryPressure() != 0.0 {
		t.Errorf("Expected initial pressure to be 0.0, got %f", pool.MemoryPressure())
	}
}

func TestMemoryPool_AllocationAndFree(t *testing.T) {
	pool := NewMemoryPool("test-pool", 1024)
	
	// Allocate memory
	data, err := pool.Allocate(512)
	if err != nil {
		t.Fatalf("Failed to allocate memory: %v", err)
	}
	
	if len(data) != 512 {
		t.Errorf("Expected allocated slice length to be 512, got %d", len(data))
	}
	
	if pool.CurrentUsage() != 512 {
		t.Errorf("Expected usage to be 512, got %d", pool.CurrentUsage())
	}
	
	if pool.AvailableSpace() != 512 {
		t.Errorf("Expected available space to be 512, got %d", pool.AvailableSpace())
	}
	
	expectedPressure := 512.0 / 1024.0
	if pool.MemoryPressure() != expectedPressure {
		t.Errorf("Expected pressure to be %f, got %f", expectedPressure, pool.MemoryPressure())
	}
	
	// Free memory
	err = pool.Free(data)
	if err != nil {
		t.Fatalf("Failed to free memory: %v", err)
	}
	
	if pool.CurrentUsage() != 0 {
		t.Errorf("Expected usage to be 0 after free, got %d", pool.CurrentUsage())
	}
	
	if pool.AvailableSpace() != 1024 {
		t.Errorf("Expected available space to be 1024 after free, got %d", pool.AvailableSpace())
	}
}

func TestMemoryPool_AllocationLimits(t *testing.T) {
	pool := NewMemoryPool("test-pool", 1024)
	
	// Allocate exactly the limit
	data, err := pool.Allocate(1024)
	if err != nil {
		t.Fatalf("Failed to allocate at limit: %v", err)
	}
	
	// Try to allocate beyond limit
	_, err = pool.Allocate(1)
	if err == nil {
		t.Error("Expected allocation beyond limit to fail")
	}
	
	// Free and try again
	err = pool.Free(data)
	if err != nil {
		t.Fatalf("Failed to free memory: %v", err)
	}
	
	// Should be able to allocate again
	_, err = pool.Allocate(512)
	if err != nil {
		t.Errorf("Failed to allocate after free: %v", err)
	}
}

func TestMemoryPool_PressureThresholds(t *testing.T) {
	pool := NewMemoryPool("test-pool", 1000)
	
	var warningCalled, criticalCalled, panicCalled bool
	var warningPressure, criticalPressure, panicPressure float64
	
	pool.SetPressureHandlers(
		func(p float64) { 
			warningCalled = true
			warningPressure = p
		},
		func(p float64) { 
			criticalCalled = true
			criticalPressure = p
		},
		func(p float64) { 
			panicCalled = true
			panicPressure = p
		},
	)
	
	// Allocate to warning threshold (85%)
	data1, err := pool.Allocate(850)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	// Give callbacks time to execute
	time.Sleep(10 * time.Millisecond)
	
	if !warningCalled {
		t.Error("Expected warning callback to be called")
	}
	if warningPressure < 0.85 {
		t.Errorf("Expected warning pressure >= 0.85, got %f", warningPressure)
	}
	
	// Allocate to critical threshold (90%)
	data2, err := pool.Allocate(50)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	time.Sleep(10 * time.Millisecond)
	
	if !criticalCalled {
		t.Error("Expected critical callback to be called")
	}
	if criticalPressure < 0.90 {
		t.Errorf("Expected critical pressure >= 0.90, got %f", criticalPressure)
	}
	
	// Allocate to panic threshold (95%)
	data3, err := pool.Allocate(50)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	time.Sleep(10 * time.Millisecond)
	
	if !panicCalled {
		t.Error("Expected panic callback to be called")
	}
	if panicPressure < 0.95 {
		t.Errorf("Expected panic pressure >= 0.95, got %f", panicPressure)
	}
	
	// Cleanup
	pool.Free(data1)
	pool.Free(data2) 
	pool.Free(data3)
}

func TestMemoryPool_ConcurrentOperations(t *testing.T) {
	pool := NewMemoryPool("concurrent-test", 10240)
	
	numGoroutines := 100
	allocationsPerGoroutine := 10
	allocationSize := int64(10)
	
	var wg sync.WaitGroup
	var allocatedData [][]byte
	var allocatedDataMutex sync.Mutex
	
	// Concurrent allocations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for j := 0; j < allocationsPerGoroutine; j++ {
				data, err := pool.Allocate(allocationSize)
				if err != nil {
					t.Errorf("Failed to allocate: %v", err)
					return
				}
				
				allocatedDataMutex.Lock()
				allocatedData = append(allocatedData, data)
				allocatedDataMutex.Unlock()
			}
		}()
	}
	
	wg.Wait()
	
	// Check total usage
	expectedUsage := int64(numGoroutines * allocationsPerGoroutine) * allocationSize
	if pool.CurrentUsage() != expectedUsage {
		t.Errorf("Expected usage %d, got %d", expectedUsage, pool.CurrentUsage())
	}
	
	// Concurrent deallocations
	allocatedDataMutex.Lock()
	totalAllocations := len(allocatedData)
	allocatedDataMutex.Unlock()
	
	for i := 0; i < totalAllocations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			allocatedDataMutex.Lock()
			data := allocatedData[index]
			allocatedDataMutex.Unlock()
			
			err := pool.Free(data)
			if err != nil {
				t.Errorf("Failed to free: %v", err)
			}
		}(i)
	}
	
	wg.Wait()
	
	// Check that all memory was freed
	if pool.CurrentUsage() != 0 {
		t.Errorf("Expected usage to be 0 after freeing all, got %d", pool.CurrentUsage())
	}
}

func TestMemoryPool_Statistics(t *testing.T) {
	pool := NewMemoryPool("stats-test", 1024)
	
	// Make some allocations and deallocations
	data1, _ := pool.Allocate(256)
	data2, _ := pool.Allocate(256)
	pool.Free(data1)
	
	// Try failed allocation
	_, err := pool.Allocate(1024) // Should fail because 256 is still allocated
	if err == nil {
		t.Error("Expected allocation to fail")
	}
	
	stats := pool.GetStats()
	
	// Check basic stats
	if stats["name"] != "stats-test" {
		t.Errorf("Expected name 'stats-test', got %v", stats["name"])
	}
	
	if stats["max_size"] != int64(1024) {
		t.Errorf("Expected max_size 1024, got %v", stats["max_size"])
	}
	
	if stats["current_usage"] != int64(256) {
		t.Errorf("Expected current_usage 256, got %v", stats["current_usage"])
	}
	
	if stats["total_allocations"] != int64(2) {
		t.Errorf("Expected total_allocations 2, got %v", stats["total_allocations"])
	}
	
	if stats["total_deallocations"] != int64(1) {
		t.Errorf("Expected total_deallocations 1, got %v", stats["total_deallocations"])
	}
	
	if stats["allocation_failures"] != int64(1) {
		t.Errorf("Expected allocation_failures 1, got %v", stats["allocation_failures"])
	}
	
	// Cleanup
	pool.Free(data2)
}

func TestMemoryPool_Resize(t *testing.T) {
	pool := NewMemoryPool("resize-test", 1024)
	
	// Allocate some memory
	data, err := pool.Allocate(512)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	// Try to resize below current usage
	err = pool.Resize(256)
	if err == nil {
		t.Error("Expected resize below current usage to fail")
	}
	
	// Resize above current usage
	err = pool.Resize(2048)
	if err != nil {
		t.Errorf("Failed to resize above current usage: %v", err)
	}
	
	if pool.MaxSize() != 2048 {
		t.Errorf("Expected max size 2048, got %d", pool.MaxSize())
	}
	
	if pool.AvailableSpace() != 2048-512 {
		t.Errorf("Expected available space %d, got %d", 2048-512, pool.AvailableSpace())
	}
	
	// Should be able to allocate more now
	data2, err := pool.Allocate(1024)
	if err != nil {
		t.Errorf("Failed to allocate after resize: %v", err)
	}
	
	// Cleanup
	pool.Free(data)
	pool.Free(data2)
}

func TestMemoryPool_EdgeCases(t *testing.T) {
	pool := NewMemoryPool("edge-test", 1024)
	
	// Test zero and negative allocations
	_, err := pool.Allocate(0)
	if err == nil {
		t.Error("Expected zero allocation to fail")
	}
	
	_, err = pool.Allocate(-1)
	if err == nil {
		t.Error("Expected negative allocation to fail")
	}
	
	// Test freeing nil or empty slice
	err = pool.Free(nil)
	if err == nil {
		t.Error("Expected freeing nil to fail")
	}
	
	err = pool.Free([]byte{})
	if err == nil {
		t.Error("Expected freeing empty slice to fail")
	}
	
	// Test freeing untracked memory
	randomData := make([]byte, 100)
	err = pool.Free(randomData)
	if err == nil {
		t.Error("Expected freeing untracked memory to fail")
	}
}

func TestMemoryPool_CustomThresholds(t *testing.T) {
	pool := NewMemoryPool("threshold-test", 1000)
	
	// Test invalid thresholds
	err := pool.SetPressureThresholds(-0.1, 0.5, 0.8)
	if err == nil {
		t.Error("Expected negative threshold to be rejected")
	}
	
	err = pool.SetPressureThresholds(0.9, 0.8, 0.7) // Wrong order
	if err == nil {
		t.Error("Expected wrong threshold order to be rejected")
	}
	
	// Test valid thresholds
	err = pool.SetPressureThresholds(0.70, 0.80, 0.90)
	if err != nil {
		t.Errorf("Valid thresholds were rejected: %v", err)
	}
	
	var warningCalled bool
	pool.SetPressureHandlers(
		func(p float64) { warningCalled = true },
		nil,
		nil,
	)
	
	// Allocate to 75% (should trigger warning at 70%)
	data, err := pool.Allocate(750)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	time.Sleep(10 * time.Millisecond)
	
	if !warningCalled {
		t.Error("Expected warning callback to be called with custom threshold")
	}
	
	pool.Free(data)
}

// Benchmark tests
func BenchmarkMemoryPool_Allocate(b *testing.B) {
	pool := NewMemoryPool("bench-pool", int64(b.N)*100)
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := pool.Allocate(100)
		if err != nil {
			b.Fatalf("Allocation failed: %v", err)
		}
	}
}

func BenchmarkMemoryPool_AllocateAndFree(b *testing.B) {
	pool := NewMemoryPool("bench-pool", int64(b.N)*100)
	allocations := make([][]byte, b.N)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		data, err := pool.Allocate(100)
		if err != nil {
			b.Fatalf("Allocation failed: %v", err)
		}
		allocations[i] = data
	}
	
	for i := 0; i < b.N; i++ {
		err := pool.Free(allocations[i])
		if err != nil {
			b.Fatalf("Free failed: %v", err)
		}
	}
}

func BenchmarkMemoryPool_MemoryPressure(b *testing.B) {
	pool := NewMemoryPool("bench-pool", 1024)
	pool.Allocate(512) // 50% usage
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = pool.MemoryPressure()
	}
}
