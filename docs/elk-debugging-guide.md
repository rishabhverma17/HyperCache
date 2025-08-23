# ELK Stack Debugging Guide

This guide covers debugging commands for Filebeat, Elasticsearch, and### Complex query with JSON
curl -X GET "localhost:9200/hypercache-docker-logs-*/_search" -H 'Content-Type: application/json' -d'
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

# Search for Cuckoo filter operations (DEBUG level)
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&sort=@timestamp:desc&size=10&pretty"

# Search for specific filter actions
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=action:negative_lookup&sort=@timestamp:desc&size=5&pretty"
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=action:positive_lookup&sort=@timestamp:desc&size=5&pretty"ration, plus instructions for starting fresh.

## Table of Contents
- [Filebeat Debugging](#filebeat-debugging)
- [Elasticsearch Debugging](#elasticsearch-debugging)
- [Grafana Debugging](#grafana-debugging)
- [Starting Fresh](#starting-fresh)
- [Common Issues & Solutions](#common-issues--solutions)

## Filebeat Debugging

### Check Filebeat Status
```bash
# Check if Filebeat container is running
docker ps | grep filebeat

# Check Filebeat logs
docker logs hypercache-filebeat

# Check Filebeat internal logs (detailed)
docker exec hypercache-filebeat tail -f /var/log/filebeat/filebeat-$(date +%Y%m%d).ndjson

# Check Filebeat configuration
docker exec hypercache-filebeat cat /usr/share/filebeat/filebeat.yml
```

### Monitor Filebeat Registry
```bash
# Check what files Filebeat is monitoring
docker exec hypercache-filebeat ls -la /usr/share/filebeat/data/registry/filebeat/

# View registry details
docker exec hypercache-filebeat cat /usr/share/filebeat/data/registry/filebeat/meta.json
```

### Test Filebeat Connection to Elasticsearch
```bash
# Test connection from inside Filebeat container
docker exec hypercache-filebeat curl -X GET "hypercache-elasticsearch:9200/_cluster/health?pretty"

# Check if Filebeat can reach Elasticsearch
docker exec hypercache-filebeat filebeat test output
```

## Elasticsearch Debugging

### Basic Health Checks
```bash
# Check Elasticsearch cluster health
curl -X GET "localhost:9200/_cluster/health?pretty"

# Check node info
curl -X GET "localhost:9200/_nodes?pretty"

# Check indices
curl -X GET "localhost:9200/_cat/indices?v"
```

### Index Management
```bash
# List all indices with HyperCache logs
curl -X GET "localhost:9200/_cat/indices/hypercache*?v"

# Check index mapping
curl -X GET "localhost:9200/hypercache-docker-logs-*/_mapping?pretty"

# Check index settings
curl -X GET "localhost:9200/hypercache-docker-logs-*/_settings?pretty"
```

### Data Stream Operations
```bash
# List data streams
curl -X GET "localhost:9200/_data_stream?pretty"

# Get specific data stream info
curl -X GET "localhost:9200/_data_stream/hypercache-docker-logs?pretty"

# Delete data stream (CAREFUL: This deletes all data!)
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
```

### Search and Query Logs
```bash
# Count total documents
curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value'

# Search recent logs
curl -s "localhost:9200/hypercache-docker-logs-*/_search?sort=@timestamp:desc&size=10&pretty"

# Search by specific field
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=node_id:node-1&pretty&size=5"

# Search by log level
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=level:ERROR&pretty&size=5"

# Complex query with JSON
curl -X GET "localhost:9200/hypercache-docker-logs-*/_search" -H 'Content-Type: application/json' -d'
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

### Document Analysis
```bash
# Get sample document structure
curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=1&pretty" | jq '.hits.hits[0]._source'

# Check field mappings that might cause conflicts
curl -s "localhost:9200/hypercache-docker-logs-*/_mapping" | jq '.[] | keys'
```

### Index Template Management
```bash
# List index templates
curl -X GET "localhost:9200/_index_template?pretty"

# Get specific template
curl -X GET "localhost:9200/_index_template/hypercache-logs?pretty"

# Delete problematic template (if needed)
curl -X DELETE "localhost:9200/_index_template/hypercache-logs"
```

## Grafana Debugging

### Check Grafana Status
```bash
# Check if Grafana is running
docker ps | grep grafana

# Check Grafana logs
docker logs hypercache-grafana

# Access Grafana
echo "Grafana URL: http://localhost:3000"
echo "Default credentials: admin/admin123"
```

### Datasource Testing
```bash
# Test Elasticsearch connection from Grafana container
docker exec hypercache-grafana curl -X GET "hypercache-elasticsearch:9200/_cluster/health?pretty"

# Check Grafana datasource API
curl -u admin:admin123 "localhost:3000/api/datasources"

# Test specific datasource
curl -u admin:admin123 "localhost:3000/api/datasources/proxy/1/_cluster/health"
```

### Grafana Configuration
```bash
# Check Grafana configuration
docker exec hypercache-grafana cat /etc/grafana/provisioning/datasources/elasticsearch.yml

# Restart Grafana to reload config
docker restart hypercache-grafana
```

## Starting Fresh

### Complete Clean Restart
```bash
# Stop all containers
docker-compose -f docker-compose.cluster.yml down

# Remove all volumes (DESTRUCTIVE - deletes all data!)
docker volume rm hypercache_es-data hypercache_grafana-data

# Remove any orphaned containers
docker-compose -f docker-compose.cluster.yml down --volumes --remove-orphans

# Clean up any remaining data directories
sudo rm -rf data/elasticsearch/* logs/* data/node-*/*

# Start fresh
docker-compose -f docker-compose.cluster.yml up -d
```

### Selective Clean Restart

#### Clear Only HyperCache Data (Keep ELK)
```bash
# Stop only HyperCache nodes
docker stop hypercache-node-1 hypercache-node-2 hypercache-node-3

# Clear HyperCache persistence
rm -rf data/node-*/*
rm -f logs/*.log

# Restart HyperCache nodes
docker-compose -f docker-compose.cluster.yml up -d hypercache-node-1 hypercache-node-2 hypercache-node-3
```

#### Clear Only Elasticsearch Data
```bash
# Stop Elasticsearch and dependent services
docker stop hypercache-elasticsearch hypercache-kibana hypercache-grafana hypercache-filebeat

# Clear Elasticsearch data
docker volume rm hypercache_es-data

# Recreate and restart
docker-compose -f docker-compose.cluster.yml up -d hypercache-elasticsearch
# Wait for ES to start, then start others
docker-compose -f docker-compose.cluster.yml up -d hypercache-filebeat hypercache-grafana
```

#### Clear Only Logs (Keep Data)
```bash
# Delete Elasticsearch indices/data streams
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
curl -X DELETE "localhost:9200/hypercache-docker-logs-*"

# Clear local log files
rm -f logs/*.log

# Restart Filebeat to begin fresh log collection
docker restart hypercache-filebeat
```

## Common Issues & Solutions

### Issue: Filebeat Not Sending Logs
**Symptoms:** No documents in Elasticsearch, Filebeat shows no errors
```bash
# Check file permissions
docker exec hypercache-filebeat ls -la /var/lib/docker/containers/

# Check registry state
docker exec hypercache-filebeat cat /usr/share/filebeat/data/registry/filebeat/meta.json

# Solution: Restart with registry reset
docker stop hypercache-filebeat
docker volume rm hypercache_filebeat-data
docker-compose -f docker-compose.cluster.yml up -d hypercache-filebeat
```

### Issue: Elasticsearch Field Mapping Conflicts
**Symptoms:** `document_parsing_exception`, "tried to parse field [service] as object"
```bash
# Check current mapping
curl -s "localhost:9200/hypercache-docker-logs-*/_mapping" | jq '.[].mappings.properties.service'

# Solution: Delete conflicting data streams and update Filebeat config
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
# Update filebeat-docker.yml to rename conflicting fields
docker restart hypercache-filebeat
```

### Issue: Grafana Can't Connect to Elasticsearch
**Symptoms:** "Bad Gateway" or connection errors in Grafana
```bash
# Test network connectivity
docker exec hypercache-grafana curl hypercache-elasticsearch:9200

# Check Elasticsearch is ready
curl localhost:9200/_cluster/health

# Solution: Wait for ES to be fully ready, check datasource config
docker logs hypercache-elasticsearch | grep "started"
```

### Issue: Missing Cuckoo Filter Logs
**Symptoms:** No Cuckoo filter logs visible, only cache operations
**Cause:** Cuckoo filter logs are at DEBUG level, not INFO level
```bash
# Enable DEBUG level logging in node config
# Edit configs/node1-config.yaml:
logging:
  level: "debug"  # Change from "info" to "debug"

# Test Cuckoo filter operations
curl -X PUT http://localhost:9080/api/cache/test-key -H "Content-Type: application/json" -d '{"value":"test"}'
curl http://localhost:9080/api/cache/non-existent-key  # Should trigger negative_lookup
curl http://localhost:9080/api/cache/test-key          # Should trigger positive_lookup

# Check for filter logs
docker logs hypercache-node1 --tail 20 | grep -E "(negative_lookup|positive_lookup|filter)"

# Search in Elasticsearch
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&pretty"
```

### Issue: Old Logs Not Appearing
**Symptoms:** Only recent logs visible, missing historical data
```bash
# Check Filebeat scan frequency
docker exec hypercache-filebeat cat /usr/share/filebeat/filebeat.yml | grep scan_frequency

# Check what files are being monitored
docker exec hypercache-filebeat filebeat export config | grep -A 10 "paths:"

# Solution: Configure proper log rotation and retention
```

## Monitoring Commands

### Real-time Log Flow
```bash
# Watch logs being ingested in real-time
watch -n 2 'curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq ".hits.total.value"'

# Monitor Filebeat processing
docker logs hypercache-filebeat --follow

# Monitor Elasticsearch indexing
curl -s "localhost:9200/_cat/indices/hypercache*?v&s=docs.count:desc"
```

### Performance Monitoring
```bash
# Check Elasticsearch performance
curl "localhost:9200/_nodes/stats?pretty&human"

# Check index size and document counts
curl "localhost:9200/_cat/indices/hypercache*?v&h=index,docs.count,store.size&s=docs.count:desc"

# Monitor Filebeat harvest stats
docker exec hypercache-filebeat curl localhost:5066/stats
```

## Quick Reference Commands

```bash
# Essential debugging one-liners
alias es-health="curl localhost:9200/_cluster/health?pretty"
alias es-indices="curl localhost:9200/_cat/indices/hypercache*?v"
alias log-count="curl -s localhost:9200/hypercache-docker-logs-*/_search?size=0 | jq .hits.total.value"
alias filebeat-logs="docker logs hypercache-filebeat --tail 50"
alias fresh-start="docker-compose -f docker-compose.cluster.yml down --volumes && docker-compose -f docker-compose.cluster.yml up -d"
```
