# ELK Stack Quick Reference

## Quick Commands

### Start Fresh (No Data Retention)
```bash
# Complete wipe and restart
docker-compose -f docker-compose.cluster.yml down --volumes
docker-compose -f docker-compose.cluster.yml up -d

# OR use the management script
./scripts/elk-management.sh fresh-start
```

### Start Fresh (Keep HyperCache Data, Clear Logs Only)
```bash
# Clear only Elasticsearch logs
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
rm -f logs/*.log
docker restart hypercache-filebeat

# OR use the management script
./scripts/elk-management.sh clean-logs
```

### Essential Health Checks
```bash
# Quick status check
./scripts/elk-management.sh status

# Full health check
./scripts/elk-management.sh health

# Count current logs
curl -s "localhost:9200/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value'
```

### Common Debugging Commands

#### Cuckoo Filter Debug (DEBUG level required)
```bash
# Generate Cuckoo filter activity
curl -X PUT http://localhost:9080/api/cache/test-key -H "Content-Type: application/json" -d '{"value":"test"}'
curl http://localhost:9080/api/cache/missing-key  # Triggers negative_lookup
curl http://localhost:9080/api/cache/test-key     # Triggers positive_lookup

# Check filter logs (requires DEBUG level logging)
docker logs hypercache-node1 --tail 30 | grep -E "(filter|negative_lookup|positive_lookup)"

# Search filter logs in Elasticsearch
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&size=10&pretty"
```

#### Filebeat Issues
```bash
# Check Filebeat logs
docker logs hypercache-filebeat --tail 50

# Check internal Filebeat logs
docker exec hypercache-filebeat tail /var/log/filebeat/filebeat-$(date +%Y%m%d).ndjson

# Test Filebeat to ES connection
docker exec hypercache-filebeat curl hypercache-elasticsearch:9200/_cluster/health
```

#### Elasticsearch Issues
```bash
# Check ES health
curl localhost:9200/_cluster/health?pretty

# List indices
curl localhost:9200/_cat/indices/hypercache*?v

# Delete problematic data streams
curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"
```

#### Grafana Issues
```bash
# Check Grafana logs
docker logs hypercache-grafana --tail 50

# Test datasource connection
curl -u admin:admin123 "localhost:3000/api/datasources"

# Access Grafana
open http://localhost:3000  # Credentials: admin/admin123
```

## File Locations

- **Documentation**: `docs/elk-debugging-guide.md`
- **Management Script**: `scripts/elk-management.sh`
- **Filebeat Config**: `filebeat-docker.yml`
- **Docker Compose**: `docker-compose.cluster.yml`
- **Log Files**: `logs/` directory
- **Data Persistence**: `data/` directory

## Port Reference

- **Elasticsearch**: 9200
- **Grafana**: 3000
- **HyperCache HTTP**: 9080-9082
- **HyperCache RESP**: 8080-8082

## Emergency Procedures

### Complete Reset
```bash
docker-compose -f docker-compose.cluster.yml down --volumes --remove-orphans
sudo rm -rf data/* logs/*
docker-compose -f docker-compose.cluster.yml up -d
```

### Logs Not Appearing
1. Check Filebeat: `docker logs hypercache-filebeat`
2. Check ES indices: `curl localhost:9200/_cat/indices/hypercache*?v`
3. Clear and restart: `./scripts/elk-management.sh clean-logs`

### Field Mapping Conflicts
1. Check error: `docker exec hypercache-filebeat tail /var/log/filebeat/filebeat-*.ndjson`
2. Delete data stream: `curl -X DELETE "localhost:9200/_data_stream/hypercache-docker-logs"`
3. Update `filebeat-docker.yml` field names
4. Restart: `docker restart hypercache-filebeat`
