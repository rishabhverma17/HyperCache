package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hypercache/internal/persistence"
	"hypercache/internal/storage"
)

// ============================================================================
// PERSISTENCE BENCHMARKS — AOF / Snapshot / Recovery
// ============================================================================

func newPersistenceConfig(dir string) *persistence.PersistenceConfig {
	return &persistence.PersistenceConfig{
		Enabled:          true,
		DataDirectory:    dir,
		Strategy:         "hybrid",
		EnableAOF:        true,
		SyncPolicy:       "everysec",
		SyncInterval:     time.Second,
		SnapshotInterval: 15 * time.Minute,
		MaxLogSize:       100 * 1024 * 1024,
		CompressionLevel: 6,
		RetainLogs:       1,
	}
}

func newBenchStore(b *testing.B, maxMemory uint64, enablePersistence bool, dataDir string) *storage.BasicStore {
	b.Helper()
	cfg := storage.BasicStoreConfig{
		Name:      "bench-store",
		MaxMemory: maxMemory,
	}
	if enablePersistence {
		cfg.PersistenceConfig = newPersistenceConfig(dataDir)
	}
	store, err := storage.NewBasicStore(cfg)
	if err != nil {
		b.Fatal(err)
	}
	if enablePersistence {
		ctx := context.Background()
		if err := store.StartPersistence(ctx); err != nil {
			b.Fatal(err)
		}
	}
	return store
}

// BenchmarkAOF_WriteSync measures AOF write throughput with sync_policy=always.
func BenchmarkAOF_WriteSync(b *testing.B) {
	dir := b.TempDir()
	store := newBenchStore(b, 500*1024*1024, false, "")
	defer store.Close()

	pcfg := newPersistenceConfig(dir)
	pcfg.SyncPolicy = "always"
	engine := persistence.NewHybridEngine(*pcfg)
	engine.SetSnapshotDataFunc(func() map[string]interface{} { return nil })
	if err := engine.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer engine.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		entry := &persistence.LogEntry{
			Operation: "SET",
			Key:       key,
			Value:     []byte("benchmark-value-payload-1234567890"),
			TTL:       0,
		}
		if err := engine.WriteEntry(entry); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	stats := engine.GetStats()
	b.ReportMetric(float64(stats.AOFSize), "aof_bytes")
}

// BenchmarkAOF_WriteEverySec measures AOF write throughput with sync_policy=everysec.
func BenchmarkAOF_WriteEverySec(b *testing.B) {
	dir := b.TempDir()
	pcfg := newPersistenceConfig(dir)
	pcfg.SyncPolicy = "everysec"
	engine := persistence.NewHybridEngine(*pcfg)
	engine.SetSnapshotDataFunc(func() map[string]interface{} { return nil })
	if err := engine.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer engine.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		entry := &persistence.LogEntry{
			Operation: "SET",
			Key:       key,
			Value:     []byte("benchmark-value-payload-1234567890"),
			TTL:       0,
		}
		if err := engine.WriteEntry(entry); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAOF_WriteNoSync measures AOF write throughput with sync_policy=no.
func BenchmarkAOF_WriteNoSync(b *testing.B) {
	dir := b.TempDir()
	pcfg := newPersistenceConfig(dir)
	pcfg.SyncPolicy = "no"
	engine := persistence.NewHybridEngine(*pcfg)
	engine.SetSnapshotDataFunc(func() map[string]interface{} { return nil })
	if err := engine.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer engine.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		entry := &persistence.LogEntry{
			Operation: "SET",
			Key:       key,
			Value:     []byte("benchmark-value-payload-1234567890"),
			TTL:       0,
		}
		if err := engine.WriteEntry(entry); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshot_Create measures snapshot creation time at various dataset sizes.
func BenchmarkSnapshot_Create(b *testing.B) {
	sizes := []int{1000, 10000, 100000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("items_%d", size), func(b *testing.B) {
			dir := b.TempDir()
			pcfg := newPersistenceConfig(dir)
			engine := persistence.NewHybridEngine(*pcfg)

			// Build dataset
			data := make(map[string]interface{}, size)
			for i := 0; i < size; i++ {
				data[fmt.Sprintf("key-%d", i)] = fmt.Sprintf("value-%d-padding-to-make-it-realistic-length-%d", i, i)
			}
			engine.SetSnapshotDataFunc(func() map[string]interface{} { return data })
			if err := engine.Start(context.Background()); err != nil {
				b.Fatal(err)
			}
			defer engine.Stop()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := engine.CreateSnapshot(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSnapshot_Load measures snapshot load (recovery) time at various sizes.
func BenchmarkSnapshot_Load(b *testing.B) {
	sizes := []int{1000, 10000, 100000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("items_%d", size), func(b *testing.B) {
			dir := b.TempDir()
			pcfg := newPersistenceConfig(dir)
			engine := persistence.NewHybridEngine(*pcfg)

			data := make(map[string]interface{}, size)
			for i := 0; i < size; i++ {
				data[fmt.Sprintf("key-%d", i)] = fmt.Sprintf("value-%d-padding-%d", i, i)
			}
			engine.SetSnapshotDataFunc(func() map[string]interface{} { return data })
			if err := engine.Start(context.Background()); err != nil {
				b.Fatal(err)
			}
			// Create one snapshot to load from
			if err := engine.CreateSnapshot(data); err != nil {
				b.Fatal(err)
			}
			engine.Stop()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				loaded, err := engine.LoadSnapshot()
				if err != nil {
					b.Fatal(err)
				}
				if len(loaded) != size {
					b.Fatalf("expected %d items, got %d", size, len(loaded))
				}
			}
		})
	}
}

// BenchmarkRecovery_Full measures full store recovery (snapshot + AOF replay).
func BenchmarkRecovery_Full(b *testing.B) {
	sizes := []int{1000, 10000, 50000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("items_%d", size), func(b *testing.B) {
			dir := b.TempDir()

			// Phase 1: populate store with persistence
			store := newBenchStore(b, 500*1024*1024, true, dir)
			ctx := context.Background()
			for i := 0; i < size; i++ {
				key := fmt.Sprintf("key-%d", i)
				val := fmt.Sprintf("value-%d-realistic-payload", i)
				if err := store.SetWithContext(ctx, key, val, "", 0); err != nil {
					b.Fatal(err)
				}
			}
			store.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Phase 2: recover from persistence
				recovered := newBenchStore(b, 500*1024*1024, true, dir)
				recovered.Close()
			}
		})
	}
}

// ============================================================================
// THROUGHPUT BENCHMARKS — Realistic Workload Profiles
// ============================================================================

// BenchmarkWorkload_ReadHeavy simulates 95% GET / 5% SET (typical web cache).
func BenchmarkWorkload_ReadHeavy(b *testing.B) {
	store := newBenchStore(b, 500*1024*1024, false, "")
	defer store.Close()
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 10000; i++ {
		store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i), "", 0)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			if rng.Intn(100) < 5 {
				key := fmt.Sprintf("key-%d", rng.Intn(10000))
				store.SetWithContext(ctx, key, "updated-value", "", 0)
			} else {
				key := fmt.Sprintf("key-%d", rng.Intn(10000))
				store.Get(key)
			}
		}
	})
}

// BenchmarkWorkload_WriteHeavy simulates 20% GET / 80% SET (session store).
func BenchmarkWorkload_WriteHeavy(b *testing.B) {
	store := newBenchStore(b, 500*1024*1024, false, "")
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i), "", 0)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			if rng.Intn(100) < 20 {
				store.Get(fmt.Sprintf("key-%d", rng.Intn(1000)))
			} else {
				key := fmt.Sprintf("key-%d", rng.Intn(50000))
				store.SetWithContext(ctx, key, "session-data-payload-bytes", "", 0)
			}
		}
	})
}

// BenchmarkWorkload_Mixed simulates 50/50 GET/SET.
func BenchmarkWorkload_Mixed(b *testing.B) {
	store := newBenchStore(b, 500*1024*1024, false, "")
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 5000; i++ {
		store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i), "", 0)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			key := fmt.Sprintf("key-%d", rng.Intn(10000))
			if rng.Intn(2) == 0 {
				store.Get(key)
			} else {
				store.SetWithContext(ctx, key, "mixed-workload-value", "", 0)
			}
		}
	})
}

// ============================================================================
// PAYLOAD SIZE BENCHMARKS
// ============================================================================

// BenchmarkPayloadSize measures throughput across different value sizes.
func BenchmarkPayloadSize(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"64B", 64},
		{"256B", 256},
		{"1KB", 1024},
		{"4KB", 4096},
		{"16KB", 16384},
		{"64KB", 65536},
	}

	for _, s := range sizes {
		b.Run(fmt.Sprintf("Set_%s", s.name), func(b *testing.B) {
			store := newBenchStore(b, 1024*1024*1024, false, "")
			defer store.Close()
			ctx := context.Background()
			value := string(make([]byte, s.size))

			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), value, "", 0)
			}
		})

		b.Run(fmt.Sprintf("Get_%s", s.name), func(b *testing.B) {
			store := newBenchStore(b, 1024*1024*1024, false, "")
			defer store.Close()
			ctx := context.Background()
			value := string(make([]byte, s.size))

			for i := 0; i < 10000; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), value, "", 0)
			}

			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				store.Get(fmt.Sprintf("key-%d", i%10000))
			}
		})
	}
}

// ============================================================================
// MEMORY OVERHEAD BENCHMARKS
// ============================================================================

// BenchmarkMemoryOverhead measures memory consumed per key at different value sizes.
func BenchmarkMemoryOverhead(b *testing.B) {
	sizes := []struct {
		name      string
		valueSize int
		count     int
	}{
		{"1K_keys_64B", 64, 1000},
		{"10K_keys_64B", 64, 10000},
		{"100K_keys_64B", 64, 100000},
		{"10K_keys_1KB", 1024, 10000},
		{"10K_keys_4KB", 4096, 10000},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			var memBefore, memAfter runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&memBefore)

			store := newBenchStore(b, 2*1024*1024*1024, false, "")
			ctx := context.Background()
			value := string(make([]byte, s.valueSize))

			for i := 0; i < s.count; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), value, "", 0)
			}

			runtime.GC()
			runtime.ReadMemStats(&memAfter)
			store.Close()

			totalOverhead := memAfter.Alloc - memBefore.Alloc
			perKeyOverhead := totalOverhead / uint64(s.count)
			b.ReportMetric(float64(perKeyOverhead), "bytes/key")
			b.ReportMetric(float64(totalOverhead)/(1024*1024), "total_MB")
		})
	}
}

// ============================================================================
// EVICTION PERFORMANCE
// ============================================================================

// BenchmarkEviction_UnderPressure measures throughput when evictions are actively happening.
func BenchmarkEviction_UnderPressure(b *testing.B) {
	// Small memory limit to force evictions
	store := newBenchStore(b, 10*1024*1024, false, "")
	defer store.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("evict-key-%d-%d", rng.Intn(1000000), i)
			store.SetWithContext(ctx, key, "eviction-test-value-padding-bytes-here", "", 0)
			i++
		}
	})
}

// ============================================================================
// CONCURRENCY SCALING
// ============================================================================

// BenchmarkConcurrencyScaling measures how throughput scales with goroutine count.
func BenchmarkConcurrencyScaling(b *testing.B) {
	goroutineCounts := []int{1, 2, 4, 8, 16, 32, 64}
	for _, numG := range goroutineCounts {
		b.Run(fmt.Sprintf("goroutines_%d", numG), func(b *testing.B) {
			store := newBenchStore(b, 500*1024*1024, false, "")
			defer store.Close()
			ctx := context.Background()

			for i := 0; i < 10000; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), "value", "", 0)
			}

			b.SetParallelism(numG / runtime.GOMAXPROCS(0))
			if b.N < numG {
				b.N = numG
			}
			b.ResetTimer()

			var ops int64
			var wg sync.WaitGroup
			opsPerGoroutine := b.N / numG
			if opsPerGoroutine < 1 {
				opsPerGoroutine = 1
			}

			for g := 0; g < numG; g++ {
				wg.Add(1)
				go func(gid int) {
					defer wg.Done()
					rng := rand.New(rand.NewSource(time.Now().UnixNano()))
					for j := 0; j < opsPerGoroutine; j++ {
						key := fmt.Sprintf("key-%d", rng.Intn(10000))
						if rng.Intn(2) == 0 {
							store.Get(key)
						} else {
							store.SetWithContext(ctx, key, "concurrency-test", "", 0)
						}
						atomic.AddInt64(&ops, 1)
					}
				}(g)
			}
			wg.Wait()
			b.ReportMetric(float64(atomic.LoadInt64(&ops)), "total_ops")
		})
	}
}

// ============================================================================
// GC PRESSURE
// ============================================================================

// BenchmarkGCPressure measures GC pause distribution at large key counts.
func BenchmarkGCPressure(b *testing.B) {
	keyCounts := []int{10000, 100000, 1000000}
	for _, count := range keyCounts {
		b.Run(fmt.Sprintf("keys_%d", count), func(b *testing.B) {
			store := newBenchStore(b, 2*1024*1024*1024, false, "")
			ctx := context.Background()

			// Populate
			for i := 0; i < count; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("k-%d", i), fmt.Sprintf("v-%d", i), "", 0)
			}

			// Force GC and measure
			var stats runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&stats)

			b.ReportMetric(float64(stats.PauseTotalNs)/1e6, "gc_pause_ms")
			b.ReportMetric(float64(stats.NumGC), "gc_cycles")
			b.ReportMetric(float64(stats.Alloc)/(1024*1024), "heap_MB")

			// Now benchmark operations with GC pressure
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("k-%d", i%count)
				store.Get(key)
			}
			store.Close()
		})
	}
}

// ============================================================================
// PERSISTENCE + OPERATIONS (Write Amplification)
// ============================================================================

// BenchmarkSetWithPersistence measures SET throughput with persistence enabled.
func BenchmarkSetWithPersistence(b *testing.B) {
	policies := []string{"always", "everysec", "no"}
	for _, policy := range policies {
		b.Run(fmt.Sprintf("sync_%s", policy), func(b *testing.B) {
			dir := b.TempDir()
			cfg := storage.BasicStoreConfig{
				Name:      "bench-persist",
				MaxMemory: 500 * 1024 * 1024,
				PersistenceConfig: &persistence.PersistenceConfig{
					Enabled:          true,
					DataDirectory:    dir,
					Strategy:         "aof",
					EnableAOF:        true,
					SyncPolicy:       policy,
					SyncInterval:     time.Second,
					SnapshotInterval: time.Hour,
					MaxLogSize:       500 * 1024 * 1024,
					RetainLogs:       1,
				},
			}
			store, err := storage.NewBasicStore(cfg)
			if err != nil {
				b.Fatal(err)
			}
			if err := store.StartPersistence(context.Background()); err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key-%d", i)
				store.SetWithContext(ctx, key, "persistence-benchmark-value", "", 0)
			}
		})
	}
}

// ============================================================================
// THUNDERING HERD / HOT KEY
// ============================================================================

// BenchmarkHotKey_Contention measures latency when all goroutines hit the same key.
func BenchmarkHotKey_Contention(b *testing.B) {
	store := newBenchStore(b, 500*1024*1024, false, "")
	defer store.Close()
	ctx := context.Background()
	store.SetWithContext(ctx, "hot-key", "initial-value", "", 0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			if rng.Intn(10) < 3 { // 30% writes
				store.SetWithContext(ctx, "hot-key", "updated-value", "", 0)
			} else {
				store.Get("hot-key")
			}
		}
	})
}

// ============================================================================
// TTL EXPIRATION OVERHEAD
// ============================================================================

// BenchmarkTTL_SetWithExpiry measures SET cost with various TTL durations.
func BenchmarkTTL_SetWithExpiry(b *testing.B) {
	ttls := []time.Duration{0, time.Second, time.Minute, time.Hour}
	for _, ttl := range ttls {
		name := "no_ttl"
		if ttl > 0 {
			name = ttl.String()
		}
		b.Run(name, func(b *testing.B) {
			store := newBenchStore(b, 500*1024*1024, false, "")
			defer store.Close()
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				store.SetWithContext(ctx, fmt.Sprintf("ttl-key-%d", i), "ttl-value", "", ttl)
			}
		})
	}
}

// ============================================================================
// HASH RING UNDER CHURN
// ============================================================================

// BenchmarkHashRing_LookupUnderChurn measures lookup latency while nodes join/leave.
func BenchmarkHashRing_LookupUnderChurn(b *testing.B) {
	// This is tested separately in cluster benchmarks
	// Placeholder for integration-level hash ring churn testing
	b.Skip("Requires cluster package integration — run via tests/stress/")
}

// ============================================================================
// BATCH OPERATIONS
// ============================================================================

// BenchmarkBatchSet measures throughput of sequential batch writes.
func BenchmarkBatchSet(b *testing.B) {
	batchSizes := []int{10, 100, 1000}
	for _, bs := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", bs), func(b *testing.B) {
			store := newBenchStore(b, 1024*1024*1024, false, "")
			defer store.Close()
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < bs; j++ {
					key := fmt.Sprintf("batch-%d-%d", i, j)
					store.SetWithContext(ctx, key, "batch-value", "", 0)
				}
			}
			b.ReportMetric(float64(bs), "batch_size")
		})
	}
}

// ============================================================================
// HELPER: Results Summary
// ============================================================================

// TestBenchmarkEnvironment logs the test environment for reproducibility.
func TestBenchmarkEnvironment(t *testing.T) {
	t.Logf("Go Version: %s", runtime.Version())
	t.Logf("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	t.Logf("NumCPU: %d", runtime.NumCPU())
	t.Logf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))

	dir, _ := os.Getwd()
	t.Logf("Working Dir: %s", dir)
	_ = filepath.Join(dir) // suppress unused import
}
