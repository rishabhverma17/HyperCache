# HyperCache Docker Guide

Everything you need to deploy, test, and manage HyperCache with Docker.

---

## 1. Quick Start

### 30-Second Deployment

```bash
# Build the Docker image
docker build -t hypercache/hypercache:latest .

# Start the complete cluster with monitoring
docker-compose -f docker-compose.cluster.yml up -d

# Test basic functionality
curl -X PUT http://localhost:9080/api/cache/test \
  -H "Content-Type: application/json" \
  -d '{"value":"hello docker","ttl_hours":1}'

curl http://localhost:9081/api/cache/test
```

### Access Points

| Service | URL | Notes |
|---------|-----|-------|
| Node 1 HTTP | http://localhost:9080 | RESP: `localhost:8080` |
| Node 2 HTTP | http://localhost:9081 | RESP: `localhost:8081` |
| Node 3 HTTP | http://localhost:9082 | RESP: `localhost:8082` |
| Grafana | http://localhost:3000 | Credentials: `admin/admin123` |
| Elasticsearch | http://localhost:9200 | |

### Prerequisites

```bash
docker --version           # Docker 20.10+ required
docker-compose --version   # Docker Compose V2+ required

# Verify ports are available
lsof -i :8080,:8081,:8082,:9080,:9081,:9082,:7946,:7947,:7948,:3000,:9200
```

- At least 4GB RAM available
- 2+ CPU cores recommended
- 10GB+ disk space for logs and persistence

---

## 2. Building the Docker Image

### Multi-Stage Build

The Dockerfile uses a multi-stage build for optimal size and security:

```dockerfile
FROM golang:1.23.2-alpine AS builder
# ... build process ...
FROM alpine:3.18  # Final minimal runtime
```

**Image specs:**
- Final size: ~15MB (statically linked binary)
- Multi-arch: amd64, arm64
- Non-root user (UID 1000)
- Health checks included

### Build Commands

```bash
# Standard build
docker build -t hypercache/hypercache:latest .

# Check image size
docker images | grep hypercache

# Multi-arch build
docker buildx build --platform linux/amd64,linux/arm64 \
  -t hypercache/hypercache:latest .
```

### Deploy to Docker Hub

```bash
export DOCKER_USERNAME=your-username
export DOCKER_PASSWORD=your-password

# Full deploy pipeline
./scripts/docker-deploy.sh deploy

# Or step-by-step
./scripts/docker-deploy.sh build
./scripts/docker-deploy.sh start
./scripts/docker-deploy.sh test
```

### Available Tags

```bash
docker pull hypercache/hypercache:latest        # Latest stable
docker pull hypercache/hypercache:v1.x.x        # Version tags
docker pull hypercache/hypercache:main-{commit} # Development builds
```

---

## 3. Docker Compose Deployment

### Full Cluster with Monitoring (Recommended)

```bash
# Build and start (3 nodes + ELK monitoring)
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d

# Monitor startup
docker-compose -f docker-compose.cluster.yml logs -f

# Check all containers
docker-compose -f docker-compose.cluster.yml ps
```

Expected containers:

| Container | Image | Purpose |
|-----------|-------|---------|
| hypercache-node1 | hypercache/hypercache | Cache node |
| hypercache-node2 | hypercache/hypercache | Cache node |
| hypercache-node3 | hypercache/hypercache | Cache node |
| hypercache-elasticsearch | elasticsearch:8.11.0 | Log storage |
| hypercache-filebeat | filebeat:8.11.0 | Log shipping |
| hypercache-grafana | grafana:10.2.0 | Dashboards |

### Basic Cluster (No Monitoring)

```bash
docker-compose -f docker-compose.cluster.yml up -d \
  hypercache-node1 hypercache-node2 hypercache-node3
```

### Individual Containers (Manual Control)

Use this when you need fine-grained control over each node's configuration.

```bash
# Create network
docker network create --driver bridge --subnet=172.20.0.0/16 hypercache-network

# Start Node 1 (bootstrap)
docker run -d --name hypercache-node1 \
  --network hypercache-network \
  --hostname hypercache-node1 \
  -p 8080:8080 -p 9080:9080 -p 7946:7946 \
  -v $(pwd)/configs/docker/node1-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node1_data:/data \
  -v hypercache_logs:/app/logs \
  -e NODE_ID=node-1 \
  -e CLUSTER_SEEDS=hypercache-node1:7946 \
  hypercache/hypercache:latest

# Start Node 2
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  --hostname hypercache-node2 \
  -p 8081:8080 -p 9081:9080 -p 7947:7946 \
  -v $(pwd)/configs/docker/node2-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node2_data:/data \
  -e NODE_ID=node-2 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946 \
  hypercache/hypercache:latest

# Start Node 3
docker run -d --name hypercache-node3 \
  --network hypercache-network \
  --hostname hypercache-node3 \
  -p 8082:8080 -p 9082:9080 -p 7948:7946 \
  -v $(pwd)/configs/docker/node3-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node3_data:/data \
  -e NODE_ID=node-3 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946 \
  hypercache/hypercache:latest
```

### Docker Swarm (Production)

```bash
docker swarm init
docker stack deploy -c docker-compose.cluster.yml hypercache
docker service scale hypercache_hypercache-node1=5
```

### Kubernetes Deployment

```bash
kubectl apply -f k8s/hypercache-cluster.yaml
kubectl scale statefulset hypercache --replicas=5 -n hypercache
kubectl get pods -n hypercache -w
```

<details>
<summary>Full Kubernetes Manifests</summary>

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: hypercache
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: hypercache-config
  namespace: hypercache
data:
  hypercache.yaml: |
    node:
      id: "${NODE_ID}"
      region: "kubernetes"
    network:
      http_port: 9080
      resp_port: 8080
      gossip:
        port: 7946
        seeds: ["hypercache-0.hypercache-headless:7946"]
    cache:
      stores:
        - name: "main"
          type: "basic"
          max_memory: "512MB"
    logging:
      level: "info"
      format: "json"
      outputs: ["stdout"]
    persistence:
      enabled: true
      directory: "/data"
---
apiVersion: v1
kind: Service
metadata:
  name: hypercache-headless
  namespace: hypercache
spec:
  clusterIP: None
  selector:
    app: hypercache
  ports:
  - name: http
    port: 9080
  - name: resp
    port: 8080
  - name: gossip
    port: 7946
---
apiVersion: v1
kind: Service
metadata:
  name: hypercache-service
  namespace: hypercache
spec:
  selector:
    app: hypercache
  ports:
  - name: http
    port: 9080
  - name: resp
    port: 8080
  type: LoadBalancer
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: hypercache
  namespace: hypercache
spec:
  serviceName: hypercache-headless
  replicas: 3
  selector:
    matchLabels:
      app: hypercache
  template:
    metadata:
      labels:
        app: hypercache
    spec:
      containers:
      - name: hypercache
        image: hypercache/hypercache:latest
        ports:
        - containerPort: 8080
          name: resp
        - containerPort: 9080
          name: http
        - containerPort: 7946
          name: gossip
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CLUSTER_SEEDS
          value: "hypercache-0.hypercache-headless:7946,hypercache-1.hypercache-headless:7946,hypercache-2.hypercache-headless:7946"
        volumeMounts:
        - name: data
          mountPath: /data
        - name: config
          mountPath: /config
        livenessProbe:
          httpGet:
            path: /health
            port: 9080
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 9080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: hypercache-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

</details>

---

## 4. Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NODE_ID` | Unique node identifier | auto-generated |
| `CLUSTER_SEEDS` | Comma-separated seed nodes | `localhost:7946` |
| `DATA_DIR` | Data persistence directory | `/data` |
| `LOG_LEVEL` | Logging verbosity | `info` |
| `MAX_MEMORY` | Max memory per store | `256MB` |
| `REPLICATION_FACTOR` | Cluster replication factor | `3` |
| `HTTP_PORT` | HTTP API port | `9080` |
| `RESP_PORT` | Redis protocol port | `8080` |
| `GOSSIP_PORT` | Cluster communication port | `7946` |

### Volume Mounts

```bash
-v hypercache_data:/data                           # Data persistence (required for production)
-v ./custom-config.yaml:/config/hypercache.yaml:ro # Configuration override
-v hypercache_logs:/app/logs                       # Log output
-v ./certs:/certs:ro                               # TLS certificates
```

### Network Ports

| Port Range | Protocol | Purpose |
|------------|----------|---------|
| 8080-8082 | RESP | Redis-compatible protocol |
| 9080-9082 | HTTP | API and health checks |
| 7946-7948 | TCP/UDP | Gossip cluster communication |
| 3000 | HTTP | Grafana dashboard |
| 9200 | HTTP | Elasticsearch |

### Configuration File Example

```yaml
node:
  id: "node-1"
  region: "us-east-1"

network:
  http_port: 9080
  resp_port: 8080
  gossip:
    port: 7946
    seeds: ["hypercache-node1:7946"]

cache:
  stores:
    - name: "main"
      type: "basic"
      max_memory: "512MB"
      cuckoo_filter:
        enabled: true
        capacity: 1000000

logging:
  level: "info"
  format: "json"
  outputs: ["stdout"]

persistence:
  enabled: true
  directory: "/data"
```

### Resource Limits (Production)

```yaml
# docker-compose.yml
services:
  hypercache-node1:
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 4G
        reservations:
          cpus: '0.5'
          memory: 1G
    ulimits:
      nofile:
        soft: 65536
        hard: 65536
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "10"
    restart: unless-stopped
    stop_grace_period: 30s
```

### Security Configuration

```yaml
services:
  hypercache-node1:
    security_opt:
      - no-new-privileges:true
    read_only: true
    tmpfs:
      - /tmp:rw,noexec,nosuid,size=100m
    user: "1000:1000"
    cap_drop:
      - ALL
```

---

## 5. Testing / Verifying

### Health Checks

```bash
# Container-level health
docker ps --format "table {{.Names}}\t{{.Status}}"

# Application health (all nodes)
curl http://localhost:9080/health
curl http://localhost:9081/health
curl http://localhost:9082/health

# Cluster membership (should show 3 members)
curl http://localhost:9080/api/cluster/members | jq '.'
curl http://localhost:9080/api/cluster/status | jq '.'
```

Expected health response:
```json
{"status":"healthy","node_id":"node-1","cluster_size":3,"timestamp":"..."}
```

### HTTP API Cache Operations

```bash
# PUT on node 1
curl -X PUT http://localhost:9080/api/cache/test-key \
  -H "Content-Type: application/json" \
  -d '{"value":"hello from docker","ttl_hours":1}'

# GET from node 2 (cross-node)
curl http://localhost:9081/api/cache/test-key

# DELETE from node 3
curl -X DELETE http://localhost:9082/api/cache/test-key

# Verify deletion from node 1
curl http://localhost:9080/api/cache/test-key
# Expected: {"success":false,"error":"key not found"}
```

### Redis Protocol Testing

```bash
redis-cli -p 8080 ping                    # PONG
redis-cli -p 8080 set docker-key "value"  # OK
redis-cli -p 8081 get docker-key          # Cross-node read
redis-cli -p 8082 del docker-key          # (integer) 1
redis-cli -p 8080 get docker-key          # (nil)
```

### Load Testing

```bash
# Concurrent write test
for i in {1..100}; do
  curl -s -X PUT http://localhost:9080/api/cache/load-$i \
    -H "Content-Type: application/json" \
    -d '{"value":"load test data","ttl_hours":1}' &
  if (( i % 10 == 0 )); then wait; fi
done
wait

# Check data distribution
curl http://localhost:9080/api/stats | jq '.cache_stats'
curl http://localhost:9081/api/stats | jq '.cache_stats'
curl http://localhost:9082/api/stats | jq '.cache_stats'
```

### Failure Recovery Testing

```bash
# Store data
curl -X PUT http://localhost:9080/api/cache/recovery-test \
  -d '{"value":"failure test","ttl_hours":1}'

# Stop a node
docker-compose -f docker-compose.cluster.yml stop hypercache-node2

# Verify data is still accessible
curl http://localhost:9080/api/cache/recovery-test
curl http://localhost:9082/api/cache/recovery-test

# Check cluster (should show 2 members)
curl http://localhost:9080/api/cluster/members

# Restart the node
docker-compose -f docker-compose.cluster.yml start hypercache-node2
sleep 15

# Verify rejoin
curl http://localhost:9081/health
curl http://localhost:9081/api/cache/recovery-test
```

### Full System Test Workflow

```bash
# 1. Build and start
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d

# 2. Wait for cluster formation
sleep 30

# 3. Test all operations
curl -X PUT http://localhost:9080/api/cache/e2e-test \
  -d '{"value":"full system test","ttl_hours":1}'
curl http://localhost:9081/api/cache/e2e-test
curl http://localhost:9082/api/cache/e2e-test

# 4. Check monitoring
open http://localhost:3000

# 5. Cleanup
docker-compose -f docker-compose.cluster.yml down -v
```

---

## 6. Common Commands Reference

### Lifecycle Management

```bash
# Start / Stop / Restart
docker-compose -f docker-compose.cluster.yml start
docker-compose -f docker-compose.cluster.yml stop
docker-compose -f docker-compose.cluster.yml restart

# Selective service management
docker-compose -f docker-compose.cluster.yml stop hypercache-node2
docker-compose -f docker-compose.cluster.yml start hypercache-node2

# Complete removal
docker-compose -f docker-compose.cluster.yml down
docker-compose -f docker-compose.cluster.yml down -v  # Also remove volumes
```

### Container Inspection

```bash
docker ps --filter "name=hypercache"         # List containers
docker logs hypercache-node1                  # View logs
docker logs hypercache-node1 --since 10m     # Recent logs
docker logs -f hypercache-node1              # Follow logs
docker stats hypercache-node{1,2,3}          # Resource usage
docker exec -it hypercache-node1 sh          # Shell access
docker exec hypercache-node1 cat /config/hypercache.yaml  # View config
```

### Network

```bash
docker network ls | grep hypercache
docker exec hypercache-node1 ping hypercache-node2
docker exec hypercache-node1 netstat -tulpn | grep 7946
docker network inspect hypercache-cluster_hypercache-cluster
```

### Data Management

```bash
# Backup a node's data
docker run --rm -v hypercache_node1_data:/source -v $(pwd):/backup \
  alpine tar czf /backup/node1-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /source .

# Restore data
docker run --rm -v hypercache_node1_data:/target -v $(pwd):/backup \
  alpine tar xzf /backup/node1-backup.tar.gz -C /target

# Volume info
docker volume ls | grep hypercache
docker inspect hypercache-node1 | jq '.[0].Mounts'
```

### Rolling Updates (Zero-Downtime)

```bash
# Update one node at a time
docker-compose -f docker-compose.cluster.yml stop hypercache-node1
docker-compose -f docker-compose.cluster.yml rm -f hypercache-node1
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d hypercache-node1
sleep 30
curl http://localhost:9080/health
# Repeat for remaining nodes
```

### Scripts

```bash
# Docker deployment
./scripts/docker-deploy.sh build     # Build image
./scripts/docker-deploy.sh start     # Start cluster
./scripts/docker-deploy.sh test      # Run tests
./scripts/docker-deploy.sh stop      # Stop cluster
./scripts/docker-deploy.sh clean     # Clean up
./scripts/docker-deploy.sh logs      # View logs
./scripts/docker-deploy.sh status    # Check status
./scripts/docker-deploy.sh deploy    # Full pipeline (build + push to Docker Hub)

# ELK management
./scripts/elk-management.sh status
./scripts/elk-management.sh health
./scripts/elk-management.sh clean-logs
./scripts/elk-management.sh fresh-start
```

---

## 7. Troubleshooting

### Port Conflicts

```bash
# Find conflicting processes
sudo lsof -i :8080,:9080,:7946

# Use alternative ports
docker run -p 8090:8080 -p 9090:9080 -p 7950:7946 hypercache/hypercache:latest
```

### Container Won't Start

```bash
# Check logs
docker logs hypercache-node1

# Permission denied for logs directory
docker volume create hypercache_logs
docker run --rm -v hypercache_logs:/app/logs alpine sh -c "chmod 755 /app/logs && chown 1000:1000 /app/logs"

# Config file not found
docker exec hypercache-node1 ls -la /config/

# Rebuild cleanly
docker-compose -f docker-compose.cluster.yml down
docker build --no-cache -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d
```

### Cluster Formation Issues

```bash
# Check gossip connectivity
docker exec hypercache-node1 netstat -tlnp | grep 7946

# Verify DNS resolution
docker exec hypercache-node2 nslookup hypercache-node1

# Check cluster status
curl http://localhost:9080/api/cluster/members

# Force rejoin
docker restart hypercache-node2
sleep 30
curl http://localhost:9081/api/cluster/members
```

### Persistence Issues

```bash
# Check volume mounts
docker inspect hypercache-node1 | jq '.[].Mounts'

# Verify permissions
docker exec hypercache-node1 ls -la /data

# Check disk space
docker exec hypercache-node1 df -h /data
```

### Performance Issues

```bash
# Resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Application metrics
curl http://localhost:9080/api/stats | jq '.'

# Network latency
docker exec hypercache-node1 ping hypercache-node2
```

### Monitoring Stack Issues

```bash
# Elasticsearch
curl http://localhost:9200/_cluster/health
docker logs hypercache-elasticsearch

# Grafana
curl -I http://localhost:3000
docker logs hypercache-grafana

# Filebeat
docker logs hypercache-filebeat
curl http://localhost:9200/_cat/indices/hypercache*?v

# ELK management
./scripts/elk-management.sh health
./scripts/elk-management.sh clean-logs
```

### Debugging Checklist

```bash
# 1. Container health
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# 2. Application health
for port in 9080 9081 9082; do
  echo "Checking localhost:$port"
  curl -s http://localhost:$port/health | jq '.'
done

# 3. Network connectivity
docker exec hypercache-node1 ping -c 3 hypercache-node2

# 4. Cluster status
curl -s http://localhost:9080/api/cluster/members | jq '. | length'

# 5. Resource usage
docker stats --no-stream hypercache-node1 hypercache-node2 hypercache-node3

# 6. Log analysis
docker-compose -f docker-compose.cluster.yml logs --tail=50 | grep -i error
```

### Emergency Recovery

```bash
# Complete system reset
docker-compose -f docker-compose.cluster.yml down --volumes --remove-orphans
sudo rm -rf data/* logs/*
docker system prune -f
docker-compose -f docker-compose.cluster.yml up -d

# Clear logs only (keep cache data)
curl -X DELETE "http://localhost:9200/_data_stream/hypercache-docker-logs"
docker restart hypercache-filebeat
rm -f logs/*.log

# Single node recovery
docker-compose -f docker-compose.cluster.yml stop hypercache-node2
docker-compose -f docker-compose.cluster.yml rm -f hypercache-node2
docker volume rm hypercache_node2_data
docker-compose -f docker-compose.cluster.yml up -d hypercache-node2
```
