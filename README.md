# HyperCache - Production-Grade Distributed Cache

[![Status](https://img.shields.io/badge/Status-Production%20Ready-brightgreen)]()
[![Go Version](https://img.shields.io/badge/Go-1.23.2-blue)]()
[![Redis Compatible](https://img.shields.io/badge/Redis-Compatible-red)]()
[![Monitoring](https://img.shields.io/badge/Monitoring-Grafana%20%2B%20ELK-orange)]()

**HyperCache** is a high-performance, Redis-compatible distributed cache with advanced memory management, integrated probabilistic data structures (Cuckoo filters), and comprehensive monitoring stack. Built in Go for cloud-native environments.

## 🎯 **Latest Features** ✅

**Production-ready distributed cache with full observability stack:**
- ✅ Multi-node cluster deployment
- ✅ Full Redis client compatibility  
- ✅ Enterprise persistence (AOF + Snapshots)
- ✅ Real-time monitoring with Grafana
- ✅ Centralized logging with Elasticsearch + Filebeat
- ✅ HTTP API + RESP protocol support
- ✅ Advanced memory management
- ✅ Cuckoo filter integration

### 🔥 **Monitoring & Observability**
- **Grafana Dashboards**: Real-time metrics visualization
- **Elasticsearch**: Centralized log aggregation and search
- **Filebeat**: Log shipping and processing
- **Health Checks**: Built-in monitoring endpoints

## 🚀 **Quick Start**

### Start Complete System (Recommended)
```bash
# Start both cluster and monitoring stack
./scripts/start-system.sh

# Or start with clean data
./scripts/start-system.sh --clean

# Access points:
# - Cluster nodes: http://localhost:9080, 9081, 9082
# - Grafana: http://localhost:3000 (admin/admin123)
# - Elasticsearch: http://localhost:9200
```

### Start Individual Components

#### HyperCache Cluster Only
```bash
# Build and start 3-node cluster
./scripts/build-and-run.sh cluster

# Or start single node
./scripts/build-and-run.sh run node-1
```

#### Monitoring Stack Only  
```bash
# Start Elasticsearch, Grafana, and Filebeat
docker-compose -f docker-compose.logging.yml up -d
```

### Test with Redis Client
```bash
# Using redis-cli (if installed)
redis-cli -p 8080
> SET mykey "Hello HyperCache"
> GET mykey

# Using Go client
cd examples/resp-demo
go run simple_demo.go
```

### HTTP API Testing
```bash
# Store data
curl -X PUT http://localhost:9080/api/cache/testkey \
  -H "Content-Type: application/json" \
  -d '{"value": "test value", "ttl_hours": 1}'

# Retrieve data
curl http://localhost:9080/api/cache/testkey

# Health check
curl http://localhost:9080/health
```

## 🏆 **Key Features**

### **Redis Compatibility**
- Full RESP protocol implementation
- Works with any Redis client library
- Drop-in replacement for many Redis use cases
- Standard commands: GET, SET, DEL, EXISTS, PING, INFO, FLUSHALL, DBSIZE

### **Distributed Architecture**
- Multi-node cluster with consistent hashing
- Gossip protocol for node discovery
- Automatic failover and rebalancing
- HTTP API for cluster management

### **Advanced Memory Management**
- Smart memory pool with pressure monitoring
- Session-based eviction policies with LRU fallback
- Real-time memory usage tracking
- Configurable memory limits and cleanup

### **Enterprise Persistence**
- AOF (Append-Only File) logging with sub-microsecond writes
- Snapshot-based recovery with millisecond restore times
- Configurable persistence policies
- Data durability guarantees

### **Probabilistic Data Structures** 
- Integrated Cuckoo filter for bloom-like operations
- O(1) probabilistic membership testing
- Memory-efficient false positive control
- Seamless integration with cache operations

### **Production Monitoring**
- **Grafana**: Real-time dashboards and alerting
- **Elasticsearch**: Centralized log storage and search
- **Filebeat**: Automated log collection and shipping
- **Health Endpoints**: Built-in monitoring and diagnostics
- **Metrics Export**: Performance and usage statistics

## � **Project Structure**

```
HyperCache/
├── cmd/hypercache/             # Server entry point
├── scripts/                    # Deployment and management scripts
│   ├── start-system.sh         # Complete system launcher
│   ├── build-and-run.sh        # Build and cluster management
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
├── docker-compose.logging.yml  # Monitoring stack
└── filebeat.yml               # Log shipping configuration
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
│   Memory Pool   │    │   Data Storage   │    │ Cuckoo Filter   │    │   Hash Ring     │    │   Gossip Node   │
│   (Pressure     │    │   + Persistence  │    │ (Probabilistic  │    │ (Consistent     │    │   Discovery     │
│    Monitoring)  │    │   (AOF+Snapshot) │    │   Operations)   │    │   Hashing)      │    │   & Failover    │
└─────────────────┘    └──────────────────┘    └─────────────────┘    └──────────────────┘    └─────────────────┘
       │                         │                         │                         │                         │
       └─────────────────────────┼─────────────────────────┼─────────────────────────┼─────────────────────────┘
                                 │                         │                         │
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                    MONITORING STACK                                                           │
├─────────────────┬──────────────────┬─────────────────┬──────────────────┬─────────────────┬─────────────────┤
│    Filebeat     │   Elasticsearch  │     Grafana     │   Health API     │   Metrics       │   Alerting      │
│  (Log Shipper)  │  (Log Storage)   │  (Dashboards)   │  (Diagnostics)   │  (Performance)  │  (Monitoring)   │
└─────────────────┴──────────────────┴─────────────────┴──────────────────┴─────────────────┴─────────────────┘
```

## � **Monitoring & Operations**

### **Grafana Dashboards** (http://localhost:3000)
- **System Overview**: Cluster health, node status, memory usage
- **Performance Metrics**: Request rates, response times, cache hit ratios
- **Error Monitoring**: Failed requests, timeout alerts, node failures
- **Capacity Planning**: Memory trends, storage usage, growth patterns

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

## 🧪 **Testing & Development**

### Run All Tests
```bash
go test ./internal/... -v
```

### Run Benchmarks  
```bash
go test ./internal/... -bench=. -benchmem
```

### Cluster Testing
```bash
# Start cluster and test
./scripts/build-and-run.sh cluster

# Test HTTP API
curl -X PUT http://localhost:9080/api/cache/test \
  -d '{"value":"hello cluster","ttl_hours":1}'
curl http://localhost:9080/api/cache/test

# Test RESP protocol  
redis-cli -p 8080 SET mykey "test value"
redis-cli -p 8080 GET mykey
```

### Load Testing
```bash
# Generate load for dashboard testing
./scripts/generate-dashboard-load.sh
```

### Clean Up
```bash
# Stop all services
./scripts/build-and-run.sh stop
docker-compose -f docker-compose.logging.yml down

# Clean persistence data
./scripts/clean-persistence.sh --all

# Clean Elasticsearch data  
./scripts/clean-elasticsearch.sh
```

## 🔧 **Configuration**

### System Configuration
```bash
# Start complete system with monitoring
./scripts/start-system.sh --all

# Start only cluster
./scripts/start-system.sh --cluster  

# Start only monitoring
./scripts/start-system.sh --monitor

# Clean data and restart
./scripts/start-system.sh --clean --all
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
  
cache:
  max_memory: 1GB
  default_ttl: 1h
  cleanup_interval: 5m
  eviction_policy: "session"
  
persistence:
  enabled: true
  aof_enabled: true
  snapshot_enabled: true
  snapshot_interval: 300s
```

### Monitoring Configuration
```yaml
# Grafana (localhost:3000)
Username: admin
Password: admin123

# Pre-configured datasources:
- Elasticsearch (HyperCache Logs)
- Health check endpoints
```

## 📚 **Documentation**

- **[PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)**: Complete feature overview
- **[REMAINING_ITEMS.md](REMAINING_ITEMS.md)**: Future enhancements roadmap  
- **[examples/resp-demo/README.md](examples/resp-demo/README.md)**: Demo usage guide
- **[docs/](docs/)**: Technical deep-dives and architecture docs

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

# Quick start - everything in one command
./scripts/start-system.sh

# Access your system:
# - Grafana: http://localhost:3000 (admin/admin123)  
# - API: http://localhost:9080/api/cache/
# - Redis: localhost:8080 (redis-cli -p 8080)
```

### First Steps
1. **Check Cluster Health**: Visit http://localhost:9080/health
2. **Store Some Data**: `redis-cli -p 8080 SET mykey "Hello World"`
3. **View in Grafana**: Open http://localhost:3000, check dashboards
4. **Query Logs**: Visit http://localhost:9200 for Elasticsearch

### Development Workflow
```bash
# Build and test
go build -o bin/hypercache cmd/hypercache/main.go
go test ./internal/... -v

# Start development cluster
./scripts/build-and-run.sh cluster

# View logs
tail -f logs/*.log

# Stop everything
./scripts/build-and-run.sh stop
docker-compose -f docker-compose.logging.yml down
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

**Made with ❤️ in Go** | **Production Ready** | **Redis Compatible** | **Enterprise Observability**
