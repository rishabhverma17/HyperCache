#!/bin/bash
# Start a 3-node local cluster for integration testing
set -e

BASEDIR="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="$BASEDIR/bin/hypercache"

# Kill any existing instances
pkill -f "bin/hypercache" 2>/dev/null || true
sleep 1

# Clean old data
rm -rf /tmp/hypercache
mkdir -p /tmp/hypercache

echo "Starting node-1 (RESP:8080 HTTP:9080 Gossip:6080)..."
"$BINARY" -config "$BASEDIR/configs/node-1-config.yaml" -protocol resp > /tmp/hypercache/node1.log 2>&1 &
NODE1_PID=$!
sleep 2

echo "Starting node-2 (RESP:8081 HTTP:9081 Gossip:6081)..."
"$BINARY" -config "$BASEDIR/configs/node-2-config.yaml" -protocol resp > /tmp/hypercache/node2.log 2>&1 &
NODE2_PID=$!
sleep 2

echo "Starting node-3 (RESP:8082 HTTP:9082 Gossip:6082)..."
"$BINARY" -config "$BASEDIR/configs/node-3-config.yaml" -protocol resp > /tmp/hypercache/node3.log 2>&1 &
NODE3_PID=$!
sleep 3

echo ""
echo "=== HEALTH CHECK ==="
echo "Node1: $(curl -s http://localhost:9080/health)"
echo "Node2: $(curl -s http://localhost:9081/health)"
echo "Node3: $(curl -s http://localhost:9082/health)"
echo ""
echo "=== QUICK SMOKE TEST ==="
echo "PUT test:key -> node1:"
curl -s -X PUT http://localhost:9080/api/cache/test:key -H 'Content-Type: application/json' -d '{"value":"hello-world"}'
echo ""
echo "GET test:key <- node1:"
curl -s http://localhost:9080/api/cache/test:key
echo ""
sleep 2
echo "GET test:key <- node2 (cross-node):"
curl -s http://localhost:9081/api/cache/test:key
echo ""
echo "GET test:key <- node3 (cross-node):"
curl -s http://localhost:9082/api/cache/test:key
echo ""
echo ""
echo "PIDs: node1=$NODE1_PID node2=$NODE2_PID node3=$NODE3_PID"
echo "Logs: /tmp/hypercache/node{1,2,3}.log"
echo "To stop: pkill -f bin/hypercache"
