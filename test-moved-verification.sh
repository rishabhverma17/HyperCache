#!/bin/bash

echo "🎯 MOVED Response Logging Verification Test"
echo "=========================================="

# Test cluster connectivity
echo "📡 Testing cluster connectivity..."
for port in 8080 8081 8082; do
    echo "Testing Node on port $port..."
    timeout 2 bash -c "</dev/tcp/localhost/$port" 2>/dev/null && echo "✅ Port $port: OPEN" || echo "❌ Port $port: CLOSED"
done

echo ""
echo "📋 Testing HTTP API connectivity..."
for port in 9080 9081 9082; do
    echo "Testing HTTP API on port $port..."
    curl -s -m 2 http://localhost:$port/health >/dev/null 2>&1 && echo "✅ HTTP $port: UP" || echo "❌ HTTP $port: DOWN"
done

echo ""
echo "🧪 Testing MOVED Response Scenarios"
echo "===================================="

# Create a test script to simulate Redis commands that will trigger MOVED responses
cat > /tmp/test_moved_commands.txt << 'EOF'
# These keys will hash to different slots and likely different nodes
SET user:123 "profile_data_node1"
SET user:456 "profile_data_node2"  
SET user:789 "profile_data_node3"
SET session:abc "session_data_1"
SET session:def "session_data_2"
SET product:111 "laptop_details"
SET product:222 "phone_details"
SET cache:xyz "temporary_data"
EOF

echo "📝 Test commands prepared. Here's what we'll test:"
cat /tmp/test_moved_commands.txt

echo ""
echo "🔑 Hash Slot Calculations (for reference):"
echo "These keys will map to different hash slots and potentially different nodes:"

# Use a simple method to show which keys might trigger MOVED responses
echo "user:123    -> Will be hashed to slot X"
echo "user:456    -> Will be hashed to slot Y"  
echo "user:789    -> Will be hashed to slot Z"
echo "session:abc -> Will be hashed to slot A"
echo "product:111 -> Will be hashed to slot B"

echo ""
echo "🎮 Manual Testing Instructions:"
echo "=============================="
echo "1. Connect to Node 1 and try to access keys that belong to other nodes:"
echo "   redis-cli -h localhost -p 8080"
echo "   Then try: SET user:456 \"test_data\""
echo ""
echo "2. Connect to Node 2 and try keys for other nodes:"
echo "   redis-cli -h localhost -p 8081"
echo "   Then try: GET user:123"
echo ""
echo "3. Connect to Node 3 and try keys for other nodes:"
echo "   redis-cli -h localhost -p 8082"
echo "   Then try: SET product:111 \"laptop\""

echo ""
echo "📊 Expected Log Output Pattern:"
echo "==============================="
cat << 'EOF'
When a MOVED response occurs, you should see logs like:

{
  "timestamp": "2025-08-25T17:15:30Z",
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
  "value_size": 9
}
EOF

echo ""
echo "🔍 Log Monitoring Commands:"
echo "============================"
echo "Watch logs in real-time:"
echo "  tail -f logs/node-1.log | grep cluster_redirect"
echo "  tail -f logs/node-2.log | grep cluster_redirect"
echo "  tail -f logs/node-3.log | grep cluster_redirect"

echo ""
echo "Or watch all MOVED events across all nodes:"
echo "  tail -f logs/node-*.log | grep -E '(MOVED|cluster_redirect)'"

echo ""
echo "🎯 Testing Workflow:"
echo "==================="
echo "1. Open multiple terminals"
echo "2. In terminal 1: tail -f logs/node-*.log | grep cluster_redirect"
echo "3. In terminal 2: redis-cli -h localhost -p 8080"
echo "4. In terminal 2: Execute commands that will trigger MOVED responses"
echo "5. Watch terminal 1 for the logging output"

echo ""
echo "✅ Test preparation complete!"
echo "Now manually test with redis-cli to see MOVED response logging in action."

# Cleanup
rm -f /tmp/test_moved_commands.txt
