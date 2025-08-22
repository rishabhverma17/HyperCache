# HyperCache Logs - Quick Access Guide

## 🔍 View Logs in Elasticsearch

### 1. Check Available Indices
```bash
curl -X GET "http://localhost:9200/_cat/indices?v"
```

### 2. Search Recent Logs (Last 15 minutes)
```bash
curl -X GET "http://localhost:9200/hypercache-*/_search?pretty" -H 'Content-Type: application/json' -d'
{
  "query": {
    "range": {
      "@timestamp": {
        "gte": "now-15m"
      }
    }
  },
  "sort": [
    {"@timestamp": {"order": "desc"}}
  ],
  "size": 20
}'
```

### 3. Search by Node ID
```bash
curl -X GET "http://localhost:9200/hypercache-*/_search?pretty" -H 'Content-Type: application/json' -d'
{
  "query": {
    "match": {
      "node_id": "node-1"
    }
  },
  "sort": [{"@timestamp": {"order": "desc"}}],
  "size": 10
}'
```

### 4. Search by Correlation ID (for request tracing)
```bash
curl -X GET "http://localhost:9200/hypercache-*/_search?pretty" -H 'Content-Type: application/json' -d'
{
  "query": {
    "match": {
      "correlation_id": "YOUR_CORRELATION_ID_HERE"
    }
  },
  "sort": [{"@timestamp": {"order": "desc"}}]
}'
```

## 📊 View Logs in Grafana

### 1. Access Grafana
- **URL**: http://localhost:3000
- **Username**: admin
- **Password**: admin123

### 2. Navigate to Explore
1. Click **Explore** in the left sidebar
2. Select **HyperCache Logs** as data source
3. Use these queries:

### 3. Basic Log Queries

**All recent logs:**
```
{service="hypercache"}
```

**Filter by log level:**
```
{service="hypercache"} | json | level="ERROR"
```

**Filter by node:**
```
{service="hypercache"} | json | node_id="node-1"
```

**Filter by component:**
```
{service="hypercache"} | json | component="cache"
```

**Search for specific operations:**
```
{service="hypercache"} | json | action="put_request"
```

### 4. Advanced Queries

**Trace a request by correlation ID:**
```
{service="hypercache"} | json | correlation_id="9eaf6608-1f28-4f9b-a03e-16dff296d49b"
```

**Count operations by node:**
```
sum by (node_id) (count_over_time({service="hypercache"} | json | action="put_request"[5m]))
```

**Error rate by component:**
```
sum by (component) (count_over_time({service="hypercache"} | json | level="ERROR"[5m]))
```

## 🧪 Generate Test Logs

To generate some logs for testing, run these commands:

```bash
# Test PUT operation
curl -X PUT -H "Content-Type: application/json" \
  -d '{"value":"test-value","ttl_hours":1.0}' \
  http://localhost:9080/api/cache/test-key

# Test GET operation  
curl -X GET http://localhost:9080/api/cache/test-key

# Test DELETE operation
curl -X DELETE http://localhost:9080/api/cache/test-key

# Test batch operations
curl -X POST -H "Content-Type: application/json" \
  -d '{"keys":["test-key","nonexistent"]}' \
  http://localhost:9080/api/cache/batch/get
```

## 🎯 What to Look For

### In Elasticsearch:
- Index pattern: `hypercache-YYYY.MM.DD`
- JSON structure with correlation IDs
- Structured fields: `@timestamp`, `level`, `node_id`, `component`, `action`

### In Grafana:
- Real-time log streaming
- Log level filtering
- Correlation ID tracing across nodes
- Component-based filtering

## 🔧 Troubleshooting

If no logs appear:

1. **Check Filebeat status:**
```bash
docker logs hypercache-filebeat
```

2. **Check if log files exist:**
```bash
tail -f logs/node-*.log
```

3. **Restart logging stack:**
```bash
./scripts/manage-logging.sh restart
```

4. **Check services:**
```bash
./scripts/manage-logging.sh status
```
