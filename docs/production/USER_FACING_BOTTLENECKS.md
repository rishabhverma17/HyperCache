# HyperCache — User-Facing Bottlenecks & Operational Risks

This document identifies every pain point a user or operator will encounter with HyperCache in production, organized by lifecycle phase.

---

## 1. Initial Setup & Configuration

### 1.1 Configuration Complexity
**Problem:** YAML configuration has 50+ parameters across 8 config sections. A misconfigured `max_memory`, `sync_policy`, or `gossip_port` silently degrades performance or causes data loss.

**Specific risks:**
- `max_memory` uses strings ("8GB") that are parsed at runtime — typos like "8bg" or "8 GB" could cause unexpected behavior
- `sync_policy: "no"` means data written in the last OS-flush window (~30s) is lost on crash — but nothing warns the user
- `gossip_port` must be reachable between all nodes — firewalls silently break replication

**Mitigation needed:**
- Config validator that warns about dangerous settings at startup
- `hypercache config validate <path>` CLI command
- Sensible defaults that are safe (current defaults are reasonable)

### 1.2 Dependency on Seed Nodes
**Problem:** The first node must start without seeds. Subsequent nodes need at least one seed address. If all seed nodes crash before new nodes join, the cluster cannot reform.

**Specific risk:** ~~No built-in service discovery — users must hardcode seed IPs or manage DNS externally.~~

**Status:** ✅ **DNS-based seed discovery** implemented via `seed_dns` config option. Set it to a Kubernetes headless Service name or Docker Compose service, and all pod/container IPs are resolved automatically via DNS A-record lookup. Static seeds still supported for manual deployments. Bare hostnames (without port) auto-resolve and attach the gossip port.

**Config example (K8s):**
```yaml
cluster:
  seed_dns: "hypercache-headless.default.svc.cluster.local"
  seed_dns_port: 7946
```

### 1.3 Port Management
**Problem:** Each node needs 3 ports (RESP 8080, HTTP 9080, Gossip 7946). In Docker/K8s this requires explicit port mapping. Running multiple nodes on localhost requires manual port offsetting.

**Current mitigation:** `make cluster` auto-offsets ports. Docker Compose pre-configures ports.

---

## 2. Crash Recovery

### 2.1 Recovery Time
**Problem:** On startup, the node replays snapshot + AOF sequentially. Recovery time scales linearly with dataset size.

**Measured numbers:**
| Dataset Size | Recovery Time |
|-------------|---------------|
| 1,000 items | ~5ms |
| 10,000 items | ~50ms |
| 50,000 items | ~250ms |
| 100,000 items | ~500ms |
| 1,000,000 items | ~5s (estimated) |

**Risk:** Large datasets (>1M keys) will have multi-second recovery, during which the node does not serve requests.

**Mitigation needed:**
- Background recovery (serve stale reads while replaying)
- Incremental snapshot loading
- Progress indicator during recovery

### 2.2 Data Loss Window
**Problem:** With `sync_policy: "everysec"` (default), up to 1 second of writes can be lost on hard crash (kill -9, power loss).

**The tradeoff:**
| Sync Policy | Data Loss Risk | Write Throughput Impact |
|------------|---------------|----------------------|
| `always` | Zero (fsync every write) | 10-100x slower |
| `everysec` | Up to 1 second | Baseline performance |
| `no` | Up to OS buffer flush (~30s) | Fastest |

**Risk:** Users don't understand the tradeoff. Default should be safe, add docs.

### 2.3 AOF Corruption
**Problem:** If a crash happens mid-write, the last line of the AOF can be partially written. HyperCache skips corrupt lines during recovery (good), but logs a warning that may alarm operators.

**Current behavior:** Graceful — corrupt lines are skipped. This is the correct approach (same as Redis).

### 2.4 Snapshot Atomicity
**Problem:** Snapshots are written to a `.tmp` file and atomically renamed only after successful `fsync()`. If crash happens during snapshot creation, the `.tmp` is orphaned.

**Current behavior:** Correct — old snapshot remains valid. Orphaned `.tmp` files are not cleaned up.

**Mitigation needed:** Clean up `.tmp` files on startup.

---

## 3. Cluster Operations

### 3.1 Gossip Propagation Delay
**Problem:** After a write, data takes 50-500ms to propagate to other nodes via gossip. During this window, reads on other nodes return stale data or miss.

**Impact:** Session data written to node-1 may not be readable from node-2 for ~200ms.

**Current mitigation:** Read-repair queries peers on local miss (2-second timeout). This closes the gap for GETs but adds latency to misses.

**Remaining risk:** No strong consistency option — users who need read-after-write consistency must route to the same node.

### 3.2 Split-Brain
**Problem:** If a network partition divides a 3-node cluster into [1] and [2], both partitions accept writes. When the partition heals, last-write-wins (Lamport clock) resolves conflicts. This means one partition's writes are silently discarded.

**Risk level:** Medium. This is fundamental to eventually consistent systems. Redis Cluster has the same issue.

**Mitigation needed:**
- Document the consistency model explicitly
- Provide a `consistency_level: "quorum"` option (future roadmap)

### 3.3 Node Addition/Removal
**Problem:** When a node joins or leaves, the hash ring rebalances. Keys that were on node-1 may now route to node-3. The new owner must wait for gossip replication before it has the data.

**Risk:** Brief period of misses for rebalanced keys (~500ms).

**Current mitigation:** Read-repair helps. However, there's no proactive key migration.

**Mitigation needed:**
- Implement active key migration during rebalance
- Document the expected miss window

### 3.4 No Quorum Enforcement
**Problem:** A single-node cluster accepts and serves all requests with no durability beyond local persistence. There's no minimum cluster size enforcement.

---

## 4. Performance Bottlenecks

### 4.1 Memory Tracking Gap
**CRITICAL FINDING:** The MemoryPool tracks only serialized value bytes, not the Go overhead (map entries, struct headers, pointers). A key with a 40-byte value actually consumes ~300-500 bytes in Go (map bucket + CacheItem struct + string header + pointer overhead).

**Impact:** Reported memory usage is 5-10x lower than actual heap consumption. A `max_memory: "1GB"` config may actually use 5-10GB of heap.

**Measured data:**
- Store reports 3.7MB for 100K small keys
- Runtime.MemStats shows actual heap is much higher
- Evictions trigger based on tracked bytes, not real heap pressure

**Mitigation needed:**
- Track overhead per key (map entry ~100B + struct ~200B)
- Or use `runtime.MemStats` sampling to adjust the pool size
- This is a P0 issue for production

### 4.2 Global Lock Contention
**Problem:** ~~`BasicStore` uses a single `sync.RWMutex` for all operations.~~ **RESOLVED**: `BasicStore` now uses `ShardedMap` with 32 independent lock shards. Each key hashes to one shard via `xxhash(key) % 32`, and only that shard's mutex is acquired. Under high concurrency, only 1/32 of keys contend at any time.

**Measured:** Thundering herd (1000 goroutines, 1 key): p95=5.3ms, p99=8.9ms — acceptable but shows contention.

**Status:** ✅ Fixed via sharded locking. Global lock ceiling of ~96K SET/s eliminated.

### 4.3 Serialization Overhead
**Problem:** Every SET serializes the value to `[]byte` via `serializeValue()` and every GET deserializes via `deserializeValue()`. For complex types (maps, slices), this goes through `json.Marshal`/`json.Unmarshal`. Strings and `[]byte` values bypass JSON serialization (native path).

**Measured:** GET with deserialization is 18% slower than without (449ns vs 379ns per op).

**This is a conscious tradeoff:** Serialization enables true memory tracking and persistence. The overhead is acceptable for production. AOF writes are now fully off the hot path (background goroutine via buffered channel).

### 4.4 Cuckoo Filter Memory
**Problem:** The Cuckoo filter consumes fixed memory based on expected capacity, not actual items. A filter sized for 1M keys uses ~1.5MB even with 0 items.

**This is by design:** Probabilistic data structures require pre-allocated space.

---

## 5. Persistence Bottlenecks

### 5.1 AOF Growth
**Problem:** With `sync_policy: "everysec"`, the AOF file grows indefinitely until compaction or snapshot. For write-heavy workloads, AOF can grow several GB per hour.

**Current mitigation:** Background compaction when AOF exceeds `max_log_size` (default 100MB). Snapshots compact the AOF.

**Risk:** If compaction fails or lags behind writes, disk can fill up.

### 5.2 Snapshot Blocking
**Problem:** Snapshot creation serializes the entire dataset using gob encoding. During snapshot, the store is read-locked, blocking all writes.

**Measured:**
| Dataset Size | Snapshot Time |
|-------------|--------------|
| 1,000 items | ~2ms |
| 10,000 items | ~20ms |
| 100,000 items | ~200ms |

**Risk:** 200ms write pause every 15 minutes (default snapshot interval) for 100K items.

**Mitigation for future:**
- Copy-on-write snapshot (fork-based, like Redis BGSAVE)
- Or incremental snapshots

### 5.3 Compression CPU Cost
**Problem:** `compression_level: 6` (default) uses gzip which is CPU-intensive. For large snapshots, compression can consume significant CPU.

**Mitigation:** Use `compression_level: 1` for speed, or `0` to disable.

---

## 6. Observability Gaps

### 6.1 Metrics Endpoint
**Current state:** `/metrics` endpoint exists but reports basic stats. No Prometheus exposition format with histograms for latency percentiles.

**Missing metrics:**
- Latency histograms (p50/p95/p99) per operation
- Replication lag between nodes
- Gossip round-trip time
- Memory pressure level transitions
- Eviction rate over time
- AOF write queue depth

### 6.2 Alerting
**Problem:** No built-in alerting. Users must configure Grafana alerts manually.

### 6.3 Cluster Topology Visualization
**Problem:** No way to see which keys are on which nodes, or how the hash ring is distributed, without custom tooling.

---

## 7. Security

### 7.1 No Authentication
**Problem:** RESP and HTTP APIs have no authentication. Any client with network access can read/write/delete data.

**Mitigation needed:** At minimum, `requirepass` for RESP (like Redis) and API key for HTTP.

### 7.2 No TLS
**Problem:** All communication (client-to-node, node-to-node gossip, HTTP API) is unencrypted.

**Mitigation needed:** TLS for client connections, mTLS for inter-node gossip.

### 7.3 No ACL
**Problem:** No per-key or per-store access control. All clients have full access to all stores.

---

## 8. Performance Reality Check — Server Benchmark Results

**Date:** April 14, 2026 | **Platform:** Apple M3 Pro, 12 cores | **Tool:** redis-benchmark 7.2.5

### Raw Numbers (Post-Optimization)

| Test | SET ops/sec | GET ops/sec | SET p99 | GET p99 |
|------|-----------|-----------|---------|---------|
| 1 client | 38,037 | 48,031 | 0.047ms | 0.031ms |
| 10 clients | 121,507 | 167,504 | 0.175ms | 0.135ms |
| 50 clients | 171,821 | 191,205 | 0.623ms | 0.423ms |
| 100 clients | 165,017 | 189,036 | 1.079ms | 0.743ms |
| Pipeline=16 | 233,645 | 458,716 | 2.359ms | 1.383ms |
| Pipeline=64 | 242,165 | 469,545 | 2.335ms | 1.623ms |
| **Random keys (500K)** | **2,823** | **1,873** | **58.9ms** | **48.6ms** |

### Before/After Optimization Comparison

| Test | Before SET | After SET | **SET Gain** | Before GET | After GET | **GET Gain** |
|------|-----------|-----------|-------------|-----------|-----------|-------------|
| 1 client | 26,674 | 38,037 | **+43%** | 48,379 | 48,031 | ~same |
| 10 clients | 70,572 | 121,507 | **+72%** | 172,414 | 167,504 | ~same |
| 50 clients | 75,245 | 171,821 | **+128%** | 190,114 | 191,205 | ~same |
| 100 clients | 93,458 | 165,017 | **+77%** | 185,874 | 189,036 | ~same |
| Pipeline=16 | 95,420 | 233,645 | **+145%** | 413,223 | 458,716 | **+11%** |
| Pipeline=64 | 96,625 | 242,165 | **+151%** | 440,533 | 469,545 | **+7%** |
| Random 500K | 3,937 | 2,823 | -28% | 2,353 | 1,873 | -20% |

**Key takeaway:** The old global lock ceiling of ~96K SET/s is eliminated. SET now scales to **242K/s** with pipelining (2.5x improvement). The random-key test is still slow due to eviction pressure (see below).

### vs Redis (Same Hardware, Typical Numbers)

| Operation | Redis | HyperCache | Ratio |
|-----------|-------|-----------|-------|
| SET (1 client) | ~110K/s | 38K/s | **35%** |
| GET (1 client) | ~120K/s | 48K/s | **40%** |
| SET (50 clients) | ~300K/s | 172K/s | **57%** |
| GET (50 clients) | ~350K/s | 191K/s | **55%** |
| GET (pipeline=16) | ~1.2M/s | 459K/s | **38%** |

### Why HyperCache Is 2-4x Slower Than Redis

Every HyperCache SET does **6 things** that Redis doesn't:

1. `serializeValue(value)` — for strings/bytes: zero-copy assignment (~0μs); for complex types: `json.Marshal` (~5μs)
2. `memPool.Allocate()` — atomic counter increment for memory tracking (includes 500B per-key overhead)
3. `cuckooFilter.Add(key)` — hash + fingerprint + bucket insert
4. `aofChan <- entry` — non-blocking send to background AOF goroutine (off hot path)
5. `evictPolicy.OnInsert()` — linked-list manipulation for session tracking
6. All under a **per-shard `sync.RWMutex`** (1 of 32 shards, not a global lock)

Redis stores raw bytes. No serialization, no filter, no memory pool abstraction. Single-threaded event loop means zero lock overhead.

Every HyperCache GET does:
1. `item.GetRawBytes()` — returns stored `[]byte` directly (zero deserialization for RESP path)
2. `evictPolicy.OnAccess()` — update the eviction queue
3. All under a **per-shard `RLock`** (1 of 32 shards)

Redis returns the stored bytes directly. Memcpy and done.

### The Remaining Challenge — Test 7 (Random Keys Under Memory Pressure)

Random key workload (500K unique keys, pipeline=16) runs at **2,823 SET/s** — still slow compared to fixed-key tests.

**What changed (improvements):**
- Eviction no longer happens **inside the write lock** — background evictor goroutine handles it asynchronously
- The server **stays responsive** throughout (no catastrophic stall)
- Sharded locks mean concurrent operations on different keys proceed in parallel

**What didn't change (remaining bottleneck):**
- With `PerKeyOverhead` of 500 bytes, the 8GB pool fills faster with accurate tracking
- Background evictor competes for shard locks during heavy eviction
- Eviction policy still walks a linked list to find candidates — O(n) per eviction batch
- p99 latency at 59ms reflects eviction bursts, not lock contention

**Future optimization path:** Probabilistic eviction sampling (Redis-style random sampling instead of full linked-list traversal) would eliminate the eviction walk cost.

### What This Means for Positioning

- **38K-242K SET/s covers most production workloads.** A typical web app does <5K cache ops/sec.
- **470K GET/s with pipelining is genuinely fast.** Read-heavy workloads perform well.
- **SET throughput improved 2.5x** thanks to sharded locks and background AOF writes.
- **The random-key eviction scenario remains slow** but no longer collapses catastrophically. The server degrades gracefully instead of stalling.
- **The pitch is now both features AND performance.** 172K SET/s at 50 clients is competitive for a feature-rich distributed cache.

---

## 9. Operational Excellence Checklist

| Category | Status | Notes |
|----------|--------|-------|
| Config validation | ✅ Done | `hypercache config validate <path>` CLI + semantic warnings |
| Graceful shutdown | ✅ Done | Signal handling, flush persistence |
| Health endpoint | ✅ Done | `/health` returns cluster status |
| Log rotation | ✅ Done | Configurable max files and size |
| Backup/Restore | ⚠️ Manual | Copy snapshot files; no built-in tool |
| Rolling upgrades | ❌ Missing | No version-aware gossip protocol |
| Rate limiting | ❌ Missing | No per-client or per-operation limits |
| Connection pooling | ⚠️ Basic | Max connections enforced, no pool reuse |
| Timeout handling | ✅ Done | Command and idle timeouts configured |

---

## Priority Action Items for Productionization

### P0 (Must fix — these make or break the pitch) — ALL COMPLETE ✅
1. ~~**Eviction under lock collapse**~~ — ✅ **FIXED**: Background evictor goroutine. Eviction never blocks the SET/GET hot path. Memory pressure signals the evictor asynchronously.
2. ~~**Serialization bypass for strings**~~ — ✅ **FIXED**: `GetRawBytes()` returns stored bytes directly without deserialization. RESP `handleGet` uses raw bytes path — zero `string(data)` allocations on the hot path. `serializeValue` already bypasses `json.Marshal` for string/[]byte.
3. ~~**Memory tracking gap**~~ — ✅ **FIXED**: `PerKeyOverhead` constant (500 bytes) added to every allocation. Allocate() now tracks `size + 500` to account for Go map bucket, CacheItem struct, and pointers.
4. ~~**Sharded locks**~~ — ✅ **FIXED**: `ShardedMap` with 32 independent lock shards via `xxhash`. Each key locks only its shard. Eliminates the global lock ceiling.

### Architecture Improvements (Done alongside P0)
- ~~**Full replication → Hash-ring routing**~~ — ✅ Keys route to owner via consistent hashing. Non-owner nodes transparently proxy. Replicates to N replicas (not all nodes).
- ~~**Gossip data replication → Direct HTTP**~~ — ✅ Gossip used only for node discovery/health. Data replication via `POST /internal/replicate`.
- ~~**Background AOF writes**~~ — ✅ 10K buffered channel. AOF completely off the SET critical path.
- ~~**DNS-based seed discovery**~~ — ✅ `seed_dns` config for K8s headless Services and Docker service names.
- ~~**Synchronous DELETE replication**~~ — ✅ DELETEs replicate synchronously for consistency.
- ~~**CuckooFilter FPP default**~~ — ✅ Defaults to 0.01 when unset, preventing cuckoo filter creation failures.

### P1 (Should fix for credibility) — 7 of 8 COMPLETE ✅
5. **Authentication** — at least `requirepass` for RESP and API key for HTTP
6. ~~**Non-blocking snapshots**~~ — ✅ **FIXED**: `SnapshotRawData()` copies data per-shard with brief RLock, then releases. No global read lock during snapshot. Writes to other shards proceed concurrently.
7. ~~**Prometheus-compatible metrics**~~ — ✅ **FIXED**: `internal/metrics/` package with atomic counters and fixed-bucket histograms (no external deps). SET/GET/DEL latency histograms (15 buckets from 10µs to 1s), memory pool pressure/allocations/failures, operation counters. All written in Prometheus text exposition format on `/metrics`.
8. ~~**Config validation CLI**~~ — ✅ **FIXED**: `hypercache config validate <path>` subcommand. Loads, validates, and reports config summary + semantic warnings (dangerous sync_policy, port conflicts, missing seeds, high replication factor, etc.).
9. ~~**.tmp file cleanup**~~ — ✅ **FIXED**: `SnapshotManager.CleanupTempFiles()` called during `HybridEngine.Start()`. Removes orphaned `*.tmp` files from data directory on startup.
10. ~~**RESP connection reuse bug**~~ — ✅ **FIXED**: `CONFIG GET` now returns Redis-compatible key-value array (not empty array). Inline command parsing added to RESP parser (letters only — numeric/symbol prefixes still rejected). `COMMAND DOCS` handled.
11. ~~**Quorum writes**~~ — ✅ **FIXED**: `consistency_level: "quorum"` in config. SET waits for majority of hash-ring replicas to ACK before returning OK. `ReplicateToReplicasQuorum()` sends parallel HTTP replication with 5s timeout, fails early if quorum is unreachable. Works on both RESP and HTTP API paths. Default remains `"eventual"` (async, fire-and-forget).
12. ~~**Random-key eviction performance**~~ — ✅ **FIXED**: `ShardedMap.SampleKeys(5)` randomly samples keys across shards. Background evictor picks the least-recently-accessed from samples — O(1) per eviction round instead of O(n) linked-list walk.

### P2 (Nice to have)
13. **TLS support** — at least for client connections
14. **Document consistency model** — what guarantees users actually get
15. **Active key migration** — reduce miss window during rebalance
16. **Slow query log** — like Redis `SLOWLOG`
17. **`MEMORY USAGE <key>`** — per-key memory analysis
