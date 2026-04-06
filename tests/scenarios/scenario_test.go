package scenarios

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hypercache/internal/persistence"
	"hypercache/internal/storage"
	"hypercache/pkg/config"
)

// --- Helpers ---

func newTestStore(t *testing.T, name string, maxMemory uint64) *storage.BasicStore {
	t.Helper()
	store, err := storage.NewBasicStore(storage.BasicStoreConfig{
		Name:             name,
		MaxMemory:        maxMemory,
		DefaultTTL:       0,
		EnableStatistics: true,
		CleanupInterval:  time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create store %s: %v", name, err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func newTestStoreManager(t *testing.T, maxStores int) *storage.StoreManager {
	t.Helper()
	dir := t.TempDir()
	sm := storage.NewStoreManager(storage.StoreManagerConfig{
		DataDir:   dir,
		MaxStores: maxStores,
		GlobalPersistence: config.PersistenceConfig{
			Enabled:  false,
			Strategy: "disabled",
		},
		GlobalCacheConfig: config.CacheConfig{
			MaxMemory:       "1GB",
			DefaultTTL:      "0",
			CuckooFilterFPP: 0.01,
			MaxStores:       maxStores,
		},
	})
	t.Cleanup(func() { sm.Close() })
	return sm
}

func scenarioSeed(t *testing.T) int64 {
	t.Helper()
	if s := os.Getenv("SCENARIO_SEED"); s != "" {
		seed, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			t.Logf("Using SCENARIO_SEED=%d from env", seed)
			return seed
		}
	}
	seed := time.Now().UnixNano()
	t.Logf("Scenario seed=%d (set SCENARIO_SEED=%d to reproduce)", seed, seed)
	return seed
}

func makePersistenceConfig(dir string) persistence.PersistenceConfig {
	return persistence.PersistenceConfig{
		Enabled:          true,
		Strategy:         "aof",
		DataDirectory:    dir,
		EnableAOF:        true,
		SyncPolicy:       "always",
		SyncInterval:     time.Second,
		SnapshotInterval: 0,
		MaxLogSize:       100 * 1024 * 1024,
		CompressionLevel: 0,
		RetainLogs:       1,
	}
}

// =============================================
// Deterministic Scenarios — same every run
// =============================================

// TestSessionStoreOverflow fills a store past capacity and verifies eviction.
func TestSessionStoreOverflow(t *testing.T) {
	store := newTestStore(t, "session-overflow", 100*1024) // 100KB

	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("session:%04d", i)
		value := fmt.Sprintf("token-%s-pad-%0200d", key, i)
		err := store.Set(key, value, "overflow-test", 0)
		if err != nil {
			t.Logf("Set %s error (expected under pressure): %v", key, err)
		}
	}

	stats := store.Stats()
	t.Logf("After 200 writes to 100KB store: items=%d memory=%d evictions=%d",
		stats.TotalItems, stats.TotalMemory, stats.EvictionCount)

	if stats.TotalMemory > 100*1024 {
		t.Errorf("Memory %d exceeds 100KB — eviction failed", stats.TotalMemory)
	}
	if stats.TotalItems == 0 {
		t.Error("Store empty — all writes failed")
	}
}

// TestConcurrentReadWrite verifies no data races under concurrent access.
func TestConcurrentReadWrite(t *testing.T) {
	store := newTestStore(t, "concurrent-rw", 10*1024*1024) // 10MB

	const numKeys = 100
	const workers = 20
	const opsPerWorker = 500

	for i := 0; i < numKeys; i++ {
		store.Set(fmt.Sprintf("key:%d", i), fmt.Sprintf("init-%d", i), "init", time.Hour)
	}

	var wg sync.WaitGroup
	var errors atomic.Int64

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for op := 0; op < opsPerWorker; op++ {
				key := fmt.Sprintf("key:%d", (id*opsPerWorker+op)%numKeys)
				if op%3 == 0 {
					if err := store.Set(key, fmt.Sprintf("w%d-op%d", id, op), "worker", time.Hour); err != nil {
						errors.Add(1)
					}
				} else {
					store.Get(key)
				}
			}
		}(w)
	}

	wg.Wait()
	t.Logf("Concurrent: %d workers x %d ops, errors=%d", workers, opsPerWorker, errors.Load())

	if errors.Load() > int64(workers*opsPerWorker/10) {
		t.Errorf("Too many errors: %d", errors.Load())
	}
}

// TestTTLExpiry verifies keys expire within a reasonable window.
func TestTTLExpiry(t *testing.T) {
	store := newTestStore(t, "ttl-expiry", 10*1024*1024)

	for i := 0; i < 50; i++ {
		store.Set(fmt.Sprintf("ttl:%d", i), "short-lived", "ttl-test", 1*time.Second)
	}

	found := 0
	for i := 0; i < 50; i++ {
		if _, err := store.Get(fmt.Sprintf("ttl:%d", i)); err == nil {
			found++
		}
	}
	if found < 45 {
		t.Errorf("Expected most keys immediately, found %d/50", found)
	}

	time.Sleep(3 * time.Second)

	found = 0
	for i := 0; i < 50; i++ {
		if _, err := store.Get(fmt.Sprintf("ttl:%d", i)); err == nil {
			found++
		}
	}
	t.Logf("TTL: %d/50 keys remain after 3s", found)
	if found > 5 {
		t.Errorf("Expected most expired, %d/50 still exist", found)
	}
}

// TestPersistenceRecovery writes data, stops, restarts, verifies recovery.
func TestPersistenceRecovery(t *testing.T) {
	dir := t.TempDir()
	pcfg := makePersistenceConfig(dir)

	// Phase 1: write and close
	func() {
		store, err := storage.NewBasicStore(storage.BasicStoreConfig{
			Name:              "persist-test",
			MaxMemory:         10 * 1024 * 1024,
			EnableStatistics:  true,
			CleanupInterval:   time.Minute,
			PersistenceConfig: &pcfg,
		})
		if err != nil {
			t.Fatalf("Create store: %v", err)
		}
		defer store.Close()

		if err := store.StartPersistence(context.Background()); err != nil {
			t.Fatalf("Start persistence: %v", err)
		}
		for i := 0; i < 20; i++ {
			store.Set(fmt.Sprintf("p:%d", i), fmt.Sprintf("val-%d", i), "test", 0)
		}
		store.StopPersistence()
	}()

	// Phase 2: recover
	store2, err := storage.NewBasicStore(storage.BasicStoreConfig{
		Name:              "persist-test",
		MaxMemory:         10 * 1024 * 1024,
		EnableStatistics:  true,
		CleanupInterval:   time.Minute,
		PersistenceConfig: &pcfg,
	})
	if err != nil {
		t.Fatalf("Create recovery store: %v", err)
	}
	defer store2.Close()

	if err := store2.StartPersistence(context.Background()); err != nil {
		t.Fatalf("Start recovery persistence: %v", err)
	}

	recovered := 0
	for i := 0; i < 20; i++ {
		val, err := store2.Get(fmt.Sprintf("p:%d", i))
		if err == nil && val == fmt.Sprintf("val-%d", i) {
			recovered++
		}
	}
	t.Logf("Persistence: %d/20 keys recovered", recovered)
	if recovered < 20 {
		t.Errorf("Expected 20 recovered, got %d", recovered)
	}
	store2.StopPersistence()
}

// TestStoreLifecycle tests create -> fill -> drop -> verify cleanup.
func TestStoreLifecycle(t *testing.T) {
	sm := newTestStoreManager(t, 8)
	ctx := context.Background()

	err := sm.CreateStore(config.StoreConfig{
		Name: "lifecycle", EvictionPolicy: "lru", MaxMemory: "1MB", DefaultTTL: "0",
	}, ctx)
	if err != nil {
		t.Fatalf("CreateStore: %v", err)
	}

	store := sm.GetStore("lifecycle")
	if store == nil {
		t.Fatal("Store not found")
	}

	for i := 0; i < 100; i++ {
		store.Set(fmt.Sprintf("k:%d", i), fmt.Sprintf("v-%d", i), "test", time.Hour)
	}
	if store.Size() == 0 {
		t.Error("Store empty after writes")
	}

	if err := sm.DropStore("lifecycle"); err != nil {
		t.Fatalf("DropStore: %v", err)
	}
	if sm.GetStore("lifecycle") != nil {
		t.Error("Store still exists after drop")
	}
}

// TestHotKeyThunderingHerd simulates 100 goroutines hitting the same key.
func TestHotKeyThunderingHerd(t *testing.T) {
	store := newTestStore(t, "thundering-herd", 10*1024*1024)
	store.Set("hot-key", "initial-value", "setup", time.Hour)

	const concurrency = 100
	const reads = 1000
	var wg sync.WaitGroup
	var hits atomic.Int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reads; j++ {
				if _, err := store.Get("hot-key"); err == nil {
					hits.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	total := int64(concurrency * reads)
	t.Logf("Hot key: %d/%d hits (%.1f%%)", hits.Load(), total, float64(hits.Load())/float64(total)*100)
	if hits.Load() < total*95/100 {
		t.Errorf("Hit rate <95%%: %d/%d", hits.Load(), total)
	}
}

// =============================================
// Randomized Scenarios — different each run
// =============================================

// TestRandomizedMixedWorkload runs a seed-based read/write/delete workload.
func TestRandomizedMixedWorkload(t *testing.T) {
	seed := scenarioSeed(t)
	rng := rand.New(rand.NewSource(seed))
	store := newTestStore(t, "mixed-workload", 5*1024*1024)

	numKeys := 200 + rng.Intn(800)
	numOps := 1000 + rng.Intn(4000)
	writeRatio := 0.1 + rng.Float64()*0.3
	deleteRatio := rng.Float64() * 0.1

	t.Logf("Workload: keys=%d ops=%d writes=%.0f%% deletes=%.0f%%",
		numKeys, numOps, writeRatio*100, deleteRatio*100)

	for i := 0; i < numKeys/2; i++ {
		store.Set(fmt.Sprintf("rk:%d", i), fmt.Sprintf("init-%d", i), "seed", time.Hour)
	}

	var reads, writes, deletes, readHits int
	for op := 0; op < numOps; op++ {
		key := fmt.Sprintf("rk:%d", rng.Intn(numKeys))
		roll := rng.Float64()

		if roll < deleteRatio {
			store.Delete(key)
			deletes++
		} else if roll < deleteRatio+writeRatio {
			ttl := time.Duration(rng.Intn(3600)) * time.Second
			if rng.Float64() < 0.3 {
				ttl = 0
			}
			store.Set(key, fmt.Sprintf("v-%d-%d", op, rng.Int()), "mixed", ttl)
			writes++
		} else {
			_, err := store.Get(key)
			reads++
			if err == nil {
				readHits++
			}
		}
	}

	stats := store.Stats()
	t.Logf("Results: reads=%d(hits=%d) writes=%d deletes=%d | items=%d mem=%d evictions=%d",
		reads, readHits, writes, deletes, stats.TotalItems, stats.TotalMemory, stats.EvictionCount)

	if stats.TotalMemory > 5*1024*1024 {
		t.Errorf("Memory %d exceeds 5MB", stats.TotalMemory)
	}
}

// TestRandomizedBurstWrite simulates bursty write patterns with varying sizes.
func TestRandomizedBurstWrite(t *testing.T) {
	seed := scenarioSeed(t)
	rng := rand.New(rand.NewSource(seed))
	store := newTestStore(t, "burst-write", 2*1024*1024)

	numBursts := 3 + rng.Intn(5)
	var totalWritten int

	for burst := 0; burst < numBursts; burst++ {
		burstSize := 50 + rng.Intn(150)
		valueSize := 100 + rng.Intn(900)
		ttl := time.Duration(rng.Intn(5)+1) * time.Second

		t.Logf("Burst %d: %d keys x ~%dB, TTL=%v", burst, burstSize, valueSize, ttl)

		for i := 0; i < burstSize; i++ {
			key := fmt.Sprintf("b:%d:%d", burst, i)
			val := make([]byte, valueSize)
			rng.Read(val)
			store.Set(key, string(val), "burst", ttl)
			totalWritten++
		}
	}

	stats := store.Stats()
	t.Logf("After %d bursts (%d writes): items=%d mem=%d evictions=%d",
		numBursts, totalWritten, stats.TotalItems, stats.TotalMemory, stats.EvictionCount)

	if stats.TotalMemory > 2*1024*1024 {
		t.Errorf("Memory %d exceeds 2MB", stats.TotalMemory)
	}
}

// TestRandomizedConcurrentMultiStore tests multiple stores under concurrent load.
func TestRandomizedConcurrentMultiStore(t *testing.T) {
	seed := scenarioSeed(t)
	rng := rand.New(rand.NewSource(seed))
	sm := newTestStoreManager(t, 8)
	ctx := context.Background()

	sm.CreateStore(config.StoreConfig{
		Name: "default", EvictionPolicy: "lru", MaxMemory: "2MB", DefaultTTL: "0",
	}, ctx)

	numExtra := 2 + rng.Intn(3)
	storeNames := []string{"default"}
	for i := 0; i < numExtra; i++ {
		name := fmt.Sprintf("store-%d", i)
		if err := sm.CreateStore(config.StoreConfig{
			Name: name, EvictionPolicy: "lru", MaxMemory: "1MB", DefaultTTL: "0",
		}, ctx); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		storeNames = append(storeNames, name)
	}

	t.Logf("Stores: %v", storeNames)

	const workers = 10
	const opsPerWorker = 200
	var wg sync.WaitGroup
	var totalOps atomic.Int64

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lr := rand.New(rand.NewSource(seed + int64(id)))
			for op := 0; op < opsPerWorker; op++ {
				sn := storeNames[lr.Intn(len(storeNames))]
				s := sm.GetStore(sn)
				if s == nil {
					continue
				}
				key := fmt.Sprintf("w%d:%d", id, lr.Intn(50))
				if lr.Float64() < 0.3 {
					s.Set(key, fmt.Sprintf("v-%d", op), "worker", time.Hour)
				} else {
					s.Get(key)
				}
				totalOps.Add(1)
			}
		}(w)
	}

	wg.Wait()
	t.Logf("Multi-store: %d ops across %d stores", totalOps.Load(), len(storeNames))

	for _, name := range storeNames {
		if sm.GetStore(name) == nil {
			t.Errorf("Store %s disappeared", name)
		}
	}
}
