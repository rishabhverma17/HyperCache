# HyperCache — Production Benchmarking Strategy

## How Redis & Memcached Were Battle-Tested Before Production

Before Redis and Memcached were trusted in production by companies like Twitter, GitHub, and Instagram, they went through rigorous phases of validation. HyperCache needs to follow the same playbook.

### What Redis Did

1. **`redis-benchmark`** — a built-in tool that fires pipelined commands at a single node and reports ops/sec at different payload sizes, connection counts, and pipeline depths.
2. **`memtier_benchmark`** (RedisLabs) — an external tool that simulates realistic mixed workloads (GET/SET ratios, key distributions, TTL patterns) across multiple threads.
3. **Jepsen testing** — formal distributed systems testing (linearizability, partition tolerance, data loss under failure scenarios). This is what proved Redis Cluster was safe or exposed bugs.
4. **Long-running soak tests** — run the system for 72+ hours under sustained load, monitor memory growth, latency percentile drift, and GC pauses.
5. **Chaos engineering** — Netflix-style fault injection: kill nodes, partition networks, fill disks, exhaust memory.

### What Memcached Did

1. **`mcperf`** — synthetic load generator measuring latency distributions (p50/p99/p999).
2. **Facebook's `mcrouter`** — tested at Facebook scale with billions of requests/day on their fork.
3. **Real production traffic replay** — captured production traffic patterns and replayed against test clusters.

---

## HyperCache Benchmarking Plan

### Phase 1: Micro-Benchmarks (What We Have, What We Need)

**Already covered (34 benchmarks):**
- BasicStore Set/Get throughput
- Memory pool allocation
- Cuckoo filter operations
- Hash ring lookups
- Lamport clock operations
- RESP protocol parsing

**Gaps to fill:**
| Benchmark | Why It Matters | Priority |
|-----------|---------------|----------|
| Persistence write throughput (AOF) | Users need to know write amplification cost | P0 |
| Snapshot creation latency at N items | Fork-less snapshot means blocking — how long? | P0 |
| Persistence recovery time at N items | "How long until my cache is warm after restart?" | P0 |
| Gossip propagation latency (2-node, 5-node, 10-node) | Replication SLA for users | P0 |
| HTTP API end-to-end latency | Real-world latency including network stack | P1 |
| RESP server end-to-end latency | Compare directly against Redis | P1 |
| Memory overhead per key | "How much RAM do I actually need?" | P1 |
| Eviction throughput under pressure | "What happens when I hit the memory limit?" | P1 |
| Read-repair latency | "How fast does consistency heal?" | P2 |
| Store creation/drop latency | Multi-tenant operational cost | P2 |

### Phase 2: Macro-Benchmarks (System-Level)

Build a dedicated load generator (`cmd/hypercache-bench/`) that:

1. **Configurable workload profiles:**
   - Read-heavy (95% GET, 5% SET) — typical web app cache
   - Write-heavy (20% GET, 80% SET) — session store / streaming
   - Mixed (50/50) — balanced workload
   - Scan-resistant (Zipfian distribution) — realistic hot/cold key patterns

2. **Metrics collected:**
   - Throughput (ops/sec) at p50, p95, p99, p999 latency
   - Memory consumption over time
   - GC pause distribution (Go runtime)
   - Network bandwidth utilization
   - Persistence I/O overhead

3. **Scale dimensions:**
   - Payload sizes: 64B, 256B, 1KB, 4KB, 16KB, 64KB
   - Key counts: 10K, 100K, 1M, 10M
   - Connection counts: 1, 10, 50, 100, 500, 1000
   - Cluster sizes: 1, 3, 5, 10 nodes

### Phase 3: Stress & Breaking Point Tests

**Goal: Find where HyperCache breaks and document it honestly.**

| Test | What It Reveals |
|------|----------------|
| Memory exhaustion | Does it OOM-kill or gracefully evict? |
| Disk full during persistence | Does it corrupt data or handle gracefully? |
| Network partition (split-brain) | Does data diverge? Does it heal? |
| Node crash during snapshot | Is the snapshot atomic? Can it recover? |
| Node crash during AOF write | Is the AOF corrupted? Last-write durability? |
| 100% CPU saturation | Latency degradation curve |
| GC pressure (10M+ keys) | GC pause distribution |
| Thundering herd (1000 goroutines, 1 key) | Lock contention, p99 latency spike |
| Cluster membership churn (rapid join/leave) | Hash ring stability, data loss |
| Slow node (inject 100ms latency) | Does gossip mark it dead? Failover time? |

### Phase 4: Comparative Benchmarks (vs Redis)

Use the same workload profiles against Redis (single-node) and document:
- Raw throughput comparison
- Latency percentile comparison
- Memory efficiency comparison
- Feature gap analysis (what Redis has that HyperCache doesn't, and vice versa)

**Honest framing:** "HyperCache is not trying to replace Redis. Here's where it wins and where Redis is better."

### Phase 5: Long-Running Soak Tests

Run for 72 hours minimum:
- Sustained 50K ops/sec mixed workload
- Monitor: memory growth, latency drift, error rates, GC pauses
- Kill one node at 24h, observe recovery
- Kill another at 48h, observe cluster degradation
- Success criteria: no memory leaks, p99 latency < 10ms, zero data loss after recovery

---

## Benchmark Tooling to Build

### 1. `cmd/hypercache-bench/main.go`
A self-contained benchmark tool that:
- Connects via RESP protocol (like `redis-benchmark`)
- Configurable via CLI flags
- Outputs results in JSON for automated analysis
- Generates latency histograms (HDR histogram)

### 2. `tests/stress/` directory
Go test files that:
- Run long-duration tests
- Inject faults (kill processes, fill tmp dirs)
- Validate data integrity after recovery
- Can be run in CI with shorter durations

### 3. `scripts/run-benchmarks.sh`
Orchestration script that:
- Starts cluster
- Runs all benchmark phases
- Collects results into `benchmark-results/`
- Generates comparison charts (using gnuplot or chart.go)

---

## Metrics That Matter for the Pitch

| Metric | Why Investors/Users Care |
|--------|------------------------|
| p99 latency at 100K ops/sec | "Will it be fast when it matters?" |
| Memory efficiency (bytes/key) | "How much hardware do I need?" |
| Recovery time after crash | "What's my downtime?" |
| Time to first request after cold start | "How fast can I scale up?" |
| Data durability guarantee | "Will I lose data?" |
| Max cluster size tested | "Can it scale with me?" |

---

## Execution Order

1. **Week 1:** Build `cmd/hypercache-bench/` and fill micro-benchmark gaps
2. **Week 2:** Run macro-benchmarks, collect real numbers on M3 Pro + Linux VM
3. **Week 3:** Stress tests and breaking point analysis
4. **Week 4:** Comparative benchmarks vs Redis, soak tests
5. **Week 5:** Document everything, generate charts, write findings

Each phase produces artifacts in `benchmark-results/` that feed directly into the white paper.
