#!/bin/bash
# =============================================================================
# HyperCache Server Benchmark Script
# Uses redis-benchmark against a running HyperCache RESP server
# No Docker required — just Go and redis-benchmark (comes with redis)
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY="$PROJECT_DIR/bin/hypercache"
CONFIG="$PROJECT_DIR/configs/hypercache.yaml"
RESULTS_DIR="$PROJECT_DIR/benchmark-results"
PORT=8080
PID_FILE="/tmp/hypercache-bench.pid"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    if [ -f "$PID_FILE" ]; then
        kill "$(cat "$PID_FILE")" 2>/dev/null || true
        rm -f "$PID_FILE"
    fi
    pkill -f "bin/hypercache.*bench-node" 2>/dev/null || true
}
trap cleanup EXIT

# Check prerequisites
if ! command -v redis-benchmark &>/dev/null; then
    echo -e "${RED}redis-benchmark not found. Install redis:${NC}"
    echo "  brew install redis"
    exit 1
fi

if ! command -v redis-cli &>/dev/null; then
    echo -e "${RED}redis-cli not found. Install redis:${NC}"
    echo "  brew install redis"
    exit 1
fi

# Build
echo -e "${YELLOW}Building HyperCache...${NC}"
cd "$PROJECT_DIR"
make build 2>&1 | tail -1

# Kill any existing instances
pkill -f "bin/hypercache.*bench-node" 2>/dev/null || true
sleep 1

# Start server
echo -e "${YELLOW}Starting HyperCache server on port $PORT...${NC}"
"$BINARY" -protocol resp -node-id bench-node -config "$CONFIG" > /dev/null 2>&1 &
echo $! > "$PID_FILE"

# Wait for server to be ready
echo -n "Waiting for server..."
for i in $(seq 1 30); do
    if redis-cli -p "$PORT" PING 2>/dev/null | grep -q PONG; then
        echo -e " ${GREEN}ready!${NC}"
        break
    fi
    echo -n "."
    sleep 1
done

# Verify server responds
if ! redis-cli -p "$PORT" PING 2>/dev/null | grep -q PONG; then
    echo -e "\n${RED}Server failed to start on port $PORT${NC}"
    exit 1
fi

# Create results directory
mkdir -p "$RESULTS_DIR"
RESULT_FILE="$RESULTS_DIR/server-benchmarks-$(date +%Y%m%d-%H%M%S).txt"

echo "================================================================" | tee "$RESULT_FILE"
echo "HYPERCACHE SERVER BENCHMARK" | tee -a "$RESULT_FILE"
echo "Date: $(date)" | tee -a "$RESULT_FILE"
echo "Platform: $(uname -m), $(sysctl -n hw.ncpu 2>/dev/null || nproc) cores" | tee -a "$RESULT_FILE"
echo "Go: $(go version | awk '{print $3}')" | tee -a "$RESULT_FILE"
echo "redis-benchmark: $(redis-benchmark --version 2>&1 | head -1)" | tee -a "$RESULT_FILE"
echo "================================================================" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 1: Single client baseline
# ===========================================================================
# NOTE: All tests use -P (pipeline) because redis-benchmark's CONFIG probe
# on the same connection causes a connection reuse issue with HyperCache's
# RESP parser. This is a known limitation (tracked in bottlenecks doc).
# -P 1 = one command at a time (equivalent to no pipelining)

echo -e "${GREEN}[1/7] Single client, 100K requests — baseline latency${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 1 -n 100000 -t set,get -P 1 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 2: 10 concurrent clients
# ===========================================================================
echo -e "${GREEN}[2/7] 10 concurrent clients, 100K requests${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 10 -n 100000 -t set,get -P 1 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 3: 50 concurrent clients (redis-benchmark default)
# ===========================================================================
echo -e "${GREEN}[3/7] 50 concurrent clients, 100K requests${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 50 -n 100000 -t set,get -P 1 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 4: 100 concurrent clients
# ===========================================================================
echo -e "${GREEN}[4/7] 100 concurrent clients, 100K requests${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 100 -n 100000 -t set,get -P 1 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 5: Pipelined (16 commands per batch)
# ===========================================================================
echo -e "${GREEN}[5/7] 50 clients, 100K requests, pipeline=16${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 50 -n 100000 -t set,get -P 16 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 6: Pipelined (64 commands per batch)
# ===========================================================================
echo -e "${GREEN}[6/7] 50 clients, 100K requests, pipeline=64${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 50 -n 100000 -t set,get -P 64 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# ===========================================================================
# Benchmark 6: Random keys (realistic workload)
# ===========================================================================
echo -e "${GREEN}[7/7] 50 clients, 500K requests, random keys, pipeline=16${NC}" | tee -a "$RESULT_FILE"
redis-benchmark -p "$PORT" -c 50 -n 500000 -r 100000 -t set,get -P 16 -q --csv 2>&1 | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

echo "================================================================" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "================================================================" | tee -a "$RESULT_FILE"

echo -e "\n${GREEN}Done!${NC} Results: $RESULT_FILE"
