# HyperCache Complete Docker Guide

> **The Ultimate Docker Documentation** - Everything you need to know about deploying, testing, and managing HyperCache with Docker

## 📑 **Table of Contents**

- [Quick Start](#-quick-start)
- [Docker Image Details](#-docker-image-details)
- [Prerequisites](#-prerequisites)
- [Deployment Methods](#-deployment-methods)
- [Configuration](#️-configuration)
- [Testing Guide](#-testing-guide)
- [Monitoring & Logging](#-monitoring--logging)
- [Operations & Management](#️-operations--management)
- [Security](#-security)
- [Kubernetes Deployment](#️-kubernetes-deployment)
- [Troubleshooting](#-troubleshooting)
- [Performance Tuning](#-performance-tuning)
- [Best Practices](#-best-practices)
- [Scripts & Automation](#-scripts--automation)

---

## 🚀 **Quick Start**

### **Ultra-Fast Deployment (30 seconds)**
```bash
# 1. Clone and navigate to project
cd /path/to/hypercache

# 2. Build Docker image
docker build -t hypercache/hypercache:latest .

# 3. Start complete cluster with monitoring
docker-compose -f docker-compose.cluster.yml up -d

# 4. Test basic functionality
curl -X PUT http://localhost:9080/api/cache/test \
  -H "Content-Type: application/json" \
  -d '{"value":"hello docker","ttl_hours":1}'

curl http://localhost:9081/api/cache/test

# 5. Access monitoring
open http://localhost:3000  # Grafana (admin/admin123)
```

### **Deploy to Docker Hub**
```bash
# Set Docker Hub credentials
export DOCKER_USERNAME=your-username
export DOCKER_PASSWORD=your-password

# Full deployment pipeline
./scripts/docker-deploy.sh deploy
```

### **Access Points**
- **HyperCache Nodes**: 
  - Node 1: http://localhost:9080 (RESP: localhost:8080)
  - Node 2: http://localhost:9081 (RESP: localhost:8081)
  - Node 3: http://localhost:9082 (RESP: localhost:8082)
- **Grafana Dashboard**: http://localhost:3000 (admin/admin123)
- **Elasticsearch**: http://localhost:9200
- **ELK Management**: `./scripts/elk-management.sh status`

---

## 📦 **Docker Image Details**

### **Image Architecture**
```dockerfile
# Multi-stage build for optimal size and security
FROM golang:1.23.2-alpine AS builder
# ... build process ...
FROM alpine:3.18  # Final minimal runtime
```

### **Image Specifications**
- **Base Image**: Alpine 3.18 (minimal attack surface)
- **Final Image Size**: ~15MB (statically linked binary)
- **Architecture Support**: Multi-arch (amd64, arm64)
- **Security Features**: 
  - Non-root user (UID 1000)
  - Minimal dependencies
  - Health checks included
  - Read-only filesystem (except data directory)

### **Available Tags**
```bash
# Public Docker Hub repository
docker pull hypercache/hypercache:latest        # Latest stable
docker pull hypercache/hypercache:v1.x.x        # Version tags
docker pull hypercache/hypercache:main-{commit} # Development builds
```

### **Exposed Ports**
- **8080**: RESP Protocol (Redis-compatible)
- **9080**: HTTP API and health checks
- **7946**: Gossip protocol for cluster communication

---

## 📋 **Prerequisites**

### **System Requirements**
```bash
# Verify Docker is running
docker --version                  # Docker 20.10+ required
docker-compose --version          # Docker Compose V2+ required

# Check available ports (should be free)
lsof -i :8080,:8081,:8082,:9080,:9081,:9082,:7946,:7947,:7948,:3000,:9200

# Resource requirements
# - At least 4GB RAM available
# - 2+ CPU cores recommended
# - 10GB+ disk space for logs and persistence
```

### **Development Environment**
```bash
# Ensure you're in the project root
cd /Users/rishabhverma/Documents/HobbyProjects/Cache

# Verify required files exist
ls -la Dockerfile docker-compose.cluster.yml scripts/docker-deploy.sh

# Check Go version (for building from source)
go version  # Go 1.23+ required
```

### **Common Permission Fixes**
```bash
# If you encounter log directory permission errors
docker volume create hypercache_logs
docker run --rm -v hypercache_logs:/app/logs alpine sh -c "chmod 755 /app/logs && chown 1000:1000 /app/logs"
```

---

## 🚀 **Deployment Methods**

### **Method 1: Docker Compose (Recommended)**

#### **Full Cluster with Monitoring**
```bash
# Build the image locally
docker build -t hypercache/hypercache:latest .

# Start complete stack (3 nodes + ELK monitoring)
docker-compose -f docker-compose.cluster.yml up -d

# Monitor startup progress
docker-compose -f docker-compose.cluster.yml logs -f

# Check all containers are running
docker-compose -f docker-compose.cluster.yml ps
```

**Expected Output:**
```
NAME                     IMAGE                    STATUS
hypercache-node1         hypercache/hypercache    Up (healthy)
hypercache-node2         hypercache/hypercache    Up (healthy) 
hypercache-node3         hypercache/hypercache    Up (healthy)
hypercache-elasticsearch elasticsearch:8.11.0     Up (healthy)
hypercache-filebeat      filebeat:8.11.0          Up
hypercache-grafana       grafana:10.2.0           Up
```

#### **Basic Cluster (No Monitoring)**
```bash
# Start only HyperCache nodes
docker-compose -f docker-compose.cluster.yml up -d hypercache-node1 hypercache-node2 hypercache-node3

# Scale specific services
docker-compose -f docker-compose.cluster.yml up -d --scale hypercache-node2=2
```

### **Method 2: Individual Containers**

#### **Create Custom Network**
```bash
# Create dedicated network for HyperCache
docker network create --driver bridge --subnet=172.20.0.0/16 hypercache-network
```

#### **Start Node 1 (Bootstrap Node)**
```bash
docker run -d --name hypercache-node1 \
  --network hypercache-network \
  --ip 172.20.0.10 \
  -p 8080:8080 -p 9080:9080 -p 7946:7946 \
  -v $(pwd)/configs/docker/node1-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node1_data:/data \
  -v hypercache_logs:/app/logs \
  hypercache/hypercache:latest
```

#### **Start Additional Nodes**
```bash
# Node 2
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  --ip 172.20.0.11 \
  -p 8081:8080 -p 9081:9080 -p 7947:7946 \
  -v $(pwd)/configs/docker/node2-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node2_data:/data \
  hypercache/hypercache:latest

# Node 3
docker run -d --name hypercache-node3 \
  --network hypercache-network \
  --ip 172.20.0.12 \
  -p 8082:8080 -p 9082:9080 -p 7948:7946 \
  -v $(pwd)/configs/docker/node3-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node3_data:/data \
  hypercache/hypercache:latest
```

### **Method 3: Docker Swarm (Production)**
```bash
# Initialize swarm mode
docker swarm init

# Deploy as stack
docker stack deploy -c docker-compose.cluster.yml hypercache

# Scale services
docker service scale hypercache_hypercache-node1=5
```

---

## ⚙️ **Configuration**

### **Environment Variables**
| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NODE_ID` | Unique node identifier | auto-generated | `node-1` |
| `CLUSTER_SEEDS` | Seed nodes for discovery | `localhost:7946` | `node1:7946,node2:7946` |
| `DATA_DIR` | Data persistence directory | `/data` | `/data` |
| `LOG_LEVEL` | Logging verbosity | `info` | `debug`, `info`, `warn`, `error` |
| `MAX_MEMORY` | Max memory per store | `256MB` | `512MB`, `1GB` |
| `REPLICATION_FACTOR` | Cluster replication factor | `3` | `3`, `5` |
| `HTTP_PORT` | HTTP API port | `9080` | `9080` |
| `RESP_PORT` | Redis protocol port | `8080` | `8080` |
| `GOSSIP_PORT` | Cluster communication port | `7946` | `7946` |

### **Volume Mounts**
```bash
# Data persistence (required for production)
-v hypercache_data:/data

# Configuration override
-v ./custom-config.yaml:/config/hypercache.yaml:ro

# Log output (for file-based logging)
-v hypercache_logs:/app/logs

# TLS certificates (if using HTTPS)
-v ./certs:/certs:ro
```

### **Configuration File Structure**
```yaml
# Node-specific config example
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

### **Network Ports Overview**
```bash
# HyperCache Ports
8080-8082   # RESP Protocol (Redis-compatible)
9080-9082   # HTTP API and health checks
7946-7948   # Gossip protocol for cluster communication

# Monitoring Stack Ports
3000        # Grafana dashboard
9200        # Elasticsearch
5601        # Kibana (if enabled)
```

---

## 🧪 **Testing Guide**

### **Health Checks**
```bash
# Container-level health
docker ps --format "table {{.Names}}\t{{.Status}}"

# Application health checks
curl http://localhost:9080/health  # Node 1
curl http://localhost:9081/health  # Node 2
curl http://localhost:9082/health  # Node 3

# Expected response
# {"status":"healthy","node_id":"node-1","cluster_size":3,"timestamp":"..."}

# Cluster membership verification
curl http://localhost:9080/api/cluster/members | jq '.'
curl http://localhost:9080/api/cluster/status | jq '.'
```

### **Basic Cache Operations**
```bash
# HTTP API Testing
# PUT operation on node 1
curl -X PUT http://localhost:9080/api/cache/test-key \
  -H "Content-Type: application/json" \
  -d '{"value":"hello from docker","ttl_hours":1}'

# GET operation from node 2 (cross-node access)
curl http://localhost:9081/api/cache/test-key

# Expected: {"success":true,"data":"hello from docker","ttl":...}

# PUT data on node 2
curl -X PUT http://localhost:9081/api/cache/cross-test \
  -H "Content-Type: application/json" \
  -d '{"value":"distributed data","ttl_hours":2}'

# GET from node 3 (full distribution test)
curl http://localhost:9082/api/cache/cross-test

# DELETE from node 3
curl -X DELETE http://localhost:9082/api/cache/test-key

# Verify deletion from node 1
curl http://localhost:9080/api/cache/test-key
# Expected: {"success":false,"error":"key not found"}
```

### **Redis Protocol Testing**
```bash
# Test Redis compatibility (if redis-cli is installed)
redis-cli -p 8080 ping                    # Expected: PONG
redis-cli -p 8080 set docker-key "value"  # Expected: OK
redis-cli -p 8081 get docker-key          # Cross-node access
redis-cli -p 8082 del docker-key          # Expected: (integer) 1

# Test from different node
redis-cli -p 8080 get docker-key          # Expected: (nil)
```

### **Load Testing**
```bash
# Concurrent operations test
for i in {1..100}; do
  curl -X PUT http://localhost:9080/api/cache/load-$i \
    -H "Content-Type: application/json" \
    -d '{"value":"load test data","ttl_hours":1}' &
  if (( i % 10 == 0 )); then wait; fi
done
wait

# Verify data distribution
curl http://localhost:9080/api/stats | jq '.cache_stats'
curl http://localhost:9081/api/stats | jq '.cache_stats'
curl http://localhost:9082/api/stats | jq '.cache_stats'
```

### **Failure Recovery Testing**
```bash
# Test node failure scenario
curl -X PUT http://localhost:9080/api/cache/recovery-test \
  -d '{"value":"failure test","ttl_hours":1}'

# Stop node 2
docker-compose -f docker-compose.cluster.yml stop hypercache-node2

# Verify data still accessible from nodes 1 and 3
curl http://localhost:9080/api/cache/recovery-test
curl http://localhost:9082/api/cache/recovery-test

# Check cluster status (should show 2 members)
curl http://localhost:9080/api/cluster/members

# Restart node 2
docker-compose -f docker-compose.cluster.yml start hypercache-node2

# Wait for rejoin and verify
sleep 15
curl http://localhost:9081/health
curl http://localhost:9081/api/cache/recovery-test
```

### **Performance Benchmarking**
```bash
# Simple benchmark script
#!/bin/bash
echo "Starting performance benchmark..."

# Warm up
for i in {1..50}; do
  curl -s -X PUT http://localhost:9080/api/cache/warmup-$i \
    -d '{"value":"warmup","ttl_hours":1}' > /dev/null
done

# Benchmark writes
start_time=$(date +%s.%N)
for i in {1..1000}; do
  curl -s -X PUT http://localhost:9080/api/cache/bench-$i \
    -d '{"value":"benchmark data","ttl_hours":1}' > /dev/null
done
end_time=$(date +%s.%N)

write_duration=$(echo "$end_time - $start_time" | bc)
write_ops_per_sec=$(echo "1000 / $write_duration" | bc -l)

echo "Write Operations: 1000 in ${write_duration}s (${write_ops_per_sec} ops/sec)"

# Benchmark reads
start_time=$(date +%s.%N)
for i in {1..1000}; do
  curl -s http://localhost:9081/api/cache/bench-$i > /dev/null
done
end_time=$(date +%s.%N)

read_duration=$(echo "$end_time - $start_time" | bc)
read_ops_per_sec=$(echo "1000 / $read_duration" | bc -l)

echo "Read Operations: 1000 in ${read_duration}s (${read_ops_per_sec} ops/sec)"
```

---

## 🔍 **Monitoring & Logging**

### **Integrated ELK Stack**
The Docker deployment includes comprehensive logging infrastructure:

```bash
# Access Grafana dashboards
open http://localhost:3000
# Login credentials: admin/admin123

# Check Elasticsearch health
curl http://localhost:9200/_cluster/health | jq '.'

# View log indices
curl http://localhost:9200/_cat/indices/hypercache*?v

# Search recent logs
curl -X GET "http://localhost:9200/hypercache-docker-logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": {
      "range": {
        "@timestamp": {"gte": "now-10m"}
      }
    },
    "size": 50,
    "sort": [{"@timestamp": {"order": "desc"}}]
  }' | jq '.hits.hits[]._source'

# Monitor log shipping
docker logs hypercache-filebeat
```

### **Component-Specific Debugging**
```bash
# Cuckoo Filter operations (requires DEBUG level)
curl -X PUT http://localhost:9080/api/cache/filter-test \
  -d '{"value":"test","ttl_hours":1}'

curl http://localhost:9080/api/cache/missing-key  # Triggers negative_lookup
curl http://localhost:9080/api/cache/filter-test  # Triggers positive_lookup

# Search for Cuckoo filter logs
docker logs hypercache-node1 --tail 30 | grep -E "(filter|negative_lookup|positive_lookup)"

# Or search in Elasticsearch
curl -s "localhost:9200/hypercache-docker-logs-*/_search?q=component:filter&size=10&pretty"
```

### **Real-time Monitoring Commands**
```bash
# Container resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Live log streaming
docker-compose -f docker-compose.cluster.yml logs -f

# Specific node logs
docker-compose -f docker-compose.cluster.yml logs -f hypercache-node1

# ELK Stack monitoring
./scripts/elk-management.sh status
./scripts/elk-management.sh health
./scripts/elk-management.sh logs-count
```

### **Performance Monitoring Dashboards**
Built-in Grafana dashboards provide:
- **Real-time Metrics**: Request rates, latencies, error rates
- **Cluster Health**: Node status, replication lag, membership changes  
- **Resource Usage**: Memory, CPU, storage utilization per node
- **Cache Performance**: Hit/miss ratios, eviction rates, TTL effectiveness
- **Network Stats**: Gossip protocol health, message rates
- **Error Analysis**: Error tracking, failure patterns, recovery times

---

## 🛠️ **Operations & Management**

### **Container Lifecycle Management**
```bash
# Start/Stop operations
docker-compose -f docker-compose.cluster.yml start
docker-compose -f docker-compose.cluster.yml stop
docker-compose -f docker-compose.cluster.yml restart

# Selective service management
docker-compose -f docker-compose.cluster.yml stop hypercache-node2
docker-compose -f docker-compose.cluster.yml start hypercache-node2

# Scale specific services
docker-compose -f docker-compose.cluster.yml up -d --scale hypercache-node2=2

# Complete removal
docker-compose -f docker-compose.cluster.yml down
docker-compose -f docker-compose.cluster.yml down -v  # Also remove volumes
```

### **Data Management**
```bash
# Backup data volumes
docker run --rm -v hypercache_node1_data:/source -v $(pwd):/backup \
  alpine tar czf /backup/node1-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /source .

# Backup all nodes
for node in {1..3}; do
  docker run --rm -v hypercache_node${node}_data:/source -v $(pwd):/backup \
    alpine tar czf /backup/node${node}-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /source .
done

# Restore data volume
docker run --rm -v hypercache_node1_data:/target -v $(pwd):/backup \
  alpine tar xzf /backup/node1-backup-20240823-143022.tar.gz -C /target

# Clean data (remove all cached data)
docker-compose -f docker-compose.cluster.yml down
docker volume rm hypercache_node1_data hypercache_node2_data hypercache_node3_data
docker-compose -f docker-compose.cluster.yml up -d
```

### **Rolling Updates**
```bash
# Zero-downtime update process
# 1. Update one node at a time
docker-compose -f docker-compose.cluster.yml stop hypercache-node1
docker-compose -f docker-compose.cluster.yml rm -f hypercache-node1

# 2. Build new image (if needed)
docker build -t hypercache/hypercache:latest .

# 3. Start updated node
docker-compose -f docker-compose.cluster.yml up -d hypercache-node1

# 4. Wait for rejoin and verify
sleep 30
curl http://localhost:9080/health
curl http://localhost:9080/api/cluster/members

# 5. Repeat for other nodes
# docker-compose -f docker-compose.cluster.yml stop hypercache-node2
# ... repeat process
```

### **Configuration Updates**
```bash
# Update configuration without rebuilding
docker-compose -f docker-compose.cluster.yml stop hypercache-node1

# Edit configuration file
vi configs/docker/node1-config.yaml

# Restart with new config
docker-compose -f docker-compose.cluster.yml start hypercache-node1

# Verify configuration change
curl http://localhost:9080/api/config | jq '.'
```

### **Log Management**
```bash
# View container logs
docker logs hypercache-node1 --tail 100
docker logs hypercache-node1 --since 10m
docker logs hypercache-node1 --follow

# Clear container logs
docker-compose -f docker-compose.cluster.yml down
docker system prune -f
docker-compose -f docker-compose.cluster.yml up -d

# Clear Elasticsearch logs (keep cache data)
curl -X DELETE "http://localhost:9200/_data_stream/hypercache-docker-logs"
rm -f logs/*.log

# Fresh start (clear everything)
./scripts/elk-management.sh fresh-start
```

---

## 🔒 **Security**

### **Container Security**
```yaml
# Security-focused Docker Compose configuration
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
    cap_add:
      - CHOWN
      - SETGID  
      - SETUID
```

### **Network Security**
```bash
# Create isolated network
docker network create --driver bridge \
  --subnet=172.20.0.0/16 \
  --opt com.docker.network.bridge.enable_icc=false \
  hypercache-secure

# Use custom networks
docker run --network hypercache-secure ...

# Network policy example (Kubernetes)
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: hypercache-policy
spec:
  podSelector:
    matchLabels:
      app: hypercache
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: hypercache
    ports:
    - protocol: TCP
      port: 7946
```

### **Secrets Management**
```bash
# Using environment files (development)
echo "ADMIN_PASSWORD=secure-password" > .env
docker-compose --env-file .env -f docker-compose.cluster.yml up -d

# Using Docker secrets (production)
echo "admin-password" | docker secret create hypercache-admin-pass -
docker service create --secret hypercache-admin-pass hypercache/hypercache:latest

# Using external secret management
# HashiCorp Vault, AWS Secrets Manager, Azure Key Vault integration
```

### **TLS/SSL Configuration**
```bash
# Generate self-signed certificates for testing
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout certs/hypercache.key \
  -out certs/hypercache.crt \
  -subj "/CN=hypercache.local"

# Mount certificates
docker run -v ./certs:/certs:ro \
  -e TLS_ENABLED=true \
  -e TLS_CERT_FILE=/certs/hypercache.crt \
  -e TLS_KEY_FILE=/certs/hypercache.key \
  hypercache/hypercache:latest
```

---

## ☸️ **Kubernetes Deployment**

### **Complete Kubernetes Manifests**
```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: hypercache
  labels:
    name: hypercache

---
# configmap.yaml
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
# service.yaml
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
    targetPort: 9080
  - name: resp
    port: 8080  
    targetPort: 8080
  - name: gossip
    port: 7946
    targetPort: 7946

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
    targetPort: 9080
  - name: resp
    port: 8080
    targetPort: 8080
  type: LoadBalancer

---
# statefulset.yaml
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

### **Deployment Commands**
```bash
# Deploy to Kubernetes
kubectl apply -f k8s/

# Check deployment status  
kubectl get pods -n hypercache -w
kubectl get services -n hypercache
kubectl get statefulsets -n hypercache

# Scale the cluster
kubectl scale statefulset hypercache --replicas=5 -n hypercache

# Rolling update
kubectl set image statefulset/hypercache hypercache=hypercache/hypercache:v1.2.0 -n hypercache

# Monitor rollout
kubectl rollout status statefulset/hypercache -n hypercache

# Access services
kubectl port-forward -n hypercache svc/hypercache-service 9080:9080
curl http://localhost:9080/health
```

### **Kubernetes Monitoring**
```bash
# Pod logs
kubectl logs -n hypercache hypercache-0 -f
kubectl logs -n hypercache -l app=hypercache -f --tail=100

# Pod shell access
kubectl exec -n hypercache -it hypercache-0 -- sh

# Pod resource usage
kubectl top pods -n hypercache
kubectl describe pod -n hypercache hypercache-0

# Events
kubectl get events -n hypercache --sort-by='.lastTimestamp'
```

---

## 🐛 **Troubleshooting**

### **Common Issues and Solutions**

#### **1. Port Conflicts**
```bash
# Problem: Port already in use
# Solution: Find conflicting processes
sudo lsof -i :8080,:9080,:7946

# Use different ports
docker run -p 8090:8080 -p 9090:9080 -p 7950:7946 hypercache/hypercache:latest

# Or modify docker-compose.yml port mappings
```

#### **2. Container Won't Start**
```bash
# Check container logs
docker logs hypercache-node1
docker-compose -f docker-compose.cluster.yml logs hypercache-node1

# Common issues:
# - Permission denied: Fix with volume permissions
docker run --rm -v hypercache_logs:/app/logs alpine chown -R 1000:1000 /app/logs

# - Config file not found: Verify mount paths  
docker exec hypercache-node1 ls -la /config/

# - Port binding error: Check for conflicts
netstat -tulpn | grep :8080
```

#### **3. Cluster Formation Problems**
```bash
# Check gossip connectivity
docker exec hypercache-node1 netstat -tlnp | grep 7946

# Verify DNS resolution between containers
docker exec hypercache-node2 nslookup hypercache-node1

# Check cluster status
curl http://localhost:9080/api/cluster/members
curl http://localhost:9080/api/cluster/status

# Force cluster rejoin
docker restart hypercache-node2
sleep 30
curl http://localhost:9081/api/cluster/members
```

#### **4. Performance Issues**
```bash
# Check resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Memory issues
curl http://localhost:9080/api/stats | jq '.memory'

# Disk space issues
docker exec hypercache-node1 df -h /data

# Network latency
docker exec hypercache-node1 ping hypercache-node2
```

#### **5. Data Persistence Issues**
```bash
# Check volume mounts
docker inspect hypercache-node1 | jq '.[].Mounts'

# Verify data directory permissions
docker exec hypercache-node1 ls -la /data
docker exec hypercache-node1 touch /data/test.txt

# Check disk space
docker exec hypercache-node1 df -h /data

# Volume creation issues
docker volume ls
docker volume inspect hypercache_node1_data
```

#### **6. Monitoring Stack Issues**
```bash
# Elasticsearch not accessible
curl http://localhost:9200/_cluster/health
docker logs hypercache-elasticsearch

# Grafana not loading
curl -I http://localhost:3000
docker logs hypercache-grafana

# Filebeat not shipping logs
docker logs hypercache-filebeat
curl http://localhost:9200/_cat/indices/hypercache*?v

# Fix common ELK issues
./scripts/elk-management.sh health
./scripts/elk-management.sh clean-logs
./scripts/elk-management.sh fresh-start
```

### **Debugging Checklist**
```bash
# 1. Container Health
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# 2. Application Health  
for port in 9080 9081 9082; do
  echo "Checking localhost:$port"
  curl -s http://localhost:$port/health | jq '.'
done

# 3. Network Connectivity
docker network ls | grep hypercache
docker exec hypercache-node1 ping -c 3 hypercache-node2

# 4. Cluster Status
curl -s http://localhost:9080/api/cluster/members | jq '. | length'
curl -s http://localhost:9080/api/cluster/status | jq '.health'

# 5. Resource Usage
docker stats --no-stream hypercache-node1 hypercache-node2 hypercache-node3

# 6. Log Analysis
docker-compose -f docker-compose.cluster.yml logs --tail=50 | grep -i error
```

### **Emergency Recovery Procedures**
```bash
# Complete system reset
docker-compose -f docker-compose.cluster.yml down --volumes --remove-orphans
sudo rm -rf data/* logs/*
docker system prune -f
docker-compose -f docker-compose.cluster.yml up -d

# Partial recovery (keep cache data, clear logs only)
curl -X DELETE "http://localhost:9200/_data_stream/hypercache-docker-logs"
docker restart hypercache-filebeat
rm -f logs/*.log

# Single node recovery
docker-compose -f docker-compose.cluster.yml stop hypercache-node2
docker-compose -f docker-compose.cluster.yml rm -f hypercache-node2
docker volume rm hypercache_node2_data
docker-compose -f docker-compose.cluster.yml up -d hypercache-node2
```

---

## ⚡ **Performance Tuning**

### **Container Resource Limits**
```yaml
# docker-compose.yml optimized configuration
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
      memlock:
        soft: -1
        hard: -1
```

### **Storage Optimization**
```bash
# Use SSD volumes for better I/O performance
docker volume create --driver local \
  --opt type=none \
  --opt o=bind \
  --opt device=/mnt/ssd/hypercache \
  hypercache_high_perf_data

# Optimize Docker storage driver
# Add to /etc/docker/daemon.json
{
  "storage-driver": "overlay2",
  "storage-opts": [
    "overlay2.override_kernel_check=true"
  ]
}
```

### **Network Performance**
```yaml
# Use host networking for maximum performance (less secure)
services:
  hypercache-node1:
    network_mode: host
    
# Or optimize bridge networking
networks:
  hypercache-cluster:
    driver: bridge
    driver_opts:
      com.docker.network.driver.mtu: 9000  # Jumbo frames
```

### **Memory Management**
```yaml
# Optimized cache configuration
cache:
  stores:
    - name: "main"
      type: "basic"
      max_memory: "2GB"          # Increase based on available RAM
      eviction_policy: "lru"     # Optimize for your use case
      cuckoo_filter:
        enabled: true
        capacity: 10000000       # Scale with expected key count
        false_positive_rate: 0.01
```

### **Concurrent Connections**
```yaml
# Increase connection limits
network:
  http_port: 9080
  resp_port: 8080
  max_connections: 10000       # Increase for high load
  read_timeout: "30s"
  write_timeout: "30s"
  keep_alive_timeout: "60s"
```

### **Performance Monitoring**
```bash
# Container performance metrics
docker stats --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.DiskIO}}"

# Application performance metrics
curl -s http://localhost:9080/api/stats | jq '{
  cache_hits: .cache_stats.hits,
  cache_misses: .cache_stats.misses,
  hit_ratio: (.cache_stats.hits / (.cache_stats.hits + .cache_stats.misses)),
  memory_usage: .memory_stats.used_bytes,
  request_rate: .network_stats.requests_per_second
}'

# Network latency between nodes
for i in {1..3}; do
  echo "Testing node $i connectivity:"
  time curl -s http://localhost:908$((i-1))/health > /dev/null
done
```

---

## 📋 **Best Practices**

### **Development Best Practices**
```bash
# 1. Always use specific image tags in production
docker-compose.yml:
  image: hypercache/hypercache:v1.2.0  # Not 'latest'

# 2. Use health checks
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9080/health || exit 1

# 3. Implement graceful shutdown
docker-compose.yml:
  stop_grace_period: 30s

# 4. Use non-root users
FROM alpine:3.18
RUN addgroup -g 1000 hypercache && adduser -D -u 1000 -G hypercache hypercache
USER hypercache
```

### **Production Best Practices**
```bash
# 1. Resource limits and requests
resources:
  limits:
    cpu: "1000m"
    memory: "2Gi"
  requests:
    cpu: "500m"
    memory: "1Gi"

# 2. Persistent volumes in Kubernetes
volumeClaimTemplates:
- metadata:
    name: data
  spec:
    accessModes: ["ReadWriteOnce"]
    storageClassName: "fast-ssd"
    resources:
      requests:
        storage: 50Gi

# 3. Multiple replicas with anti-affinity
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app: hypercache
        topologyKey: kubernetes.io/hostname

# 4. Monitoring and alerting
# Prometheus metrics endpoint
curl http://localhost:9080/metrics
```

### **Security Best Practices**
```bash
# 1. Regular image updates
# Check for vulnerabilities
docker scan hypercache/hypercache:latest

# 2. Secrets management
# Never hardcode secrets in images or configs
kubectl create secret generic hypercache-secrets \
  --from-literal=admin-password=secure-password

# 3. Network policies
# Restrict inter-pod communication
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: hypercache-netpol
spec:
  podSelector:
    matchLabels:
      app: hypercache
  policyTypes:
  - Ingress
  - Egress

# 4. Read-only root filesystem  
security_opt:
  - no-new-privileges:true
read_only: true
tmpfs:
  - /tmp:rw,noexec,nosuid,size=100m
```

### **Operational Best Practices**
```bash
# 1. Automated backups
# Backup script
#!/bin/bash
DATE=$(date +%Y%m%d-%H%M%S)
for node in {1..3}; do
  docker run --rm \
    -v hypercache_node${node}_data:/source:ro \
    -v /backups:/backup \
    alpine tar czf /backup/hypercache-node${node}-${DATE}.tar.gz -C /source .
done

# 2. Log retention
# Configure log rotation
docker-compose.yml:
  logging:
    driver: "json-file"
    options:
      max-size: "100m"
      max-file: "10"

# 3. Health check automation
# Monitor and auto-restart unhealthy containers
docker-compose.yml:
  restart: unless-stopped
  healthcheck:
    test: ["CMD", "wget", "--spider", "http://localhost:9080/health"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 40s

# 4. Configuration validation
# Test configs before deployment
docker run --rm -v $(pwd)/configs/docker/node1-config.yaml:/config/hypercache.yaml \
  hypercache/hypercache:latest --config /config/hypercache.yaml --validate-config
```

---

## 🛠 **Scripts & Automation**

### **Docker Deployment Script**
The project includes a comprehensive deployment script at `scripts/docker-deploy.sh`:

```bash
# Full deployment to Docker Hub
export DOCKER_USERNAME=your-username
export DOCKER_PASSWORD=your-password
./scripts/docker-deploy.sh deploy

# Local development
./scripts/docker-deploy.sh build    # Build image locally
./scripts/docker-deploy.sh start    # Start cluster
./scripts/docker-deploy.sh test     # Run tests
./scripts/docker-deploy.sh stop     # Stop cluster
./scripts/docker-deploy.sh clean    # Clean up

# Advanced options
./scripts/docker-deploy.sh logs     # View logs
./scripts/docker-deploy.sh status   # Check status
./scripts/docker-deploy.sh restart  # Restart cluster
```

### **ELK Management Script**
Comprehensive logging management with `scripts/elk-management.sh`:

```bash
# Health checks and status
./scripts/elk-management.sh status        # Overall status
./scripts/elk-management.sh health        # Detailed health check
./scripts/elk-management.sh logs-count    # Count ingested logs

# Data management
./scripts/elk-management.sh clean-logs    # Clear logs, keep cache data
./scripts/elk-management.sh fresh-start   # Complete reset
./scripts/elk-management.sh backup-logs   # Backup log indices

# Testing and debugging
./scripts/elk-management.sh test-cuckoo   # Test Cuckoo filter
./scripts/elk-management.sh cuckoo-logs   # Show Cuckoo filter logs
./scripts/elk-management.sh search "error" # Search logs
```

### **Custom Automation Scripts**
```bash
# Create custom management script
#!/bin/bash
# hypercache-docker-manager.sh

case "$1" in
    "deploy")
        docker build -t hypercache/hypercache:latest .
        docker-compose -f docker-compose.cluster.yml up -d
        sleep 30
        curl http://localhost:9080/health
        ;;
    "scale")
        REPLICAS=${2:-3}
        kubectl scale statefulset hypercache --replicas=$REPLICAS -n hypercache
        ;;
    "backup")
        DATE=$(date +%Y%m%d-%H%M%S)
        for node in {1..3}; do
            docker run --rm -v hypercache_node${node}_data:/data -v $(pwd):/backup \
                alpine tar czf /backup/node${node}-${DATE}.tar.gz -C /data .
        done
        ;;
    "monitor")
        watch -n 5 'curl -s http://localhost:9080/api/stats | jq "{hits: .cache_stats.hits, misses: .cache_stats.misses}"'
        ;;
    *)
        echo "Usage: $0 {deploy|scale|backup|monitor}"
        ;;
esac
```

### **CI/CD Integration**
GitHub Actions workflow example:

```yaml
# .github/workflows/docker-deploy.yml
name: Docker Build and Deploy

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Build Docker image
      run: docker build -t hypercache/hypercache:${{ github.sha }} .
    
    - name: Run tests
      run: |
        docker-compose -f docker-compose.cluster.yml up -d
        sleep 30
        curl http://localhost:9080/health
        ./scripts/docker-deploy.sh test
    
    - name: Login to Docker Hub
      if: github.ref == 'refs/heads/main'
      uses: docker/login-action@v2
      with:
        username: ${{ secrets.DOCKER_USERNAME }}
        password: ${{ secrets.DOCKER_PASSWORD }}
    
    - name: Push to Docker Hub
      if: github.ref == 'refs/heads/main'
      run: |
        docker tag hypercache/hypercache:${{ github.sha }} hypercache/hypercache:latest
        docker push hypercache/hypercache:latest
        docker push hypercache/hypercache:${{ github.sha }}
```

---

## 🎯 **Quick Reference Commands**

### **Essential Commands**
```bash
# 🚀 Quick start
docker build -t hypercache/hypercache:latest . && docker-compose -f docker-compose.cluster.yml up -d

# 🔍 Health check all nodes
for port in 9080 9081 9082; do curl http://localhost:$port/health; done

# 📊 View cluster status
curl http://localhost:9080/api/cluster/members | jq '.'

# 🧪 Test cache operations
curl -X PUT http://localhost:9080/api/cache/test -d '{"value":"docker test","ttl_hours":1}'
curl http://localhost:9081/api/cache/test

# 📈 Open monitoring
open http://localhost:3000  # Grafana dashboard

# 🛠 ELK management
./scripts/elk-management.sh status

# 🗑 Clean up
docker-compose -f docker-compose.cluster.yml down -v
```

### **Emergency Commands**
```bash
# 🚨 Complete reset
docker-compose -f docker-compose.cluster.yml down --volumes --remove-orphans
docker system prune -f
./scripts/elk-management.sh fresh-start

# 🔧 Fix permissions
docker run --rm -v hypercache_logs:/logs alpine chown -R 1000:1000 /logs

# 💾 Emergency backup
for node in {1..3}; do
  docker run --rm -v hypercache_node${node}_data:/data -v $(pwd):/backup \
    alpine tar czf /backup/emergency-node${node}-$(date +%s).tar.gz -C /data .
done
```

---

## 📚 **Additional Resources**

- **[Main Project README](../README.md)** - Project overview and architecture
- **[Configuration Guide](../docs/configuration-guide.md)** - Detailed configuration options
- **[Performance Benchmarks](../docs/performance-benchmarks.md)** - Performance testing results
- **[Multi-VM Deployment](../docs/multi-vm-deployment-guide.md)** - Production multi-VM setup
- **[ELK Debugging Guide](../docs/elk-debugging-guide.md)** - Comprehensive logging troubleshooting
- **[Redis CLI Testing](../docs/redis-cli-testing-guide.md)** - Redis protocol compatibility testing

### **Community and Support**
- **GitHub Repository**: [HyperCache](https://github.com/rishabhverma17/hypercache)
- **Docker Hub**: [hypercache/hypercache](https://hub.docker.com/r/hypercache/hypercache)
- **Issues and Bug Reports**: [GitHub Issues](https://github.com/rishabhverma17/hypercache/issues)

---

**This complete Docker guide provides everything needed to deploy, test, monitor, and maintain HyperCache in any Docker environment. From development testing to production Kubernetes deployments, all scenarios are covered with practical examples and troubleshooting guidance.**

*Last updated: August 23, 2025*
