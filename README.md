# HyperCache - Distributed Cache

<img src="./logo/HyperCache-logo-transparent.png" alt="HyperCache Logo" width="200" height="auto">

[![Static Badge](https://img.shields.io/badge/HyperCache-Active_Development-blue)]()
[![Status](https://img.shields.io/badge/Status-Production%20Ready-brightgreen)]()
[![Go Version](https://img.shields.io/badge/Go-1.23.2-blue)]()
[![Redis Compatible](https://img.shields.io/badge/Redis-Compatible-red)]()
[![Docker Hub](https://img.shields.io/docker/pulls/rishabhverma17/hypercache?label=Docker%20Hub&logo=docker)](https://hub.docker.com/r/rishabhverma17/hypercache)
[![Monitoring](https://img.shields.io/badge/Monitoring-Grafana%20%2B%20ELK-orange)]()

[![CI](https://github.com/rishabhverma17/HyperCache/workflows/HyperCache%20CI/badge.svg)](https://github.com/rishabhverma17/HyperCache/actions/workflows/ci.yml)
[![Tests](https://github.com/rishabhverma17/HyperCache/workflows/HyperCache%20Comprehensive%20Testing/badge.svg)](https://github.com/rishabhverma17/HyperCache/actions/workflows/test-comprehensive.yml)
[![Unit Tests](https://img.shields.io/badge/Unit%20Tests-100%25%20Passing-brightgreen)]()
[![Coverage](https://img.shields.io/badge/Coverage-85%25%2B-brightgreen)]()
[![Cuckoo Filter](https://img.shields.io/badge/Cuckoo%20Filter-0.33%25%20FPR-success)]()
[![Performance](https://img.shields.io/badge/Performance-18.8M%20ops%2Fsec-blue)]()

**HyperCache** is a high-performance, Redis-compatible distributed cache with advanced memory management, integrated probabilistic data structures (Cuckoo filters), and comprehensive monitoring stack. Built in Go for cloud-native environments.

## 🎯 **Latest Features** ✅

**Production-ready distributed cache with full observability stack:**
- ✅ Multi-node cluster deployment with hash-ring partitioned routing
- ✅ Full Redis client compatibility (RESP protocol + inline commands)
- ✅ Lamport timestamps for causal ordering of distributed writes
- ✅ Quorum writes — `consistency_level: "quorum"` waits for majority ACKs
- ✅ Read-repair for replication propagation window
- ✅ Sharded locks (32 independent shards) for high-concurrency writes
- ✅ Non-blocking snapshots — per-shard copy, no global read lock
- ✅ Enterprise persistence (AOF + Snapshots) with background writes
- ✅ Probabilistic eviction sampling (Redis-style random-key selection)
- ✅ Prometheus metrics with latency histograms (p50/p95/p99)
- ✅ Config validation CLI — `hypercache config validate <path>`
- ✅ Structured JSON logging with correlation ID tracing
- ✅ HTTP API + RESP protocol support
- ✅ Advanced memory management with pressure detection
- ✅ Cuckoo filter integration for negative lookup acceleration

### 🔥 **Monitoring & Observability**
- **Prometheus Metrics**: Latency histograms, memory pressure, operation counters on `/metrics`
- **Grafana Dashboards**: Pre-built Prometheus dashboard (throughput, latency p50/p95/p99, memory, cluster)
- **Elasticsearch + Filebeat**: Centralized log aggregation and search
- **Health Checks**: Built-in monitoring endpoints

## 🚀 **Quick Start**

### 🐳 Docker (Recommended — no setup required)

Pull from [Docker Hub](https://hub.docker.com/r/rishabhverma17/hypercache) and start the full stack:

```bash
# Download the compose file
curl -O https://raw.githubusercontent.com/rishabhverma17/HyperCache/main/docker-compose.cluster.yml

# Start everything (3 HyperCache nodes + Elasticsearch + Grafana + Filebeat)
docker compose -f docker-compose.cluster.yml up -d
```

That's it. All configs are baked into the Docker image — no cloning, no local files needed.

```bash
# Verify the cluster
curl http://localhost:9080/health

# Store a key
curl -X PUT http://localhost:9080/api/cache/hello \
  -H "Content-Type: application/json" -d '{"value": "world"}'

# Read it from a different node (replication)
curl http://localhost:9082/api/cache/hello

# Open Grafana dashboards
open http://localhost:3000  # admin / admin123
```

### Prerequisites (Local Development)
- Go 1.23.2+
- `redis-cli` (optional, for RESP testing)

### Local Cluster (N nodes)
```bash
# Start a 3-node cluster (default)
make cluster

# Start a 5-node cluster
make cluster NODES=5

# Start a 10-node cluster
make cluster NODES=10

# Check cluster health
curl -s http://localhost:9080/health | python3 -m json.tool

# Add a node to a running cluster (auto-discovers ports and seeds)
./scripts/add-node.sh
./scripts/add-node.sh --node-name=node-6

# Stop the cluster
make cluster-stop

# Full reset (stop + wipe data/logs/binaries + restart)
make cluster-stop && make clean && make cluster
```

### Single Node
```bash
make run
```

### Docker Deployment
```bash
# Pull the latest image from Docker Hub
docker pull rishabhverma17/hypercache:latest

# Start full stack (3-node cluster + Elasticsearch + Grafana + Filebeat)
docker compose -f docker-compose.cluster.yml up -d

# Add a 4th node to a running Docker cluster (auto-joins via gossip)
docker run -d --name hypercache-node4 \
  --network cache_hypercache-cluster \
  -p 8083:8080 -p 9083:9080 \
  rishabhverma17/hypercache:latest \
  --protocol resp --node-id node-4

# Or build locally and start
make docker-build && make docker-up

# Stop
docker compose -f docker-compose.cluster.yml down
```

### Kubernetes
```bash
# Prerequisites: minikube (brew install minikube) + kubectl (brew install kubectl)

# Start Minikube
minikube start

# Deploy full stack (HyperCache + Elasticsearch + Filebeat + Grafana)
make k8s-up
# or: kubectl apply -f k8s/

# Check cluster status
make k8s-status

# Scale to 5 nodes (or use Minikube Dashboard UI)
make k8s-scale NODES=5

# Access HyperCache from your machine
kubectl port-forward -n hypercache svc/hypercache 9080:9080 8080:8080
# Then: curl http://localhost:9080/health
# Then: redis-cli -p 8080 PING

# Open dashboards
make k8s-dashboard    # Kubernetes Dashboard (select "hypercache" namespace)
make k8s-grafana      # Grafana (same dashboards as Docker — admin/admin123)

# View logs
make k8s-logs

# Tear down
make k8s-down
```

### 📊 Access Points

**Local / Docker:**
| Service | URL | Notes |
|---------|-----|-------|
| Node N HTTP API | http://localhost:9079+N | Health, cache, stores, filter, metrics |
| Node N RESP | `redis-cli -p 8079+N` | Redis-compatible |
| Prometheus Metrics | http://localhost:9080/metrics | Per-node metrics |
| Grafana | http://localhost:3000 | admin / admin123 |
| Elasticsearch | http://localhost:9200 | |

Default 3-node cluster: HTTP on 9080/9081/9082, RESP on 8080/8081/8082.

**Kubernetes** (after `kubectl port-forward -n hypercache svc/hypercache 9080:9080 8080:8080`):
| Service | Access | Notes |
|---------|--------|-------|
| HTTP API | http://localhost:9080 | Same APIs as local |
| RESP | `redis-cli -p 8080` | Same commands as local |
| Grafana | `make k8s-grafana` | Opens browser via Minikube tunnel |
| K8s Dashboard | `make k8s-dashboard` | Select "hypercache" namespace |

## 🧪 **Testing**

### Unit Tests
```bash
make test-unit
```

### Lint & Format
```bash
make lint
make fmt
```

### Benchmarks
```bash
make bench
```

### Postman Collection
Import `HyperCache.postman_collection.json` into Postman for a full test suite covering:
health, metrics, CRUD, cross-node replication, delete replication, value types, Cuckoo filter, and cleanup.

### HTTP API Examples
```bash
# --- Default store (backward-compatible) ---
curl -X PUT http://localhost:9080/api/cache/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "hello world"}'

curl http://localhost:9080/api/cache/mykey
curl -X DELETE http://localhost:9080/api/cache/mykey

# --- Multi-store APIs ---

# List all stores
curl http://localhost:9080/api/stores

# Create a new store (config is immutable after creation)
curl -X POST http://localhost:9080/api/stores \
  -H "Content-Type: application/json" \
  -d '{"name":"sessions","eviction_policy":"ttl","max_memory":"1GB","default_ttl":"30m","cuckoo_filter":true,"persistence":"aof"}'

# Get store info and stats
curl http://localhost:9080/api/stores/sessions

# Write to a specific store
curl -X PUT http://localhost:9080/api/stores/sessions/cache/user:123 \
  -H "Content-Type: application/json" \
  -d '{"value": {"token":"abc","role":"admin"}}'

# Read from a specific store
curl http://localhost:9080/api/stores/sessions/cache/user:123

# Drop a store (cannot drop "default")
curl -X DELETE http://localhost:9080/api/stores/sessions

# Cuckoo filter stats
curl http://localhost:9080/api/filter/stats

# Prometheus metrics
curl http://localhost:9080/metrics
```

### Redis CLI
```bash
redis-cli -p 8080 SET foo bar
redis-cli -p 8080 GET foo
redis-cli -p 8081 GET foo   # verify replication
redis-cli -p 8080 DEL foo
redis-cli -p 8080 INFO
redis-cli -p 8080 DBSIZE

# Multi-store commands
redis-cli -p 8080 STORES             # list all stores
redis-cli -p 8080 SELECT sessions    # switch to "sessions" store
redis-cli -p 8080 SET user:123 token # writes to "sessions" store
redis-cli -p 8080 SELECT default     # switch back
```

### Scenario Tests
Real-world pattern tests that run on every push and daily:
```bash
# Run all scenario tests
go test -v -race ./tests/scenarios/...

# Reproduce a randomized test that failed in CI
SCENARIO_SEED=1712438400 go test -run TestRandomizedMixedWorkload ./tests/scenarios/...
```

**Deterministic** (same every run): session overflow, concurrent read/write, TTL expiry, persistence recovery, store lifecycle, hot key thundering herd.

**Randomized** (seed-based, different each run): mixed workload, burst writes, concurrent multi-store.

### Makefile Reference
```
make build              Build the binary
make run                Run single node (RESP)
make cluster            Start N-node cluster (default 3)
make cluster NODES=5    Start 5-node cluster
make cluster-stop       Stop all HyperCache processes
make clean              Remove binaries, logs, data
make test-unit          Run unit tests with coverage
make test-integration   Run integration tests
make bench              Run benchmarks
make lint               Run golangci-lint
make fmt                Format code
make docker-build       Build Docker image
make docker-up          Start Docker stack
make docker-down        Stop Docker stack
make k8s-up             Deploy to Kubernetes (full stack)
make k8s-down           Remove from Kubernetes
make k8s-scale NODES=5  Scale to N replicas
make k8s-status         Show K8s pod/service status
make k8s-dashboard      Open Minikube dashboard
make k8s-grafana        Open Grafana (K8s)
make k8s-logs           Tail HyperCache pod logs
make deps               Download and tidy dependencies
```

## 🏆 **Key Features**

### **Redis Compatibility**
- Full RESP protocol implementation
- Works with any Redis client library
- Drop-in replacement for many Redis use cases
- Standard commands: GET, SET, DEL, EXISTS, PING, INFO, FLUSHALL, DBSIZE
- Multi-store commands: SELECT, STORES

### **Distributed Resilience**
- **Hash-Ring Routing**: Consistent hashing with 256 virtual nodes routes each key to its primary owner. Non-owner nodes transparently proxy requests to the correct node
- **Quorum Writes**: `consistency_level: "quorum"` waits for majority of hash-ring replicas to ACK before returning OK. Parallel replication with 5s timeout and early-fail if quorum is unreachable. Default is `"eventual"` (async fire-and-forget)
- **Targeted Replication**: Writes replicate to N hash-ring replicas (default 3) via direct HTTP — not gossip broadcast to all nodes
- **Lamport Timestamps**: Logical clocks for causal ordering of distributed operations. Stale writes from out-of-order replication are automatically rejected
- **Read-Repair**: On local cache miss, hash-ring replicas are queried before returning 404. Bridges the replication propagation window
- **Sharded Concurrency**: 32 independent lock shards eliminate the global mutex bottleneck. Each key locks only its shard
- **Probabilistic Eviction**: Redis-style random sampling (5 keys per round, evict least-recently-accessed) — O(1) per eviction instead of O(n) linked-list walk
- **Non-Blocking Snapshots**: `SnapshotRawData()` copies data per-shard with brief RLock, then releases. Writes to other shards proceed concurrently during snapshot
- **Correlation ID Tracing**: Every request gets a unique ID that flows across all nodes for end-to-end debugging

### **Enterprise Persistence & Recovery**
- **Hybrid Persistence**: AOF (Append-Only File) + Snapshot dual strategy
- **Background AOF**: 10,000-entry buffered channel — writes never block the SET critical path
- **Configurable per Store**: Each data store can have independent persistence policies
- **Fast Recovery**: Complete data restoration from AOF replay + snapshot loading
- **Snapshot Support**: Point-in-time recovery with configurable intervals
- **Durability Guarantees**: Configurable sync policies (always, everysec, no)

### **Containerized Deployment**
- **Docker Hub Integration**: Pre-built multi-arch images (amd64, arm64)
- **Docker Compose Support**: One-command cluster deployment with monitoring
- **Scalable Clusters**: `make cluster NODES=N` for local, `kubectl scale` for K8s
- **Dynamic Node Addition**: Add nodes to a running cluster — gossip handles join
- **Kubernetes Ready**: StatefulSet manifests with service discovery
- **CI/CD Pipeline**: GitHub Actions for lint, test, build, and publish

### **Multi-Store Architecture**
- **Named Stores**: Create independent stores with different configs (LRU, LFU, TTL, FIFO)
- **Per-Store Isolation**: Independent memory limits, eviction policies, TTL, persistence, cuckoo filters
- **Runtime Management**: Create/drop stores via HTTP API or RESP `SELECT` command
- **Immutable Config**: Store settings are set once at creation — drop and recreate to change
- **Store Registry**: Runtime-created stores survive restarts (`stores.json` persistence)
- **Environment Variable Config**: 8 env vars for Docker/K8s — no YAML needed

### **Advanced Memory Management**
- **Per-Store Eviction Policies**: Independent LRU, LFU, or session-based eviction per store
- **Smart Memory Pool**: Pressure monitoring (warning/critical/panic) with background eviction
- **Accurate Tracking**: 500-byte per-key overhead included in memory accounting (map bucket + struct + pointers)
- **Real-time Usage Tracking**: Memory statistics and structured alerts
- **Configurable Limits**: Store-specific memory boundaries

### **Probabilistic Data Structures**
- **Per-Store Cuckoo Filters**: Negative lookup acceleration — instant "definitely not here" for keys that don't exist
- **Configurable False Positive Rate**: Tune precision vs memory (default 0.01)
- **O(1) Membership Testing**: Sub-microsecond filter checks before any store lookup
- **Supports Delete**: Unlike Bloom filters, Cuckoo filters allow key removal

### **Distributed Architecture**
- **Multi-node Clustering**: Serf gossip protocol for node discovery and health monitoring
- **Consistent Hash Ring**: 256 virtual nodes with xxhash64 for uniform key distribution
- **Automatic Failover**: Node failure detection and traffic redistribution via gossip
- **Inter-node Communication**: HTTP-based read-repair and peer discovery via gossip metadata

### **Production Monitoring**
- **Structured JSON Logging**: Every log line has timestamp, level, component, action, correlation ID
- **Prometheus Metrics**: `/metrics` endpoint with latency histograms (SET/GET/DEL p50/p95/p99), memory pressure, allocation rates, operation counters — all in Prometheus text exposition format
- **Grafana Dashboards**: 4 pre-built dashboards — Health, Performance, System Components (Elasticsearch), and Prometheus Metrics (16 panels: throughput, latency percentiles, memory pressure, cluster health)
- **Elasticsearch + Filebeat**: Centralized log aggregation with container-scoped filtering
- **Configurable Log Levels**: debug/info/warn/error/fatal — tunable per node at runtime
- **Config Validation CLI**: `hypercache config validate <path>` — validates config and warns about dangerous settings (sync_policy, port conflicts, missing seeds)

## � **Project Structure**

```
HyperCache/
├── cmd/hypercache/             # Server entry point
├── scripts/                    # Deployment and management scripts
│   ├── start-3node-local.sh    # Local 3-node integration testing
│   ├── start-cluster.sh        # Production cluster launcher
│   ├── add-node.sh             # Add node to running cluster
│   ├── run-server-benchmarks.sh # redis-benchmark test suite
│   └── clean-*.sh              # Cleanup utilities
├── configs/                    # Node configuration files
│   ├── node1-config.yaml       # Node 1 configuration
│   ├── node2-config.yaml       # Node 2 configuration  
│   └── node3-config.yaml       # Node 3 configuration
├── internal/
│   ├── cache/                  # Cache interfaces and policies  
│   ├── storage/                # Storage with persistence
│   ├── filter/                 # Cuckoo filter implementation
│   ├── cluster/                # Distributed coordination
│   ├── network/resp/           # RESP protocol server
│   └── logging/                # Structured logging
├── grafana/                    # Grafana dashboards and config
├── examples/                   # Client demos and examples
├── docs/                       # Technical documentation
├── logs/                       # Application logs (Filebeat source)
├── data/                       # Persistence data (node storage)
├── docker-compose.logging.yml     # ELK logging stack
├── docker-compose.monitoring.yml  # Prometheus + Grafana metrics stack
├── prometheus.yml                 # Prometheus scrape config
└── filebeat.yml                   # Log shipping configuration
```

## 🔧 **Architecture Overview**

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Redis Client  │────│   RESP Protocol  │────│  HyperCache     │
│   (Any Library) │    │     Server       │    │   Cluster       │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                                         │
       ┌─────────────────────────────────────────────────┼─────────────────────────────────────────────────┐
       │                                                 │                                                 │
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Memory Pool   │    │   Data Storage   │    │ Cuckoo Filter   │    │   Hash Ring     │    │   Gossip Node    │
│   (Pressure     │    │   + Persistence  │    │ (Probabilistic  │    │ (Consistent     │    │   Discovery      │
│    Monitoring)  │    │   (AOF+Snapshot) │    │   Operations)   │    │   Hashing)      │    │   & Failover     │
└─────────────────┘    └──────────────────┘    └─────────────────┘    └──────────────────┘    └─────────────────┘
       │                         │                         │                         │                         │
       └─────────────────────────┼─────────────────────────┼─────────────────────────┼─────────────────────────┘
                                 │                         │                         │
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                    MONITORING STACK                                                         │
├─────────────────┬──────────────────┬─────────────────┬──────────────────┬─────────────────┬─────────────────┤
│    Filebeat     │   Elasticsearch  │     Grafana     │   Health API     │   Metrics       │   Alerting      │
│  (Log Shipper)  │  (Log Storage)   │  (Dashboards)   │  (Diagnostics)   │  (Performance)  │  (Monitoring)   │
└─────────────────┴──────────────────┴─────────────────┴──────────────────┴─────────────────┴─────────────────┘
```

## � **Monitoring & Operations**

### **Grafana Dashboards**

**ELK Dashboards** (http://localhost:3000 — via `docker-compose.logging.yml`):
- **System Overview**: Cluster health, node status, memory usage
- **Performance Metrics**: Request rates, response times, cache hit ratios
- **Error Monitoring**: Failed requests, timeout alerts, node failures

**Prometheus Dashboard** (http://localhost:3001 — via `docker-compose.monitoring.yml`):
- **Overview Row**: Cluster health, size, items, memory, pressure, hit rate
- **Throughput Row**: SET/GET/DEL ops/sec, hits & misses/sec
- **Latency Row**: SET/GET/DEL p50, p95, p99 histograms
- **Memory Row**: Usage over time, pressure with threshold bands, allocations/sec, errors & evictions/sec
- **Cluster Row**: Cluster size over time, items per node

```bash
# Start Prometheus + Grafana monitoring stack
docker compose -f docker-compose.monitoring.yml up -d
# Grafana: http://localhost:3001 (admin / admin123)
# Prometheus: http://localhost:9090
```

### **Elasticsearch Logs** (http://localhost:9200)
- **Centralized Logging**: All cluster nodes, operations, and errors
- **Search & Analysis**: Query logs by node, operation type, or time range
- **Error Tracking**: Exception traces, failed operations, debug information
- **Audit Trail**: Configuration changes, cluster events, admin operations

### **Health Monitoring**
```bash
# Cluster health
curl http://localhost:9080/health
curl http://localhost:9081/health  
curl http://localhost:9082/health

# Node statistics
curl http://localhost:9080/stats

# Memory usage
curl http://localhost:9080/api/cache/stats
```

### **Operational Commands**
```bash
# View cluster logs in real-time
docker logs -f hypercache-filebeat

# Query Elasticsearch directly
curl "http://localhost:9200/logs-*/_search?q=level:ERROR"

# Monitor resource usage
docker stats hypercache-elasticsearch hypercache-grafana

# Backup persistence data
tar -czf hypercache-backup-$(date +%Y%m%d).tar.gz data/
```

### **Logging & Log Levels**

HyperCache uses structured JSON logging with correlation IDs for full request tracing across all cluster nodes.

**Available log levels** (from most to least verbose):

| Level | What it includes |
|-------|-----------------|
| `debug` | Everything: cuckoo filter decisions, event bus routing, gossip internals, health checks, snapshot ticks |
| `info` | Business operations: request lifecycle (start → operation → result), replication flow, cluster membership changes, persistence events |
| `warn` | Potential issues: memory pressure warnings, failed joins, missing event bus |
| `error` | Failures: replication errors, deserialization failures, storage errors |
| `fatal` | Unrecoverable: startup failures |

**Changing the log level:**

Edit the node config YAML (e.g., `configs/docker/node1-config.yaml`):

```yaml
logging:
  level: "info"     # Change to "debug" for troubleshooting, "warn" for quieter logs
  max_file_size: "100MB"
  max_files: 5
  output: ["console", "file"]
  structured: true
  log_dir: "/app/logs"
```

For Docker deployments, update all three node configs and rebuild:

```bash
# Edit configs/docker/node1-config.yaml, node2-config.yaml, node3-config.yaml
# Then rebuild and redeploy:
docker compose -f docker-compose.cluster.yml up -d --build
```

**Request tracing with correlation IDs:**

Every request gets a `correlation_id` that flows through the entire lifecycle — from HTTP entry through cache operations to cross-node replication. Use it to trace any request across all nodes:

```bash
# Trace a specific request across all nodes
docker logs hypercache-node1 2>&1 | grep "abc-123-correlation-id"
docker logs hypercache-node2 2>&1 | grep "abc-123-correlation-id"
docker logs hypercache-node3 2>&1 | grep "abc-123-correlation-id"

# Find all errors in the last hour
docker logs --since 1h hypercache-node1 2>&1 | grep '"level":"ERROR"'

# Find all replication events
docker logs hypercache-node1 2>&1 | grep '"action":"replication"'
```

You can also pass your own correlation ID via the `X-Correlation-ID` HTTP header for end-to-end tracing from your application:

```bash
curl -X PUT http://localhost:9080/api/cache/mykey \
  -H "Content-Type: application/json" \
  -H "X-Correlation-ID: my-trace-id-123" \
  -d '{"value": "hello"}'
```

## 📖 **Documentation**

See [docs/README.md](docs/README.md) for the full documentation index:
- **Architecture** — Consistent hashing, Cuckoo filter internals, RESP protocol, Raft consensus
- **Guides** — Development setup, Docker, observability, multi-VM deployment
- **Reference** — Benchmarks, persistence paths, known issues
```

### Clean Up
```bash
# Stop all local nodes
pkill -f bin/hypercache

# Stop Docker cluster
docker compose -f docker-compose.cluster.yml down

# Clean persistence data
./scripts/clean-persistence.sh --all

# Clean Elasticsearch data  
./scripts/clean-elasticsearch.sh
```

## 🔧 **Configuration**

### System Configuration
```bash
# Start 3-node local cluster for development/testing
./scripts/start-3node-local.sh

# Start custom N-node cluster
make cluster NODES=5

# Stop cluster
make cluster-stop
pkill -f bin/hypercache

# Docker cluster with full monitoring stack
docker compose -f docker-compose.cluster.yml up -d
docker compose -f docker-compose.cluster.yml down
```

### Node Configuration
```yaml
# configs/node1-config.yaml
node:
  id: "node-1"
  data_dir: "./data/node-1"
  
network:
  resp_port: 8080
  http_port: 9080
  gossip_port: 7946
  
cluster:
  seeds: ["10.0.1.10:7946"]          # Static seeds (IP:port or hostname)
  seed_dns: ""                       # DNS-based discovery (K8s headless Service)
  seed_dns_port: 7946                # Port for DNS-discovered seeds
  replication_factor: 3
  consistency_level: "eventual"  # "eventual" (async) or "quorum" (wait for majority ACKs)
  
cache:
  max_memory: "8GB"
  default_ttl: "0"            # 0 = infinite (no expiry); set per-store or per-key
  cuckoo_filter_fpp: 0.01     # 1% false positive rate
  max_stores: 16              # max stores allowed (1-64)
  
persistence:
  enabled: true
  strategy: "hybrid"          # "aof", "snapshot", "hybrid"
  sync_policy: "everysec"     # "always", "everysec", "no"
```

**Seed Discovery Modes:**
| Mode | Config | Use Case |
|------|--------|----------|
| Static | `seeds: ["ip:port"]` | Manual deployments |
| DNS | `seed_dns: "headless-svc.ns.svc.cluster.local"` | Kubernetes StatefulSet |
| Hostname | `seeds: ["node-1"]` | Docker Compose (auto-resolves via Docker DNS) |

### Environment Variable Overrides (Docker / K8s)
Environment variables have highest priority and override both defaults and YAML config:

| Variable | Description | Example |
|----------|-------------|---------|
| `HYPERCACHE_DEFAULT_MEMORY` | Default store max memory | `4GB` |
| `HYPERCACHE_DEFAULT_TTL` | Default store TTL (`0` = infinite) | `0`, `1h`, `30m` |
| `HYPERCACHE_DEFAULT_EVICTION` | Default store eviction policy | `lru`, `lfu`, `fifo`, `ttl` |
| `HYPERCACHE_DEFAULT_CUCKOO` | Default store cuckoo filter toggle | `true`, `false` |
| `HYPERCACHE_MAX_STORES` | Maximum stores allowed | `16` |
| `HYPERCACHE_CUCKOO_FILTER_FPP` | Cuckoo filter false positive rate | `0.01` |
| `HYPERCACHE_PERSISTENCE_ENABLED` | Global persistence toggle | `true`, `false` |
| `HYPERCACHE_PERSISTENCE_STRATEGY` | Global persistence strategy | `hybrid`, `aof`, `snapshot` |

```bash
# Docker example: override default store config without a YAML file
docker run -e HYPERCACHE_DEFAULT_MEMORY=4GB \
           -e HYPERCACHE_DEFAULT_TTL=0 \
           -e HYPERCACHE_DEFAULT_EVICTION=lru \
           -e HYPERCACHE_DEFAULT_CUCKOO=true \
           -e HYPERCACHE_PERSISTENCE_ENABLED=true \
           -e HYPERCACHE_PERSISTENCE_STRATEGY=hybrid \
           rishabhverma17/hypercache
```

### Per-Store Configuration
```yaml
# Only "default" store ships out of the box.
# Store config is immutable — to change settings, drop and recreate the store.
# Additional stores can be defined in YAML or created at runtime via API.
stores:
  - name: "default"
    eviction_policy: "lru"       # LRU eviction
    max_memory: "8GB"
    default_ttl: "0"             # 0 = infinite
    cuckoo_filter: true          # Enable probabilistic lookups
    persistence: "hybrid"        # "hybrid", "aof", "snapshot", "disabled"
    
  - name: "sessions"
    eviction_policy: "ttl"       # TTL-based eviction
    max_memory: "1GB"
    default_ttl: "30m"
    cuckoo_filter: true
    persistence: "aof"           # Write-ahead logging only
    
  - name: "temporary_data"
    eviction_policy: "lfu"       # Least frequently used
    max_memory: "512MB"
    default_ttl: "15m"
    cuckoo_filter: false          # Disable for pure cache
    persistence: "disabled"       # In-memory only
```

### Monitoring Configuration
```yaml
# Grafana — ELK stack (localhost:3000)
Username: admin
Password: admin123
Datasource: Elasticsearch (HyperCache Logs)

# Grafana — Prometheus stack (localhost:3001)
Username: admin
Password: admin123
Datasource: Prometheus (auto-provisioned)
Dashboard: "HyperCache — Prometheus Metrics" (auto-loaded)
```

### Config Validation
```bash
# Validate a config file before deploying
hypercache config validate configs/hypercache.yaml

# Output:
# Config: configs/hypercache.yaml
# Node ID: node-1
# RESP port: 8080, HTTP port: 9080, Gossip port: 7946
# Stores: 1 (max 16)
# Persistence: enabled=true, strategy=hybrid, sync_policy=everysec
# Warnings:
#   ⚠  compression_level=6: high CPU cost for snapshots; consider level 1
#   ⚠  no cluster seeds or seed_dns configured — node will run standalone
# Validation: PASS
```

## 🛠️ **Core Technologies**

### **RESP (Redis Serialization Protocol)**
- **What**: Binary protocol for Redis compatibility
- **Why**: Enables seamless integration with existing Redis clients and tools
- **Features**: Full command set support, pipelining, pub/sub ready
- **Performance**: Zero-copy parsing, minimal overhead

### **GOSSIP Protocol**
- **What**: Decentralized node discovery and health monitoring
- **Why**: Eliminates single points of failure in cluster coordination
- **Features**: Automatic node detection, failure detection, metadata propagation
- **Scalability**: O(log n) message complexity, handles thousands of nodes

### **RAFT Consensus**
- **What**: Distributed consensus algorithm for cluster coordination
- **Why**: Ensures data consistency and handles leader election
- **Features**: Strong consistency guarantees, partition tolerance, log replication
- **Reliability**: Proven algorithm used by etcd, Consul, and other systems

### **Hash Ring (Consistent Hashing)**
- **What**: Distributed data placement using consistent hashing
- **Why**: Minimizes data movement during cluster changes
- **Features**: Virtual nodes for load balancing, configurable replication
- **Efficiency**: O(log n) lookup time, minimal rehashing on topology changes

### **AOF + WAL Persistence**
- **AOF (Append-Only File)**: Sequential write logging for durability
- **WAL (Write-Ahead Logging)**: Transaction-safe write ordering
- **Hybrid Approach**: Combines speed of WAL with simplicity of AOF
- **Recovery**: Fast startup with complete data restoration

### **Cuckoo Filters**
- **What**: Space-efficient probabilistic data structure
- **Why**: Better than Bloom filters - supports deletions and has better locality
- **Features**: Configurable false positive rates, O(1) operations
- **Use Cases**: Membership testing, cache admission policies, duplicate detection

## 📚 **Documentation**
 
- **[docs/](docs/)**: Technical deep-dives and architecture docs

## 💾 **Persistence & Recovery Deep Dive**

### **Dual Persistence Architecture**

HyperCache implements a sophisticated dual-persistence system combining the best of both AOF and WAL approaches:

#### **AOF (Append-Only File)**
```yaml
# Ultra-fast sequential writes
Write Latency: 2.7µs average
Throughput: 370K+ operations/sec
File Format: Human-readable command log
Recovery: Sequential replay of operations
```

#### **WAL (Write-Ahead Logging)**
```yaml
# Transaction-safe write ordering
Consistency: ACID compliance
Durability: Configurable fsync policies
Crash Recovery: Automatic rollback/forward
Performance: Batched writes, zero-copy I/O
```

### **Recovery Scenarios**

#### **Fast Startup Recovery**
```bash
# Measured Performance (Production Test)
✅ Data Set: 10 entries
✅ Recovery Time: 160µs
✅ Success Rate: 100% (5/5 tests)
✅ Memory Overhead: <1MB
```

#### **Point-in-Time Recovery**
```bash
# Snapshot-based recovery
✅ Snapshot Creation: 3.7ms for 7 entries  
✅ File Size: 555B snapshot + 573B AOF
✅ Recovery Strategy: Snapshot + AOF replay
✅ Data Integrity: Checksum verification
```

### **Configurable Persistence Policies**

#### **Per-Store Persistence Settings**
```yaml
stores:
  - name: "critical_data"
    persistence: "hybrid"         # Full durability (AOF + snapshots)
    # Uses global persistence settings for sync_policy, snapshot_interval, etc.
      
  - name: "session_cache"
    persistence: "aof"            # Write-ahead logging only
      
  - name: "temporary_cache"
    persistence: "disabled"       # In-memory only — no disk I/O overhead
```

#### **Durability vs Performance Tuning**
```yaml
# High Durability (Financial/Critical Data)
fsync: "always"              # Every write synced
batch_size: 1                # Individual operations
compression: false           # No CPU overhead

# Balanced (General Purpose)  
fsync: "periodic"            # 1-second sync intervals
batch_size: 100              # Batch writes
compression: true            # Space efficiency

# High Performance (Analytics/Temporary)
fsync: "never"               # OS manages sync
batch_size: 1000             # Large batches
compression: false           # CPU for throughput
```

### **Recovery Guarantees**

#### **Crash Recovery**
- **Zero Data Loss**: With `fsync: always` configuration
- **Automatic Recovery**: Self-healing on restart
- **Integrity Checks**: Checksums on all persisted data
- **Partial Recovery**: Recovers valid data even from corrupted files

#### **Network Partition Recovery**
- **Consensus-Based**: RAFT ensures consistency across partitions
- **Split-Brain Protection**: Majority quorum prevents conflicts  
- **Automatic Reconciliation**: Rejoining nodes sync automatically
- **Data Validation**: Cross-node checksum verification

### **Operational Commands**

```bash
# Manual snapshot creation
curl -X POST http://localhost:9080/api/admin/snapshot

# Force AOF rewrite (compact logs)
curl -X POST http://localhost:9080/api/admin/aof-rewrite

# Check persistence status
curl http://localhost:9080/api/admin/persistence-stats

# Backup current state
./scripts/backup-persistence.sh

# Restore from backup
./scripts/restore-persistence.sh backup-20250822.tar.gz
```

## 🎯 **Use Cases**

### **Enterprise Deployment**
- High-performance caching layers for microservices
- Session storage with automatic failover
- Redis replacement with lower memory costs and better observability
- Distributed caching with real-time monitoring

### **Development & Testing**
- Local development with production-like monitoring
- Load testing with comprehensive metrics
- Log analysis and debugging with Elasticsearch
- Performance monitoring with Grafana dashboards

### **Production Examples**

#### Web Application Cache
```bash
# Store user session
curl -X PUT http://localhost:9080/api/cache/user:123:session \
  -d '{"value":"{\"user_id\":123,\"role\":\"admin\"}", "ttl_hours":2}'

# Retrieve session
curl http://localhost:9080/api/cache/user:123:session
```

#### Redis Client Usage
```go
import "github.com/redis/go-redis/v9"

// Connect to any cluster node
client := redis.NewClient(&redis.Options{
    Addr: "localhost:8080", // Node 1 RESP port
})

// Use exactly like Redis!
client.Set(ctx, "user:123:profile", userData, 30*time.Minute)
client.Incr(ctx, "page:views")
client.LPush(ctx, "notifications", "New message")
```

#### HTTP API Usage
```bash
# Rate limiting counters
curl -X PUT http://localhost:9080/api/cache/rate:user:456 \
  -d '{"value":"10", "ttl_hours":1}'

# Feature flags
curl -X PUT http://localhost:9080/api/cache/feature:new_ui \
  -d '{"value":"enabled", "ttl_hours":24}'
```

## 🚀 **Getting Started Guide**

### Prerequisites
- **Go 1.23.2+**
- **Docker & Docker Compose** (for monitoring stack)
- **Git** (for cloning)

### Installation
```bash
git clone <your-repository-url>
cd Cache

# Build
make build

# Quick start — local 3-node cluster
./scripts/start-3node-local.sh

# Or Docker with full monitoring stack
docker compose -f docker-compose.cluster.yml up -d

# Access your system:
# - HTTP API: http://localhost:9080/api/cache/
# - RESP:    redis-cli -p 8080
# - Grafana: http://localhost:3000 (admin/admin123, Docker only)
```

### First Steps
1. **Check Cluster Health**: Visit http://localhost:9080/health
2. **Store Some Data**: `redis-cli -p 8080 SET mykey "Hello World"`
3. **View in Grafana**: Open http://localhost:3000, check dashboards
4. **Query Logs**: Visit http://localhost:9200 for Elasticsearch

### Development Workflow
```bash
# Build and test
make build
make test-unit

# Start 3-node development cluster
./scripts/start-3node-local.sh

# Run Postman integration tests
# Import HyperCache.postman_collection.json → Run Collection

# Run server benchmarks (requires running server)
make bench-server

# Stop cluster
pkill -f bin/hypercache

# View logs (Docker)
docker compose -f docker-compose.logging.yml up -d
```

## 📚 **Documentation**

- **[HTTP API Documentation](docs/markdown/api/http-api-documentation.md)**: Complete HTTP API reference with examples
- **[Technical Deep-Dives](docs/)**: Architecture, implementation details
- **[Configuration Guide](docs/multi-vm-deployment-guide.md)**: Production deployment  
- **[RESP Protocol Reference](examples/resp-demo/README.md)**: Redis compatibility examples
- **[Performance Benchmarks](docs/performance-benchmarks.md)**: Throughput and latency tests
- **[Monitoring Setup](docs/grafana-dashboards-guide.md)**: Dashboard configuration

## 🤝 **Contributing**

This project demonstrates enterprise-grade Go development with:
- **Clean Architecture**: Domain-driven design with clear interfaces
- **Observability First**: Comprehensive logging, metrics, and monitoring
- **Production Ready**: Persistence, clustering, and operational tooling
- **Protocol Compatibility**: Full Redis RESP implementation
- **Performance Focused**: Benchmarked and optimized for high throughput

## 📄 **License**

MIT License - feel free to use in your projects!

---

## 🎉 **Enterprise Success Story**

**From Concept to Production-Grade System:**
- **Vision**: Redis-compatible distributed cache with advanced monitoring
- **Built**: Full production system with ELK stack integration  
- **Achieved**: Multi-node clusters, real-time observability, enterprise persistence
- **Result**: Complete caching platform ready for cloud deployment

**Features that set HyperCache apart:**
- 🔄 **Zero-downtime deployments** with cluster coordination
- 📊 **Real-time monitoring** with Grafana + Elasticsearch
- 💾 **Enterprise persistence** with AOF + snapshot recovery  
- 🔍 **Full observability** with centralized logging and metrics
- ⚡ **Redis compatibility** drop-in replacement capability

---

**Made with ❤️ in Go** | **Redis Compatible** | **Enterprise Observability**
