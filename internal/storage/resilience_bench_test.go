package storage

import (
	"fmt"
	"testing"
	"time"

	"hypercache/internal/filter"
)

// BenchmarkSetWithTimestamp measures the overhead of Lamport timestamp checking on writes.
// Compare with BenchmarkBasicStoreWithFilter_Set to see the cost of the timestamp check.
func BenchmarkSetWithTimestamp(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i)
		_, err := store.SetWithTimestamp(nil, key, "value", "bench", time.Hour, uint64(i+1))
		if err != nil {
			b.Fatalf("SetWithTimestamp failed: %v", err)
		}
	}
}

// BenchmarkSetWithTimestamp_Parallel measures concurrent write throughput with timestamps.
func BenchmarkSetWithTimestamp_Parallel(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d_%d", b.N, i)
			store.SetWithTimestamp(nil, key, "value", "bench", time.Hour, uint64(i+1))
			i++
		}
	})
}

// BenchmarkSetWithTimestamp_StaleRejection measures how fast stale writes are rejected.
// This is the fast path — a stale replication event arriving after a newer local write.
func BenchmarkSetWithTimestamp_StaleRejection(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	// Pre-populate with a high timestamp
	store.SetWithTimestamp(nil, "contested-key", "new-value", "bench", time.Hour, 1_000_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Every call should be rejected (timestamp 1 < 1_000_000)
		store.SetWithTimestamp(nil, "contested-key", "stale-value", "bench", time.Hour, 1)
	}
}

// BenchmarkGetTimestamp measures the cost of reading a key's Lamport timestamp.
func BenchmarkGetTimestamp(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	store.Set("existing-key", "value", "bench", time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetTimestamp("existing-key")
	}
}

// BenchmarkFilterAdd measures the cost of pre-populating the Cuckoo filter
// (the early filter sync done on gossip receive).
func BenchmarkFilterAdd(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.FilterAdd(fmt.Sprintf("key_%d", i))
	}
}

// BenchmarkSetThenGet measures full round-trip: write a key, then read it back.
func BenchmarkSetThenGet(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("rt_%d", i)
		store.Set(key, "value", "bench", time.Hour)
		_, err := store.Get(key)
		if err != nil {
			b.Fatalf("Get after Set failed: %v", err)
		}
	}
}

// BenchmarkGetMiss_FilterRejects measures GET latency when the Cuckoo filter
// rejects immediately (key never existed). This is the fastest path.
func BenchmarkGetMiss_FilterRejects(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get(fmt.Sprintf("nonexistent_%d", i))
	}
}

// BenchmarkGetHit measures GET latency for keys that exist (cache hit path).
func BenchmarkGetHit(b *testing.B) {
	store := createBenchStore(b)
	defer store.Close()

	// Pre-populate 1000 keys
	for i := 0; i < 1000; i++ {
		store.Set(fmt.Sprintf("hit_%d", i), "value", "bench", time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get(fmt.Sprintf("hit_%d", i%1000))
	}
}

func createBenchStore(b *testing.B) *BasicStore {
	b.Helper()
	config := BasicStoreConfig{
		Name:             "bench-resilience",
		MaxMemory:        256 * 1024 * 1024, // 256MB
		DefaultTTL:       time.Hour,
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
		FilterConfig: &filter.FilterConfig{
			FilterType:          "cuckoo",
			ExpectedItems:       1_000_000,
			FalsePositiveRate:   0.01,
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
	return store
}
