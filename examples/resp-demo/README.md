# HyperCache RESP Demo

This demo shows how to use HyperCache with any Redis client library. It demonstrates Redis protocol compatibility and showcases various cache operations through the standard Redis Go client.

## Overview

The demo connects to a HyperCache RESP server using the popular `github.com/redis/go-redis/v9` client library, proving that HyperCache is fully compatible with existing Redis tooling and applications.

## Features Demonstrated

- **Basic Operations**: SET, GET, EXISTS, DEL
- **TTL Operations**: Expiration and time-based eviction
- **Bulk Operations**: Multiple key operations
- **Concurrent Access**: Multi-goroutine stress testing
- **Server Information**: Database size and server stats
- **Database Management**: FLUSHALL operations

## Prerequisites

1. **Go 1.19+** installed on your system
2. **HyperCache RESP server** running (see instructions below)

## Quick Start

### 1. Start HyperCache RESP Server

In your main project directory:

```bash
# Navigate to the main project
cd /Users/rishabhverma/Documents/HobbyProjects/Cache

# Run the HyperCache server with RESP protocol
go run cmd/hypercache/main.go --config configs/hypercache.yaml --protocol resp --port 6379
```

The server will start on `localhost:6379` (standard Redis port).

### 2. Run the Demo

In a new terminal:

```bash
# Navigate to the demo directory
cd examples/resp-demo

# Install dependencies
go mod tidy

# Run the demo
go run main.go
```

## Expected Output

The demo will run through various test scenarios:

```
🚀 HyperCache RESP Server Demo
==============================
Testing connection... ✅ Connected successfully

=== Testing Basic Operations ===
Testing PING... ✅ Response: PONG
Testing SET... ✅ Key set successfully
Testing GET... ✅ Retrieved: Hello HyperCache!
Testing EXISTS... ✅ Exists: 1 keys
Testing DEL... ✅ Deleted: 1 keys
Verifying deletion... ✅ Key correctly deleted

=== Testing TTL Operations ===
Setting key with 3 second TTL... ✅ Key set with TTL
Checking key immediately... ✅ Retrieved: This will expire
Waiting 4 seconds for expiration... ✅ Key correctly expired

=== Testing Bulk Operations ===
Setting multiple keys... ✅ Multiple keys set
Checking existence of multiple keys... ✅ Found 3 out of 4 keys
Deleting multiple keys... ✅ Deleted 3 keys

=== Testing Concurrent Operations ===
Running 10 goroutines with 20 operations each... ✅ All 10 workers completed successfully

=== Testing Server Information ===
Getting server info... ✅ Server info retrieved:
   # Server...
Getting database size... ✅ Database size: 2 keys

=== Testing Database Operations ===
Adding test data... ✅ Test data added
Checking database size... ✅ Database size: 5 keys
Flushing database... ✅ Database flushed
Verifying database is empty... ✅ Database size after flush: 0 keys

🎉 All tests completed successfully!
HyperCache RESP server is working correctly with Redis clients.
```

## Configuration

The demo connects to `localhost:6379` by default. You can modify the connection settings in `main.go`:

```go
// Create Redis client - works with any Redis-compatible server
client := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379", // Change this to your server address
    Password: "",               // No password for demo
    DB:       0,               // Default DB
    
    // Connection settings
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
    
    // Pool settings for better performance
    PoolSize:           10,
    MinIdleConns:       3,
    MaxConnAge:         30 * time.Minute,
    PoolTimeout:        4 * time.Second,
    IdleTimeout:        5 * time.Minute,
    IdleCheckFrequency: time.Minute,
})
```

## Troubleshooting

### Connection Failed

```
❌ Failed to connect to HyperCache server: dial tcp [::1]:6379: connect: connection refused
```

**Solution**: Make sure the HyperCache RESP server is running:

```bash
# In the main project directory
go run cmd/hypercache/main.go --config configs/hypercache.yaml --protocol resp --port 6379
```

### Command Not Implemented

If you see warnings like:
```
⚠️ INFO command may not be fully implemented: ERR unknown command 'INFO'
```

This means the RESP server doesn't implement that specific Redis command yet. This is normal - HyperCache implements core Redis commands for caching operations.

### Dependencies Issue

```bash
# Clean and reinstall dependencies
go mod tidy
go mod download
```

## Integration with Existing Applications

This demo proves that HyperCache can be used as a drop-in replacement for Redis in existing applications. Simply:

1. Start HyperCache RESP server on your desired port
2. Point your Redis client configuration to the HyperCache server
3. Your application will work without code changes!

Example integration patterns:

```go
// Web application cache
func (app *WebApp) initCache() {
    app.cache = redis.NewClient(&redis.Options{
        Addr: "hypercache-server:6379", // Your HyperCache server
    })
}

// Session storage
func (s *SessionStore) Set(sessionID, data string) error {
    return s.client.Set(ctx, sessionID, data, 30*time.Minute).Err()
}

// Rate limiting
func (rl *RateLimiter) CheckLimit(userID string) (bool, error) {
    current, err := rl.client.Incr(ctx, "rate:"+userID).Result()
    if err != nil {
        return false, err
    }
    if current == 1 {
        rl.client.Expire(ctx, "rate:"+userID, time.Hour)
    }
    return current <= rl.maxRequests, nil
}
```

## Performance Notes

The demo includes concurrent testing with multiple goroutines to demonstrate HyperCache's thread-safety and performance characteristics. The RESP protocol adds minimal overhead while providing full Redis compatibility.

For production use, consider:
- Connection pooling (already configured in demo)
- Appropriate timeouts for your use case
- Monitoring and observability
- Clustering configuration for distributed deployments

## Next Steps

- Explore the HyperCache source code to understand the implementation
- Check out the performance benchmarks in the main project
- Review the distributed architecture documentation
- Try integrating HyperCache into your existing applications
