package storage

import (
	"testing"
	"time"

	"hypercache/internal/filter"
)

func TestSetWithTimestamp_NewerWins(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Write with timestamp 5
	applied, err := store.SetWithTimestamp(nil, "key1", "value-v1", "test", time.Hour, 5)
	if err != nil {
		t.Fatalf("SetWithTimestamp failed: %v", err)
	}
	if !applied {
		t.Fatal("expected first write to be applied")
	}

	// Write with higher timestamp 10 — should overwrite
	applied, err = store.SetWithTimestamp(nil, "key1", "value-v2", "test", time.Hour, 10)
	if err != nil {
		t.Fatalf("SetWithTimestamp failed: %v", err)
	}
	if !applied {
		t.Fatal("expected newer timestamp to be applied")
	}

	val, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value-v2" {
		t.Fatalf("expected value-v2, got %v", val)
	}
}

func TestSetWithTimestamp_StaleRejected(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Write with timestamp 10
	applied, err := store.SetWithTimestamp(nil, "key1", "value-new", "test", time.Hour, 10)
	if err != nil {
		t.Fatalf("SetWithTimestamp failed: %v", err)
	}
	if !applied {
		t.Fatal("expected first write to be applied")
	}

	// Write with lower timestamp 3 — should be rejected
	applied, err = store.SetWithTimestamp(nil, "key1", "value-stale", "test", time.Hour, 3)
	if err != nil {
		t.Fatalf("SetWithTimestamp failed: %v", err)
	}
	if applied {
		t.Fatal("expected stale timestamp to be rejected")
	}

	// Value should still be the newer one
	val, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value-new" {
		t.Fatalf("expected value-new, got %v", val)
	}
}

func TestSetWithTimestamp_EqualRejected(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Write with timestamp 5
	store.SetWithTimestamp(nil, "key1", "first", "test", time.Hour, 5)

	// Write with same timestamp — should be rejected (>= check)
	applied, _ := store.SetWithTimestamp(nil, "key1", "second", "test", time.Hour, 5)
	if applied {
		t.Fatal("expected equal timestamp to be rejected")
	}
}

func TestGetTimestamp(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Non-existent key returns 0
	if ts := store.GetTimestamp("missing"); ts != 0 {
		t.Fatalf("expected 0 for missing key, got %d", ts)
	}

	// Set with timestamp 42
	store.SetWithTimestamp(nil, "key1", "val", "test", time.Hour, 42)
	if ts := store.GetTimestamp("key1"); ts != 42 {
		t.Fatalf("expected 42, got %d", ts)
	}

	// Regular Set writes timestamp 0
	store.Set("key2", "val", "test", time.Hour)
	if ts := store.GetTimestamp("key2"); ts != 0 {
		t.Fatalf("expected 0 for regular Set, got %d", ts)
	}
}

func TestFilterAdd_PrePopulates(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Key doesn't exist — filter should say not here
	if store.FilterContains("phantom-key") {
		t.Fatal("expected filter to not contain phantom-key before FilterAdd")
	}

	// Pre-populate the filter without storing data
	store.FilterAdd("phantom-key")

	// Filter should now say "maybe here" even though store doesn't have it
	if !store.FilterContains("phantom-key") {
		t.Fatal("expected filter to contain phantom-key after FilterAdd")
	}

	// Actual store lookup should still miss
	_, err := store.Get("phantom-key")
	if err == nil {
		t.Fatal("expected Get to fail for phantom-key (no data stored)")
	}
}

// createTestStore creates a BasicStore with filter enabled for testing
func createTestStore(t *testing.T) *BasicStore {
	t.Helper()
	cfg := BasicStoreConfig{
		Name:             "test",
		MaxMemory:        64 * 1024 * 1024, // 64MB
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       1000,
			FalsePositiveRate:   0.01,
			FingerprintSize:     12,
			BucketSize:          4,
			MaxEvictionAttempts: 500,
			EnableAutoResize:    true,
			EnableStatistics:    true,
		},
	}
	store, err := NewBasicStore(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return store
}
