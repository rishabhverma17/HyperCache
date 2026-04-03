# HyperCache Redis-CLI Testing Guide

This guide provides comprehensive redis-cli commands and testing procedures for manually validating HyperCache's distributed functionality.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Connection Commands](#connection-commands)
- [Basic Operations](#basic-operations)
- [Replication Testing](#replication-testing)
- [Advanced Test Scenarios](#advanced-test-scenarios)
- [Complete Manual Test Script](#complete-manual-test-script)
- [Expected Behaviors](#expected-behaviors)
- [Troubleshooting](#troubleshooting)

## Prerequisites

1. **Start the HyperCache cluster:**
   ```bash
   ./scripts/build-and-run.sh cluster
   ```

2. **Wait for cluster formation:**
   ```bash
   sleep 5
   ```

3. **Ensure redis-cli is installed:**
   ```bash
   # On macOS
   brew install redis
   
   # Or check if already installed
   which redis-cli
   ```

## Connection Commands

### Connect to Individual Nodes

```bash
# Node 1 - RESP Protocol (Port 8080)
redis-cli -h localhost -p 8080

# Node 2 - RESP Protocol (Port 8081)
redis-cli -h localhost -p 8081

# Node 3 - RESP Protocol (Port 8082)
redis-cli -h localhost -p 8082
```

### Test Basic Connectivity

```bash
# Ping each node
redis-cli -h localhost -p 8080 ping
redis-cli -h localhost -p 8081 ping
redis-cli -h localhost -p 8082 ping

# Expected response: PONG
```

## Basic Operations

### 1. SET Operations

```bash
# Connect to any node (e.g., Node 1)
redis-cli -h localhost -p 8080

# Basic SET commands
127.0.0.1:8080> SET user:1 "John Doe"
127.0.0.1:8080> SET user:2 "Jane Smith"
127.0.0.1:8080> SET counter 42
127.0.0.1:8080> SET config:timeout 300
127.0.0.1:8080> SET session:abc123 "active_user_session"

# Expected response for each: OK
```

### 2. GET Operations

```bash
# Get the values back
127.0.0.1:8080> GET user:1
# Expected: "John Doe"

127.0.0.1:8080> GET user:2
# Expected: "Jane Smith"

127.0.0.1:8080> GET counter
# Expected: "42"

127.0.0.1:8080> GET config:timeout
# Expected: "300"

127.0.0.1:8080> GET session:abc123
# Expected: "active_user_session"

# Test non-existent key
127.0.0.1:8080> GET nonexistent
# Expected: (nil)
```

### 3. DELETE Operations

```bash
# Delete single keys
127.0.0.1:8080> DEL user:1
# Expected: (integer) 1

127.0.0.1:8080> DEL counter
# Expected: (integer) 1

# Verify deletion
127.0.0.1:8080> GET user:1
# Expected: (nil)

127.0.0.1:8080> GET counter
# Expected: (nil)

# Delete multiple keys at once
127.0.0.1:8080> SET temp1 "value1"
127.0.0.1:8080> SET temp2 "value2"
127.0.0.1:8080> SET temp3 "value3"
127.0.0.1:8080> DEL temp1 temp2 temp3
# Expected: (integer) 3

# Verify multiple deletion
127.0.0.1:8080> GET temp1
127.0.0.1:8080> GET temp2
127.0.0.1:8080> GET temp3
# All should return: (nil)
```

## Replication Testing

### Cross-Node Data Availability

#### Terminal 1 - Node 1 Operations:
```bash
redis-cli -h localhost -p 8080

127.0.0.1:8080> SET replication:test "from_node_1"
127.0.0.1:8080> SET cluster:data "distributed_value"
127.0.0.1:8080> SET node1:exclusive "created_on_node1"
```

#### Terminal 2 - Node 2 Verification:
```bash
redis-cli -h localhost -p 8081

# Should be able to read data set on Node 1
127.0.0.1:8081> GET replication:test
# Expected: "from_node_1"

127.0.0.1:8081> GET cluster:data
# Expected: "distributed_value"

127.0.0.1:8081> GET node1:exclusive
# Expected: "created_on_node1"

# Set data from Node 2
127.0.0.1:8081> SET node2:exclusive "only_on_node2"
```

#### Terminal 3 - Node 3 Verification:
```bash
redis-cli -h localhost -p 8082

# Should be able to read data from both Node 1 and Node 2
127.0.0.1:8082> GET replication:test
# Expected: "from_node_1"

127.0.0.1:8082> GET cluster:data
# Expected: "distributed_value"

127.0.0.1:8082> GET node2:exclusive
# Expected: "only_on_node2"

# Set data from Node 3
127.0.0.1:8082> SET node3:data "from_third_node"
```

### Delete Replication Test

```bash
# Terminal 1 (Node 1) - Set a key
redis-cli -h localhost -p 8080
127.0.0.1:8080> SET delete:test "will_be_deleted"

# Terminal 2 (Node 2) - Verify it exists
redis-cli -h localhost -p 8081
127.0.0.1:8081> GET delete:test
# Expected: "will_be_deleted"

# Terminal 3 (Node 3) - Delete the key
redis-cli -h localhost -p 8082
127.0.0.1:8082> DEL delete:test
# Expected: (integer) 1

# Terminal 1 (Node 1) - Verify deletion replicated
127.0.0.1:8080> GET delete:test
# Expected: (nil)

# Terminal 2 (Node 2) - Verify deletion replicated
127.0.0.1:8081> GET delete:test
# Expected: (nil)
```

## Advanced Test Scenarios

### Hash Ring Distribution Test

```bash
# Connect to Node 1
redis-cli -h localhost -p 8080

# Set multiple keys that should distribute across nodes
127.0.0.1:8080> SET user:001 "Alice"
127.0.0.1:8080> SET user:002 "Bob"
127.0.0.1:8080> SET user:003 "Charlie"
127.0.0.1:8080> SET user:004 "Diana"
127.0.0.1:8080> SET user:005 "Eve"
127.0.0.1:8080> SET product:laptop "MacBook Pro"
127.0.0.1:8080> SET product:phone "iPhone"
127.0.0.1:8080> SET config:app "production"

# Verify all keys are accessible from any node
# Connect to Node 2
redis-cli -h localhost -p 8081
127.0.0.1:8081> GET user:001
127.0.0.1:8081> GET user:002
127.0.0.1:8081> GET product:laptop
127.0.0.1:8081> GET config:app

# Connect to Node 3
redis-cli -h localhost -p 8082
127.0.0.1:8082> GET user:003
127.0.0.1:8082> GET user:004
127.0.0.1:8082> GET product:phone
127.0.0.1:8082> GET config:app
```

### Concurrent Operations Test

```bash
# Terminal 1 (Node 1)
redis-cli -h localhost -p 8080
127.0.0.1:8080> SET concurrent:key1 "from_node1"

# Terminal 2 (Node 2) - Simultaneously
redis-cli -h localhost -p 8081
127.0.0.1:8081> SET concurrent:key2 "from_node2"

# Terminal 3 (Node 3) - Simultaneously
redis-cli -h localhost -p 8082
127.0.0.1:8082> SET concurrent:key3 "from_node3"

# Verify all operations succeeded and replicated
# From any terminal:
GET concurrent:key1
GET concurrent:key2
GET concurrent:key3
# All should return their respective values
```

## Complete Manual Test Script

### Step-by-Step Testing Procedure

```bash
# 1. Start the cluster
./scripts/build-and-run.sh cluster

# 2. Wait for cluster formation
sleep 5

# 3. Test basic connectivity
redis-cli -h localhost -p 8080 ping
redis-cli -h localhost -p 8081 ping
redis-cli -h localhost -p 8082 ping

# 4. Open 3 terminals for concurrent testing
```

#### Terminal 1 (Node 1 Testing):
```bash
redis-cli -h localhost -p 8080

# Basic operations
SET test:key1 "value_from_node1"
SET shared:data "replicated_value"
SET numbers:one 1
GET test:key1
GET shared:data
```

#### Terminal 2 (Node 2 Testing):
```bash
redis-cli -h localhost -p 8081

# Verify replication
GET test:key1
# Expected: "value_from_node1"

GET shared:data
# Expected: "replicated_value"

# Add more data
SET test:key2 "value_from_node2"
SET numbers:two 2
```

#### Terminal 3 (Node 3 Testing):
```bash
redis-cli -h localhost -p 8082

# Verify all data is available
GET test:key1
# Expected: "value_from_node1"

GET test:key2
# Expected: "value_from_node2"

GET shared:data
# Expected: "replicated_value"

# Test deletion
DEL test:key1
DEL numbers:one
```

#### Verification (Any Terminal):
```bash
# Verify deletions replicated
GET test:key1
# Expected: (nil)

GET numbers:one
# Expected: (nil)

# Verify other data still exists
GET test:key2
# Expected: "value_from_node2"

GET shared:data
# Expected: "replicated_value"
```

### Persistence Test (If Enabled)

```bash
# 1. Set some data
redis-cli -h localhost -p 8080
SET persist:test "should_survive_restart"
SET persist:counter 100
SET persist:config "important_setting"

# 2. Stop cluster
./scripts/build-and-run.sh stop

# 3. Restart cluster
./scripts/build-and-run.sh cluster
sleep 5

# 4. Reconnect and verify data persisted
redis-cli -h localhost -p 8080
GET persist:test
# Expected: "should_survive_restart"

GET persist:counter
# Expected: "100"

GET persist:config
# Expected: "important_setting"
```

## Expected Behaviors

### ✅ Successful Operations Should Show:

1. **SET commands**: Return `OK`
2. **GET commands**: Return the stored value or `(nil)` for non-existent keys
3. **DEL commands**: Return `(integer) N` where N is the number of keys deleted
4. **Replication**: Data set on one node should be readable from all nodes
5. **Delete Replication**: Keys deleted from one node should be removed from all nodes
6. **Persistence**: Data should survive cluster restarts (if persistence enabled)
7. **Connectivity**: `PING` should return `PONG`

### ❌ Issues to Watch For:

1. **Connection refused**: Node might not be running or wrong port
2. **Data not replicated**: Check cluster formation and event bus
3. **Inconsistent data**: Possible replication lag or failure
4. **Keys not deleted everywhere**: DELETE replication issue
5. **Data lost after restart**: Persistence not working or not enabled

## Troubleshooting

### Connection Issues

```bash
# Check if nodes are running
ps aux | grep hypercache

# Check which ports are in use
lsof -i :8080
lsof -i :8081
lsof -i :8082

# Try connecting with timeout
redis-cli -h localhost -p 8080 --connect-timeout 5 ping
```

### Cluster Issues

```bash
# Check startup logs
tail -f node1_startup.log
tail -f node2_startup.log
tail -f node3_startup.log

# Stop and restart cluster
./scripts/build-and-run.sh stop
sleep 2
./scripts/build-and-run.sh cluster
```

### Data Consistency Issues

```bash
# Check the same key from all nodes
redis-cli -h localhost -p 8080 GET test:key
redis-cli -h localhost -p 8081 GET test:key
redis-cli -h localhost -p 8082 GET test:key

# If values differ, there's a replication issue
```

### Performance Testing

```bash
# Test with redis-benchmark (if available)
redis-benchmark -h localhost -p 8080 -t set,get -n 1000 -c 10

# Or manual performance test
redis-cli -h localhost -p 8080 --latency
```

## Additional Commands

### Information and Debugging

```bash
# Get server info (if implemented)
redis-cli -h localhost -p 8080 INFO

# Test with different data types
SET string:test "hello"
SET number:test 42

# Batch operations
redis-cli -h localhost -p 8080 --pipe < commands.txt
```

### Cleanup Commands

```bash
# Stop cluster when done testing
./scripts/build-and-run.sh stop

# Clean up persistence data
./scripts/clean-persistence.sh

# Full cleanup
./scripts/final-cleanup.sh
```

---

## Quick Reference

| Command | Purpose |
|---------|---------|
| `SET key value` | Store a key-value pair |
| `GET key` | Retrieve value for a key |
| `DEL key [key ...]` | Delete one or more keys |
| `PING` | Test connection |
| `INFO` | Get server information |

| Node | RESP Port | HTTP Port |
|------|-----------|-----------|
| Node 1 | 8080 | 9080 |
| Node 2 | 8081 | 9081 |
| Node 3 | 8082 | 9082 |

This comprehensive guide should help you thoroughly test your HyperCache distributed functionality using redis-cli!
