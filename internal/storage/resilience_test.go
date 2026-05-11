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

// TestDeleteTombstone_BlocksStaleSet verifies that a DELETE recorded with a
// Lamport timestamp creates a tombstone that suppresses any later-arriving
// SET replication carrying an older timestamp. This guards against the race
// where SET replication is delayed (e.g. async fire-and-forget) and arrives
// at a peer after that peer has already processed the DELETE for the same
// key — without the tombstone TS check the stale SET would resurrect the
// deleted key. Regression for the Newman 5.2 "Verify 'user:2' gone from Node 1"
// failure.
func TestDeleteTombstone_BlocksStaleSet(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Simulate replication arriving in this order on a peer:
	//   1) DELETE (TS=10) — owner deleted the key after some other writer set it
	//   2) SET    (TS=5)  — stale in-flight replication of the original SET
	if err := store.DeleteWithTimestamp("user:2", 10); err != nil {
		t.Fatalf("DeleteWithTimestamp failed: %v", err)
	}

	applied, err := store.SetWithTimestamp(nil, "user:2", "Bob", "replication", time.Hour, 5)
	if err != nil {
		t.Fatalf("SetWithTimestamp returned error: %v", err)
	}
	if applied {
		t.Fatal("stale SET (TS=5) must be rejected when a tombstone with TS=10 is present")
	}

	if _, err := store.Get("user:2"); err == nil {
		t.Fatal("Get must report miss after DELETE; tombstone allowed resurrection")
	}
}

// TestDeleteTombstone_RecordedEvenWhenAbsent verifies that DeleteWithTimestamp
// records a tombstone even when the key is not present locally. This is the
// case on a peer that has not yet received the original SET replication when
// the DELETE replication arrives first.
func TestDeleteTombstone_RecordedEvenWhenAbsent(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Key was never set locally — DeleteWithTimestamp must still tombstone.
	if err := store.DeleteWithTimestamp("phantom", 7); err != nil {
		t.Fatalf("DeleteWithTimestamp returned error for absent key: %v", err)
	}

	if !store.IsTombstoned("phantom") {
		t.Fatal("expected tombstone to be recorded for absent key")
	}

	// A stale SET with TS <= 7 must be rejected.
	applied, _ := store.SetWithTimestamp(nil, "phantom", "v", "replication", time.Hour, 7)
	if applied {
		t.Fatal("stale SET (TS=7) must be rejected; tombstone TS=7")
	}

	// A newer SET with TS > 7 must succeed and clear the tombstone.
	applied, err := store.SetWithTimestamp(nil, "phantom", "v-new", "replication", time.Hour, 8)
	if err != nil {
		t.Fatalf("SetWithTimestamp returned error: %v", err)
	}
	if !applied {
		t.Fatal("newer SET (TS=8) must overwrite tombstone (TS=7)")
	}
}

// TestDelete_LocalDoesNotBlockFutureSets verifies that the lamport-less
// Delete (used by eviction, expiry and local-only API calls) does not create
// a tombstone that would block subsequent legitimate writes via the timestamp
// path. Regression: an over-eager fix could accidentally block all writes.
func TestDelete_LocalDoesNotBlockFutureSets(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	if err := store.Set("k", "v1", "test", time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Delete("k"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// A SetWithTimestamp(TS=1) must succeed because the local Delete used
	// tombstone TS=0 (sentinel meaning "no causal block").
	applied, err := store.SetWithTimestamp(nil, "k", "v2", "test", time.Hour, 1)
	if err != nil {
		t.Fatalf("SetWithTimestamp failed: %v", err)
	}
	if !applied {
		t.Fatal("SET with TS=1 after local Delete should be applied (tombstone TS=0)")
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
