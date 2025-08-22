# HyperCache Structured Logging System

## Overview

We've implemented a comprehensive structured logging system for HyperCache that provides:

- **Correlation IDs**: Random GUIDs that flow through entire request lifecycle
- **Structured JSON Logs**: Machine-readable logs for Grafana and Kibana integration
- **Component-based Logging**: Organized logging across all internal components
- **Performance Optimized**: Asynchronous, non-blocking logging
- **Production Ready**: Log rotation, persistence, and configurable levels

## Features Implemented

### 1. Correlation ID Flow ✅
- Automatic generation of correlation IDs for all requests
- HTTP middleware that adds `X-Correlation-ID` headers
- Context propagation throughout the entire request lifecycle
- Preserved across all component boundaries

### 2. Structured JSON Logging ✅
```json
{
  "@timestamp": "2025-08-21T20:30:15.123Z",
  "level": "INFO",
  "message": "HTTP request started",
  "correlation_id": "550e8400-e29b-41d4-a716-446655440000",
  "node_id": "node-1",
  "component": "http",
  "action": "request",
  "duration_ms": 45,
  "fields": {
    "method": "GET",
    "path": "/cache/default/key1",
    "status_code": 200,
    "remote_ip": "127.0.0.1:54321"
  },
  "file": "main.go",
  "line": 234,
  "function": "handleCacheRequest"
}
```

### 3. Component Coverage ✅
Logging is implemented across all major components:

- **RESP Protocol**: Request/response logging with timing
- **HTTP API**: Full request lifecycle with correlation IDs
- **Cluster Management**: Node join/leave, gossip events
- **Event Bus**: Event publishing and subscription
- **Persistence**: AOF operations, snapshots, recovery
- **Storage**: Cache operations, evictions, memory pressure
- **Coordinator**: Distributed coordination events
- **Main System**: Startup, shutdown, configuration

### 4. Action-based Logging ✅
Standard actions across all components:
- `start`, `stop`, `request`, `response`
- `connect`, `disconnect`, `join`, `leave`
- `replication`, `persist`, `restore`, `snapshot`
- `election`, `consensus`, `sync`, `validation`
- `timeout`, `retry`, `failover`, `cleanup`

### 5. Configuration System ✅
```yaml
logging:
  level: "info"              # debug, info, warn, error, fatal
  enable_console: true       # Console output
  enable_file: true         # File persistence
  log_dir: "logs"           # Log directory
  buffer_size: 1000         # Async buffer size
  max_file_size: "100MB"    # File rotation size
  max_files: 10             # Retention policy
```

### 6. Performance Optimizations ✅
- **Asynchronous Processing**: Non-blocking log writes
- **Buffered Channel**: Configurable buffer size (default 1000)
- **Multiple Writers**: Console + File output simultaneously
- **Graceful Shutdown**: Ensures all logs are flushed
- **Memory Efficient**: JSON marshaling with minimal allocations

## Integration Points

### HTTP Middleware
```go
// Automatic correlation ID injection
router.Use(logging.HTTPMiddleware)
```

### Component Logging
```go
// Standard usage pattern
logging.Info(ctx, logging.ComponentCache, logging.ActionRequest, 
    "Cache operation started", map[string]interface{}{
        "key": key,
        "store": storeName,
        "operation": "GET",
    })
```

### Timing Operations
```go
// Performance measurement
timer := logging.StartTimer(ctx, logging.ComponentStorage, 
    logging.ActionPersist, "Snapshot creation")
defer timer()
```

### Error Logging
```go
// Error with correlation ID flow
logging.Error(ctx, logging.ComponentCluster, logging.ActionJoin, 
    "Failed to join cluster", err, map[string]interface{}{
        "seeds": seedNodes,
        "retry_count": retries,
    })
```

## Grafana Integration Ready

### Log Format
- **@timestamp**: ISO8601 timestamp for time-based queries
- **level**: For alert filtering (ERROR, WARN levels)
- **correlation_id**: For request tracing across services
- **node_id**: For multi-node cluster analysis
- **component**: For service-level dashboards
- **action**: For operation-specific metrics
- **duration_ms**: For performance dashboards

### Recommended Grafana Queries
```promql
# Request latency by component
histogram_quantile(0.95, 
  sum(rate(hypercache_duration_ms_bucket[5m])) by (component, le))

# Error rate by node
sum(rate(hypercache_logs{level="ERROR"}[5m])) by (node_id)

# Correlation ID tracing
hypercache_logs{correlation_id="550e8400-e29b-41d4-a716-446655440000"}
```

## Kibana Integration Ready

### Index Pattern
```
hypercache-logs-*
```

### Key Fields for Search
- `correlation_id` - Full request tracing
- `component` + `action` - Service operation analysis
- `node_id` - Multi-node cluster monitoring
- `level` - Error analysis and alerting
- `fields.*` - Context-specific data

### Sample Kibana Queries
```
# All errors in the last hour
level:ERROR AND @timestamp:[now-1h TO now]

# Cluster events
component:cluster AND (action:join OR action:leave)

# Slow operations (>1000ms)
duration_ms:>1000

# Specific request trace
correlation_id:"550e8400-e29b-41d4-a716-446655440000"
```

## Log Persistence Strategy

### File Organization
```
logs/
├── node-1.log          # Node-specific logs
├── node-2.log          # Separate per node
├── node-3.log          # For cluster deployments
└── archived/           # Rotated logs
    ├── node-1.log.1
    └── node-1.log.2
```

### Retention Policy
- **Max File Size**: 100MB per node
- **Max Files**: 10 files per node (1GB total per node)
- **Rotation**: Automatic when size limit reached
- **Compression**: Available for archived logs

## Testing and Validation

### Log Validation Scripts
```bash
# Test correlation ID flow
./scripts/test-correlation-flow.sh

# Validate JSON format
./scripts/validate-log-format.sh

# Performance impact assessment
./scripts/logging-performance-test.sh
```

### Sample Log Validation
```bash
# Ensure all logs are valid JSON
cat logs/node-1.log | jq . >/dev/null && echo "Valid JSON"

# Check correlation ID presence
grep -c "correlation_id" logs/node-1.log

# Verify component coverage
cat logs/node-1.log | jq -r '.component' | sort | uniq
```

## Benefits Achieved

### 1. **Production Observability** ✅
- Complete request tracing with correlation IDs
- Performance monitoring with duration tracking
- Error tracking with context preservation
- Component-level service monitoring

### 2. **Debugging & Troubleshooting** ✅
- Correlation ID enables end-to-end request tracing
- Structured fields provide rich context
- Component/action organization simplifies log analysis
- File persistence enables historical analysis

### 3. **Performance Monitoring** ✅
- Built-in timing for all operations
- Non-blocking architecture prevents performance impact
- Configurable verbosity levels
- Metrics-friendly JSON structure

### 4. **Compliance & Audit** ✅
- Immutable log records with timestamps
- Complete audit trail of all operations
- Configurable retention policies
- Structured format for compliance tools

## Next Steps for Advanced Features

### 1. **Log Shipping** (Future Enhancement)
- Filebeat/Fluentd integration
- Direct Elasticsearch shipping
- Kafka log streaming
- Cloud logging integration (CloudWatch, Stackdriver)

### 2. **Advanced Analytics** (Future Enhancement)
- Real-time anomaly detection
- Predictive failure analysis
- Automated alert correlation
- Business intelligence dashboards

### 3. **Compliance Features** (Future Enhancement)
- Log signing and verification
- Encrypted log storage
- GDPR compliance features
- Audit report generation

## Configuration Examples

### Development Environment
```yaml
logging:
  level: "debug"
  enable_console: true
  enable_file: true
  buffer_size: 100
```

### Production Environment
```yaml
logging:
  level: "info"
  enable_console: false  # Only file logging
  enable_file: true
  buffer_size: 5000     # Larger buffer
  max_file_size: "500MB"
  max_files: 20         # Longer retention
```

### High-Volume Environment
```yaml
logging:
  level: "warn"         # Reduce volume
  enable_console: false
  enable_file: true
  buffer_size: 10000    # Large buffer
  max_file_size: "1GB"
  max_files: 50
```

---

**Status**: ✅ **COMPLETE**

The structured logging system is fully implemented, tested, and ready for production deployment with Grafana/Kibana integration.
