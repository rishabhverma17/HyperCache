# HyperCache Docker Deployment Guide

## 🐳 **Quick Start**

### Deploy Full Cluster with Monitoring
```bash
# 1. Build and deploy to Docker Hub
DOCKER_USERNAME=your-username DOCKER_PASSWORD=your-password \
  ./scripts/docker-deploy.sh deploy

# 2. Or start locally (without pushing to Docker Hub)
./scripts/docker-deploy.sh build
./scripts/docker-deploy.sh start

# 3. Test the cluster
./scripts/docker-deploy.sh test
```

### Access Points
- **HyperCache Nodes**: 
  - Node 1: http://localhost:9080 (RESP: localhost:8080)
  - Node 2: http://localhost:9081 (RESP: localhost:8081)
  - Node 3: http://localhost:9082 (RESP: localhost:8082)
- **Grafana**: http://localhost:3000 (admin/admin123)
- **Elasticsearch**: http://localhost:9200

## 📦 **Docker Image Details**

### Image Specifications
- **Base Image**: `scratch` (minimal attack surface)
- **Binary Size**: ~15MB (statically linked)
- **Architecture**: Multi-arch (amd64, arm64)
- **Security**: Non-root user, minimal dependencies

### Available Tags
- `latest` - Latest stable release
- `v1.x.x` - Specific version tags
- `main-{commit}` - Development builds
- `develop-{commit}` - Feature branch builds

### Docker Hub Repository
```bash
docker pull hypercache/hypercache:latest
```

## 🚀 **Deployment Options**

### 1. Docker Compose (Recommended for Development)
```bash
# Start cluster with monitoring
docker-compose -f docker-compose.cluster.yml up -d

# Scale to 5 nodes
docker-compose -f docker-compose.cluster.yml up -d --scale hypercache-node2=5

# View logs
docker-compose -f docker-compose.cluster.yml logs -f hypercache-node1
```

### 2. Standalone Docker Containers
```bash
# Create network
docker network create hypercache-network

# Start Node 1 (Bootstrap)
docker run -d --name hypercache-node1 \
  --network hypercache-network \
  -p 8080:8080 -p 9080:9080 -p 7946:7946 \
  -e NODE_ID=node-1 \
  -e CLUSTER_SEEDS=hypercache-node1:7946 \
  -v hypercache_node1_data:/data \
  hypercache/hypercache:latest

# Start Node 2
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  -p 8081:8080 -p 9081:9080 -p 7947:7946 \
  -e NODE_ID=node-2 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946 \
  -v hypercache_node2_data:/data \
  hypercache/hypercache:latest
```

### 3. Kubernetes Deployment
```bash
# Deploy to Kubernetes
kubectl apply -f k8s/hypercache-cluster.yaml

# Scale the StatefulSet
kubectl scale statefulset hypercache --replicas=5 -n hypercache

# Monitor pods
kubectl get pods -n hypercache -w
```

## ⚙️ **Configuration**

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `NODE_ID` | Unique node identifier | auto-generated |
| `CLUSTER_SEEDS` | Comma-separated seed nodes | localhost:7946 |
| `DATA_DIR` | Data persistence directory | /data |
| `LOG_LEVEL` | Logging level | info |
| `MAX_MEMORY` | Maximum memory per store | 256MB |
| `REPLICATION_FACTOR` | Cluster replication factor | 3 |

### Volume Mounts
```bash
# Data persistence
-v hypercache_data:/data

# Configuration override
-v ./custom-config.yaml:/config/hypercache.yaml:ro

# Log output
-v hypercache_logs:/app/logs
```

### Network Ports
- **8080**: RESP Protocol (Redis-compatible)
- **9080**: HTTP API and health checks
- **7946**: Gossip protocol for cluster communication

## 🔍 **Monitoring & Logging**

### Integrated ELK Stack
The Docker deployment includes full logging infrastructure:

```bash
# View real-time logs in Grafana
open http://localhost:3000

# Query logs in Elasticsearch
curl "http://localhost:9200/hypercache-docker-logs-*/_search?pretty" \
  -H "Content-Type: application/json" \
  -d '{"query":{"match":{"level":"ERROR"}}}'

# Monitor log shipping
docker logs hypercache-filebeat
```

### Health Checks
```bash
# Container health
docker ps --format "table {{.Names}}\t{{.Status}}"

# Application health
curl http://localhost:9080/health
curl http://localhost:9081/health
curl http://localhost:9082/health

# Cluster status
curl http://localhost:9080/api/cluster/status
```

### Performance Monitoring
Built-in Grafana dashboards provide:
- **Real-time Metrics**: Request rates, latencies, error rates
- **Cluster Health**: Node status, replication lag
- **Resource Usage**: Memory, CPU, storage utilization
- **Log Analysis**: Error tracking, performance analysis

## 🔧 **Operations**

### Scaling Operations
```bash
# Add new node to existing cluster
docker run -d --name hypercache-node4 \
  --network hypercache-network \
  -p 8083:8080 -p 9083:9080 -p 7949:7946 \
  -e NODE_ID=node-4 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946 \
  hypercache/hypercache:latest

# Kubernetes scaling
kubectl scale statefulset hypercache --replicas=5 -n hypercache
```

### Data Management
```bash
# Backup data volumes
docker run --rm -v hypercache_node1_data:/data -v $(pwd):/backup \
  alpine tar czf /backup/node1-backup.tar.gz -C /data .

# Restore data volumes
docker run --rm -v hypercache_node1_data:/data -v $(pwd):/backup \
  alpine tar xzf /backup/node1-backup.tar.gz -C /data
```

### Rolling Updates
```bash
# Update single node (zero-downtime)
docker stop hypercache-node2
docker rm hypercache-node2
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  -p 8081:8080 -p 9081:9080 -p 7947:7946 \
  -e NODE_ID=node-2 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946 \
  -v hypercache_node2_data:/data \
  hypercache/hypercache:latest

# Kubernetes rolling update
kubectl set image statefulset/hypercache hypercache=hypercache/hypercache:v1.2.0 -n hypercache
```

## 🔒 **Security Considerations**

### Container Security
- **Non-root User**: Runs as UID 1000
- **Minimal Attack Surface**: Scratch base image
- **Read-only Filesystem**: Only data directory is writable
- **Resource Limits**: CPU and memory constraints

### Network Security
```bash
# Custom networks for isolation
docker network create --driver bridge \
  --subnet=172.20.0.0/16 \
  --ip-range=172.20.240.0/20 \
  hypercache-secure

# TLS termination (production)
docker run -d --name nginx-proxy \
  -p 443:443 -p 80:80 \
  -v /etc/ssl/certs:/etc/ssl/certs:ro \
  nginx:alpine
```

### Secrets Management
```bash
# Using Docker secrets
echo "admin-password" | docker secret create hypercache-admin-pass -
docker service create --secret hypercache-admin-pass hypercache/hypercache:latest

# Kubernetes secrets
kubectl create secret generic hypercache-secrets \
  --from-literal=admin-password=secure-password \
  -n hypercache
```

## 🐛 **Troubleshooting**

### Common Issues

#### 1. Port Conflicts
```bash
# Find conflicting processes
sudo lsof -i :8080,:9080,:7946

# Use different ports
docker run -p 8090:8080 -p 9090:9080 -p 7950:7946 hypercache/hypercache:latest
```

#### 2. Cluster Formation Issues
```bash
# Check gossip connectivity
docker exec hypercache-node1 netstat -tlnp | grep 7946

# Verify DNS resolution
docker exec hypercache-node2 nslookup hypercache-node1

# Check cluster status
curl http://localhost:9080/api/cluster/members
```

#### 3. Persistence Issues
```bash
# Check volume mounts
docker inspect hypercache-node1 | jq '.[].Mounts'

# Verify data directory permissions
docker exec hypercache-node1 ls -la /data

# Check disk space
docker exec hypercache-node1 df -h /data
```

#### 4. Log Analysis
```bash
# Container logs
docker logs hypercache-node1 --since 10m

# Application logs
docker exec hypercache-node1 tail -f /app/logs/node-1.log

# Elasticsearch query
curl -X GET "localhost:9200/hypercache-docker-logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"range":{"@timestamp":{"gte":"now-1h"}}}}'
```

## 📚 **Advanced Configurations**

### Custom Configuration
```yaml
# custom-hypercache-config.yaml
node:
  id: "${NODE_ID}"
  data_dir: "/data"

cache:
  stores:
    - name: "sessions"
      eviction_policy: "lru"
      max_memory: "1GB"
      ttl: "30m"
      enable_cuckoo_filter: true
      cuckoo_filter_fpp: 0.001

    - name: "catalog"
      eviction_policy: "lfu" 
      max_memory: "2GB"
      ttl: "24h"
      enable_cuckoo_filter: false

# Use custom config
docker run -v ./custom-hypercache-config.yaml:/config/hypercache.yaml:ro \
  hypercache/hypercache:latest
```

### Production Deployment with Docker Swarm
```bash
# Initialize swarm
docker swarm init

# Deploy stack
docker stack deploy -c docker-compose.cluster.yml hypercache

# Scale services
docker service scale hypercache_hypercache-node1=3
```

## 🎯 **Performance Tuning**

### Resource Allocation
```yaml
# docker-compose.yml resource limits
resources:
  limits:
    cpus: '2.0'
    memory: 4G
  reservations:
    cpus: '0.5'
    memory: 1G
```

### Volume Performance
```bash
# Use SSD volumes for better performance
docker volume create --driver local \
  --opt type=none \
  --opt o=bind \
  --opt device=/mnt/ssd/hypercache \
  hypercache_high_perf_data
```

This comprehensive Docker deployment plan provides everything needed to containerize HyperCache, deploy it to Docker Hub, and run it in various environments while maintaining integration with your existing ELK monitoring stack.
