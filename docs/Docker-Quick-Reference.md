# HyperCache Docker Quick Reference

## 🚀 **Quick Commands**

### Method 1: Docker Compose (Recommended)
```bash
# Build & Start Complete System
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d

# Test Basic Operations
curl -X PUT http://localhost:9080/api/cache/test \
  -d '{"value":"hello","ttl_hours":1}'
curl http://localhost:9081/api/cache/test

# Monitor & Access
open http://localhost:3000  # Grafana (admin/admin123)
docker-compose -f docker-compose.cluster.yml logs -f

# Cleanup
docker-compose -f docker-compose.cluster.yml down -v
```

### Method 2: Individual Containers
```bash
# Setup Network
docker network create hypercache-network

# Start Nodes
docker run -d --name hypercache-node1 \
  --network hypercache-network \
  -p 8080:8080 -p 9080:9080 -p 7946:7946 \
  -v $(pwd)/configs/docker/node1-config.yaml:/config/hypercache.yaml:ro \
  hypercache/hypercache:latest

# Repeat for node2 (ports 8081:8080, 9081:9080, 7947:7946)
# Repeat for node3 (ports 8082:8080, 9082:9080, 7948:7946)

# Cleanup
docker stop hypercache-node{1,2,3}
docker rm hypercache-node{1,2,3}
docker network rm hypercache-network
```

## 🔍 **Testing Commands**

### Health Checks
```bash
curl http://localhost:9080/health        # Node 1
curl http://localhost:9081/health        # Node 2
curl http://localhost:9082/health        # Node 3
curl http://localhost:9080/api/cluster/members  # Cluster status
```

### Cache Operations
```bash
# HTTP API
curl -X PUT http://localhost:9080/api/cache/KEY \
  -d '{"value":"VALUE","ttl_hours":1}'
curl http://localhost:9081/api/cache/KEY
curl -X DELETE http://localhost:9082/api/cache/KEY

# Redis Protocol (if redis-cli installed)
redis-cli -p 8080 set key value
redis-cli -p 8081 get key
redis-cli -p 8082 del key
```

### Cross-Node Testing
```bash
# Store on node 1, retrieve from node 2
curl -X PUT http://localhost:9080/api/cache/cross-test \
  -d '{"value":"cross node data","ttl_hours":2}'
curl http://localhost:9081/api/cache/cross-test

# Should return: {"success":true,"data":"cross node data",...}
```

## 🔧 **Troubleshooting Commands**

### Container Management
```bash
docker ps --filter "name=hypercache"    # List containers
docker logs hypercache-node1             # View logs
docker stats hypercache-node{1,2,3}     # Resource usage
docker exec -it hypercache-node1 sh     # Shell access
```

### Network & Connectivity
```bash
docker network ls | grep hypercache
docker exec hypercache-node1 ping hypercache-node2
docker exec hypercache-node1 netstat -tulpn | grep 7946
```

### Port Conflicts
```bash
sudo lsof -i :8080,:9080,:7946          # Find port usage
docker run -p 8090:8080 ...              # Use different ports
```

### Data & Volumes
```bash
docker volume ls | grep hypercache       # List volumes
docker inspect hypercache-node1 | jq '.[0].Mounts'  # Check mounts
docker exec hypercache-node1 ls -la /data/          # Data directory
```

## 📊 **Monitoring Commands**

### Log Analysis
```bash
# Real-time logs
docker logs -f hypercache-node1

# Search logs
docker logs hypercache-node1 2>&1 | grep -i error
docker logs hypercache-node1 2>&1 | grep -i "cluster\|member"

# ELK Stack (Method 1 only)
curl http://localhost:9200/_cluster/health
curl "http://localhost:9200/hypercache-docker-logs-*/_search?size=10"
```

### Performance Testing
```bash
# Load test script
for i in {1..100}; do
  curl -X PUT http://localhost:9080/api/cache/load-$i \
    -d '{"value":"test data","ttl_hours":1}' &
  if (( i % 10 == 0 )); then wait; fi
done
wait

# Verify distribution
curl http://localhost:9080/api/stats
curl http://localhost:9081/api/stats  
curl http://localhost:9082/api/stats
```

## 🛠 **Development Commands**

### Build & Development
```bash
# Rebuild image
docker build -t hypercache/hypercache:latest .

# Check image size
docker images | grep hypercache

# Multi-arch build
docker buildx build --platform linux/amd64,linux/arm64 \
  -t hypercache/hypercache:latest .
```

### Configuration Testing
```bash
# View effective config
docker exec hypercache-node1 cat /config/hypercache.yaml

# Test custom config
docker run -v ./custom-config.yaml:/config/hypercache.yaml:ro \
  hypercache/hypercache:latest
```

## 🔄 **Common Workflows**

### Full System Test
```bash
# 1. Build and start
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d

# 2. Wait for ready
sleep 30

# 3. Test operations
curl -X PUT http://localhost:9080/api/cache/workflow-test \
  -d '{"value":"full system test","ttl_hours":1}'
curl http://localhost:9081/api/cache/workflow-test
curl http://localhost:9082/api/cache/workflow-test

# 4. Check monitoring
open http://localhost:3000

# 5. Cleanup
docker-compose -f docker-compose.cluster.yml down -v
```

### Failure Recovery Test
```bash
# 1. Start cluster
docker-compose -f docker-compose.cluster.yml up -d

# 2. Add test data
curl -X PUT http://localhost:9080/api/cache/recovery-test \
  -d '{"value":"recovery test data","ttl_hours":1}'

# 3. Stop node 2
docker-compose -f docker-compose.cluster.yml stop hypercache-node2

# 4. Verify data still accessible
curl http://localhost:9080/api/cache/recovery-test
curl http://localhost:9082/api/cache/recovery-test

# 5. Restart node 2
docker-compose -f docker-compose.cluster.yml start hypercache-node2

# 6. Verify rejoin
sleep 15
curl http://localhost:9081/api/cache/recovery-test
```

### Performance Comparison
```bash
# Test native vs Docker performance
time curl -X PUT http://localhost:9080/api/cache/perf-test \
  -d '{"value":"performance test","ttl_hours":1}'

# Compare with native binary (if running)
time curl -X PUT http://localhost:8080/api/cache/perf-test \
  -d '{"value":"performance test","ttl_hours":1}'
```

## 📚 **Reference Links**

- **[Complete Testing Guide](Local-Docker-Testing-Guide.md)** - Detailed step-by-step instructions
- **[Docker Deployment Guide](Docker-Deployment-Guide.md)** - Production deployment information  
- **[Docker Implementation Plan](Docker-Implementation-Plan.md)** - Architecture and planning document
- **[Main README](../README.md)** - Project overview and quick start
