package storage

import (
	"fmt"
	"testing"
	"time"

	"hypercache/internal/filter"
)

// BenchmarkBasicStoreWithFilter_Set benchmarks Set operations with filter enabled
func BenchmarkBasicStoreWithFilter_Set(b *testing.B) {
	config := BasicStoreConfig{
		Name:             "bench-filter-set",
		MaxMemory:        100 * 1024 * 1024, // 100MB
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       uint64(b.N),
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 500,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d_%d", b.N, i)
			value := fmt.Sprintf("value_%d_%d", b.N, i)
			err := store.Set(key, value, "session1", 0)
			if err != nil {
				b.Errorf("Set failed: %v", err)
			}
			i++
		}
	})
}

// BenchmarkBasicStoreWithFilter_Get benchmarks Get operations with filter enabled
func BenchmarkBasicStoreWithFilter_Get(b *testing.B) {
	config := BasicStoreConfig{
		Name:             "bench-filter-get",
		MaxMemory:        500 * 1024 * 1024, // 500MB - much larger to avoid eviction during setup
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       1000, // Smaller dataset
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 500,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate with smaller dataset
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		err := store.Set(key, value, "session1", 0)
		if err != nil {
			b.Fatalf("Pre-population failed: %v", err)
		}
	}

	// Verify population worked
	stats := store.Stats()
	if stats.TotalItems == 0 {
		b.Fatalf("No items in cache after population - memory eviction occurred")
	}
	b.Logf("Pre-populated store with %d items", stats.TotalItems)

	// Use non-parallel benchmark to avoid goroutine counter issues
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%1000) // Hit existing keys
		_, err := store.Get(key)
		if err != nil {
			b.Errorf("Get failed for key %s: %v", key, err)
		}
	}
}

// BenchmarkBasicStoreWithFilter_GetMiss benchmarks Get operations with filter (cache misses)
func BenchmarkBasicStoreWithFilter_GetMiss(b *testing.B) {
	config := BasicStoreConfig{
		Name:             "bench-filter-get-miss",
		MaxMemory:        100 * 1024 * 1024, // 100MB
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       10000,
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 500,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate with some data
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("existing_key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		err := store.Set(key, value, "session1", 0)
		if err != nil {
			b.Fatalf("Pre-population failed: %v", err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			key := fmt.Sprintf("missing_key_%d_%d", b.N, counter) // Keys that don't exist
			_, err := store.Get(key)
			if err == nil {
				b.Errorf("Expected miss for key %s", key)
			}
			counter++
		}
	})
}

// BenchmarkBasicStoreWithoutFilter_Get benchmarks Get operations without filter (for comparison)
func BenchmarkBasicStoreWithoutFilter_Get(b *testing.B) {
	config := BasicStoreConfig{
		Name:             "bench-no-filter-get",
		MaxMemory:        500 * 1024 * 1024, // 500MB - much larger to avoid eviction during setup
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig:     nil, // No filter
	}

	store, err := NewBasicStore(config)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate with smaller dataset
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		err := store.Set(key, value, "session1", 0)
		if err != nil {
			b.Fatalf("Pre-population failed: %v", err)
		}
	}

	// Verify population worked
	stats := store.Stats()
	if stats.TotalItems == 0 {
		b.Fatalf("No items in cache after population - memory eviction occurred")
	}
	b.Logf("Pre-populated store with %d items", stats.TotalItems)

	// Use non-parallel benchmark to avoid goroutine counter issues
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%1000) // Hit existing keys
		_, err := store.Get(key)
		if err != nil {
			b.Errorf("Get failed for key %s: %v", key, err)
		}
	}
}

// BenchmarkBasicStoreWithoutFilter_GetMiss benchmarks Get operations without filter (cache misses)
func BenchmarkBasicStoreWithoutFilter_GetMiss(b *testing.B) {
	config := BasicStoreConfig{
		Name:             "bench-no-filter-get-miss",
		MaxMemory:        100 * 1024 * 1024, // 100MB
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig:     nil, // No filter
	}

	store, err := NewBasicStore(config)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate with some data
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("existing_key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		err := store.Set(key, value, "session1", 0)
		if err != nil {
			b.Fatalf("Pre-population failed: %v", err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			key := fmt.Sprintf("missing_key_%d_%d", b.N, counter) // Keys that don't exist
			_, err := store.Get(key)
			if err == nil {
				b.Errorf("Expected miss for key %s", key)
			}
			counter++
		}
	})
}
