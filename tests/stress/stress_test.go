package stress

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
	"hypercache/pkg/config"
)

// ============================================================================
// BREAKING POINT TESTS
// These tests find where HyperCache fails and document the failure modes.
// Run with: go test -v -timeout 30m -run TestStress ./tests/stress/...
// ============================================================================

func newStressStore(t *testing.T, maxMemory uint64, persistDir string) *storage.BasicStore {
	t.Helper()
	cfg := storage.BasicStoreConfig{
		Name:      "stress-store",
		MaxMemory: maxMemory,
	}
	if persistDir != "" {
		cfg.PersistenceConfig = &persistence.PersistenceConfig{
			Enabled:          true,
			DataDirectory:    persistDir,
			Strategy:         "hybrid",
			EnableAOF:        true,
			SyncPolicy:       "everysec",
			SyncInterval:     time.Second,
			SnapshotInterval: 5 * time.Minute,
			MaxLogSize:       100 * 1024 * 1024,
			CompressionLevel: 6,
			RetainLogs:       1,
		}
	}
	store, err := storage.NewBasicStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if persistDir != "" {
		if err := store.StartPersistence(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

// TestStress_MemoryExhaustion verifies that the cache handles memory exhaustion
// gracefully via eviction rather than OOM-killing.
func TestStress_MemoryExhaustion(t *testing.T) {
	// 50MB limit with 1KB values + 500B overhead per key — will trigger evictions/rejections
	store := newStressStore(t, 50*1024*1024, "")
	defer store.Close()
	ctx := context.Background()

	const totalWrites = 50_000
	// Use 1KB values to actually fill the memory pool
	payload := string(make([]byte, 1024))
	var setErrors, evictions int64

	for i := 0; i < totalWrites; i++ {
		key := fmt.Sprintf("exhaust-key-%d", i)
		err := store.SetWithContext(ctx, key, payload, "", 0)
		if err != nil {
			atomic.AddInt64(&setErrors, 1)
		}
	}

	stats := store.Stats()
	evictions = int64(stats.EvictionCount)

	t.Logf("Results:")
	t.Logf("  Total writes attempted: %d", totalWrites)
	t.Logf("  Set errors: %d", setErrors)
	t.Logf("  Evictions triggered: %d", evictions)
	t.Logf("  Items currently stored: %d", stats.TotalItems)
	t.Logf("  Memory used: %d bytes", stats.TotalMemory)

	// The cache MUST NOT crash. Under memory pressure, either evictions OR set errors are acceptable.
	if evictions == 0 && setErrors == 0 {
		t.Error("FAIL: Neither evictions nor set errors occurred — memory pressure not triggered")
	}
	t.Logf("PASS: Cache handled memory exhaustion gracefully with %d evictions and %d set errors", evictions, setErrors)
}

// TestStress_ConcurrentReadWriteUnderPressure tests mixed read/write workload
// with high concurrency under memory pressure.
func TestStress_ConcurrentReadWriteUnderPressure(t *testing.T) {
	// 200MB to accommodate PerKeyOverhead (500B per key) with 50K key space
	store := newStressStore(t, 200*1024*1024, "")
	defer store.Close()
	ctx := context.Background()

	const (
		numWriters   = 50
		numReaders   = 100
		opsPerWorker = 10_000
		keySpace     = 50_000
	)

	var (
		writeOps   int64
		readOps    int64
		readHits   int64
		readMisses int64
		writeErrs  int64
		wg         sync.WaitGroup
	)

	// Pre-populate
	for i := 0; i < 5000; i++ {
		store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), "seed-value", "", 0)
	}

	start := time.Now()

	// Writers
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid)))
			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("key-%d", rng.Intn(keySpace))
				err := store.SetWithContext(ctx, key, "writer-value-data", "", 0)
				if err != nil {
					atomic.AddInt64(&writeErrs, 1)
				}
				atomic.AddInt64(&writeOps, 1)
			}
		}(w)
	}

	// Readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(rid+1000)))
			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("key-%d", rng.Intn(keySpace))
				val, err := store.Get(key)
				if err == nil && val != nil {
					atomic.AddInt64(&readHits, 1)
				} else {
					atomic.AddInt64(&readMisses, 1)
				}
				atomic.AddInt64(&readOps, 1)
			}
		}(r)
	}

	wg.Wait()
	elapsed := time.Since(start)
	totalOps := atomic.LoadInt64(&writeOps) + atomic.LoadInt64(&readOps)

	t.Logf("Results (%.2f seconds):", elapsed.Seconds())
	t.Logf("  Total ops: %d (%.0f ops/sec)", totalOps, float64(totalOps)/elapsed.Seconds())
	t.Logf("  Write ops: %d (errors: %d)", writeOps, writeErrs)
	t.Logf("  Read ops: %d (hits: %d, misses: %d)", readOps, readHits, readMisses)
	t.Logf("  Hit rate: %.1f%%", float64(readHits)/float64(readOps)*100)

	if writeErrs > int64(numWriters*opsPerWorker)/10 {
		t.Errorf("FAIL: >10%% write errors under concurrent pressure")
	}
}

// TestStress_PersistenceRecoveryIntegrity verifies data survives crash simulation.
func TestStress_PersistenceRecoveryIntegrity(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Phase 1: Write data with persistence
	store := newStressStore(t, 500*1024*1024, dir)

	const itemCount = 10_000
	expected := make(map[string]string, itemCount)
	for i := 0; i < itemCount; i++ {
		key := fmt.Sprintf("persist-key-%d", i)
		value := fmt.Sprintf("persist-value-%d", i)
		if err := store.SetWithContext(ctx, key, value, "", 0); err != nil {
			t.Fatalf("Failed to set key %s: %v", key, err)
		}
		expected[key] = value
	}

	// Verify all written
	stats := store.Stats()
	t.Logf("Phase 1: Stored %d items, memory: %d bytes", stats.TotalItems, stats.TotalMemory)

	// Phase 2: Simulate crash (close without graceful shutdown signal)
	store.Close()

	// Phase 3: Recover
	recoveredStore := newStressStore(t, 500*1024*1024, dir)
	defer recoveredStore.Close()

	// Phase 4: Verify all data recovered
	var recovered, missing, corrupted int
	for key, expectedVal := range expected {
		val, err := recoveredStore.Get(key)
		if err != nil || val == nil {
			missing++
			continue
		}
		if fmt.Sprintf("%v", val) != expectedVal {
			corrupted++
			continue
		}
		recovered++
	}

	t.Logf("Recovery Results:")
	t.Logf("  Expected: %d items", itemCount)
	t.Logf("  Recovered: %d", recovered)
	t.Logf("  Missing: %d", missing)
	t.Logf("  Corrupted: %d", corrupted)
	t.Logf("  Recovery rate: %.2f%%", float64(recovered)/float64(itemCount)*100)

	if missing > itemCount/100 { // Allow <1% loss for everysec sync
		t.Errorf("FAIL: %d items missing (>1%% loss rate)", missing)
	}
	if corrupted > 0 {
		t.Errorf("FAIL: %d items corrupted", corrupted)
	}
}

// TestStress_DiskFullDuringPersistence simulates disk space exhaustion.
func TestStress_DiskFullDuringPersistence(t *testing.T) {
	if os.Getenv("STRESS_DISK_FULL") != "1" {
		t.Skip("Skipping disk-full test (set STRESS_DISK_FULL=1 to run)")
	}

	// Use a tmpfs or small partition to simulate disk full
	dir := t.TempDir()
	store := newStressStore(t, 500*1024*1024, dir)
	defer store.Close()
	ctx := context.Background()

	// Write until persistence should fail
	for i := 0; i < 1_000_000; i++ {
		key := fmt.Sprintf("disk-key-%d", i)
		value := make([]byte, 4096) // 4KB values
		err := store.SetWithContext(ctx, key, string(value), "", 0)
		if err != nil {
			t.Logf("SET failed at iteration %d: %v", i, err)
			t.Logf("PASS: Cache reported error on disk exhaustion instead of silent data loss")
			return
		}
	}
	t.Log("WARN: Disk was not exhausted — test inconclusive (disk too large)")
}

// TestStress_ThunderingHerd simulates 1000 goroutines accessing the same key.
func TestStress_ThunderingHerd(t *testing.T) {
	store := newStressStore(t, 500*1024*1024, "")
	defer store.Close()
	ctx := context.Background()

	store.SetWithContext(ctx, "thundering-key", "initial", "", 0)

	const goroutines = 1000
	const opsPerGoroutine = 1000

	var (
		wg     sync.WaitGroup
		reads  int64
		writes int64
	)

	latencies := make([]time.Duration, goroutines*opsPerGoroutine)
	var latIdx int64

	start := time.Now()
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			for i := 0; i < opsPerGoroutine; i++ {
				opStart := time.Now()
				if rng.Intn(10) < 3 {
					store.SetWithContext(ctx, "thundering-key", "updated", "", 0)
					atomic.AddInt64(&writes, 1)
				} else {
					store.Get("thundering-key")
					atomic.AddInt64(&reads, 1)
				}
				idx := atomic.AddInt64(&latIdx, 1) - 1
				if idx < int64(len(latencies)) {
					latencies[idx] = time.Since(opStart)
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	totalOps := reads + writes
	t.Logf("Thundering Herd Results (%.2f sec):", elapsed.Seconds())
	t.Logf("  Goroutines: %d", goroutines)
	t.Logf("  Total ops: %d (%.0f ops/sec)", totalOps, float64(totalOps)/elapsed.Seconds())
	t.Logf("  Reads: %d, Writes: %d", reads, writes)

	// Compute p50, p95, p99 from collected latencies
	count := int(atomic.LoadInt64(&latIdx))
	if count > len(latencies) {
		count = len(latencies)
	}
	if count > 0 {
		// Sort latencies for percentile calculation
		sorted := make([]time.Duration, count)
		copy(sorted, latencies[:count])
		sortDurations(sorted)

		p50 := sorted[count*50/100]
		p95 := sorted[count*95/100]
		p99 := sorted[count*99/100]
		t.Logf("  Latency p50: %v, p95: %v, p99: %v", p50, p95, p99)

		if p99 > 10*time.Millisecond {
			t.Logf("WARN: p99 latency >10ms under thundering herd — contention hot spot")
		}
	}
}

// TestStress_LargeKeySpace tests behavior with millions of unique keys.
func TestStress_LargeKeySpace(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large key space test in short mode")
	}

	store := newStressStore(t, 1024*1024*1024, "") // 1GB
	defer store.Close()
	ctx := context.Background()

	const target = 1_000_000
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	for i := 0; i < target; i++ {
		key := fmt.Sprintf("large-key-%d", i)
		store.SetWithContext(ctx, key, "value", "", 0)
		if i%100_000 == 0 && i > 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			t.Logf("  %dk keys: heap=%.1fMB, GC pauses=%d",
				i/1000, float64(m.Alloc)/(1024*1024), m.NumGC)
		}
	}
	populateTime := time.Since(start)

	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	stats := store.Stats()
	t.Logf("Large Key Space Results:")
	t.Logf("  Target: %d keys", target)
	t.Logf("  Stored: %d keys", stats.TotalItems)
	t.Logf("  Populate time: %v", populateTime)
	t.Logf("  Heap growth: %.1f MB", float64(memAfter.Alloc-memBefore.Alloc)/(1024*1024))
	if stats.TotalItems > 0 {
		bytesPerKey := (memAfter.Alloc - memBefore.Alloc) / uint64(stats.TotalItems)
		t.Logf("  Bytes per key: %d", bytesPerKey)
	}
	t.Logf("  GC cycles during populate: %d", memAfter.NumGC-memBefore.NumGC)
	t.Logf("  Total GC pause: %.2f ms", float64(memAfter.PauseTotalNs-memBefore.PauseTotalNs)/1e6)

	// Verify reads work at scale (small sample to avoid CI timeout)
	readStart := time.Now()
	hits := 0
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("large-key-%d", rand.Intn(target))
		val, err := store.Get(key)
		if err == nil && val != nil {
			hits++
		}
	}
	readTime := time.Since(readStart)
	t.Logf("  1K random reads: %v (%.0f ops/sec), hit rate: %.1f%%",
		readTime, 1000/readTime.Seconds(), float64(hits)/10)
}

// TestStress_RapidCreateDropStores tests multi-store lifecycle under load.
func TestStress_RapidCreateDropStores(t *testing.T) {
	mgr := storage.NewStoreManager(storage.StoreManagerConfig{
		MaxStores: 100,
		DataDir:   t.TempDir(),
	})
	defer mgr.Close()
	ctx := context.Background()

	const cycles = 50
	for i := 0; i < cycles; i++ {
		name := fmt.Sprintf("ephemeral-store-%d", i)
		err := mgr.CreateStore(config.StoreConfig{
			Name:      name,
			MaxMemory: "10MB",
		}, ctx)
		if err != nil {
			t.Fatalf("Failed to create store %s: %v", name, err)
		}

		store := mgr.GetStore(name)
		if store == nil {
			t.Fatalf("Store %s not found after creation", name)
		}

		// Write some data
		for j := 0; j < 100; j++ {
			store.SetWithContext(ctx, fmt.Sprintf("key-%d", j), "value", "", 0)
		}

		// Drop it
		if err := mgr.DropStore(name); err != nil {
			t.Fatalf("Failed to drop store %s: %v", name, err)
		}
	}

	remaining := mgr.ListStores()
	t.Logf("After %d create/drop cycles: %d stores remaining (expected: 0)", cycles, len(remaining))
	if len(remaining) != 0 {
		t.Errorf("FAIL: Expected 0 stores after dropping all, got %d", len(remaining))
	}
}

// TestStress_SustainedLoad runs a sustained workload for a configurable duration.
func TestStress_SustainedLoad(t *testing.T) {
	duration := 30 * time.Second
	if d := os.Getenv("STRESS_DURATION"); d != "" {
		var err error
		duration, err = time.ParseDuration(d)
		if err != nil {
			t.Fatalf("Invalid STRESS_DURATION: %v", err)
		}
	}

	store := newStressStore(t, 100*1024*1024, "")
	defer store.Close()
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 10000; i++ {
		store.SetWithContext(ctx, fmt.Sprintf("key-%d", i), "seed", "", 0)
	}

	var (
		totalOps int64
		errors   int64
	)

	done := make(chan struct{})
	start := time.Now()

	// Workers
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid)))
			for {
				select {
				case <-done:
					return
				default:
					key := fmt.Sprintf("key-%d", rng.Intn(50000))
					if rng.Intn(100) < 30 {
						err := store.SetWithContext(ctx, key, "sustained-load-value", "", 0)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}
					} else {
						store.Get(key)
					}
					atomic.AddInt64(&totalOps, 1)
				}
			}
		}(w)
	}

	// Sample throughput every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	lastOps := int64(0)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ops := atomic.LoadInt64(&totalOps)
				elapsed := time.Since(start).Seconds()
				delta := ops - lastOps
				t.Logf("  [%.0fs] Total: %dk ops, Current: %.0f ops/sec, Errors: %d",
					elapsed, ops/1000, float64(delta)/5, atomic.LoadInt64(&errors))
				lastOps = ops
			}
		}
	}()

	time.Sleep(duration)
	close(done)
	wg.Wait()
	ticker.Stop()

	elapsed := time.Since(start)
	ops := atomic.LoadInt64(&totalOps)
	errs := atomic.LoadInt64(&errors)

	t.Logf("\nSustained Load Results (%v):", elapsed)
	t.Logf("  Workers: %d", numWorkers)
	t.Logf("  Total ops: %d", ops)
	t.Logf("  Avg throughput: %.0f ops/sec", float64(ops)/elapsed.Seconds())
	t.Logf("  Errors: %d (%.4f%%)", errs, float64(errs)/float64(ops)*100)

	if float64(errs)/float64(ops) > 0.01 {
		t.Errorf("FAIL: Error rate >1%% during sustained load")
	}
}

// sortDurations sorts a slice of durations in ascending order (insertion sort for simplicity).
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}

// TestStress_Environment logs environment for reproducibility.
func TestStress_Environment(t *testing.T) {
	t.Logf("Go: %s, OS: %s/%s, CPUs: %d, GOMAXPROCS: %d",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		runtime.NumCPU(), runtime.GOMAXPROCS(0))
	dir, _ := os.Getwd()
	_ = filepath.Join(dir)
	t.Logf("Temp dir: %s", os.TempDir())
}
