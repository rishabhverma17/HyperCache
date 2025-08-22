#!/bin/bash

echo "🧹 Cleaning up any existing processes..."
pkill -f hypercache 2>/dev/null || true
sleep 2

echo "🚀 Starting HyperCache nodes with proper configuration..."

echo "Starting Node 1 (RESP: 8080, HTTP: 9080, Gossip: 7946)..."
./bin/hypercache -config configs/node1-config.yaml -node-id node-1 -protocol resp &
NODE1_PID=$!
echo "Node 1 PID: $NODE1_PID"
sleep 3

echo "Starting Node 2 (RESP: 8081, HTTP: 9081, Gossip: 7947)..."
./bin/hypercache -config configs/node2-config.yaml -node-id node-2 -protocol resp &
NODE2_PID=$!
echo "Node 2 PID: $NODE2_PID"
sleep 3

echo "Starting Node 3 (RESP: 8082, HTTP: 9082, Gossip: 7948)..."
./bin/hypercache -config configs/node3-config.yaml -node-id node-3 -protocol resp &
NODE3_PID=$!
echo "Node 3 PID: $NODE3_PID"
sleep 5  # Give more time for cluster formation

echo "✅ All nodes started!"
echo "PIDs: Node1=$NODE1_PID, Node2=$NODE2_PID, Node3=$NODE3_PID"
echo ""

echo "🧪 Testing connectivity..."
echo "Node 1 HTTP health check (port 9080):"
curl -m 5 -X GET "http://localhost:9080/health" 2>/dev/null && echo "✅ Node 1 OK" || echo "❌ Node 1 not responding"

echo "Node 2 HTTP health check (port 9081):"
curl -m 5 -X GET "http://localhost:9081/health" 2>/dev/null && echo "✅ Node 2 OK" || echo "❌ Node 2 not responding"

echo "Node 3 HTTP health check (port 9082):"
curl -m 5 -X GET "http://localhost:9082/health" 2>/dev/null && echo "✅ Node 3 OK" || echo "❌ Node 3 not responding"

echo ""
echo "🧪 Testing RESP connectivity..."
echo "Node 1 RESP ping (port 8080):"
redis-cli -h localhost -p 8080 ping 2>/dev/null && echo "✅ Node 1 RESP OK" || echo "❌ Node 1 RESP not responding"

echo "Node 2 RESP ping (port 8081):"
redis-cli -h localhost -p 8081 ping 2>/dev/null && echo "✅ Node 2 RESP OK" || echo "❌ Node 2 RESP not responding"

echo "Node 3 RESP ping (port 8082):"
redis-cli -h localhost -p 8082 ping 2>/dev/null && echo "✅ Node 3 RESP OK" || echo "❌ Node 3 RESP not responding"

echo ""
echo "📋 All logs are now in the main log files:"
echo "- logs/node-1.log"
echo "- logs/node-2.log" 
echo "- logs/node-3.log"
echo ""
echo "💡 To monitor logs in real-time:"
echo "   tail -f logs/node-1.log"
echo "   tail -f logs/node-2.log"
echo "   tail -f logs/node-3.log"
