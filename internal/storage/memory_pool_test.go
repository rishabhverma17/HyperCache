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
	pool := NewMemoryPool("test-pool", 2048)

	// Allocate memory (actual usage = 512 + PerKeyOverhead)
	data, err := pool.Allocate(512)
	if err != nil {
		t.Fatalf("Failed to allocate memory: %v", err)
	}

	if len(data) != 512 {
		t.Errorf("Expected allocated slice length to be 512, got %d", len(data))
	}

	expectedUsage := int64(512 + PerKeyOverhead)
	if pool.CurrentUsage() != expectedUsage {
		t.Errorf("Expected usage to be %d, got %d", expectedUsage, pool.CurrentUsage())
	}

	expectedAvail := int64(2048) - expectedUsage
	if pool.AvailableSpace() != expectedAvail {
		t.Errorf("Expected available space to be %d, got %d", expectedAvail, pool.AvailableSpace())
	}

	expectedPressure := float64(expectedUsage) / 2048.0
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

	if pool.AvailableSpace() != 2048 {
		t.Errorf("Expected available space to be 2048 after free, got %d", pool.AvailableSpace())
	}
}

func TestMemoryPool_AllocationLimits(t *testing.T) {
	// Pool must be large enough for value + PerKeyOverhead
	poolSize := int64(1024 + PerKeyOverhead)
	pool := NewMemoryPool("test-pool", poolSize)

	// Allocate exactly the limit (value=1024, total=1024+500)
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
	// Pool size accounts for overhead: we want to reach 85%/90%/95% of pool
	// Each alloc adds PerKeyOverhead, so plan values accordingly
	pool := NewMemoryPool("test-pool", 10000)

	var mu sync.Mutex
	var warningCalled, criticalCalled, panicCalled bool
	var warningPressure, criticalPressure, panicPressure float64

	pool.SetPressureHandlers(
		func(p float64) {
			mu.Lock()
			warningCalled = true
			warningPressure = p
			mu.Unlock()
		},
		func(p float64) {
			mu.Lock()
			criticalCalled = true
			criticalPressure = p
			mu.Unlock()
		},
		func(p float64) {
			mu.Lock()
			panicCalled = true
			panicPressure = p
			mu.Unlock()
		},
	)

	// Allocate to warning threshold (85%) = 8500 bytes total
	// value = 8500 - 500 = 8000
	data1, err := pool.Allocate(8000)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !warningCalled {
		t.Error("Expected warning callback to be called")
	}
	if warningPressure < 0.85 {
		t.Errorf("Expected warning pressure >= 0.85, got %f", warningPressure)
	}
	mu.Unlock()

	// Allocate to critical threshold (90%) = need 500 more total
	data2, err := pool.Allocate(50)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !criticalCalled {
		t.Error("Expected critical callback to be called")
	}
	if criticalPressure < 0.90 {
		t.Errorf("Expected critical pressure >= 0.90, got %f", criticalPressure)
	}
	mu.Unlock()

	// Allocate to panic threshold (95%) = need 500 more total
	data3, err := pool.Allocate(50)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !panicCalled {
		t.Error("Expected panic callback to be called")
	}
	if panicPressure < 0.95 {
		t.Errorf("Expected panic pressure >= 0.95, got %f", panicPressure)
	}
	mu.Unlock()

	// Cleanup
	_ = pool.Free(data1)
	_ = pool.Free(data2)
	_ = pool.Free(data3)
}

func TestMemoryPool_ConcurrentOperations(t *testing.T) {
	// Each alloc = 10 + 500 = 510, total = 100*10*510 = 510,000
	pool := NewMemoryPool("concurrent-test", 510000)

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

	// Check total usage (including PerKeyOverhead per allocation)
	expectedUsage := int64(numGoroutines*allocationsPerGoroutine) * (allocationSize + PerKeyOverhead)
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
	// Needs room for 2 allocations of 256 + overhead each
	pool := NewMemoryPool("stats-test", 2048)

	data1, _ := pool.Allocate(256)
	data2, _ := pool.Allocate(256)
	pool.Free(data1)

	// Try failed allocation (remaining = 2048 - (256+500) = 1292, need 1024+500=1524)
	_, err := pool.Allocate(1024)
	if err == nil {
		t.Error("Expected allocation to fail")
	}

	stats := pool.GetStats()

	if stats["name"] != "stats-test" {
		t.Errorf("Expected name 'stats-test', got %v", stats["name"])
	}

	expectedUsage := int64(256 + PerKeyOverhead)
	if stats["current_usage"] != expectedUsage {
		t.Errorf("Expected current_usage %d, got %v", expectedUsage, stats["current_usage"])
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

	pool.Free(data2)
}

func TestMemoryPool_Resize(t *testing.T) {
	// 512 + 500 overhead = 1012 tracked
	pool := NewMemoryPool("resize-test", 2048)

	data, err := pool.Allocate(512)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}

	// Try to resize below current usage (1012)
	err = pool.Resize(256)
	if err == nil {
		t.Error("Expected resize below current usage to fail")
	}

	// Resize to larger
	err = pool.Resize(4096)
	if err != nil {
		t.Errorf("Failed to resize above current usage: %v", err)
	}

	if pool.MaxSize() != 4096 {
		t.Errorf("Expected max size 4096, got %d", pool.MaxSize())
	}

	// Should be able to allocate more now
	data2, err := pool.Allocate(1024)
	if err != nil {
		t.Errorf("Failed to allocate after resize: %v", err)
	}

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
	// 750 + 500 = 1250, so pool must be >= 1250
	pool := NewMemoryPool("threshold-test", 2000)

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

	var mu sync.Mutex
	var warningCalled bool
	pool.SetPressureHandlers(
		func(p float64) {
			mu.Lock()
			warningCalled = true
			mu.Unlock()
		},
		nil,
		nil,
	)

	// Allocate to ~62.5% (1250/2000) which is below 70% warning — bump it up
	data, err := pool.Allocate(950)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !warningCalled {
		t.Error("Expected warning callback to be called with custom threshold")
	}
	mu.Unlock()

	_ = pool.Free(data)
}

// Benchmark tests
func BenchmarkMemoryPool_Allocate(b *testing.B) {
	pool := NewMemoryPool("bench-pool", int64(b.N)*(100+PerKeyOverhead))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := pool.Allocate(100)
		if err != nil {
			b.Fatalf("Allocation failed: %v", err)
		}
	}
}

func BenchmarkMemoryPool_AllocateAndFree(b *testing.B) {
	pool := NewMemoryPool("bench-pool", int64(b.N)*(100+PerKeyOverhead))
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
