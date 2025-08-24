#!/bin/bash

# HyperCache Build and Run Script
# This script builds all the binaries and provides options         # Start three nodes with proper configuration
        echo -e "${YELLOW}Starting Node 1 (RESP: 8080, HTTP: 9080, Gossip: 7946)...${NC}"
        ./bin/hypercache -config configs/node1-config.yaml -node-id node-1 -protocol resp &
        NODE1_PID=$!
        echo "Node 1 PID: $NODE1_PID"
        sleep 3
        
        echo -e "${YELLOW}Starting Node 2 (RESP: 8081, HTTP: 9081, Gossip: 7947)...${NC}"
        ./bin/hypercache -config configs/node2-config.yaml -node-id node-2 -protocol resp &
        NODE2_PID=$!
        echo "Node 2 PID: $NODE2_PID"
        sleep 3
        
        echo -e "${YELLOW}Starting Node 3 (RESP: 8082, HTTP: 9082, Gossip: 7948)...${NC}"
        ./bin/hypercache -config configs/node3-config.yaml -node-id node-3 -protocol resp &
        NODE3_PID=$!
        echo "Node 3 PID: $NODE3_PID"t color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Function to display help
show_help() {
    echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│         HyperCache Build & Run          │${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"
    echo -e "${YELLOW}Usage:${NC}"
    echo "  ./scripts/build-and-run.sh [command]"
    echo ""
    echo -e "${YELLOW}Commands:${NC}"
    echo "  build             Build all binaries"
    echo "  run <node-id>     Run a single node (e.g., node-1, node-2, node-3)"
    echo "  cluster           Start a 3-node cluster"
    echo "  stop              Stop all running hypercache nodes"
    echo "  clean             Clean up build artifacts and logs"
    echo "  test              Run basic tests"
    echo "  benchmark         Run performance benchmarks"
    echo "  help              Show this help"
    echo ""
    echo -e "${YELLOW}Examples:${NC}"
    echo "  ./scripts/build-and-run.sh build"
    echo "  ./scripts/build-and-run.sh run node-1"
    echo "  ./scripts/build-and-run.sh cluster"
    echo "  ./scripts/build-and-run.sh stop"
}

# Function to build binaries
build_binaries() {
    echo -e "${BLUE}Building HyperCache binary...${NC}"
    
    # Ensure bin directory exists
    mkdir -p bin
    
    # Build main hypercache binary
    echo -e "${YELLOW}Building main hypercache binary...${NC}"
    go build -o bin/hypercache cmd/hypercache/main.go
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Successfully built hypercache${NC}"
    else
        echo -e "${RED}✗ Failed to build hypercache${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}HyperCache binary built successfully!${NC}"
}

# Function to run a single node
run_node() {
    if [ -z "$1" ]; then
        echo -e "${RED}Error: Node ID required${NC}"
        echo "Usage: ./scripts/build-and-run.sh run <node-id>"
        exit 1
    fi
    
    NODE_ID=$1
    
    # Determine port based on node ID
    case $NODE_ID in
        "node-1")
            PORT=8080
            ;;
        "node-2")
            PORT=8081
            ;;
        "node-3")
            PORT=8082
            ;;
        *)
            echo -e "${RED}Error: Unknown node ID. Use node-1, node-2, or node-3${NC}"
            exit 1
            ;;
    esac
    
    echo -e "${BLUE}Starting HyperCache node ${NODE_ID} on port ${PORT}...${NC}"
    ./bin/hypercache -node-id $NODE_ID -protocol resp -port $PORT
}

# Function to start a 3-node cluster
run_cluster() {
    echo -e "${BLUE}Starting HyperCache 3-node cluster...${NC}"
    
    # Run the start-cluster script if it exists
    if [ -f "./scripts/start-cluster.sh" ]; then
        ./scripts/start-cluster.sh
    else
        echo -e "${RED}Error: start-cluster.sh script not found${NC}"
        
        # Create a backup method to start cluster
        echo -e "${YELLOW}Using backup method to start cluster...${NC}"
        
        # Kill any existing hypercache processes
        pkill -f hypercache
        sleep 2
        
        # Start three nodes with proper configuration
        echo -e "${YELLOW}Starting Node 1 (RESP: 8080, HTTP: 9080)...${NC}"
        ./bin/hypercache -config configs/node1-config.yaml -node-id node-1 -protocol resp > node1_startup.log 2>&1 &
        NODE1_PID=$!
        echo "Node 1 PID: $NODE1_PID"
        sleep 3
        
        echo -e "${YELLOW}Starting Node 2 (RESP: 8081, HTTP: 9081)...${NC}"
        ./bin/hypercache -config configs/node2-config.yaml -node-id node-2 -protocol resp > node2_startup.log 2>&1 &
        NODE2_PID=$!
        echo "Node 2 PID: $NODE2_PID"
        sleep 3
        
        echo -e "${YELLOW}Starting Node 3 (RESP: 8082, HTTP: 9082)...${NC}"
        ./bin/hypercache -config configs/node3-config.yaml -node-id node-3 -protocol resp > node3_startup.log 2>&1 &
        NODE3_PID=$!
        echo "Node 3 PID: $NODE3_PID"
        sleep 5  # Give more time for cluster formation
        
        echo -e "${GREEN}✓ All nodes started!${NC}"
        echo "PIDs: Node1=$NODE1_PID, Node2=$NODE2_PID, Node3=$NODE3_PID"
        echo ""
        
        # Test connectivity
        echo -e "${BLUE}Testing connectivity...${NC}"
        echo "Node 1 HTTP API (port 9080):"
        curl -m 5 -s -X GET "http://localhost:9080/health" || echo -e "${RED}❌ Node 1 HTTP API not responding${NC}"
        
        echo "Node 2 HTTP API (port 9081):"
        curl -m 5 -s -X GET "http://localhost:9081/health" || echo -e "${RED}❌ Node 2 HTTP API not responding${NC}"
        
        echo "Node 3 HTTP API (port 9082):"
        curl -m 5 -s -X GET "http://localhost:9082/health" || echo -e "${RED}❌ Node 3 HTTP API not responding${NC}"
    fi
}

# Function to stop all hypercache nodes
stop_cluster() {
    echo -e "${BLUE}Stopping all HyperCache nodes...${NC}"
    
    # Find all hypercache processes
    PIDS=$(pgrep -f hypercache)
    
    if [ -z "$PIDS" ]; then
        echo -e "${YELLOW}No HyperCache processes found running${NC}"
        return 0
    fi
    
    echo -e "${YELLOW}Found HyperCache processes: $PIDS${NC}"
    
    # Try graceful shutdown first
    echo -e "${YELLOW}Attempting graceful shutdown...${NC}"
    pkill -TERM -f hypercache
    sleep 3
    
    # Check if any processes are still running
    REMAINING_PIDS=$(pgrep -f hypercache)
    
    if [ -n "$REMAINING_PIDS" ]; then
        echo -e "${YELLOW}Some processes still running, forcing shutdown...${NC}"
        pkill -KILL -f hypercache
        sleep 1
    fi
    
    # Final check
    FINAL_PIDS=$(pgrep -f hypercache)
    if [ -z "$FINAL_PIDS" ]; then
        echo -e "${GREEN}✓ All HyperCache nodes stopped successfully${NC}"
    else
        echo -e "${RED}✗ Some processes may still be running: $FINAL_PIDS${NC}"
    fi
}

# Function to clean up
clean_up() {
    echo -e "${BLUE}Cleaning up...${NC}"
    if [ -f "./scripts/final-cleanup.sh" ]; then
        ./scripts/final-cleanup.sh
    else
        echo -e "${RED}Error: final-cleanup.sh script not found${NC}"
        exit 1
    fi
}

# Function to run basic tests
run_tests() {
    echo -e "${BLUE}Running tests...${NC}"
    echo -e "${YELLOW}Running Go tests...${NC}"
    go test ./internal/... -v
    
    if [ -f "./scripts/test-bench.sh" ]; then
        echo -e "${YELLOW}Running unified test and benchmark script...${NC}"
        ./scripts/test-bench.sh test all
    fi
    
    if [ -f "./scripts/integration-tests.sh" ]; then
        echo -e "${YELLOW}Running integration tests for sanity checks...${NC}"
        ./scripts/integration-tests.sh
    fi
}

# Function to run benchmarks
run_benchmarks() {
    echo -e "${BLUE}Running benchmarks...${NC}"
    echo -e "${YELLOW}Running Go benchmarks...${NC}"
    go test ./internal/... -bench=. -benchmem
    
    if [ -f "./scripts/test-bench.sh" ]; then
        echo -e "${YELLOW}Running unified benchmark script...${NC}"
        ./scripts/test-bench.sh benchmark
    fi
}

# Main script logic
case "$1" in
    "build")
        build_binaries
        ;;
    "run")
        run_node "$2"
        ;;
    "cluster")
        run_cluster
        ;;
    "stop")
        stop_cluster
        ;;
    "clean")
        clean_up
        ;;
    "test")
        run_tests
        ;;
    "benchmark")
        run_benchmarks
        ;;
    "help")
        show_help
        ;;
    *)
        show_help
        ;;
esac
