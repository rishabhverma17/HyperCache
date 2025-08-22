# HyperCache System Management

This directory contains the essential scripts for managing your HyperCache distributed cache system.

## Quick Start

### Start Everything (Recommended)
```bash
./scripts/start-system.sh
```
This starts both the HyperCache cluster and monitoring stack (Elasticsearch, Grafana, Filebeat).

### Start Options
```bash
./scripts/start-system.sh --cluster      # Start only HyperCache cluster
./scripts/start-system.sh --monitor      # Start only monitoring stack  
./scripts/start-system.sh --clean        # Clean data first, then start
./scripts/start-system.sh --rebuild      # Rebuild binaries, then start
```

## Management Scripts

### System Control
- **`start-system.sh`** - Master script to start cluster and/or monitoring
- **`build-and-run.sh`** - Build and run HyperCache cluster
- **`build-hypercache.sh`** - Build HyperCache binaries only

### Data Management
- **`clean-persistence.sh`** - Clean HyperCache node persistence data
  ```bash
  ./scripts/clean-persistence.sh --all          # Clean all nodes
  ./scripts/clean-persistence.sh --node node-1  # Clean specific node
  ```

- **`clean-elasticsearch.sh`** - Clean Elasticsearch logs and indices
  ```bash
  ./scripts/clean-elasticsearch.sh --status     # Show current indices
  ./scripts/clean-elasticsearch.sh --logs       # Clean only log indices
  ./scripts/clean-elasticsearch.sh --logs --restart  # Clean logs + restart Filebeat
  ```

### Utilities
- **`cleanup.sh`** - General cleanup utilities
- **`test-bench.sh`** - Performance benchmarking

## Access URLs

When the system is running:

- **HyperCache Nodes:**
  - Node 1: http://localhost:9080
  - Node 2: http://localhost:9081  
  - Node 3: http://localhost:9082

- **Monitoring:**
  - Grafana: http://localhost:3000 (admin/admin123)
  - Elasticsearch: http://localhost:9200

### 📊 Grafana Dashboards

Your HyperCache system includes 5 comprehensive monitoring dashboards:

1. **Health Dashboard** - System status, node health, error rates
2. **Performance Metrics** - P99/P95/P50 latencies for GET/PUT/DELETE operations
3. **System Components** - Gossip, CuckooFilter, RAFT, Events monitoring
4. **Operational Dashboard** - Cache hit/miss, memory pressure, consistency
5. **Log Stream** - Real-time structured log viewer

**Quick Start:**
```bash
# Generate test data for dashboards
./scripts/generate-dashboard-load.sh

# View detailed guide
open docs/grafana-dashboards-guide.md
```

## Quick Test

After starting the system:
```bash
# Store a value
curl -X PUT http://localhost:9080/api/cache/test \
  -H "Content-Type: application/json" \
  -d '{"value":"hello world","ttl_hours":1}'

# Retrieve the value  
curl http://localhost:9080/api/cache/test
```

## Stopping Services

```bash
# Stop HyperCache cluster
pkill -f hypercache

# Stop monitoring stack
docker-compose -f docker-compose.logging.yml down
```

## Troubleshooting

1. **Port conflicts**: Check for processes using ports 8080-8082 and 9080-9082
2. **Docker issues**: Ensure Docker is running for monitoring stack
3. **No logs in Grafana**: Check time range (logs may be from previous day)
4. **Clean start**: Use `./scripts/start-system.sh --clean` for fresh data

## Log Files

- **HyperCache logs**: `./logs/node-*.log`
- **Docker logs**: `docker logs <container-name>`
