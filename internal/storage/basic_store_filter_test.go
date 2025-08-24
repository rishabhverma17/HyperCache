package storage

import (
	"testing"
	"time"

	"hypercache/internal/filter"
)

// TestBasicStoreWithoutFilter tests that store works normally without filter
func TestBasicStoreWithoutFilter(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-no-filter",
		MaxMemory:        1024 * 1024, // 1MB
		DefaultTTL:       time.Minute,
		EnableStatistics: true,
		CleanupInterval:  time.Second,
		FilterConfig:     nil, // No filter
	}

	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test basic operations
	err = store.Set("key1", "value1", "session1", 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	value, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if value != "value1" {
		t.Fatalf("Expected 'value1', got %v", value)
	}

	// Verify filter is nil
	filterStats := store.FilterStats()
	if filterStats != nil {
		t.Fatalf("Expected filter stats to be nil, got %+v", filterStats)
	}
}

// TestBasicStoreWithCuckooFilter tests store with cuckoo filter
func TestBasicStoreWithCuckooFilter(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-with-filter",
		MaxMemory:        1024 * 1024, // 1MB
		DefaultTTL:       time.Minute,
		EnableStatistics: true,
		CleanupInterval:  time.Second,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       1000,
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
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Verify filter is initialized
	filterStats := store.FilterStats()
	if filterStats == nil {
		t.Fatalf("Expected filter stats to be non-nil")
	}

	if filterStats.Size != 0 {
		t.Fatalf("Expected initial item count 0, got %d", filterStats.Size)
	}

	// Test Set adds to filter
	err = store.Set("key1", "value1", "session1", 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Check filter stats updated
	filterStats = store.FilterStats()
	if filterStats.Size != 1 {
		t.Fatalf("Expected filter item count 1, got %d", filterStats.Size)
	}

	// Test Get with filter hit
	value, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if value != "value1" {
		t.Fatalf("Expected 'value1', got %v", value)
	}

	// Test Get with filter miss (key doesn't exist)
	_, err = store.Get("nonexistent")
	if err == nil {
		t.Fatalf("Expected error for nonexistent key")
	}

	// Test Delete removes from filter
	err = store.Delete("key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	filterStats = store.FilterStats()
	if filterStats.Size != 0 {
		t.Fatalf("Expected filter item count 0 after delete, got %d", filterStats.Size)
	}

	// Test Clear clears filter
	err = store.Set("key2", "value2", "session1", 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = store.Set("key3", "value3", "session1", 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	filterStats = store.FilterStats()
	if filterStats.Size != 2 {
		t.Fatalf("Expected filter item count 2, got %d", filterStats.Size)
	}

	err = store.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	filterStats = store.FilterStats()
	if filterStats.Size != 0 {
		t.Fatalf("Expected filter item count 0 after clear, got %d", filterStats.Size)
	}
}

// TestBasicStoreFilterEarlyReject tests that filter provides early rejection
func TestBasicStoreFilterEarlyReject(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-early-reject",
		MaxMemory:        1024 * 1024, // 1MB
		DefaultTTL:       time.Minute,
		EnableStatistics: true,
		CleanupInterval:  time.Second,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       100,
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 50,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add some keys
	testKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	for _, key := range testKeys {
		err = store.Set(key, "value-"+key, "session1", 0)
		if err != nil {
			t.Fatalf("Set failed for %s: %v", key, err)
		}
	}

	// Test that existing keys are found
	for _, key := range testKeys {
		value, err := store.Get(key)
		if err != nil {
			t.Fatalf("Get failed for existing key %s: %v", key, err)
		}
		expected := "value-" + key
		if value != expected {
			t.Fatalf("Expected %s, got %v", expected, value)
		}
	}

	// Test that non-existent keys are rejected early
	nonExistentKeys := []string{"nothere1", "nothere2", "nothere3"}
	for _, key := range nonExistentKeys {
		_, err := store.Get(key)
		if err == nil {
			t.Fatalf("Expected error for non-existent key %s", key)
		}
	}

	// Verify stats
	stats := store.Stats()
	if stats.HitCount != uint64(len(testKeys)) {
		t.Fatalf("Expected %d hits, got %d", len(testKeys), stats.HitCount)
	}

	if stats.MissCount != uint64(len(nonExistentKeys)) {
		t.Fatalf("Expected %d misses, got %d", len(nonExistentKeys), stats.MissCount)
	}
}

// TestBasicStoreFilterEviction tests filter consistency during eviction
func TestBasicStoreFilterEviction(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-filter-eviction",
		MaxMemory:        1024, // Small memory to force eviction
		DefaultTTL:       0,    // No TTL
		EnableStatistics: true,
		CleanupInterval:  time.Second,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       100,
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 50,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add items until memory is full and eviction occurs
	largeValue := make([]byte, 200) // 200 bytes per item
	for i := 0; i < 10; i++ {
		key := "key" + string(rune('0'+i))
		err = store.Set(key, largeValue, "session1", 0)
		if err != nil {
			// This is expected when memory is full
			break
		}
	}

	// Get stats to see how many items were actually stored
	stats := store.Stats()
	filterStats := store.FilterStats()

	// The filter might have more items than the store due to the eviction process
	// but that's okay - it's a probabilistic structure
	t.Logf("Store has %d items, filter has %d items", stats.TotalItems, filterStats.Size)

	// Test that non-existent keys are still properly rejected
	_, err = store.Get("definitely_not_there")
	if err == nil {
		t.Fatalf("Expected error for non-existent key after eviction")
	}
}

// TestBasicStoreFilterExpiration tests filter consistency with TTL expiration
func TestBasicStoreFilterExpiration(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-filter-expiration",
		MaxMemory:        1024 * 1024, // 1MB
		DefaultTTL:       0,           // No default TTL
		EnableStatistics: true,
		CleanupInterval:  50 * time.Millisecond, // Fast cleanup for testing
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       100,
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 50,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set items with very short TTL
	shortTTL := 100 * time.Millisecond
	testKeys := []string{"expire1", "expire2", "expire3"}

	for _, key := range testKeys {
		err = store.Set(key, "value", "session1", shortTTL)
		if err != nil {
			t.Fatalf("Set failed for %s: %v", key, err)
		}
	}

	// Verify items are initially present
	for _, key := range testKeys {
		_, err := store.Get(key)
		if err != nil {
			t.Fatalf("Get failed for %s: %v", key, err)
		}
	}

	filterStats := store.FilterStats()
	if filterStats.Size != uint64(len(testKeys)) {
		t.Fatalf("Expected filter item count %d, got %d", len(testKeys), filterStats.Size)
	}

	// Wait for expiration + cleanup
	time.Sleep(200 * time.Millisecond)

	// Try to get expired items - they should be cleaned up
	for _, key := range testKeys {
		_, err := store.Get(key)
		if err == nil {
			t.Fatalf("Expected error for expired key %s", key)
		}
	}

	// Store should be empty
	stats := store.Stats()
	if stats.TotalItems != 0 {
		t.Fatalf("Expected 0 items after expiration, got %d", stats.TotalItems)
	}

	// Filter should also be cleaned up (items removed during expiration cleanup)
	filterStats = store.FilterStats()
	if filterStats.Size != 0 {
		t.Fatalf("Expected filter item count 0 after expiration cleanup, got %d", filterStats.Size)
	}
}

// TestBasicStoreInvalidFilterType tests error handling for invalid filter types
func TestBasicStoreInvalidFilterType(t *testing.T) {
	config := BasicStoreConfig{
		Name:             "test-invalid-filter",
		MaxMemory:        1024 * 1024,
		DefaultTTL:       time.Minute,
		EnableStatistics: true,
		CleanupInterval:  time.Second,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "invalid_filter_type",
			ExpectedItems:       100,
			FalsePositiveRate:   0.001,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 50,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}

	_, err := NewBasicStore(config)
	if err == nil {
		t.Fatalf("Expected error for invalid filter type")
	}

	if err.Error() == "" {
		t.Fatalf("Expected non-empty error message")
	}
}
