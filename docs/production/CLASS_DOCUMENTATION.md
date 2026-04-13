# HyperCache — Complete Class & Module Documentation

This document provides detailed documentation of every class (struct), interface, and module in HyperCache: what it does, how it works internally, why it exists, and the design decisions behind it.

---

## Table of Contents

1. [Entry Point — `cmd/hypercache/main.go`](#1-entry-point)
2. [Configuration — `pkg/config`](#2-configuration)
3. [Cache Abstractions — `internal/cache`](#3-cache-abstractions)
4. [Storage Engine — `internal/storage`](#4-storage-engine)
5. [Probabilistic Filter — `internal/filter`](#5-probabilistic-filter)
6. [Persistence — `internal/persistence`](#6-persistence)
7. [Cluster Coordination — `internal/cluster`](#7-cluster-coordination)
8. [Network Layer — `internal/network`](#8-network-layer)
9. [Logging — `internal/logging`](#9-logging)

---

## 1. Entry Point

### `main.go` — Application Bootstrap

**What it does:** Wires all components together and starts the application lifecycle.

**Boot sequence:**
1. Parse CLI flags (`-config`, `-node-id`, `-protocol`, `-port`)
2. Load YAML config with `config.Load()` → merges defaults + file + env vars
3. Initialize structured logging via `logging.InitializeFromConfig()`
4. Create data directories for persistence
5. Create `StoreManager` with configured stores
6. Create `DistributedCoordinator` (or `SimpleCoordinator` for single-node)
7. Start RESP protocol server on port 8080
8. Start HTTP API server on port 9080
9. Subscribe to gossip events → `handleReplicationEvent()` callback
10. Block on OS signals (SIGINT/SIGTERM) for graceful shutdown

**Why this design:** The entry point is intentionally thin — it's an orchestrator, not an implementor. Every component is injected via interfaces, making the system testable and modular.

**Replication handler (`handleReplicationEvent`):**
- Receives gossip events from other nodes (SET, DELETE)
- Calls `store.SetWithTimestamp()` which uses Lamport clock comparison
- If the incoming timestamp is newer, the write is applied; if stale, it's rejected
- This prevents "last-write-wins" from being "random-write-wins" — causality is preserved

---

## 2. Configuration

### `Config` — Central Configuration

**File:** `pkg/config/config.go`

**What it does:** Provides typed access to all runtime configuration with validation and defaults.

**Sub-configs:**

| Struct | Purpose | Key Fields |
|--------|---------|-----------|
| `NodeConfig` | Node identity | `ID`, `DataDir` |
| `NetworkConfig` | Port bindings | `RESPPort`, `HTTPPort`, `GossipPort`, `AdvertiseAddr` |
| `ClusterConfig` | Clustering | `Seeds[]`, `ReplicationFactor`, `ConsistencyLevel` |
| `StorageConfig` | WAL tuning | `WALSyncInterval`, `MemtableSize`, `CompactionThreads` |
| `CacheConfig` | Memory limits | `MaxMemory`, `DefaultTTL`, `CuckooFilterFPP`, `MaxStores` |
| `PersistenceConfig` | Durability | `Strategy`, `SyncPolicy`, `SnapshotInterval`, `CompressionLevel` |
| `LoggingConfig` | Observability | `Level`, `EnableConsole`, `EnableFile`, `BufferSize` |
| `StoreConfig` | Per-store overrides | `Name`, `EvictionPolicy`, `MaxMemory`, `DefaultTTL` |

**Why it exists:** A single source of truth prevents scattered hardcoded values. YAML format was chosen over JSON for human readability and comment support.

**Design decisions:**
- **Environment variable overrides:** Enables Docker/K8s deployment without config file changes
- **`Validate()` method:** Catches invalid combinations at startup, not at runtime
- **String-based memory sizes:** `"8GB"` is more human-friendly than `8589934592`
- **Optional fields use pointers:** `CuckooFilter *bool` allows distinguishing "not set" from "set to false"

---

## 3. Cache Abstractions

### `Cache` — Main Cache Interface

**File:** `internal/cache/interfaces.go`

**What it does:** Defines the contract for any cache implementation.

```go
type Cache interface {
    Get(key string) (interface{}, error)
    Put(key string, value interface{}) error
    Delete(key string) error
    Exists(key string) bool
    BatchGet(keys []string) (map[string]interface{}, error)
    BatchPut(entries map[string]interface{}) error
    MightContain(key string) bool
    CreateStore(config StoreConfig) error
    DropStore(name string) error
}
```

**Why it exists:** Interface-first design allows swapping implementations. During development, a simple in-memory implementation was used before the full `BasicStore` existed.

---

### `EvictionPolicy` — Eviction Strategy Interface

**What it does:** Decides which keys to remove when memory is full.

```go
type EvictionPolicy interface {
    NextEvictionCandidate() *Entry
    OnAccess(entry *Entry)
    OnInsert(entry *Entry)
    OnDelete(entry *Entry)
    ShouldEvict(entry *Entry, memoryPressure float64) bool
    PolicyName() string
}
```

**Why it exists:** Different workloads need different eviction strategies. The interface allows plugging in LRU, LFU, FIFO, TTL-based, or the custom session-aware policy without changing the store.

---

### `SessionEvictionPolicy` — Session-Aware Eviction

**File:** `internal/cache/session_eviction_policy.go`

**What it does:** Implements intelligent eviction that prioritizes keeping active user sessions alive.

**How it works internally:**
1. Maintains a **doubly-linked list** (queue) ordered by recency
2. `OnAccess()` moves the entry to the tail (most recently used) — **O(1)**
3. `NextEvictionCandidate()` returns the head (least recently used) — **O(1)**
4. `ShouldEvict()` checks three conditions:
   - **Session TTL exceeded** (30min default): session too old
   - **Idle timeout exceeded** (10min default): session inactive
   - **Memory pressure override**: if pressure > 0.9, evict regardless of session state
5. New sessions get a **grace period** (2min): recently created sessions are protected from eviction even under pressure

**Why this design:**
- Standard LRU doesn't understand sessions — it evicts based solely on access recency
- This policy knows that a session accessed 9 minutes ago is "fine" but one accessed 11 minutes ago is "idle"
- The grace period prevents the thundering herd problem where a burst of new sessions immediately evicts other new sessions

**Design tradeoff:** The O(1) queue management uses `prev`/`next` pointers on the `Entry` struct, which adds 16 bytes per entry. This was chosen over `container/list` because it avoids interface boxing and allocations.

---

### `Entry` — Cache Entry Metadata

```go
type Entry struct {
    Key         string
    SessionID   string
    CreatedAt   time.Time
    LastAccess  time.Time
    AccessCount int64
    Size        int64
    TTL         time.Duration
    prev, next  *Entry  // Doubly-linked list for O(1) eviction queue
}
```

**Why it exists:** Separates metadata (used by eviction policy) from actual data (stored in the memory pool). This allows the eviction policy to operate on lightweight metadata without touching the (potentially large) cached values.

---

## 4. Storage Engine

### `BasicStore` — Core Cache Store

**File:** `internal/storage/basic_store.go`

**What it does:** The main cache store that integrates memory management, eviction, optional filtering, and optional persistence into a single cohesive unit.

**Internal state:**
```go
type BasicStore struct {
    data          *ShardedMap              // 32-shard concurrent map (keys + allocations + tombstones)
    memPool       *MemoryPool              // Memory allocation tracker (includes PerKeyOverhead)
    evictPolicy   cache.EvictionPolicy     // Session-aware eviction
    filter        filter.ProbabilisticFilter // Optional Cuckoo filter
    persistEngine persistence.PersistenceEngine // Optional AOF+Snapshot
    mutex         sync.RWMutex             // Protects stats only (data is shard-locked)
    evictSignal   chan struct{}             // Signal background evictor
    aofChan       chan *persistence.LogEntry // Buffered AOF write channel (10K entries)
}
```

**How SET works:**
1. Serialize value to `[]byte` via `serializeValue()` (handles string, int, float, bool, complex types via JSON)
2. Check memory pressure — if insufficient space, signal background evictor and wait briefly (never evict inline)
3. Allocate memory from `MemoryPool` for the serialized bytes (includes `PerKeyOverhead` of 500 bytes for Go struct overhead)
4. Lock only the target shard (`ShardedMap.LockShard(key)`) — 1 of 32 independent shards
5. If key exists: free old allocation, update in-place
6. If key is new: insert into shard
7. Unlock shard
8. Update eviction policy metadata (`OnInsert` or `OnAccess`)
9. Add to Cuckoo filter (if enabled)
10. Send persistence entry to background AOF channel (non-blocking)

**How GET works:**
1. Check Cuckoo filter (if enabled) — O(1) early rejection for definite misses
2. Lookup key in `ShardedMap` — acquires shard read lock, O(1)
3. If yes: lock shard briefly to update access stats, deserialize value, return
4. If no: increment miss counter, return nil

**Why `serializeValue()` / `deserializeValue()` exist:**
Values are stored as `[]byte` in allocated memory pools, not as Go interfaces. This enables:
- Accurate memory tracking (we know exact byte size + 500-byte per-key overhead)
- Persistence (can write bytes directly to AOF)
- Type safety (explicit type tag stored with each item)

String and `[]byte` values bypass JSON serialization entirely (native path). The ~18% GET latency overhead only applies to complex types (maps, slices, structs).

**Background Eviction:**
Memory pressure callbacks signal a dedicated evictor goroutine. The evictor runs outside the hot path, collecting expired items and using the eviction policy to remove least-accessed items until memory pressure drops below 75%.

**Background AOF:**
Persistence writes go through a 10,000-entry buffered channel. A background goroutine drains the channel and writes to the AOF file. This completely eliminates AOF from the SET critical path.

**Tombstones — Why they exist:**
When node-1 deletes a key, node-2 might not have received the DELETE replication yet. If a client reads from node-2, read-repair would fetch the key from the owner (where it's deleted) and find nothing. But if a replica hasn't processed the delete yet, read-repair might fetch the old value and "resurrect" the deleted key. Tombstones prevent this by marking recently-deleted keys as "definitely deleted — don't bring back."

**SetWithTimestamp — Replication-aware writes:**
```go
func (s *BasicStore) SetWithTimestamp(ctx, key, value, sessionID, ttl, lamportTS) (bool, error)
```
Used exclusively by the replication handler. Compares incoming Lamport timestamp against local timestamp. Only applies the write if `incoming > local`. This ensures causal ordering: if node-1 writes "A" then "B", node-2 will never see "A" overwrite "B" regardless of replication ordering.

---

### `CacheItem` — Individual Cached Value

```go
type CacheItem struct {
    Value      []byte        // Serialized value in allocated memory
    ValueType  string        // Original Go type (e.g., "string", "map[string]interface {}")
    CreatedAt  time.Time
    ExpiresAt  time.Time     // Zero means no expiry
    SessionID  string
    Size       int64         // Serialized byte count
    LamportTS  uint64        // Causal ordering timestamp
}
```

**Design decision:** Storing the type tag enables faithful round-trip serialization. A `string` value comes back as `string`, a `map` comes back as `map`. Without this, everything would deserialize to `[]byte` and the HTTP API would return base64-encoded garbage.

---

### `MemoryPool` — Memory Tracker

**File:** `internal/storage/memory_pool.go`

**What it does:** Tracks how much memory has been allocated for cache values and triggers pressure callbacks when thresholds are exceeded.

**How it works:**
- `Allocate(size)`: atomically increments usage counter, returns a `[]byte` slice
- `Free(ptr)`: atomically decrements usage counter
- `MemoryPressure()`: returns `currentUsage / maxSize` as a float (0.0 to 1.0)
- Pressure callbacks fire at configurable thresholds:
  - **Warning (0.85):** Log a warning, slow down non-critical operations
  - **Critical (0.90):** Trigger aggressive eviction
  - **Panic (0.95):** Emergency eviction, reject new writes if needed

**Why it exists:** Go's GC manages heap memory, but caches need deterministic memory limits. Without explicit tracking, a cache could consume all available RAM before Go's GC decides to collect. The memory pool provides predictable, deterministic memory bounds.

**Known limitation:** Only tracks serialized value bytes, not Go struct overhead. See [User-Facing Bottlenecks](USER_FACING_BOTTLENECKS.md#41-memory-tracking-gap).

**Design decision — O(1) everything:** Both `Allocate` and `Free` use `atomic.AddInt64` — no locks, no contention, O(1) guaranteed. This is critical because every SET and DELETE operation touches the memory pool.

---

### `StoreManager` — Multi-Store Orchestrator

**File:** `internal/storage/store_manager.go`

**What it does:** Manages multiple independent cache stores (like Redis databases 0-15, but named and independently configured).

**How it works:**
- Stores are created via YAML config (at boot) or HTTP API / RESP `CREATE` command (at runtime)
- Each store has its own `BasicStore` with independent memory limit, eviction policy, and persistence
- Runtime-created stores are persisted to `stores.json` so they survive restarts
- The "default" store always exists and cannot be dropped

**Why it exists:** Multi-tenancy. A single HyperCache cluster can serve sessions (TTL eviction, 30min TTL), API responses (LRU eviction, 1GB limit), and feature flags (no eviction, persistence) without interference.

**Design decision — Why not Redis-style numbered databases:**
Named stores are self-documenting (`SELECT sessions` vs `SELECT 3`). They also support different configurations per store, which Redis databases don't.

---

## 5. Probabilistic Filter

### `ProbabilisticFilter` — Filter Interface

**File:** `internal/filter/interfaces.go`

```go
type ProbabilisticFilter interface {
    Add(key []byte) error
    Contains(key []byte) bool
    Delete(key []byte) bool     // Cuckoo filter advantage
    Clear() error
    Size() uint64
    LoadFactor() float64
    FalsePositiveRate() float64
    GetStats() *FilterStats
}
```

**Why it exists:** Enables plugging in different filter implementations (Cuckoo, Bloom, etc.) without changing the storage engine.

---

### `CuckooFilter` — Probabilistic Membership Filter

**File:** `internal/filter/cuckoo_filter.go`

**What it does:** Answers "is this key **definitely not** in the cache?" in O(1) time with zero false negatives and <0.1% false positives.

**How it works internally:**

1. **Hashing:** Key → xxhash64 → 64-bit hash
2. **Fingerprinting:** Extract 12-bit fingerprint from the hash (configurable)
3. **Primary bucket:** `hash % numBuckets` → try to insert fingerprint
4. **Alternate bucket:** `primaryBucket XOR hash(fingerprint)` → second chance
5. **Eviction chain:** If both buckets are full, randomly kick out an existing fingerprint and relocate it to its alternate bucket. Repeat up to 500 times.
6. **Auto-resize:** If eviction chain exhausts all attempts, double the bucket count and rehash

**Memory layout:**
```
numBuckets = nextPowerOfTwo(expectedItems / bucketSize)
Each bucket = 4 slots × 12 bits = 48 bits = 6 bytes
Total memory ≈ numBuckets × 6 bytes
For 1M items: ~375KB
```

**Why Cuckoo filter instead of Bloom filter:**
1. **Deletion support:** Bloom filters cannot delete entries. Cuckoo filters can — critical for cache eviction.
2. **Better space efficiency:** At the same FPR, Cuckoo filters use ~40% less memory than Bloom filters ([Fan et al., 2014](https://www.cs.cmu.edu/~dga/papers/cuckoo-conext2014.pdf)).
3. **Simpler to reason about:** Load factor directly correlates with performance.

**Design decisions:**
- **12-bit fingerprints:** Provides ~0.1% FPR. 8-bit would give ~3% FPR (too high). 16-bit would waste space.
- **4 slots per bucket:** Optimal for cache line alignment and Cuckoo filter theory. 2 slots would require more buckets. 8 slots would increase eviction chain length.
- **xxhash64:** Chosen for speed (>10GB/s) and excellent distribution. SHA256 was tested but 10x slower.
- **500 max eviction attempts:** Empirically determined. At 85% load factor, 99.9% of insertions succeed within 100 attempts. 500 provides safety margin.

**Production value proposition:**
- Filter check: ~4.3ns per miss
- Database query prevented: ~1-10ms per miss
- ROI: 200,000x to 2,000,000x cost savings for negative lookups

---

## 6. Persistence

### `HybridEngine` — Persistence Orchestrator

**File:** `internal/persistence/hybrid_engine.go`

**What it does:** Combines AOF (write-ahead log) and snapshots for data durability. Manages the lifecycle of both, including background snapshot scheduling and AOF compaction.

**How it works:**
```
WRITE PATH:  Set("key", "value") → AOFManager.LogSet() → [optional fsync based on policy]
SNAPSHOT:    Timer fires every 15min → SnapshotManager.CreateSnapshot(allData) → gob+gzip to file
RECOVERY:    LoadSnapshot() → Replay AOF entries after snapshot timestamp → Apply to store
COMPACTION:  AOF > MaxLogSize → Create new snapshot → Truncate AOF
```

**Background goroutines:**
1. **Snapshot timer:** Creates periodic snapshots (default 15min)
2. **Compaction checker:** Every 5 minutes, checks if AOF exceeds `MaxLogSize`. If yes, triggers compaction.

**Why hybrid instead of just AOF or just snapshot:**
- **AOF alone:** Every write is logged, so recovery replays every mutation since the beginning of time. For 1M writes/hour, recovery after 24 hours replays 24M entries.
- **Snapshot alone:** Point-in-time only. Data between snapshots is lost.
- **Hybrid:** Snapshot provides a "checkpoint", AOF provides incremental durability since the last snapshot. Recovery is: load snapshot + replay small AOF. Best of both worlds.

**Design inspiration:** Redis uses the same hybrid approach (`RDB` snapshots + `AOF`). HyperCache's implementation is simpler but follows the same principles.

---

### `AOFManager` — Append-Only File

**File:** `internal/persistence/aof.go`

**What it does:** Logs every mutation (SET, DELETE, EXPIRE, CLEAR) as a line in a text file.

**File format:**
```
1691234567890|SET|user:123|45|{"token":"abc","role":"admin"}|1800000000000|session-42
1691234567891|DEL|user:456|0||0|
```
Format: `TIMESTAMP|OPERATION|KEY|VALUE_LENGTH|VALUE|TTL_NS|SESSION_ID`

**Why text-based format:**
- Human-readable (debugging)
- Line-oriented (partial write = only last line corrupted, rest is intact)
- `grep`-able (find all operations on a specific key)

**Tradeoff:** Text format is 2-3x larger than binary. For production, the compression in snapshots compensates.

**Write buffering:** Uses a 64KB `bufio.Writer`. Calls `Flush()` based on sync policy:
- `always`: Flush + fsync after every write (safest, slowest)
- `everysec`: Flush only (buffer → OS cache), background fsync every second
- `no`: Neither (OS decides when to flush — fastest, riskiest)

---

### `SnapshotManager` — Point-in-Time Snapshots

**File:** `internal/persistence/snapshot.go`

**What it does:** Creates compressed binary snapshots of the entire dataset at a point in time.

**File format:**
```
[SnapshotHeader]  // NodeID, Timestamp, EntryCount, Compressed flag
[SnapshotEntry]   // Key, Value, TTL, SessionID, LamportTS, CreatedAt
[SnapshotEntry]
...
```

**How it works:**
1. Receive `map[string]interface{}` from the store's snapshot callback
2. Write to `snapshot_<timestamp>.tmp` file
3. Encode header + entries using `gob` encoding
4. If `CompressionLevel > 0`: wrap writer in `gzip.Writer`
5. `fsync()` the file
6. Atomically rename `.tmp` → `snapshot_<timestamp>.rdb`
7. Clean up old snapshots (keep `RetainLogs` count)

**Why atomic rename:** If the process crashes during snapshot write, the `.tmp` file is incomplete. On recovery, only files matching `snapshot_*.rdb` (renamed after successful write) are considered. The old snapshot remains valid.

**Why gob encoding:** Go's native binary format — faster than JSON, supports nested types, no schema needed. Alternative considered: Protocol Buffers (rejected because it requires schema definition and code generation, adding build complexity).

---

## 7. Cluster Coordination

### `DistributedCoordinator` — Cluster Orchestrator

**File:** `internal/cluster/distributed_coordinator.go`

**What it does:** Integrates three sub-components into a cohesive distributed system:
1. `GossipMembership` — who's in the cluster
2. `HashRing` — where data lives
3. `DistributedEventBus` — how operations replicate

**Background tasks:**
- `membershipSync()`: Watches gossip for node join/leave events, updates hash ring accordingly
- `heartbeatLoop()`: Periodic health check (every 5s)

**Why a coordinator instead of letting components communicate directly:**
Components need orchestrated lifecycle management. The hash ring needs to know about membership changes, the event bus needs membership for routing, and membership needs the event bus for failures. The coordinator breaks this circular dependency.

---

### `HashRing` — Consistent Hashing

**File:** `internal/cluster/hashring.go`

**What it does:** Maps keys to nodes using consistent hashing with virtual nodes, providing even distribution and minimal key movement during cluster changes.

**How it works:**
1. Each physical node is mapped to 256 virtual nodes (default) on a ring of uint64 values
2. Virtual node position: `hash(nodeID + "-vnode-" + index)` using xxhash64
3. Key lookup: `hash(key)` → binary search for the next virtual node clockwise → map to physical node
4. Replica lookup: Continue clockwise, skipping virtual nodes that map to already-selected physical nodes

**Why 256 virtual nodes:**
- Fewer vnodes (e.g., 10): Uneven distribution — some nodes get 3x more keys than others
- More vnodes (e.g., 1000): Wastes memory and slows lookups
- 256 is the sweet spot: standard deviation of load per node < 5% with 3+ nodes

**LRU lookup cache:**
Hot keys (e.g., session:abc) are looked up repeatedly. Caching the hash ring lookup result for the 10,000 most recently looked-up keys avoids repeated binary searches. Cache is invalidated whenever the ring topology changes.

**Design decision — xxhash64 vs SHA256:**
xxhash64 is ~10x faster and provides sufficient distribution quality for hash ring placement. SHA256's cryptographic properties are unnecessary here.

---

### `GossipMembership` — Cluster Membership via Gossip

**File:** `internal/cluster/gossip_membership.go`

**What it does:** Uses HashiCorp's Serf library to manage cluster membership through gossip protocol.

**How gossip works:**
1. Node-1 starts and listens on gossip port
2. Node-2 joins by connecting to node-1's gossip port
3. Serf's SWIM protocol sends periodic health probes (every 1s)
4. If a node doesn't respond to probes, it's marked "suspected" (5s), then "dead" (10s)
5. Membership changes are propagated to all nodes via gossip (convergence time: O(log N) rounds)

**Why Serf instead of building our own gossip:**
Serf is battle-tested at HashiCorp scale (Consul, Nomad). Reimplementing SWIM would take months and introduce subtle correctness bugs. By using Serf, HyperCache gets:
- Failure detection with configurable timeouts
- Event broadcasting with size limits
- Encryption support (Serf supports encryption key rotation)

**User event handler:** HyperCache registers a callback for Serf user events, which is used to propagate cache operations (SET/DELETE) to all cluster members.

---

### `DistributedEventBus` — Cluster Event Distribution

**File:** `internal/cluster/distributed_event_bus.go`

**What it does:** Publishes cache operations to all cluster members and delivers received events to local subscribers.

**How it works:**
1. When a local SET/DELETE happens, the store publishes a `ClusterEvent` to the event bus
2. The event bus serializes the event and sends it via Serf's user event mechanism
3. All cluster members receive the event via gossip
4. Local subscribers (registered via `Subscribe()`) receive the event on their channel
5. The replication handler in `main.go` applies the event to the local store

**Why channels for subscribers:**
Go channels provide built-in backpressure. If a subscriber is slow, the channel blocks, which the publisher can detect and handle (drop events, buffer, or block).

---

### `LamportClock` — Logical Clock

**File:** `internal/cluster/lamport_clock.go`

**What it does:** Provides causal ordering of events across distributed nodes without requiring synchronized wall clocks.

**How it works:**
- `Tick()`: Atomically increment the counter — used for local operations
- `Witness(observed)`: Set counter to `max(local, observed) + 1` — used when receiving remote events
- `Current()`: Read without incrementing

**Why Lamport clocks instead of vector clocks:**
- Lamport clocks are simpler (single uint64 vs N-element vector)
- Sufficient for last-writer-wins conflict resolution
- O(1) space and time, compared to O(N) for vector clocks where N = cluster size
- HyperCache doesn't need to detect concurrent writes (which vector clocks provide) — it only needs to order them

**Implementation detail:** Uses `atomic.AddUint64` for `Tick()` and a CAS loop for `Witness()`. The CAS loop handles the race where two goroutines witness different remote values simultaneously.

---

### `NodeCommunicator` — Direct Node Communication

**File:** `internal/cluster/node_communication.go`

**What it does:** Sends HTTP requests directly to specific nodes for operations that need targeted delivery (not broadcast).

**When it's used:**
- `ReqTypeReplicateData`: Send data to a specific replica
- `ReqTypeGetData`: Read-repair — fetch a key from a peer
- `ReqTypeHealthCheck`: Direct liveness check
- `ReqTypeMigrateKeys`: Key migration during rebalance (future)

**Why both gossip AND direct communication:**
- **Gossip** is best for broadcasting to all nodes (eventual consistency, fire-and-forget)
- **Direct HTTP** is best for targeted operations (read-repair from a specific peer, health checks)

---

### `ReadRepairer` — Consistency Healer

**File:** `internal/cluster/read_repairer.go`

**What it does:** When a local GET misses but the key might exist on another node (gossip hasn't propagated yet), the read repairer fetches from peers.

**How it works:**
1. Local GET returns nil
2. `ReadRepairer.TryPeers()` is called
3. Gets the list of alive peers from the coordinator
4. Sends HTTP GET requests to each peer (with 2-second timeout)
5. If any peer has the value, return it and apply locally
6. If all peers miss, return nil

**Why 2-second timeout:** Gossip propagation takes 50-500ms. If a peer doesn't respond in 2 seconds, it's either down or the key truly doesn't exist. Longer timeouts would degrade GET latency.

**Why read-repair exists:** In an eventually consistent system, there's always a window between a write and its replication. Without read-repair, every GET during that window returns a miss, even though the data exists somewhere in the cluster. Read-repair closes this gap.

---

### `SimpleCoordinator` — Single-Node Mode

**File:** `internal/cluster/simple_coordinator.go`

**What it does:** Minimal implementation of the coordinator interface for single-node operation. No gossip, no cluster membership — just hash ring for future expansion and local event bus.

**Why it exists:** Running a full Serf gossip stack for a single node adds unnecessary complexity and port requirements. The simple coordinator provides the same interface without networking overhead.

---

## 8. Network Layer

### `Server` — RESP Protocol Server

**File:** `internal/network/resp/server.go`

**What it does:** Redis-compatible socket server that handles client connections using the RESP (Redis Serialization Protocol) format.

**Supported commands:**
| Command | Implementation |
|---------|---------------|
| `PING` | Returns PONG |
| `SET key value [EX seconds]` | Store with optional TTL |
| `GET key` | Retrieve value |
| `DEL key` | Delete key |
| `EXISTS key` | Check existence |
| `DBSIZE` | Key count |
| `INFO` | Server statistics |
| `FLUSHDB` | Clear all keys |
| `SELECT storename` | Switch active store |
| `CREATE storename` | Create new store |
| `STORES` | List all stores |

**Connection handling:**
1. Accept TCP connection
2. Create `ClientConn` with per-connection state (selected store, buffer)
3. Parse incoming RESP commands via `Parser`
4. Execute command on the selected store
5. Format response via `Formatter`
6. Handle pipelining (up to 100 commands in flight)

**Pipelining:**
Redis clients often send multiple commands without waiting for responses. The server reads commands in a loop and writes responses in order, enabling high throughput for batch operations.

**Why RESP protocol:**
- Redis client ecosystem is massive (every language has a Redis client)
- Users can test with `redis-cli` without installing anything special
- RESP is simple to parse (type prefix byte + length + data)
- Drop-in replacement scenario: point existing Redis clients at HyperCache

---

### `Parser` — RESP Protocol Parser

**File:** `internal/network/resp/protocol.go`

**What it does:** Parses the Redis wire protocol into typed Go values.

**RESP types handled:**
- `+` Simple string (e.g., `+OK\r\n`)
- `-` Error (e.g., `-ERR unknown command\r\n`)
- `:` Integer (e.g., `:42\r\n`)
- `$` Bulk string (e.g., `$5\r\nhello\r\n`)
- `*` Array (e.g., `*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n`)

---

### HTTP API Server

**Defined in:** `cmd/hypercache/main.go` (`startHTTPServer()`)

**Endpoints:**
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Cluster health status |
| GET | `/metrics` | Prometheus-format metrics |
| GET | `/api/cache/:key` | Get value |
| PUT | `/api/cache/:key` | Set value |
| DELETE | `/api/cache/:key` | Delete value |
| GET | `/api/stores` | List stores |
| POST | `/api/stores` | Create store |
| GET | `/api/stores/:name` | Store info |
| DELETE | `/api/stores/:name` | Drop store |
| GET | `/api/stores/:name/cache/:key` | Get from specific store |
| PUT | `/api/stores/:name/cache/:key` | Set in specific store |
| GET | `/api/filter/stats` | Cuckoo filter statistics |

**Why both HTTP and RESP:**
- RESP: For applications that already use Redis clients (migration path)
- HTTP: For applications that prefer REST APIs, monitoring tools, load balancers that health-check via HTTP

---

## 9. Logging

### `Logger` — Structured Async Logger

**File:** `internal/logging/logger.go`

**What it does:** Provides structured JSON logging with correlation IDs, asynchronous write processing, and configurable output targets.

**Log entry structure:**
```json
{
  "timestamp": "2025-08-20T10:30:45.123Z",
  "level": "INFO",
  "component": "storage",
  "action": "set",
  "message": "Key stored successfully",
  "correlation_id": "abc-123",
  "caller": "basic_store.go:245",
  "duration_ms": 0.042,
  "fields": {"key": "user:123", "size": 1024}
}
```

**How async logging works:**
1. `Info()` / `Error()` / etc. create a `LogEntry` struct
2. Entry is sent to a buffered channel (default capacity: 1000)
3. A background goroutine (`processLogs()`) reads from the channel and writes to outputs
4. If the channel is full, the log call blocks (backpressure)

**Why async:**
Synchronous logging (like `fmt.Println`) blocks the caller until I/O completes. For a cache serving 1M+ ops/sec, even 1μs of logging overhead per operation = 1 second of blocked time per second. Async logging moves I/O off the hot path.

**Correlation IDs:**
Every HTTP request gets a `X-Correlation-ID` header (via middleware). This ID propagates through all log entries for that request, enabling distributed tracing without a full tracing infrastructure.

**Why structured JSON instead of plain text:**
- Machine-parseable for Elasticsearch/Grafana
- Filterable by component, action, level
- Supports arbitrary fields without format string changes
- Standard in cloud-native observability

---

### `HTTPMiddleware` — Request Logging & Correlation

**File:** `internal/logging/middleware.go`

**What it does:** Wraps HTTP handlers to:
1. Generate/propagate correlation IDs
2. Log request start (method, path, source IP)
3. Capture response status code and bytes sent
4. Log request completion with duration

**Why middleware instead of explicit logging in each handler:**
DRY principle. Without middleware, every handler would need 3 lines of boilerplate for correlation ID, timing, and logging. With middleware, it's zero lines — all handlers get observability for free.

---

## Architectural Design Principles

### 1. Interface-First Design
Every major component defines an interface before implementation. This enables:
- Testing with mocks
- Swapping implementations (e.g., different eviction policies)
- Clear contracts between packages

### 2. O(1) on the Hot Path
Critical operations (GET, SET, eviction candidate selection, filter lookup, memory allocation) are all O(1) or amortized O(1). Hash ring lookup is O(log N) with LRU caching.

### 3. Eventual Consistency with Causal Ordering
Not random last-write-wins — Lamport timestamps ensure causally related writes are correctly ordered. Read-repair closes the gossip propagation window.

### 4. Graceful Degradation
- Memory pressure → eviction → not OOM
- Peer failure → gossip detection → read-repair from remaining nodes
- AOF corruption → skip corrupt lines → continue with remaining data
- Snapshot failure → keep old snapshot → no data loss

### 5. Composition over Inheritance
`BasicStore` is composed of `MemoryPool`, `EvictionPolicy`, `ProbabilisticFilter`, and `PersistenceEngine`. Each can be nil (disabled). This avoids the complexity of deep class hierarchies.
