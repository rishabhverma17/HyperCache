# HyperCache Observability Guide

Structured logging, ELK stack integration, and Grafana dashboards for monitoring HyperCache.

---

## 1. Overview

HyperCache ships with a comprehensive observability stack:

- **Structured JSON Logging** — Every log entry includes correlation IDs, node IDs, components, and timestamps for full request tracing across the distributed cluster.
- **ELK Stack (Elasticsearch + Filebeat)** — Logs are shipped from each node via Filebeat into Elasticsearch for centralized querying and analysis.
- **Grafana Dashboards** — Five pre-built dashboards cover health, performance, system components, operations, and raw logs.

### Architecture

```
HyperCache Nodes → JSON log files → Filebeat → Elasticsearch → Grafana
                                                     ↓
                                               curl / API queries
```

### Access Points

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana | http://localhost:3000 | `admin/admin123` |
| Elasticsearch | http://localhost:9200 | — |

---

## 2. Logging

### Structured JSON Format

All HyperCache log entries are structured JSON with consistent fields:

| Field | Description | Example |
|-------|-------------|---------|
| `@timestamp` | ISO 8601 timestamp | `2025-08-23T10:30:00Z` |
| `level` | Log level | `INFO`, `DEBUG`, `WARN`, `ERROR` |
| `node_id` | Originating node | `node-1` |
| `component` | System component | `cache`, `cluster`, `filter`, `http` |
| `action` | Operation type | `put_request`, `get_operation`, `negative_lookup` |
| `correlation_id` | Request trace ID | `9eaf6608-1f28-4f9b-a03e-16dff296d49b` |

### Log Levels

| Level | Usage |
|-------|-------|
| `ERROR` | Failures requiring attention |
| `WARN` | Degraded states, potential issues |
| `INFO` | Normal operational events (default) |
| `DEBUG` | Detailed internals, Cuckoo filter operations |

Change the log level in your node config:

```yaml
logging:
  level: "debug"   # info | debug | warn | error
  format: "json"
  outputs: ["stdout"]
```

### Correlation IDs

Every HTTP request generates a correlation ID that propagates through all internal operations. Use it to trace a single request across nodes:

```bash
# Search by correlation ID in Elasticsearch
curl -X GET "http://localhost:9200/hypercache-*/_search?pretty" \
  -H 'Content-Type: application/json' -d'
{
  "query": { "match": { "correlation_id": "YOUR_CORRELATION_ID" } },
  "sort": [{"@timestamp": {"order": "asc"}}]
}'
```

In Grafana Explore:

```
{service="hypercache"} | json | correlation_id="9eaf6608-1f28-4f9b-a03e-16dff296d49b"
```

### Viewing Container Logs Directly

```bash
# Real-time logs
docker logs -f hypercache-node1

# Recent logs
docker logs hypercache-node1 --tail 50 --since 10m

# Filter by pattern
docker logs hypercache-node1 2>&1 | grep -i error
docker logs hypercache-node1 2>&1 | grep -i "cluster\|member"

# All nodes at once
docker-compose -f docker-compose.cluster.yml logs -f
```

### Generating Test Logs

```bash
# PUT
curl -X PUT -H "Content-Type: application/json" \
  -d '{"value":"test-value","ttl_hours":1.0}' \
  http://localhost:9080/api/cache/test-key

# GET
curl http://localhost:9080/api/cache/test-key

# DELETE
curl -X DELETE http://localhost:9080/api/cache/test-key

# Batch GET
curl -X POST -H "Content-Type: application/json" \
  -d '{"keys":["test-key","nonexistent"]}' \
  http://localhost:9080/api/cache/batch/get
```

---

## 3. Elasticsearch

### Health Checks

```bash
# Cluster health
curl http://localhost:9200/_cluster/health?pretty

# Node info
curl http://localhost:9200/_nodes?pretty

# List HyperCache indices
curl http://localhost:9200/_cat/indices/hypercache*?v
```

### Querying Logs

#### Recent Logs (last 15 minutes)

```bash
curl -X GET "http://localhost:9200/hypercache-*/_search?pretty" \
  -H 'Content-Type: application/json' -d'
{
  "query": {
    "range": { "@timestamp": { "gte": "now-15m" } }
  },
  "sort": [{"@timestamp": {"order": "desc"}}],
  "size": 20
}'
```

#### By Node

```bash
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?q=node_id:node-1&pretty&size=5"
```

#### By Log Level

```bash
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?q=level:ERROR&pretty&size=5"
```

#### Complex Bool Query

```bash
curl -X GET "http://localhost:9200/hypercache-docker-logs-*/_search" \
  -H 'Content-Type: application/json' -d'
{
  "query": {
    "bool": {
      "must": [
        {"range": {"@timestamp": {"gte": "now-1h"}}},
        {"term": {"level.keyword": "INFO"}}
      ]
    }
  },
  "size": 10,
  "sort": [{"@timestamp": {"order": "desc"}}]
}'
```

#### Cuckoo Filter Operations (requires DEBUG level)

```bash
# Generate filter activity
curl -X PUT http://localhost:9080/api/cache/test-key \
  -H "Content-Type: application/json" -d '{"value":"test"}'
curl http://localhost:9080/api/cache/missing-key   # triggers negative_lookup
curl http://localhost:9080/api/cache/test-key       # triggers positive_lookup

# Search filter logs
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&size=10&pretty"
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?q=action:negative_lookup&sort=@timestamp:desc&size=5&pretty"
```

### Index & Data Stream Management

```bash
# Count total documents
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value'

# Check index mapping
curl http://localhost:9200/hypercache-docker-logs-*/_mapping?pretty

# List data streams
curl http://localhost:9200/_data_stream?pretty

# List index templates
curl http://localhost:9200/_index_template?pretty

# Get sample document structure
curl -s "http://localhost:9200/hypercache-docker-logs-*/_search?size=1&pretty" | jq '.hits.hits[0]._source'
```

### Debugging Elasticsearch

#### Field Mapping Conflicts

Symptom: `document_parsing_exception`, "tried to parse field as object"

```bash
# Check current mapping
curl -s "http://localhost:9200/hypercache-docker-logs-*/_mapping" | jq '.[].mappings.properties.service'

# Fix: delete data stream and restart Filebeat
curl -X DELETE "http://localhost:9200/_data_stream/hypercache-docker-logs"
# Update filebeat-docker.yml to rename conflicting fields
docker restart hypercache-filebeat
```

#### Clearing Log Data

```bash
# Delete only log indices (keep cache data)
curl -X DELETE "http://localhost:9200/_data_stream/hypercache-docker-logs"
curl -X DELETE "http://localhost:9200/hypercache-docker-logs-*"
rm -f logs/*.log
docker restart hypercache-filebeat

# Or use the management script
./scripts/elk-management.sh clean-logs
```

---

## 4. Grafana Dashboards

All dashboards are auto-provisioned and available at http://localhost:3000 (`admin/admin123`).

### Dashboard Inventory

| Dashboard | File | Focus |
|-----------|------|-------|
| **Health** | `health-dashboard.json` | System-wide health, error rates, node status |
| **Performance Metrics** | `performance-metrics.json` | Latency percentiles (P50/P95/P99), request rates |
| **System Components** | `system-components.json` | Cuckoo filter, cluster replication, event bus |
| **Operational** | `operational-dashboard.json` | Hit/miss ratio, memory pressure, data distribution |
| **Logs** | `hypercache-logs.json` | Real-time structured log stream |

### Health Dashboard

Key panels: active node count, health check success rate, error rate, request volume, node status timeline, recent error log.

**Best for:** Operations, incident response, SLA monitoring.

### Performance Metrics Dashboard

Key panels: latency distributions, P50/P95/P99 per operation (GET/PUT/DELETE), request rate by method, HTTP status code distribution.

**Latency thresholds:**
- P95 < 10ms — Excellent
- P95 < 50ms — Good
- P95 > 100ms — Investigate

**Best for:** Performance tuning, capacity planning.

### System Components Dashboard

Key panels: cluster health & replication, Cuckoo filter operations and false positive rates, event bus activity, node communication matrix, replication events.

**Cuckoo filter metrics to watch:**
- `add_success` — keys added to filter
- `negative_lookup` — keys filtered out (avoiding expensive lookups)
- High negative_lookup ratio = good filter effectiveness

**Best for:** System engineering, debugging.

### Operational Dashboard

Key panels: cache hit/miss ratio, memory pressure indicators, consistency & replication health, data distribution across nodes, persistence layer status.

**Best for:** DevOps, capacity planning, business metrics.

### Logs Dashboard

Real-time structured log viewer connected to Elasticsearch.

**Best for:** Debugging, troubleshooting.

### Grafana Query Patterns

In the Explore view, select **HyperCache Logs** as the data source:

```
# All recent logs
{service="hypercache"}

# Filter by level
{service="hypercache"} | json | level="ERROR"

# Filter by node
{service="hypercache"} | json | node_id="node-1"

# Filter by component
{service="hypercache"} | json | component="cache"

# Specific operations
{service="hypercache"} | json | action="put_request"

# Count operations by node (5m window)
sum by (node_id) (count_over_time({service="hypercache"} | json | action="put_request"[5m]))

# Error rate by component
sum by (component) (count_over_time({service="hypercache"} | json | level="ERROR"[5m]))
```

### Elasticsearch Query Syntax (for custom panels)

```
component:cache AND action:get_operation
level:ERROR
node_id:node-1 AND duration_ms:>100
component:cluster AND action:replication
component:filter AND (action:add_success OR action:negative_lookup)
component:http AND fields.bytes_sent:*
```

### Creating Alerts

1. Navigate to **Alerting** → **Alert Rules**
2. Set query and thresholds
3. Configure notification channels (Slack, email, etc.)

Example alert rules:

```yaml
- alert: HighErrorRate
  expr: rate(hypercache_errors[5m]) > 0.1
  for: 2m
  annotations:
    summary: "HyperCache error rate is high"

- alert: NodeDown
  expr: hypercache_nodes_up < 3
  for: 1m
  annotations:
    summary: "HyperCache node is down"

- alert: HighLatency
  expr: hypercache_p95_latency > 100
  for: 5m
  annotations:
    summary: "HyperCache P95 latency is high"
```

### Troubleshooting Dashboards

```bash
# Dashboard not loading
curl localhost:9200  # Verify Elasticsearch is up
curl localhost:9200/_cat/indices  # Verify indices exist
docker restart hypercache-grafana

# No data in panels — check time range, generate load
./scripts/generate-dashboard-load.sh

# Test Grafana datasource connection
docker exec hypercache-grafana curl hypercache-elasticsearch:9200/_cluster/health?pretty
curl -u admin:admin123 "localhost:3000/api/datasources"

# Check Grafana provisioning config
docker exec hypercache-grafana cat /etc/grafana/provisioning/datasources/elasticsearch.yml
```

---

## 5. Filebeat Configuration

Filebeat ships logs from HyperCache containers to Elasticsearch.

### Check Status

```bash
# Container running?
docker ps | grep filebeat

# Filebeat logs
docker logs hypercache-filebeat --tail 50

# Internal Filebeat logs
docker exec hypercache-filebeat tail /var/log/filebeat/filebeat-$(date +%Y%m%d).ndjson

# Test connection to Elasticsearch
docker exec hypercache-filebeat curl hypercache-elasticsearch:9200/_cluster/health
docker exec hypercache-filebeat filebeat test output
```

### Monitor Registry

```bash
# What files is Filebeat tracking?
docker exec hypercache-filebeat ls -la /usr/share/filebeat/data/registry/filebeat/
docker exec hypercache-filebeat cat /usr/share/filebeat/data/registry/filebeat/meta.json
```

### Common Issues

#### Filebeat Not Sending Logs

Symptom: No documents in Elasticsearch, no errors in Filebeat logs.

```bash
# Check file permissions
docker exec hypercache-filebeat ls -la /var/lib/docker/containers/

# Reset registry and restart
docker stop hypercache-filebeat
docker volume rm hypercache_filebeat-data
docker-compose -f docker-compose.cluster.yml up -d hypercache-filebeat
```

#### Old Logs Not Appearing

```bash
# Check scan frequency
docker exec hypercache-filebeat cat /usr/share/filebeat/filebeat.yml | grep scan_frequency

# Check monitored paths
docker exec hypercache-filebeat filebeat export config | grep -A 10 "paths:"
```

### Configuration File

The Filebeat configuration lives at `filebeat-docker.yml` in the project root. Key settings:

- Input paths for HyperCache container logs
- Elasticsearch output with index pattern `hypercache-docker-logs-*`
- JSON parsing for structured log fields
- Field renaming to avoid mapping conflicts

---

## 6. Quick Reference Commands

### Essential Status Checks

```bash
# ELK management script
./scripts/elk-management.sh status       # Overall status
./scripts/elk-management.sh health       # Detailed health
./scripts/elk-management.sh logs-count   # Count ingested logs

# Elasticsearch health
curl localhost:9200/_cluster/health?pretty

# Index listing
curl localhost:9200/_cat/indices/hypercache*?v

# Document count
curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value'
```

### Log Searching

```bash
# Recent logs (last 10 min)
curl -X GET "localhost:9200/hypercache-docker-logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"range":{"@timestamp":{"gte":"now-10m"}}},"size":50,"sort":[{"@timestamp":{"order":"desc"}}]}' \
  | jq '.hits.hits[]._source'

# Errors only
curl "localhost:9200/hypercache-docker-logs-*/_search?pretty" \
  -H "Content-Type: application/json" \
  -d '{"query":{"match":{"level":"ERROR"}}}'

# By node
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=node_id:node-1&size=10&pretty"

# Cuckoo filter logs
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&size=10&pretty"

# Container logs with grep
docker logs hypercache-node1 --tail 30 | grep -E "(filter|negative_lookup|positive_lookup)"
```

### Real-Time Monitoring

```bash
# Watch log count grow
watch -n 2 'curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq ".hits.total.value"'

# Container resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Live log streaming
docker-compose -f docker-compose.cluster.yml logs -f

# Filebeat processing
docker logs hypercache-filebeat --follow

# Elasticsearch index sizes
curl "localhost:9200/_cat/indices/hypercache*?v&h=index,docs.count,store.size&s=docs.count:desc"
```

### Data Management

```bash
# Clear logs only (keep cache data)
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
rm -f logs/*.log
docker restart hypercache-filebeat

# Complete fresh start
docker-compose -f docker-compose.cluster.yml down --volumes
docker-compose -f docker-compose.cluster.yml up -d

# Clear only Elasticsearch data
docker stop hypercache-elasticsearch hypercache-grafana hypercache-filebeat
docker volume rm hypercache_es-data
docker-compose -f docker-compose.cluster.yml up -d hypercache-elasticsearch
docker-compose -f docker-compose.cluster.yml up -d hypercache-filebeat hypercache-grafana

# Clear only HyperCache data (keep ELK)
docker stop hypercache-node-1 hypercache-node-2 hypercache-node-3
rm -rf data/node-*/* logs/*.log
docker-compose -f docker-compose.cluster.yml up -d hypercache-node-1 hypercache-node-2 hypercache-node-3
```

### Shell Aliases

```bash
alias es-health="curl localhost:9200/_cluster/health?pretty"
alias es-indices="curl localhost:9200/_cat/indices/hypercache*?v"
alias log-count="curl -s localhost:9200/hypercache-docker-logs-*/_search?size=0 | jq .hits.total.value"
alias filebeat-logs="docker logs hypercache-filebeat --tail 50"
alias fresh-start="docker-compose -f docker-compose.cluster.yml down --volumes && docker-compose -f docker-compose.cluster.yml up -d"
```

### File Locations

| File | Purpose |
|------|---------|
| `filebeat-docker.yml` | Filebeat configuration |
| `docker-compose.cluster.yml` | Full stack Docker Compose |
| `grafana/provisioning/` | Grafana datasource & dashboard provisioning |
| `scripts/elk-management.sh` | ELK management script |
| `scripts/generate-dashboard-load.sh` | Generate test data for dashboards |
| `logs/` | Local log files |

### Port Reference

| Port | Service |
|------|---------|
| 9200 | Elasticsearch |
| 3000 | Grafana |
| 9080-9082 | HyperCache HTTP API |
| 8080-8082 | HyperCache RESP protocol |
| 7946-7948 | Gossip cluster communication |
