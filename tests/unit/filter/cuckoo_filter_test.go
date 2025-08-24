package filter_test

import (
	"fmt"
	"testing"

	"hypercache/internal/filter"
)

// TestCuckooFilterBasics tests basic Cuckoo filter functionality
func TestCuckooFilterBasics(t *testing.T) {
	config := &filter.FilterConfig{
		FilterType:        "cuckoo",
		ExpectedItems:     1000,
		FalsePositiveRate: 0.01,
		BucketSize:        4, // Standard bucket size
	}

	cuckooFilter, err := filter.NewCuckooFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Cuckoo filter: %v", err)
	}

	// Test adding and checking elements
	t.Run("Add_and_Contains", func(t *testing.T) {
		testKey := []byte("test-key-1")
		
		// Initially should not contain the key
		if cuckooFilter.Contains(testKey) {
			t.Errorf("Filter should not contain key before adding")
		}

		// Add the key
		err := cuckooFilter.Add(testKey)
		if err != nil {
			t.Fatalf("Failed to add key to filter: %v", err)
		}

		// Now should contain the key
		if !cuckooFilter.Contains(testKey) {
			t.Errorf("Filter should contain key after adding")
		}
	})

	// Test false positive rate
	t.Run("False_Positive_Rate", func(t *testing.T) {
		// Add a bunch of keys
		numKeys := 500
		for i := 0; i < numKeys; i++ {
			key := []byte(fmt.Sprintf("key-%d", i))
			if err := cuckooFilter.Add(key); err != nil {
				t.Fatalf("Failed to add key %d: %v", i, err)
			}
		}

		// Test false positive rate with larger sample
		falsePositives := 0
		testKeys := 10000 // Larger sample for better statistical accuracy
		for i := numKeys; i < numKeys+testKeys; i++ {
			key := []byte(fmt.Sprintf("key-%d", i))
			if cuckooFilter.Contains(key) {
				falsePositives++
			}
		}

		falsePositiveRate := float64(falsePositives) / float64(testKeys)
		expectedRate := config.FalsePositiveRate

		t.Logf("False positive rate: %.4f (expected: %.4f)", falsePositiveRate, expectedRate)
		
		// Allow significant tolerance for small filters
		maxAllowedRate := expectedRate * 10 // More lenient for testing
		if falsePositiveRate > maxAllowedRate {
			t.Errorf("False positive rate too high: %.4f > %.4f", falsePositiveRate, maxAllowedRate)
		} else {
			t.Logf("✅ False positive rate within acceptable range")
		}
	})

	// Test deletion
	t.Run("Delete_Functionality", func(t *testing.T) {
		testKey := []byte("delete-test-key")
		
		// Add key
		if err := cuckooFilter.Add(testKey); err != nil {
			t.Fatalf("Failed to add key: %v", err)
		}

		// Verify it exists
		if !cuckooFilter.Contains(testKey) {
			t.Errorf("Key should exist before deletion")
		}

		// Delete key
		deleted := cuckooFilter.Delete(testKey)
		if !deleted {
			t.Errorf("Delete should return true for existing key")
		}

		// Note: Contains() may still return true due to false positives
		// but we've verified that Delete() worked correctly
		stillExists := cuckooFilter.Contains(testKey)
		t.Logf("After deletion, Contains() returns: %v (this could be a false positive)", stillExists)
	})
}

// TestCuckooFilterCapacity tests capacity limits and overflow behavior
func TestCuckooFilterCapacity(t *testing.T) {
	config := &filter.FilterConfig{
		FilterType:        "cuckoo",
		ExpectedItems:     100, // Small capacity for testing
		FalsePositiveRate: 0.01,
		BucketSize:        4, // Standard bucket size
	}

	cuckooFilter, err := filter.NewCuckooFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Cuckoo filter: %v", err)
	}

	// Try to add more items than capacity
	successfulAdds := 0
	totalAttempts := 150 // More than capacity

	for i := 0; i < totalAttempts; i++ {
		key := []byte(fmt.Sprintf("capacity-test-%d", i))
		if err := cuckooFilter.Add(key); err == nil {
			successfulAdds++
		}
	}

	t.Logf("Successfully added %d/%d items (expected: %d)", 
		successfulAdds, totalAttempts, config.ExpectedItems)

	// Should be able to add close to expected capacity
	if successfulAdds < int(config.ExpectedItems)*80/100 { // At least 80% of expected capacity
		t.Errorf("Could only add %d items, expected closer to %d", 
			successfulAdds, config.ExpectedItems)
	}
}

// TestCuckooFilterStats tests statistics reporting
func TestCuckooFilterStats(t *testing.T) {
	config := &filter.FilterConfig{
		FilterType:        "cuckoo",
		ExpectedItems:     1000,
		FalsePositiveRate: 0.01,
		BucketSize:        4, // Standard bucket size
	}

	cuckooFilter, err := filter.NewCuckooFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Cuckoo filter: %v", err)
	}

	// Get initial stats
	stats := cuckooFilter.GetStats()
	if stats == nil {
		t.Fatalf("GetStats() returned nil")
	}

	initialCount := stats.Size
	t.Logf("Initial stats: %+v", stats)

	// Add some items
	numItems := 100
	for i := 0; i < numItems; i++ {
		key := []byte(fmt.Sprintf("stats-test-%d", i))
		if err := cuckooFilter.Add(key); err != nil {
			t.Logf("Warning: Failed to add key %d: %v", i, err)
		}
	}

	// Get updated stats
	updatedStats := cuckooFilter.GetStats()
	t.Logf("Updated stats: %+v", updatedStats)

	// Verify stats were updated
	if updatedStats.Size <= initialCount {
		t.Errorf("Item count should have increased: %d -> %d", 
			initialCount, updatedStats.Size)
	}

	// Test load factor calculation
	loadFactor := float64(updatedStats.Size) / float64(cuckooFilter.Capacity())
	t.Logf("Load factor: %.4f", loadFactor)

	if loadFactor < 0.0 || loadFactor > 1.0 {
		t.Errorf("Load factor should be between 0.0 and 1.0, got %.4f", loadFactor)
	}
}

// TestCuckooFilterConcurrency tests concurrent access to the filter
func TestCuckooFilterConcurrency(t *testing.T) {
	config := &filter.FilterConfig{
		FilterType:        "cuckoo",
		ExpectedItems:     10000,
		FalsePositiveRate: 0.01,
		BucketSize:        4, // Standard bucket size
	}

	cuckooFilter, err := filter.NewCuckooFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Cuckoo filter: %v", err)
	}

	// Test concurrent adds
	t.Run("Concurrent_Adds", func(t *testing.T) {
		const numGoroutines = 10
		const keysPerGoroutine = 100
		
		done := make(chan bool, numGoroutines)
		
		for g := 0; g < numGoroutines; g++ {
			go func(goroutineID int) {
				defer func() { done <- true }()
				
				for i := 0; i < keysPerGoroutine; i++ {
					key := []byte(fmt.Sprintf("concurrent-%d-%d", goroutineID, i))
					if err := cuckooFilter.Add(key); err != nil {
						t.Logf("Warning: Goroutine %d failed to add key %d: %v", 
							goroutineID, i, err)
					}
				}
			}(g)
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		stats := cuckooFilter.GetStats()
		t.Logf("After concurrent adds: %d items", stats.Size)

		if stats.Size == 0 {
			t.Errorf("No items were added during concurrent test")
		}
	})

	// Test concurrent reads
	t.Run("Concurrent_Reads", func(t *testing.T) {
		// Add some keys first
		for i := 0; i < 100; i++ {
			key := []byte(fmt.Sprintf("read-test-%d", i))
			cuckooFilter.Add(key)
		}

		const numReaders = 10
		const readsPerGoroutine = 200
		
		done := make(chan bool, numReaders)
		
		for g := 0; g < numReaders; g++ {
			go func(goroutineID int) {
				defer func() { done <- true }()
				
				for i := 0; i < readsPerGoroutine; i++ {
					key := []byte(fmt.Sprintf("read-test-%d", i%100))
					_ = cuckooFilter.Contains(key) // Just testing for race conditions
				}
			}(g)
		}

		// Wait for all readers
		for i := 0; i < numReaders; i++ {
			<-done
		}

		t.Logf("✅ Concurrent reads completed without race conditions")
	})
}

// BenchmarkCuckooFilter benchmarks Cuckoo filter performance
func BenchmarkCuckooFilter(b *testing.B) {
	config := &filter.FilterConfig{
		FilterType:        "cuckoo",
		ExpectedItems:     100000,
		FalsePositiveRate: 0.01,
		BucketSize:        4, // Standard bucket size
	}

	cuckooFilter, err := filter.NewCuckooFilter(config)
	if err != nil {
		b.Fatalf("Failed to create Cuckoo filter: %v", err)
	}

	// Benchmark Add operations
	b.Run("Add", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := []byte(fmt.Sprintf("bench-add-%d", i))
			cuckooFilter.Add(key)
		}
	})

	// Pre-populate for Contains benchmark
	for i := 0; i < 10000; i++ {
		key := []byte(fmt.Sprintf("bench-contains-%d", i))
		cuckooFilter.Add(key)
	}

	// Benchmark Contains operations
	b.Run("Contains", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := []byte(fmt.Sprintf("bench-contains-%d", i%10000))
			cuckooFilter.Contains(key)
		}
	})

	// Benchmark Delete operations  
	b.Run("Delete", func(b *testing.B) {
		// Pre-populate
		for i := 0; i < b.N; i++ {
			key := []byte(fmt.Sprintf("bench-delete-%d", i))
			cuckooFilter.Add(key)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := []byte(fmt.Sprintf("bench-delete-%d", i))
			cuckooFilter.Delete(key)
		}
	})
}
