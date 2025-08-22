# HyperCache HTTP API Documentation

## Base URL
```
http://localhost:9080  # Node 1
http://localhost:9081  # Node 2  
http://localhost:9082  # Node 3
```

## Authentication
Currently no authentication required. All endpoints are publicly accessible.

---

## Cache Operations

### GET - Retrieve Value
Retrieve a value from the cache.

**Endpoint:** `GET /api/cache/{key}`

**Parameters:**
- `key` (path): The cache key to retrieve

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "key": "mykey",
  "value": "myvalue",
  "ttl_remaining": "3540s",
  "node": "node-1",
  "correlation_id": "uuid-here"
}
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "Key not found",
  "key": "nonexistent",
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X GET http://localhost:9080/api/cache/mykey
```

---

### PUT - Set Value
Set a key-value pair in the cache with optional TTL.

**Endpoint:** `PUT /api/cache/{key}`

**Parameters:**
- `key` (path): The cache key to set

**Request Body:**
```json
{
  "value": "string",
  "ttl_hours": 1.0    // Optional, defaults to 1 hour
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "key": "mykey",
  "value": "myvalue", 
  "ttl_hours": 1.0,
  "replicated": true,
  "node": "node-1",
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X PUT http://localhost:9080/api/cache/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "Hello World", "ttl_hours": 2.0}'
```

---

### DELETE - Remove Value
Remove a key from the cache.

**Endpoint:** `DELETE /api/cache/{key}`

**Parameters:**
- `key` (path): The cache key to delete

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "key": "mykey",
  "deleted": true,
  "replicated": true,
  "node": "node-1",
  "correlation_id": "uuid-here"
}
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "Key not found",
  "key": "nonexistent",
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:9080/api/cache/mykey
```

---

## Health & Status

### Health Check
Check if the node is healthy and operational.

**Endpoint:** `GET /health`

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "healthy": true,
  "node": "node-1",
  "cluster_size": 3,
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X GET http://localhost:9080/health
```

---

## Cluster Information

### Cluster Status
Get detailed information about the cluster state.

**Endpoint:** `GET /api/cluster/status`

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "cluster_name": "hypercache",
  "node_id": "node-1",
  "cluster_size": 3,
  "nodes": [
    {
      "id": "node-1",
      "address": "127.0.0.1:7946",
      "status": "alive",
      "role": "active"
    },
    {
      "id": "node-2", 
      "address": "127.0.0.1:7947",
      "status": "alive",
      "role": "active"
    },
    {
      "id": "node-3",
      "address": "127.0.0.1:7948", 
      "status": "alive",
      "role": "active"
    }
  ],
  "replication_factor": 3,
  "consistency_level": "eventual",
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X GET http://localhost:9080/api/cluster/status
```

### Node Metrics
Get performance metrics for this node.

**Endpoint:** `GET /api/node/metrics`

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "node_id": "node-1",
  "uptime": "2h34m12s",
  "cache_stats": {
    "total_keys": 1523,
    "memory_usage": "45.2MB",
    "hit_rate": 0.847,
    "operations_per_second": 234.5
  },
  "replication_stats": {
    "events_published": 156,
    "events_received": 312,
    "replication_lag_avg": "12ms"
  },
  "persistence_stats": {
    "aof_size": "12.3MB",
    "last_snapshot": "2025-08-21T17:30:00Z",
    "recovery_time": "340ms"
  },
  "filter_stats": {
    "false_positive_rate": 0.008,
    "filter_size": "2.1MB",
    "total_insertions": 1523,
    "total_lookups": 8745
  },
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X GET http://localhost:9080/api/node/metrics
```

---

## Configuration

### Get Configuration
Retrieve current node configuration (read-only).

**Endpoint:** `GET /api/config`

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "node": {
    "id": "node-1",
    "data_dir": "/tmp/hypercache/node-1"
  },
  "network": {
    "resp_port": 8080,
    "http_port": 9080,
    "gossip_port": 7946
  },
  "cluster": {
    "seeds": ["127.0.0.1:7946", "127.0.0.1:7947", "127.0.0.1:7948"],
    "replication_factor": 3,
    "consistency_level": "eventual"
  },
  "cache": {
    "max_memory": "8GB",
    "default_ttl": "1h",
    "cuckoo_filter_fpp": 0.01
  },
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X GET http://localhost:9080/api/config
```

---

## Bulk Operations

### Batch Get
Retrieve multiple keys in a single request.

**Endpoint:** `POST /api/cache/batch/get`

**Request Body:**
```json
{
  "keys": ["key1", "key2", "key3", "key4"]
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "results": [
    {
      "key": "key1",
      "value": "value1",
      "found": true,
      "ttl_remaining": "3540s"
    },
    {
      "key": "key2", 
      "found": false,
      "error": "Key not found"
    },
    {
      "key": "key3",
      "value": "value3", 
      "found": true,
      "ttl_remaining": "1800s"
    }
  ],
  "node": "node-1",
  "total_requested": 3,
  "total_found": 2,
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X POST http://localhost:9080/api/cache/batch/get \
  -H "Content-Type: application/json" \
  -d '{"keys": ["key1", "key2", "key3"]}'
```

### Batch Set
Set multiple key-value pairs in a single request.

**Endpoint:** `POST /api/cache/batch/set`

**Request Body:**
```json
{
  "items": [
    {
      "key": "key1",
      "value": "value1",
      "ttl_hours": 2.0
    },
    {
      "key": "key2", 
      "value": "value2",
      "ttl_hours": 1.0
    }
  ]
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "results": [
    {
      "key": "key1",
      "success": true,
      "replicated": true
    },
    {
      "key": "key2",
      "success": true, 
      "replicated": true
    }
  ],
  "node": "node-1",
  "total_processed": 2,
  "total_successful": 2,
  "correlation_id": "uuid-here"
}
```

**Example:**
```bash
curl -X POST http://localhost:9080/api/cache/batch/set \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"key": "key1", "value": "value1", "ttl_hours": 2.0},
      {"key": "key2", "value": "value2", "ttl_hours": 1.0}
    ]
  }'
```

---

## Error Handling

All endpoints return consistent error responses:

### Common Error Codes
- `400` - Bad Request (invalid JSON, missing fields)
- `404` - Not Found (key doesn't exist)
- `500` - Internal Server Error (system failure)
- `503` - Service Unavailable (node unhealthy)

### Error Response Format
```json
{
  "error": "Detailed error message",
  "code": "ERROR_CODE", 
  "correlation_id": "uuid-here",
  "timestamp": "2025-08-21T17:53:16.266861Z"
}
```

---

## Request/Response Headers

### Common Request Headers
```http
Content-Type: application/json
Accept: application/json
User-Agent: your-application/1.0
```

### Common Response Headers
```http
Content-Type: application/json
X-Correlation-ID: uuid-here
X-Node-ID: node-1
X-Response-Time: 12ms
```

---

## Examples & Use Cases

### Basic CRUD Operations
```bash
# Set a value
curl -X PUT http://localhost:9080/api/cache/user:123 \
  -H "Content-Type: application/json" \
  -d '{"value": "{\"name\":\"John\",\"age\":30}", "ttl_hours": 24}'

# Get the value
curl -X GET http://localhost:9080/api/cache/user:123

# Delete the value  
curl -X DELETE http://localhost:9080/api/cache/user:123
```

### Session Management
```bash
# Store session data
curl -X PUT http://localhost:9080/api/cache/session:abc123 \
  -H "Content-Type: application/json" \
  -d '{"value": "{\"user_id\":123,\"expires\":1693234567}", "ttl_hours": 8}'
```

### Cache Warming
```bash
# Batch load cache
curl -X POST http://localhost:9080/api/cache/batch/set \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"key": "config:app", "value": "{\"theme\":\"dark\"}", "ttl_hours": 168},
      {"key": "config:db", "value": "{\"pool_size\":10}", "ttl_hours": 168}
    ]
  }'
```

### Health Monitoring
```bash
# Check cluster health
for port in 9080 9081 9082; do
  echo "Node $port:"
  curl -X GET http://localhost:$port/health
  echo ""
done
```

---

## Rate Limits & Performance

- **Throughput**: ~10,000 requests/sec per node (hardware dependent)
- **Batch Size**: Maximum 100 items per batch operation
- **Key Size**: Maximum 250 characters
- **Value Size**: Maximum 1MB per value
- **TTL Range**: 1 second to 365 days

---

## Integration Examples

### Python
```python
import requests
import json

# HyperCache client
class HyperCache:
    def __init__(self, base_url="http://localhost:9080"):
        self.base_url = base_url
    
    def get(self, key):
        response = requests.get(f"{self.base_url}/api/cache/{key}")
        return response.json() if response.status_code == 200 else None
    
    def set(self, key, value, ttl_hours=1.0):
        data = {"value": value, "ttl_hours": ttl_hours}
        response = requests.put(f"{self.base_url}/api/cache/{key}", json=data)
        return response.status_code == 200
    
    def delete(self, key):
        response = requests.delete(f"{self.base_url}/api/cache/{key}")
        return response.status_code == 200

# Usage
cache = HyperCache()
cache.set("user:123", '{"name":"John","age":30}', ttl_hours=2.0)
user = cache.get("user:123")
```

### Node.js
```javascript
const axios = require('axios');

class HyperCache {
  constructor(baseUrl = 'http://localhost:9080') {
    this.baseUrl = baseUrl;
  }

  async get(key) {
    try {
      const response = await axios.get(`${this.baseUrl}/api/cache/${key}`);
      return response.data;
    } catch (error) {
      return null;
    }
  }

  async set(key, value, ttlHours = 1.0) {
    try {
      const response = await axios.put(`${this.baseUrl}/api/cache/${key}`, {
        value,
        ttl_hours: ttlHours
      });
      return response.status === 200;
    } catch (error) {
      return false;
    }
  }

  async delete(key) {
    try {
      const response = await axios.delete(`${this.baseUrl}/api/cache/${key}`);
      return response.status === 200;
    } catch (error) {
      return false;
    }
  }
}

// Usage
const cache = new HyperCache();
await cache.set('user:123', JSON.stringify({name: 'John', age: 30}), 2.0);
const user = await cache.get('user:123');
```
