# MOVED Response Logging Verification Guide

## 🎯 Overview
This guide helps you test the comprehensive MOVED response logging we implemented for Redis-style client routing.

## 🔄 What We're Testing
When a client connects to the wrong node for a key, the system should:
1. Calculate the correct hash slot for the key
2. Determine which node owns that slot
3. Send a MOVED response to the client
4. **Log comprehensive metadata about the redirect**

## 🧪 Step-by-Step Testing

### Step 1: Monitor Logs in Real-Time
Open a terminal and run:
```bash
cd /Users/rishabhverma/Documents/HobbyProjects/Cache
tail -f logs/node-*.log | grep -E "(cluster_redirect|MOVED)"
```

### Step 2: Test Basic Connectivity
In another terminal, verify cluster is running:
```bash
# Test RESP ports
nc -zv localhost 8080 8081 8082

# Test HTTP APIs  
curl http://localhost:9080/health
curl http://localhost:9081/health
curl http://localhost:9082/health
```

### Step 3: Connect to Node 1 and Test Cross-Node Keys
```bash
redis-cli -h localhost -p 8080
```

Then execute these commands (each will likely trigger MOVED responses):
```redis
SET user:456 "test_data_456"
SET user:789 "test_data_789" 
SET session:abc "session_data"
SET product:111 "laptop_details"
SET cache:xyz "temporary_cache"
GET user:456
GET user:789
DEL session:abc
```

### Step 4: Expected Log Output
You should see logs like this in your monitoring terminal:

```json
{
  "timestamp": "2025-08-25T17:16:00Z",
  "level": "INFO",
  "component": "resp",
  "action": "cluster_redirect",
  "message": "MOVED response sent for SET command",
  "key": "user:456",
  "command": "SET", 
  "hash_slot": 8532,
  "local_node": "node-1",
  "target_node": "node-2",
  "target_address": "localhost:8081",
  "client_redirect": true,
  "value_size": 12
}
```

### Step 5: Test Other Nodes
Repeat the process connecting to other nodes:

**Node 2:**
```bash
redis-cli -h localhost -p 8081
SET user:123 "node2_test"
GET product:222
```

**Node 3:**
```bash
redis-cli -h localhost -p 8082
SET user:999 "node3_test"
DEL cache:temp
```

## 📊 Log Analysis

### Key Logging Features to Verify:

1. **INFO Level Logs**: All MOVED responses are logged at INFO level
2. **Complete Metadata**: Each log contains:
   - `key`: The Redis key being accessed
   - `command`: GET/SET/DEL command type
   - `hash_slot`: Calculated CRC16 hash slot (0-16383)
   - `local_node`: Current node identifier
   - `target_node`: Correct node for the key
   - `target_address`: Full address for redirect
   - `client_redirect`: Always true for MOVED responses
   - `value_size`: Size of value (for SET commands)

3. **DEBUG Level Logs**: Local operations (when key belongs to current node)

## 🔍 Troubleshooting

### If No MOVED Logs Appear:
1. **Keys might be on the correct node**: Try different key patterns
2. **Single node cluster**: Verify all 3 nodes are running
3. **Log level**: Ensure logging is set to INFO or DEBUG level

### Common Key Patterns That Trigger MOVED:
- `user:{random_numbers}` - High chance of cross-node access
- `session:{random_letters}` - Distributed across nodes  
- `cache:{uuid}` - Random distribution
- `product:{id}` - Number-based distribution

## ✅ Success Criteria

The test is successful when you see:
- [ ] MOVED responses logged with full metadata
- [ ] Different hash slots calculated for different keys
- [ ] Correct target node identification
- [ ] Client redirect information captured
- [ ] Both SET and GET operations logged
- [ ] Value size captured for SET operations

## 🎯 Key Benefits Demonstrated

1. **Observability**: Clear visibility into client redirections
2. **Debugging**: Full context for troubleshooting routing issues
3. **Metrics**: Data for monitoring cluster balance and client behavior
4. **Performance**: Understanding redirect patterns for optimization

## 📈 Next Steps

After verifying the logging works:
1. Monitor logs during normal application load
2. Set up log aggregation for production monitoring
3. Create alerts for excessive MOVED responses
4. Analyze key distribution patterns for optimization
