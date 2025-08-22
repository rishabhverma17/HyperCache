# Logging Infrastructure Setup

## Architecture Overview

```
┌─────────────────┐    ┌──────────┐    ┌──────────────┐    ┌─────────┐
│   HyperCache    │───▶│ Filebeat │───▶│ Elasticsearch │───▶│ Grafana │
│   JSON Logs     │    │          │    │              │    │         │
│ logs/node-*.log │    │ Shipper  │    │ Search &     │    │ Dashbd  │
└─────────────────┘    └──────────┘    │ Analytics    │    └─────────┘
                                       └──────────────┘
```

## 1. Current Log Format (Already Production-Ready)

HyperCache produces structured JSON logs perfect for Elasticsearch:

```json
{
  "@timestamp": "2025-08-21T17:53:16.266861Z",
  "level": "INFO",
  "message": "Cache PUT operation started",
  "correlation_id": "9eaf6608-1f28-4f9b-a03e-16dff296d49b",
  "node_id": "node-1",
  "component": "cache",
  "action": "put_request",
  "fields": {
    "key": "mykey",
    "node_id": "node-1"
  },
  "file": "/path/to/file.go",
  "line": 447,
  "function": "main.function"
}
```

## 2. Elasticsearch Setup

### Option A: Docker Compose (Recommended for Development)

Create `docker-compose.logging.yml`:

```yaml
version: '3.8'
services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.11.0
    environment:
      - discovery.type=single-node
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m"
      - xpack.security.enabled=false
    ports:
      - "9200:9200"
    volumes:
      - elasticsearch_data:/usr/share/elasticsearch/data
    networks:
      - logging

  filebeat:
    image: docker.elastic.co/beats/filebeat:8.11.0
    user: root
    volumes:
      - ./filebeat.yml:/usr/share/filebeat/filebeat.yml:ro
      - ./logs:/var/log/hypercache:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
    depends_on:
      - elasticsearch
    networks:
      - logging

  grafana:
    image: grafana/grafana:10.2.0
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin123
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning
    depends_on:
      - elasticsearch
    networks:
      - logging

volumes:
  elasticsearch_data:
  grafana_data:

networks:
  logging:
    driver: bridge
```

### Filebeat Configuration

Create `filebeat.yml`:

```yaml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/hypercache/node-*.log
  json.keys_under_root: true
  json.add_error_key: true
  fields:
    service: hypercache
    environment: production
  fields_under_root: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "hypercache-logs-%{+yyyy.MM.dd}"

setup.template.enabled: true
setup.template.name: "hypercache"
setup.template.pattern: "hypercache-*"

logging.level: info
logging.to_files: true
logging.files:
  path: /var/log/filebeat
  name: filebeat
  keepfiles: 7
  permissions: 0644
```

### Option B: Elasticsearch Cloud (Production)

```bash
# Install Filebeat
curl -L -O https://artifacts.elastic.co/downloads/beats/filebeat/filebeat-8.11.0-darwin-x86_64.tar.gz
tar xzvf filebeat-8.11.0-darwin-x86_64.tar.gz

# Configure for Elastic Cloud
vim filebeat.yml
```

## 3. Grafana Dashboard Setup

### Elasticsearch Data Source

1. Add Elasticsearch data source in Grafana:
   - URL: `http://elasticsearch:9200` (or your Elastic Cloud URL)
   - Index: `hypercache-*`
   - Time field: `@timestamp`

### Pre-built Dashboards

**Dashboard 1: Operations Overview**
- Request rate by node and operation type
- Error rate and response times
- Cache hit/miss ratios
- Top accessed keys

**Dashboard 2: Replication Monitoring**
- Cross-node replication events
- Correlation ID tracing
- Replication lag and success rates
- Event bus activity

**Dashboard 3: Performance Metrics**
- Response time percentiles
- Throughput by endpoint
- Cuckoo filter performance
- Memory and persistence metrics

## 4. Key Log Queries for Grafana

### Request Tracing by Correlation ID
```json
{
  "query": {
    "match": {
      "correlation_id": "9eaf6608-1f28-4f9b-a03e-16dff296d49b"
    }
  }
}
```

### Error Rate by Component
```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"level": "ERROR"}},
        {"range": {"@timestamp": {"gte": "now-1h"}}}
      ]
    }
  },
  "aggs": {
    "by_component": {
      "terms": {"field": "component.keyword"}
    }
  }
}
```

### Replication Events
```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"component": "cluster"}},
        {"match": {"action": "replication"}}
      ]
    }
  }
}
```

## 5. Metrics Integration (Optional)

For metrics (separate from logs), add Prometheus:

```yaml
# Add to docker-compose.logging.yml
  prometheus:
    image: prom/prometheus:v2.45.0
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    networks:
      - logging
```

## 6. Quick Start

```bash
# 1. Create logging stack
docker-compose -f docker-compose.logging.yml up -d

# 2. Start HyperCache cluster
./scripts/build-and-run.sh cluster

# 3. Generate some logs
curl -X PUT -H "Content-Type: application/json" \
  -d '{"value":"test"}' http://localhost:9080/api/cache/testkey

# 4. Access Grafana
open http://localhost:3000
# Login: admin / admin123

# 5. Import HyperCache dashboards
# (Dashboard JSON files will be provided)
```

## 7. Production Considerations

- **Retention**: Configure index lifecycle policies in Elasticsearch
- **Scaling**: Use Elasticsearch cluster for high volume
- **Security**: Enable authentication and TLS
- **Monitoring**: Monitor the logging infrastructure itself
- **Backup**: Regular snapshots of log data
