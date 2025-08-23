# HyperCache Local Docker Testing Guide

This guide provides step-by-step instructions for testing HyperCache with Docker locally using two different approaches:
1. **Docker Compose Method**: Full 3-node cluster with monitoring stack
2. **Individual Containers Method**: Manual container management with different configs

## 📋 **Prerequisites**

### System Requirements
```bash
# Ensure you're in the project root
cd /Users/rishabhverma/Documents/HobbyProjects/Cache

# Verify Docker is running
docker --version
docker-compose --version

# Check available ports (should be free)
lsof -i :8080,:8081,:8082,:9080,:9081,:9082,:7946,:7947,:7948,:3000,:9200
```

### Build Requirements
- Docker Desktop running
- At least 4GB RAM available
- Ports 8080-8082, 9080-9082, 7946-7948, 3000, 9200 available

### Common Issues and Solutions
**Permission Denied for Logs Directory:**
If you see `FATAL: Failed to initialize logging: failed to create log directory: mkdir logs: permission denied`, this is a Docker permissions issue. The updated Dockerfile and Docker Compose files handle this automatically, but if you encounter this:

```bash
# Quick fix: Create and set permissions for log volume
docker volume create hypercache_logs
docker run --rm -v hypercache_logs:/app/logs alpine sh -c "chmod 755 /app/logs && chown 1000:1000 /app/logs"

# Or rebuild the image to pick up the fixes
docker-compose -f docker-compose.cluster.yml down
docker build -t hypercache/hypercache:latest .
docker-compose -f docker-compose.cluster.yml up -d
```

---

## 🚀 **Method 1: Docker Compose Cluster**

### Overview
This method starts a complete HyperCache cluster with monitoring using a single command. It includes:
- 3 HyperCache nodes with auto-discovery
- Elasticsearch for log storage
- Filebeat for log shipping  
- Grafana for monitoring dashboards

### Step 1: Build the Docker Image
```bash
# Build the HyperCache Docker image locally
docker build -t hypercache/hypercache:latest .

# Verify the image was created (should show ~15MB image)
docker images | grep hypercache
```

### Step 2: Start the Complete Cluster
```bash
# Start the entire stack (3 nodes + ELK monitoring)
docker-compose -f docker-compose.cluster.yml up -d

# Monitor startup progress
docker-compose -f docker-compose.cluster.yml logs -f

# Check all containers are running (should show 6 containers)
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

### Step 3: Verify Cluster Health
```bash
# Wait for all nodes to be ready (30-60 seconds)
sleep 30

# Check each node's health endpoint
curl http://localhost:9080/health  # Node 1
curl http://localhost:9081/health  # Node 2  
curl http://localhost:9082/health  # Node 3

# Check cluster membership (should show 3 members)
curl http://localhost:9080/api/cluster/members | jq '.'

# Verify cluster status
curl http://localhost:9080/api/cluster/status | jq '.'
```

**Expected Health Response:**
```json
{
  "status": "healthy",
  "node_id": "node-1", 
  "cluster_size": 3,
  "timestamp": "2025-08-23T..."
}
```

### Step 4: Test Basic Cache Operations
```bash
# PUT data on node 1
curl -X PUT http://localhost:9080/api/cache/test1 \
  -H "Content-Type: application/json" \
  -d '{"value":"hello from docker","ttl_hours":1}'

# Expected: {"success":true,"message":"stored"}

# GET data from node 2 (cross-node access)
curl http://localhost:9081/api/cache/test1

# Expected: {"success":true,"data":"hello from docker","ttl":...}

# PUT data on node 2
curl -X PUT http://localhost:9081/api/cache/test2 \
  -H "Content-Type: application/json" \
  -d '{"value":"distributed cache","ttl_hours":2}'

# GET from node 3 (testing full distribution)
curl http://localhost:9082/api/cache/test2

# DELETE from node 3
curl -X DELETE http://localhost:9082/api/cache/test1

# Verify deletion from node 1
curl http://localhost:9080/api/cache/test1
# Expected: {"success":false,"error":"key not found"}
```

### Step 5: Test Redis Protocol Compatibility
```bash
# If redis-cli is installed, test RESP protocol
redis-cli -p 8080 ping
# Expected: PONG

redis-cli -p 8080 set dockerkey "redis compatible"  
# Expected: OK

redis-cli -p 8081 get dockerkey  # Cross-node Redis access
# Expected: "redis compatible"

redis-cli -p 8082 del dockerkey
# Expected: (integer) 1

# Test from different node
redis-cli -p 8080 get dockerkey
# Expected: (nil)
```

### Step 6: Access Monitoring Stack
```bash
# Wait for Grafana to be ready (may take 1-2 minutes)
curl -I http://localhost:3000

# Open Grafana dashboards
open http://localhost:3000
# Login: admin / admin123

# Check Elasticsearch health
curl http://localhost:9200/_cluster/health | jq '.'

# View log indices (should show hypercache logs)
curl http://localhost:9200/_cat/indices?v | grep hypercache

# Search recent logs
curl -X GET "http://localhost:9200/hypercache-docker-logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"range":{"@timestamp":{"gte":"now-5m"}}},"size":10}' | jq '.'
```

### Step 7: Monitor Real-time Activity
```bash
# View logs from all services
docker-compose -f docker-compose.cluster.yml logs -f

# View logs from specific node
docker-compose -f docker-compose.cluster.yml logs -f hypercache-node1

# Monitor resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Check container networking
docker network ls | grep hypercache
docker network inspect hypercache-cluster_hypercache-cluster
```

### Step 8: Test Failure Scenarios
```bash
# Stop node 2 to test resilience
docker-compose -f docker-compose.cluster.yml stop hypercache-node2

# Verify data still accessible from nodes 1 and 3
curl http://localhost:9080/api/cache/test2
curl http://localhost:9082/api/cache/test2

# Check cluster status (should show 2 members)
curl http://localhost:9080/api/cluster/members

# Restart node 2
docker-compose -f docker-compose.cluster.yml start hypercache-node2

# Wait for rejoin and verify
sleep 15
curl http://localhost:9081/health
curl http://localhost:9081/api/cache/test2  # Should work after rejoin
```

### Step 9: Cleanup Method 1
```bash
# Stop and remove all containers
docker-compose -f docker-compose.cluster.yml down

# Remove volumes (this deletes all data)
docker-compose -f docker-compose.cluster.yml down -v

# Clean up Docker system (optional)
docker system prune -f
```

---

## 🔧 **Method 2: Individual Containers**

### Overview
This method gives you fine-grained control over each container, similar to running individual binaries with different config files. Each node is started separately with its own configuration.

### Step 1: Build Image and Setup
```bash
# Build the image (if not done already)
docker build -t hypercache/hypercache:latest .

# Create a custom network for cluster communication
docker network create hypercache-network \
  --driver bridge \
  --subnet=172.20.0.0/16

# Verify network creation
docker network ls | grep hypercache-network
```

### Step 2: Start Node 1 (Bootstrap Node)
```bash
# Start first node with node1-config.yaml
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

# Wait for node1 to start and become ready
sleep 10

# Check node1 health
curl http://localhost:9080/health

# View node1 startup logs
docker logs hypercache-node1 | tail -20
```

**Expected Log Output:**
```
[INFO] Starting HyperCache node: node-1
[INFO] Loading configuration from: /config/hypercache.yaml
[INFO] Starting RESP server on 0.0.0.0:8080
[INFO] Starting HTTP API server on 0.0.0.0:9080
[INFO] Starting gossip protocol on 0.0.0.0:7946
[INFO] Node node-1 is ready and healthy
```

### Step 3: Start Node 2 (Joins Cluster)
```bash
# Start second node with node2-config.yaml
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  --hostname hypercache-node2 \
  -p 8081:8080 -p 9081:9080 -p 7947:7946 \
  -v $(pwd)/configs/docker/node2-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node2_data:/data \
  -v hypercache_logs:/app/logs \
  -e NODE_ID=node-2 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946 \
  hypercache/hypercache:latest

# Wait for cluster formation
sleep 10

# Check node2 health
curl http://localhost:9081/health

# Check if node2 joined the cluster
docker logs hypercache-node2 | grep -i "joined cluster"
docker logs hypercache-node1 | grep -i "member joined"
```

### Step 4: Start Node 3 (Completes Cluster)
```bash
# Start third node with node3-config.yaml
docker run -d --name hypercache-node3 \
  --network hypercache-network \
  --hostname hypercache-node3 \
  -p 8082:8080 -p 9082:9080 -p 7948:7946 \
  -v $(pwd)/configs/docker/node3-config.yaml:/config/hypercache.yaml:ro \
  -v hypercache_node3_data:/data \
  -v hypercache_logs:/app/logs \
  -e NODE_ID=node-3 \
  -e CLUSTER_SEEDS=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946 \
  hypercache/hypercache:latest

# Wait for full cluster formation
sleep 15

# Check node3 health
curl http://localhost:9082/health

# Verify all nodes see each other
curl http://localhost:9080/api/cluster/members | jq '.members | length'
# Expected: 3
```

### Step 5: Verify Cluster Formation
```bash
# Check all containers are running
docker ps --filter "name=hypercache-node" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Verify cluster membership from each node's perspective
echo "=== Node 1 View ==="
curl http://localhost:9080/api/cluster/members | jq '.members[].id'

echo "=== Node 2 View ==="
curl http://localhost:9081/api/cluster/members | jq '.members[].id'

echo "=== Node 3 View ==="  
curl http://localhost:9082/api/cluster/members | jq '.members[].id'

# Check cluster events in logs
echo "=== Cluster Formation Events ==="
docker logs hypercache-node1 2>&1 | grep -i "member\|cluster\|gossip" | tail -10
docker logs hypercache-node2 2>&1 | grep -i "member\|cluster\|gossip" | tail -10
docker logs hypercache-node3 2>&1 | grep -i "member\|cluster\|gossip" | tail -10
```

### Step 6: Test Cross-Node Operations
```bash
# Store data on different nodes with different keys
echo "=== Storing Data on Each Node ==="
curl -X PUT http://localhost:9080/api/cache/data1 \
  -H "Content-Type: application/json" \
  -d '{"value":"stored on node1","ttl_hours":2}'

curl -X PUT http://localhost:9081/api/cache/data2 \
  -H "Content-Type: application/json" \
  -d '{"value":"stored on node2","ttl_hours":2}'

curl -X PUT http://localhost:9082/api/cache/data3 \
  -H "Content-Type: application/json" \
  -d '{"value":"stored on node3","ttl_hours":2}'

# Test cross-node data retrieval
echo "=== Testing Cross-Node Access ==="
echo "Getting data1 from node2:"
curl http://localhost:9081/api/cache/data1

echo "Getting data2 from node3:"
curl http://localhost:9082/api/cache/data2

echo "Getting data3 from node1:"
curl http://localhost:9080/api/cache/data3

# Test Redis protocol cross-node access (if redis-cli available)
echo "=== Testing Redis Protocol ==="
redis-cli -p 8080 set redis-key "hello from redis"
redis-cli -p 8081 get redis-key  # Should retrieve from different node
redis-cli -p 8082 get redis-key  # Should retrieve from third node
```

### Step 7: Test Configuration Differences
```bash
# Inspect each container's configuration
echo "=== Node 1 Configuration ==="
docker exec hypercache-node1 cat /config/hypercache.yaml | grep -A 3 -B 3 "node:"

echo "=== Node 2 Configuration ==="
docker exec hypercache-node2 cat /config/hypercache.yaml | grep -A 3 -B 3 "node:"

echo "=== Node 3 Configuration ==="
docker exec hypercache-node3 cat /config/hypercache.yaml | grep -A 3 -B 3 "node:"

# Check data directories and persistence
echo "=== Data Directory Contents ==="
docker exec hypercache-node1 ls -la /data/
docker exec hypercache-node2 ls -la /data/
docker exec hypercache-node3 ls -la /data/
```

### Step 8: Test Individual Node Failures
```bash
# Test node failure and recovery
echo "=== Testing Node 2 Failure ==="
docker stop hypercache-node2

# Wait for failure detection
sleep 10

# Verify data still accessible from nodes 1 and 3
echo "Accessing data2 from node1 (node2 is down):"
curl http://localhost:9080/api/cache/data2

echo "Accessing data2 from node3 (node2 is down):"
curl http://localhost:9082/api/cache/data2

# Check cluster membership (should show 2 members)
curl http://localhost:9080/api/cluster/members | jq '.members | length'

# Restart node2
echo "=== Restarting Node 2 ==="
docker start hypercache-node2

# Wait for rejoin
sleep 15

# Verify rejoin
curl http://localhost:9081/health
echo "Cluster size after rejoin:"
curl http://localhost:9081/api/cluster/members | jq '.members | length'

# Test data access from rejoined node
echo "Accessing data1 from rejoined node2:"
curl http://localhost:9081/api/cache/data1
```

### Step 9: Monitor Individual Containers
```bash
# View logs from each container (run in separate terminals)
docker logs -f hypercache-node1   # Terminal 1
docker logs -f hypercache-node2   # Terminal 2
docker logs -f hypercache-node3   # Terminal 3

# Or view recent logs from all nodes
echo "=== Recent Node 1 Logs ==="
docker logs --tail 20 hypercache-node1

echo "=== Recent Node 2 Logs ==="
docker logs --tail 20 hypercache-node2

echo "=== Recent Node 3 Logs ==="
docker logs --tail 20 hypercache-node3

# Check resource usage
docker stats hypercache-node1 hypercache-node2 hypercache-node3

# Inspect container networking details
echo "=== Network Configuration ==="
docker inspect hypercache-node1 | jq '.[0].NetworkSettings.Networks'
docker inspect hypercache-node2 | jq '.[0].NetworkSettings.Networks'
docker inspect hypercache-node3 | jq '.[0].NetworkSettings.Networks'

# Check volume mounts
echo "=== Volume Mounts ==="
docker inspect hypercache-node1 | jq '.[0].Mounts'
```

### Step 10: Performance Testing
```bash
# Run performance test across all nodes
echo "=== Performance Testing ==="

# Load test on node 1
for i in {1..100}; do
  curl -X PUT http://localhost:9080/api/cache/perf-test-$i \
    -H "Content-Type: application/json" \
    -d '{"value":"performance test data","ttl_hours":1}' &
done
wait

# Retrieve from different nodes
for i in {1..100}; do
  NODE_PORT=$((9080 + (i % 3)))
  curl -s http://localhost:$NODE_PORT/api/cache/perf-test-$i > /dev/null &
done
wait

echo "Performance test completed"

# Check cache statistics
curl http://localhost:9080/api/stats | jq '.'
curl http://localhost:9081/api/stats | jq '.'
curl http://localhost:9082/api/stats | jq '.'
```

### Step 11: Cleanup Method 2
```bash
# Stop all nodes
echo "=== Stopping All Nodes ==="
docker stop hypercache-node1 hypercache-node2 hypercache-node3

# Remove containers
echo "=== Removing Containers ==="
docker rm hypercache-node1 hypercache-node2 hypercache-node3

# Remove volumes (WARNING: This deletes all data)
echo "=== Removing Data Volumes ==="
docker volume rm hypercache_node1_data hypercache_node2_data hypercache_node3_data hypercache_logs

# Remove network
echo "=== Removing Network ==="
docker network rm hypercache-network

# Verify cleanup
docker ps -a | grep hypercache
docker volume ls | grep hypercache
docker network ls | grep hypercache
```

---

## 🔍 **Comparison: Method 1 vs Method 2**

### Method 1 (Docker Compose) - Best For:
✅ **Full system testing** with monitoring stack  
✅ **Quick setup and teardown**  
✅ **Integration testing** with ELK stack  
✅ **Production-like environment** testing  
✅ **Automated dependency management**  

**Advantages:**
- One command startup: `docker-compose up -d`
- Includes monitoring and logging infrastructure
- Automatic networking and service discovery
- Health checks and restart policies
- Volume management handled automatically

**Disadvantages:**
- Less control over individual node configuration
- All nodes use similar configs (with env var overrides)
- Harder to test specific failure scenarios
- More resource intensive (includes ELK stack)

### Method 2 (Individual Containers) - Best For:
✅ **Individual node configuration testing**  
✅ **Specific failure scenario testing**  
✅ **Configuration validation**  
✅ **Step-by-step cluster formation testing**  
✅ **Resource-constrained environments**  

**Advantages:**
- Complete control over each node's configuration
- Can use different YAML files per node
- Easy to test individual node failures
- More similar to manual binary execution
- Lower resource usage (no monitoring stack)

**Disadvantages:**
- Manual network and dependency management
- More commands required for setup
- No built-in monitoring stack
- Manual cleanup required
- More prone to configuration errors

---

## 🎯 **Testing Scenarios**

### Scenario 1: Basic Functionality Test
```bash
# Use either method, then run:
curl -X PUT http://localhost:9080/api/cache/basic-test \
  -d '{"value":"basic functionality","ttl_hours":1}'
  
curl http://localhost:9081/api/cache/basic-test
curl -X DELETE http://localhost:9082/api/cache/basic-test
curl http://localhost:9080/api/cache/basic-test  # Should return not found
```

### Scenario 2: High Availability Test  
```bash
# Method 1: docker-compose stop hypercache-node2
# Method 2: docker stop hypercache-node2

# Verify data accessible from remaining nodes
curl http://localhost:9080/api/cache/ha-test
curl http://localhost:9082/api/cache/ha-test

# Restart and verify rejoin
# Method 1: docker-compose start hypercache-node2  
# Method 2: docker start hypercache-node2
```

### Scenario 3: Performance and Load Test
```bash
# Store 1000 keys across all nodes
for i in {1..1000}; do
  NODE_PORT=$((9080 + (i % 3)))
  curl -X PUT http://localhost:$NODE_PORT/api/cache/load-test-$i \
    -d '{"value":"load test data '$i'","ttl_hours":1}' &
  
  # Limit concurrent requests
  if (( i % 50 == 0 )); then wait; fi
done
wait

# Verify cross-node retrieval
for i in {1..1000}; do
  NODE_PORT=$((9080 + ((i+1) % 3)))  # Get from different node than stored
  curl -s http://localhost:$NODE_PORT/api/cache/load-test-$i > /dev/null &
  
  if (( i % 50 == 0 )); then wait; fi
done
wait
```

### Scenario 4: Configuration Validation
```bash
# Method 2 only - Test different configs
# Start nodes with custom memory limits, TTLs, etc.
# Verify each node respects its individual configuration

# Check each node's effective configuration
curl http://localhost:9080/api/config | jq '.cache.stores[0].max_memory'
curl http://localhost:9081/api/config | jq '.cache.stores[0].max_memory'  
curl http://localhost:9082/api/config | jq '.cache.stores[0].max_memory'
```

## 🔧 **Troubleshooting Common Issues**

### Permission Issues
```bash
# If you see "mkdir logs: permission denied"
# Solution 1: Fix the volume permissions
docker volume create hypercache_logs
docker run --rm -v hypercache_logs:/app/logs alpine sh -c "chmod 755 /app/logs && chown 1000:1000 /app/logs"

# Solution 2: Rebuild with the updated Dockerfile
docker build --no-cache -t hypercache/hypercache:latest .

# Solution 3: Use local directories instead of volumes
# In docker-compose.cluster.yml, replace:
#   - hypercache_logs:/app/logs
# With:
#   - ./logs:/app/logs
# Then create the directory: mkdir -p logs && chmod 755 logs
```

### Port Conflicts
```bash
# Find what's using the ports
sudo lsof -i :8080,:9080,:7946

# Kill conflicting processes
sudo kill -9 <PID>

# Or use different ports
docker run -p 8090:8080 -p 9090:9080 ...
```

### DNS Resolution Issues
```bash
# Test container DNS
docker exec hypercache-node1 nslookup hypercache-node2
docker exec hypercache-node1 ping hypercache-node2

# Check Docker network
docker network inspect hypercache-network
```

### Container Won't Start
```bash
# Check logs for errors
docker logs hypercache-node1

# Verify image
docker images | grep hypercache

# Check disk space
df -h
docker system df
```

### Cluster Formation Issues
```bash
# Verify gossip port connectivity
docker exec hypercache-node1 netstat -tulpn | grep 7946

# Check cluster seeds configuration
docker exec hypercache-node1 cat /config/hypercache.yaml | grep -A 5 "seeds:"

# Monitor cluster events
docker logs -f hypercache-node1 | grep -i "cluster\|member\|gossip"
```

This comprehensive guide provides everything needed to test HyperCache locally with Docker using both orchestrated and manual approaches.
