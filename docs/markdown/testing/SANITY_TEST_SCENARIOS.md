# HyperCache Sanity Test Scenarios

This document describes two sanity test scenarios to validate the distributed cache functionality using cURL commands.

## Pre-requisites

1. **Build the project**:
   ```bash
   cd /Users/rishabhverma/Documents/HobbyProjects/Cache
   go build -o bin/multi-node-demo cmd/multi-node-demo/main.go
   ```

2. **Make scripts executable**:
   ```bash
   chmod +x scripts/scenario1-basic-cross-node.sh
   chmod +x scripts/scenario2-node-failure-recovery.sh
   ```

## Scenario 1: Basic Cross-Node Operations Test

**Purpose**: Validates data distribution and cross-node retrieval capabilities.

**What it tests**:
- 3-node cluster formation with replication factor 3
- Data storage on different nodes
- Cross-node data retrieval (data stored on one node accessible from others)
- DELETE operations with cluster-wide consistency

**Key operations**:
```bash
# PUT data on different nodes
curl -X POST http://localhost:8080/api/cache/data1 -H "Content-Type: application/json" -d '{"value":"value_from_node1","ttl":3600}'
curl -X POST http://localhost:8081/api/cache/data2 -H "Content-Type: application/json" -d '{"value":"value_from_node2","ttl":3600}'
curl -X POST http://localhost:8082/api/cache/data3 -H "Content-Type: application/json" -d '{"value":"value_from_node3","ttl":3600}'

# GET data from different nodes (cross-node access)
curl http://localhost:8080/api/cache/data2  # data2 from node1
curl http://localhost:8081/api/cache/data1  # data1 from node2
curl http://localhost:8082/api/cache/data1  # data1 from node3

# DELETE from any node
curl -X DELETE http://localhost:8082/api/cache/data1
```

**Expected behavior**:
- All PUT operations should succeed with `{"success":true}`
- Cross-node GET operations should retrieve data regardless of which node originally stored it
- DELETE operation should remove data cluster-wide

**Run the test**:
```bash
./scripts/scenario1-basic-cross-node.sh
```

## Scenario 2: Node Failure and Recovery Test

**Purpose**: Tests resilience and data availability during node failures.

**What it tests**:
- Cluster formation and data replication
- Data accessibility during node failure
- Node recovery and cluster re-formation
- Data persistence and integrity after recovery

**Key flow**:
1. **Setup**: 3-node cluster with critical data stored across nodes
2. **Failure simulation**: Kill Node 2 (simulates hardware/network failure)
3. **Resilience test**: Verify data still accessible from Nodes 1 and 3
4. **Recovery**: Restart Node 2 and verify cluster re-formation
5. **Verification**: Confirm all data is accessible from all nodes

**Critical data tested**:
- `user:1001:session` - User session data
- `product:5678:inventory` - Product inventory
- `order:9999:status` - Order status
- `config:main:settings` - Configuration data
- `cache:stats:global` - Global cache statistics

**Expected behavior**:
- Data should remain accessible during single node failure
- Recovered node should rejoin cluster and sync data
- Post-recovery: 100% data availability across all nodes

**Run the test**:
```bash
./scripts/scenario2-node-failure-recovery.sh
```

## Manual Testing with cURL

For manual testing, you can run the multi-node demo and use these cURL commands:

### Start a 3-node cluster:
```bash
# Terminal 1 - Node 1 (Bootstrap)
./bin/multi-node-demo 1

# Terminal 2 - Node 2 (Join cluster)
./bin/multi-node-demo 2 127.0.0.1:7946

# Terminal 3 - Node 3 (Join cluster)  
./bin/multi-node-demo 3 127.0.0.1:7946
```

### Basic operations:
```bash
# Health check
curl http://localhost:8080/health
curl http://localhost:8081/health
curl http://localhost:8082/health

# Store data
curl -X POST http://localhost:8080/api/cache/mykey \
  -H "Content-Type: application/json" \
  -d '{"value":"hello world","ttl":3600}'

# Retrieve data from any node
curl http://localhost:8081/api/cache/mykey
curl http://localhost:8082/api/cache/mykey

# Delete data
curl -X DELETE http://localhost:8080/api/cache/mykey

# Verify deletion
curl http://localhost:8081/api/cache/mykey  # Should return not found
```

### Advanced scenarios:
```bash
# Store multiple keys with different TTLs
curl -X POST http://localhost:8080/api/cache/short-lived \
  -H "Content-Type: application/json" \
  -d '{"value":"expires soon","ttl":30}'

curl -X POST http://localhost:8081/api/cache/long-lived \
  -H "Content-Type: application/json" \
  -d '{"value":"expires later","ttl":7200}'

# Test cross-node access patterns
curl http://localhost:8082/api/cache/short-lived
curl http://localhost:8080/api/cache/long-lived
```

## Understanding the Output

### Success indicators:
- `{"success":true,"data":"value","ttl":remaining_seconds}` - Successful GET
- `{"success":true,"message":"stored"}` - Successful PUT
- `{"success":true,"message":"deleted"}` - Successful DELETE

### Failure indicators:
- `{"success":false,"error":"key not found"}` - Key doesn't exist
- `{"success":false,"error":"..."}` - Other errors

### Cluster health:
- `{"status":"healthy","node":"X","cluster_size":3}` - Healthy node
- Connection refused - Node is down

## Log Analysis

Both scenarios generate detailed logs:
- `scenario1_nodeX.log` - Node operations for scenario 1
- `scenario2_nodeX.log` - Node operations for scenario 2  
- `scenario2_node2_recovery.log` - Node 2 recovery process

Key log indicators:
- `Cluster member joined:` - Node joining
- `Cluster member left:` - Node leaving  
- `PUT request for key` - Data storage
- `GET request for key` - Data retrieval
- `DELETE request for key` - Data deletion
- `Forwarding request to node` - Inter-node communication

## Troubleshooting

**Common issues**:

1. **Port conflicts**: Kill existing processes
   ```bash
   pkill -f multi-node-demo
   lsof -ti :8080,:8081,:8082,:7946,:7947,:7948 | xargs kill -9
   ```

2. **Build issues**: Ensure Go modules are up to date
   ```bash
   go mod tidy
   go build -o bin/multi-node-demo cmd/multi-node-demo/main.go
   ```

3. **Network issues**: Check localhost connectivity
   ```bash
   curl -I http://localhost:8080/health
   ```

4. **Timing issues**: Allow more time for cluster formation (adjust sleep values in scripts)

These scenarios provide comprehensive validation of HyperCache's distributed capabilities, including data replication, cross-node access, failure resilience, and recovery mechanisms.
