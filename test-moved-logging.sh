#!/bin/bash

echo "🧪 Testing MOVED Response Logging Enhancements"
echo "=============================================="

# Start a single node for testing
echo "Starting HyperCache node for testing..."
./bin/hypercache -config configs/test-config.yaml -node-id test-node -protocol resp &
TEST_PID=$!
echo "Test node PID: $TEST_PID"

# Wait for startup
sleep 3

# Test MOVED response logging (should generate logs)
echo ""
echo "📋 Testing MOVED response logging..."
echo "Note: All operations will succeed locally since we have only one node"
echo "To see MOVED responses, you need a multi-node cluster"

# Test basic operations (will be handled locally)
redis-cli -h localhost -p 8080 SET test:key1 "value1" || echo "Redis CLI not available"
redis-cli -h localhost -p 8080 GET test:key1 || echo "Redis CLI not available"
redis-cli -h localhost -p 8080 DEL test:key1 || echo "Redis CLI not available"

echo ""
echo "🔍 Checking log output for local operations..."
echo "Look for DEBUG messages about local operations in the node logs"

# Cleanup
sleep 2
echo ""
echo "🧹 Cleaning up..."
kill $TEST_PID 2>/dev/null || true
wait $TEST_PID 2>/dev/null || true

echo ""
echo "✅ Test completed!"
echo "To see MOVED response logs, start a 3-node cluster and use:"
echo "  ./scripts/start-cluster.sh"
echo "Then connect to one node and access keys that belong to other nodes"
