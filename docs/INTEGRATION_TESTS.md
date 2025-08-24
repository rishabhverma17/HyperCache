# Integration Tests for HyperCache

This document describes the integration test suite for HyperCache sanity checks.

## Overview

The integration test suite (`scripts/integration-tests.sh`) provides comprehensive testing of HyperCache's distributed cache functionality using HTTP API calls. These tests validate core distributed cache operations without requiring external dependencies like `redis-cli`.

## Test Suite Architecture

### No External Dependencies
- Uses only `curl` and standard Unix tools (available in CI environments)
- Self-contained cluster management (start/stop test nodes)
- JSON response parsing using grep/cut commands
- Automatic base64 decoding for replicated data

### Test Configuration
- **Cluster Size**: 3 nodes
- **Ports**: HTTP API (9080-9082), RESP (8080-8082), Gossip (10080-10082)
- **Test Data Directory**: `./test-data/`
- **Results Directory**: `./integration-test-results/`

## Test Cases

### Test 1: HTTP API Health Check ✅
**Purpose**: Validate basic connectivity and cluster formation

**What it tests**:
- HTTP health endpoints respond correctly
- All nodes report healthy status
- Cluster formation successful

**Expected Results**:
- HTTP 200 responses from `/health` on all nodes
- JSON response indicates healthy cluster state

### Test 2: Basic Cache Operations ✅
**Purpose**: Validate fundamental cache operations on a single node

**What it tests**:
- **PUT**: Store key-value pairs with TTL
- **GET**: Retrieve stored values
- **DELETE**: Remove keys from cache
- **Verification**: Confirm deletion (404 on GET)

**Expected Results**:
- PUT operations return success status
- GET operations return correct values
- DELETE operations remove keys completely

### Test 3: Cross-Node Data Access ✅
**Purpose**: Validate distributed cache replication

**What it tests**:
- Store data on one node
- Retrieve same data from other nodes
- Data consistency across cluster
- Replication timing and reliability

**Expected Results**:
- Data stored on Node 1 accessible from Nodes 2 and 3
- Values match exactly (handles both plain text and base64 encoding)
- Replication completes within 2 seconds

### Test 4: Multiple Key Operations ✅
**Purpose**: Validate cluster load balancing and distribution

**What it tests**:
- Store multiple keys across different nodes
- Retrieve keys from different nodes than storage
- Cross-node key distribution
- Concurrent operations handling

**Expected Results**:
- All keys stored successfully across cluster
- All keys retrievable from any node
- Load balanced across cluster nodes

### Test 5: TTL (Time To Live) Functionality ✅
**Purpose**: Validate expiration behavior

**What it tests**:
- Store keys with short TTL (2 seconds)
- Immediate retrieval (should succeed)
- Post-expiration retrieval (behavioral check)

**Expected Results**:
- Keys accessible immediately after storage
- TTL behavior documented (may vary by implementation)

## Running Integration Tests

### Standalone Execution
```bash
./scripts/integration-tests.sh
```

### As Part of Build Process
```bash
./scripts/build-and-run.sh test
```

### CI Integration
Integration tests run automatically in GitHub Actions:
- After successful compilation
- Before deployment/release
- Results uploaded as artifacts

## Test Output

### Success Output
```
┌─────────────────────────────────────────┐
│    HyperCache Integration Tests         │
│         Sanity Test Suite               │
└─────────────────────────────────────────┘

Integration Test Summary:
Passed: 15
Failed: 0
Total:  15

✓ All integration tests passed!
```

### Log Files
- `integration-test-results/integration_tests.log` - Test execution log
- `integration-test-results/node-*.log` - Individual node logs
- `integration-test-results/debug.log` - Debug information for failures

## Troubleshooting

### Common Issues

1. **Port Conflicts**
   ```bash
   # Kill existing processes
   pkill -f hypercache
   lsof -ti :9080,:9081,:9082 | xargs kill -9
   ```

2. **Binary Not Found**
   ```bash
   # Build the binary first
   ./scripts/build-and-run.sh build
   ```

3. **Cluster Formation Issues**
   - Check firewall settings for gossip ports (10080-10082)
   - Verify localhost connectivity
   - Allow more time for cluster formation (increase sleep values)

### Debug Information

The test suite provides comprehensive debug logging:
- HTTP response codes and bodies
- JSON parsing results
- Base64 decoding attempts
- Timing information
- Node startup/shutdown status

### Test Customization

Key variables in `integration-tests.sh`:
```bash
NODE_COUNT=3                    # Number of test nodes
TEST_PORTS=(8080 8081 8082)    # RESP ports
HTTP_PORTS=(9080 9081 9082)    # HTTP API ports
GOSSIP_PORTS=(10080 10081 10082) # Gossip ports
```

## Integration with CI/CD

### GitHub Actions
The integration tests are automatically executed in the CI pipeline:

```yaml
- name: Run integration tests
  run: |
    chmod +x scripts/integration-tests.sh
    ./scripts/integration-tests.sh
  timeout-minutes: 10
```

### Test Artifacts
- Test results uploaded to GitHub Actions artifacts
- Available for debugging failed builds
- Retained for 7 days

## Best Practices

1. **Run tests after code changes** affecting cache operations
2. **Check logs** if tests fail to understand root cause
3. **Clean up** test artifacts between runs
4. **Verify cluster health** before running tests
5. **Monitor timing** for performance regressions

## Future Enhancements

- **Performance benchmarking** integration
- **Failure recovery** testing scenarios  
- **Network partition** simulation
- **Load testing** under high concurrency
- **Persistence validation** across restarts

This integration test suite provides a solid foundation for validating HyperCache's core distributed cache functionality and ensures reliable behavior across different environments.