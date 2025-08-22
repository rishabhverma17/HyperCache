#!/bin/bash

# HyperCache Test and Benchmark Script
# This script runs comprehensive tests and benchmarks for the HyperCache system

# Set color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│   HyperCache Test & Benchmark Suite     │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"

# Constants
HYPERCACHE_BIN="./bin/hypercache"
BENCH_OUTPUT_DIR="./benchmark-results"
TEST_OUTPUT_DIR="./test-results"
CONFIG_DIR="./configs"
DATA_DIR="./data"
TEST_CONFIG="$CONFIG_DIR/test-config.yaml"

# Create output directories
mkdir -p $BENCH_OUTPUT_DIR
mkdir -p $TEST_OUTPUT_DIR
mkdir -p $DATA_DIR/node-{1,2,3}

# Function to check if a binary exists
check_binary() {
    if [ ! -f "$1" ]; then
        echo -e "${RED}${BOLD}Error:${NC} Binary $1 not found. Run build script first."
        exit 1
    fi
}

# Function to run a test and report results
run_test() {
    local test_name=$1
    local test_command=$2
    local test_output="$TEST_OUTPUT_DIR/${test_name}.log"
    
    echo -e "${YELLOW}Running test: ${BOLD}$test_name${NC}"
    echo -e "${BLUE}Command: $test_command${NC}"
    echo -e "${BLUE}Output: $test_output${NC}"
    
    # Run test and capture output
    eval "$test_command" > "$test_output" 2>&1
    
    # Check if test passed
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Test $test_name PASSED${NC}"
    else
        echo -e "${RED}✗ Test $test_name FAILED${NC}"
        echo -e "${YELLOW}See $test_output for details${NC}"
    fi
    echo ""
}

# Function to run a benchmark and report results
run_benchmark() {
    local bench_name=$1
    local bench_command=$2
    local bench_output="$BENCH_OUTPUT_DIR/${bench_name}.log"
    
    echo -e "${YELLOW}Running benchmark: ${BOLD}$bench_name${NC}"
    echo -e "${BLUE}Command: $bench_command${NC}"
    echo -e "${BLUE}Output: $bench_output${NC}"
    
    # Run benchmark and capture output
    eval "$bench_command" > "$bench_output" 2>&1
    
    # Display summary
    echo -e "${GREEN}Benchmark $bench_name complete${NC}"
    echo -e "${YELLOW}Results summary:${NC}"
    grep -A 10 "RESULTS" "$bench_output" || echo "No results summary found"
    echo ""
}

# Function to start a cluster of nodes
start_cluster() {
    local node_count=$1
    local ports_base=$2
    local cluster_name=$3
    
    echo -e "${YELLOW}Starting $node_count-node cluster: ${BOLD}$cluster_name${NC}"
    
    # Start each node
    for (( i=1; i<=$node_count; i++ ))
    do
        local resp_port=$(($ports_base + $i - 1))
        local http_port=$(($resp_port + 1000))
        local gossip_port=$(($resp_port + 2000))
        
        echo -e "${BLUE}Starting node-$i (RESP:$resp_port, HTTP:$http_port, Gossip:$gossip_port)${NC}"
        
        # Construct seed list for cluster nodes
        local seeds=""
        if [ $i -gt 1 ]; then
            seeds="--cluster.seeds=127.0.0.1:$(($ports_base + 2000))"
        fi
        
        # Start node with appropriate settings
        # Create node-specific config
        cp $TEST_CONFIG "$CONFIG_DIR/node-$i-test-config.yaml"
        # Update data directory in the config
        sed -i.bak "s|data_dir:.*|data_dir: \"$DATA_DIR/node-$i\"|g" "$CONFIG_DIR/node-$i-test-config.yaml"
        # Update bind address in the config
        sed -i.bak "s|bind_addr:.*|bind_addr: \"127.0.0.1:$resp_port\"|g" "$CONFIG_DIR/node-$i-test-config.yaml"
        
        $HYPERCACHE_BIN -node-id=node-$i \
                         -port=$resp_port \
                         -protocol=resp \
                         -config="$CONFIG_DIR/node-$i-test-config.yaml" \
                         > "$TEST_OUTPUT_DIR/${cluster_name}_node${i}.log" 2>&1 &
        
        # Store PID for later cleanup
        eval "${cluster_name}_node${i}_pid=$!"
    done
    
    # Give cluster time to form
    echo -e "${BLUE}Waiting for cluster to form...${NC}"
    sleep 5
    echo -e "${GREEN}Cluster $cluster_name started${NC}"
}

# Function to stop a cluster
stop_cluster() {
    local cluster_name=$1
    
    echo -e "${YELLOW}Stopping cluster: ${BOLD}$cluster_name${NC}"
    
    # Get all variables matching the pattern
    pids=$(compgen -v "${cluster_name}_node" | grep "_pid$")
    
    for pid_var in $pids
    do
        pid=${!pid_var}
        echo -e "${BLUE}Stopping process $pid${NC}"
        kill -15 $pid 2>/dev/null || echo -e "${RED}Process $pid not found${NC}"
    done
    
    echo -e "${GREEN}Cluster $cluster_name stopped${NC}"
    sleep 2
}

# Function to run HTTP API tests
run_http_api_tests() {
    local resp_port=$1
    local test_name=$2
    
    echo -e "${YELLOW}Running API tests against RESP port ${BOLD}$resp_port${NC}"
    
    # Test SET operation
    echo -e "${BLUE}Testing SET operation${NC}"
    redis-cli -p $resp_port SET test-key test-value > /dev/null
    
    # Test GET operation
    echo -e "${BLUE}Testing GET operation${NC}"
    result=$(redis-cli -p $resp_port GET test-key)
    
    # Verify result
    if [[ "$result" == "test-value" ]]; then
        echo -e "${GREEN}✓ GET test passed${NC}"
    else
        echo -e "${RED}✗ GET test failed${NC}"
    fi
    
    # Test DELETE operation
    echo -e "${BLUE}Testing DELETE operation${NC}"
    redis-cli -p $resp_port DEL test-key > /dev/null
    
    # Verify DELETE worked
    result=$(redis-cli -p $resp_port GET test-key)
    if [[ -z "$result" ]]; then
        echo -e "${GREEN}✓ DELETE test passed${NC}"
    else
        echo -e "${RED}✗ DELETE test failed${NC}"
    fi
}

# Function to run replication tests
run_replication_tests() {
    local primary_port=$1
    local secondary_port=$2
    local test_name=$3
    
    echo -e "${YELLOW}Running replication tests from port ${BOLD}$primary_port${NC} to port ${BOLD}$secondary_port${NC}"
    
    # Use redis-cli for testing
    if ! command -v redis-cli &> /dev/null; then
        echo -e "${RED}redis-cli not found. Install Redis tools to run this test.${NC}"
        return 1
    fi
    
    # Test SET on primary
    echo -e "${BLUE}Setting key on primary node${NC}"
    redis-cli -p $primary_port SET repl-key replication-test > /dev/null
    
    # Give time for replication
    sleep 5
    
    # Test GET on secondary
    echo -e "${BLUE}Getting key from secondary node${NC}"
    result=$(redis-cli -p $secondary_port GET repl-key)
    
    # Verify replication worked
    if [[ "$result" == "replication-test" ]]; then
        echo -e "${GREEN}✓ Replication SET test passed${NC}"
    else
        echo -e "${RED}✗ Replication SET test failed${NC}"
        echo -e "${YELLOW}Result from secondary: '$result'${NC}"
    fi
    
    # Test DELETE on primary
    echo -e "${BLUE}Deleting key on primary node${NC}"
    redis-cli -p $primary_port DEL repl-key > /dev/null
    
    # Give time for replication
    sleep 5
    
    # Verify DELETE replication worked
    result=$(redis-cli -p $secondary_port GET repl-key)
    if [[ -z "$result" ]]; then
        echo -e "${GREEN}✓ Replication DELETE test passed${NC}"
    else
        echo -e "${RED}✗ Replication DELETE test failed${NC}"
        echo -e "${YELLOW}Result from secondary: '$result'${NC}"
    fi
}

# Function to run persistence tests
run_persistence_tests() {
    local resp_port=$1
    local cluster_name=$2
    local node_id=$3
    
    echo -e "${YELLOW}Running persistence tests against port ${BOLD}$resp_port${NC}"
    
    # Use redis-cli for testing
    if ! command -v redis-cli &> /dev/null; then
        echo -e "${RED}redis-cli not found. Install Redis tools to run this test.${NC}"
        return 1
    fi
    
    # Set test keys
    echo -e "${BLUE}Setting persistence test keys${NC}"
    for i in {1..5}; do
        redis-cli -p $resp_port SET "persist-key-$i" "persist-value-$i" > /dev/null
    done
    
    # Force AOF persistence sync
    echo -e "${BLUE}Forcing persistence sync${NC}"
    redis-cli -p $resp_port SAVE > /dev/null 2>&1 || true
    
    # Wait for persistence to complete
    sleep 3
    
    # Stop the node
    echo -e "${BLUE}Stopping node to test persistence${NC}"
    stop_cluster "${cluster_name}"
    
    # Wait for complete shutdown
    sleep 3
    
    # Restart the node with same data directory
    echo -e "${BLUE}Restarting node to verify persistence${NC}"
    start_cluster 1 $(( 8079 + ($node_id - 1) * 10 )) "${cluster_name}_restart"
    
    # Give time to start up and load data
    sleep 10
    
    # Check if data persisted
    failures=0
    echo -e "${BLUE}Verifying persisted data${NC}"
    for i in {1..5}; do
        result=$(redis-cli -p $(( 8079 + ($node_id - 1) * 10 )) GET "persist-key-$i")
        if [[ "$result" == "persist-value-$i" ]]; then
            echo -e "${GREEN}✓ Key persist-key-$i persisted correctly${NC}"
        else
            echo -e "${RED}✗ Key persist-key-$i not persisted correctly${NC}"
            echo -e "${YELLOW}Result: '$result'${NC}"
            failures=$((failures+1))
        fi
    done
    
    # Final result
    if [ $failures -eq 0 ]; then
        echo -e "${GREEN}✓ All persistence tests passed${NC}"
    else
        echo -e "${RED}✗ $failures persistence tests failed${NC}"
    fi
    
    # Clean up
    stop_cluster "${cluster_name}_restart"
}

# Function to run benchmark test with redis-benchmark
run_redis_benchmark() {
    local port=$1
    local test_name=$2
    local num_requests=$3
    local clients=$4
    
    echo -e "${YELLOW}Running Redis benchmark on port ${BOLD}$port${NC}"
    
    # Check if redis-benchmark is installed
    if ! command -v redis-benchmark &> /dev/null; then
        echo -e "${RED}redis-benchmark not found. Install Redis tools to run this benchmark.${NC}"
        return 1
    fi
    
    # Run benchmark
    redis-benchmark -p $port -n $num_requests -c $clients -t set,get,del \
        --csv > "$BENCH_OUTPUT_DIR/${test_name}_redis_benchmark.csv"
    
    # Display summary
    echo -e "${GREEN}Redis benchmark complete.${NC}"
    echo -e "${YELLOW}Results stored in $BENCH_OUTPUT_DIR/${test_name}_redis_benchmark.csv${NC}"
    
    # Parse and display summary
    awk -F, '{printf "%-10s: %s ops/sec\n", $2, $5}' "$BENCH_OUTPUT_DIR/${test_name}_redis_benchmark.csv"
}

# Function to run custom load generation
run_custom_load() {
    local base_url=$1
    local test_name=$2
    local num_requests=$3
    local concurrent=$4
    
    echo -e "${YELLOW}Running custom load test against ${BOLD}$base_url${NC}"
    
    # Check if hey is installed
    if ! command -v hey &> /dev/null; then
        echo -e "${YELLOW}hey load testing tool not found. Using curl for basic load testing.${NC}"
        
        # Use a basic curl loop instead
        start_time=$(date +%s)
        
        # PUT requests
        echo -e "${BLUE}Running $num_requests PUT requests...${NC}"
        for (( i=1; i<=$num_requests; i++ ))
        do
            curl -s -X PUT "$base_url/api/cache/bench-key-$i" -H "Content-Type: application/json" -d "{\"value\": \"bench-value-$i\"}" > /dev/null
            if [ $(($i % 100)) -eq 0 ]; then
                echo -e "${BLUE}Progress: $i/$num_requests${NC}"
            fi
        done
        
        # GET requests
        echo -e "${BLUE}Running $num_requests GET requests...${NC}"
        for (( i=1; i<=$num_requests; i++ ))
        do
            curl -s "$base_url/api/cache/bench-key-$i" > /dev/null
            if [ $(($i % 100)) -eq 0 ]; then
                echo -e "${BLUE}Progress: $i/$num_requests${NC}"
            fi
        done
        
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        
        echo -e "${GREEN}Custom load test complete.${NC}"
        echo -e "${YELLOW}Completed $((num_requests * 2)) operations in $duration seconds${NC}"
        echo -e "${YELLOW}Average rate: $(( (num_requests * 2) / (duration == 0 ? 1 : duration) )) ops/sec${NC}"
        
        # Save results
        echo "Operations: $((num_requests * 2))" > "$BENCH_OUTPUT_DIR/${test_name}_custom_load.log"
        echo "Duration: $duration seconds" >> "$BENCH_OUTPUT_DIR/${test_name}_custom_load.log"
        echo "Rate: $(( (num_requests * 2) / (duration == 0 ? 1 : duration) )) ops/sec" >> "$BENCH_OUTPUT_DIR/${test_name}_custom_load.log"
    else
        # Use hey for load testing
        echo -e "${BLUE}Running PUT benchmark...${NC}"
        hey -n $num_requests -c $concurrent -m PUT -H "Content-Type: application/json" -d '{"value":"load-test-value"}' "$base_url/api/cache/hey-test-key" > "$BENCH_OUTPUT_DIR/${test_name}_put.log"
        
        echo -e "${BLUE}Running GET benchmark...${NC}"
        hey -n $num_requests -c $concurrent "$base_url/api/cache/hey-test-key" > "$BENCH_OUTPUT_DIR/${test_name}_get.log"
        
        echo -e "${BLUE}Running DELETE benchmark...${NC}"
        hey -n $num_requests -c $concurrent -m DELETE "$base_url/api/cache/hey-test-key" > "$BENCH_OUTPUT_DIR/${test_name}_delete.log"
        
        echo -e "${GREEN}Hey load test complete.${NC}"
        echo -e "${YELLOW}Results stored in $BENCH_OUTPUT_DIR/${test_name}_*.log${NC}"
        
        # Display summary
        for op in put get delete; do
            echo -e "${CYAN}${op^^} summary:${NC}"
            grep -A 2 "Summary:" "$BENCH_OUTPUT_DIR/${test_name}_${op}.log" | tail -n 2
            echo ""
        done
    fi
}

# Main function to run tests
main() {
    local mode=$1
    local test_type=$2
    
    case $mode in
        "test")
            case $test_type in
                "api")
                    # Start a single node
                    start_cluster 1 8080 "api_test"
                    
                    # Run HTTP API tests
                    run_http_api_tests 8080 "api_test"
                    
                    # Stop the node
                    stop_cluster "api_test"
                    ;;
                    
                "replication")
                    # Start a 3-node cluster
                    start_cluster 3 8080 "repl_test"
                    
                    # Run replication tests
                    run_replication_tests 8080 8081 "replication"
                    
                    # Stop the cluster
                    stop_cluster "repl_test"
                    ;;
                    
                "persistence")
                    # Start a single node
                    start_cluster 1 8080 "persist_test"
                    
                    # Run persistence tests
                    run_persistence_tests 8080 "persist_test" 1
                    ;;
                    
                "all")
                    echo -e "${CYAN}${BOLD}Running all tests${NC}"
                    
                    # API tests
                    start_cluster 1 8080 "api_test"
                    run_http_api_tests 8080 "api_test"
                    stop_cluster "api_test"
                    
                    # Replication tests
                    start_cluster 3 8080 "repl_test"
                    run_replication_tests 8080 8081 "replication"
                    stop_cluster "repl_test"
                    
                    # Persistence tests
                    start_cluster 1 8080 "persist_test"
                    run_persistence_tests 8080 "persist_test" 1
                    ;;
                    
                *)
                    echo -e "${RED}Unknown test type: $test_type${NC}"
                    echo -e "Valid test types: api, replication, persistence, all"
                    exit 1
                    ;;
            esac
            ;;
            
        "benchmark")
            case $test_type in
                "resp")
                    # Start a single node
                    start_cluster 1 9080 "resp_bench"
                    
                    # Run redis-benchmark
                    run_redis_benchmark 8080 "single_node" 10000 50
                    
                    # Stop the node
                    stop_cluster "resp_bench"
                    ;;
                    
                "http")
                    # Start a single node
                    start_cluster 1 9080 "http_bench"
                    
                    # Run custom load
                    run_custom_load "http://localhost:9080" "single_node" 1000 10
                    
                    # Stop the node
                    stop_cluster "http_bench"
                    ;;
                    
                "cluster")
                    # Start a 3-node cluster
                    start_cluster 3 9080 "cluster_bench"
                    
                    # Run RESP benchmark on each node
                    for i in {1..3}; do
                        run_redis_benchmark $((8079 + i)) "node${i}" 5000 25
                    done
                    
                    # Run HTTP benchmark on each node
                    for i in {1..3}; do
                        run_custom_load "http://localhost:$((9079 + i))" "node${i}" 500 10
                    done
                    
                    # Stop the cluster
                    stop_cluster "cluster_bench"
                    ;;
                    
                "all")
                    echo -e "${CYAN}${BOLD}Running all benchmarks${NC}"
                    
                    # RESP benchmarks
                    start_cluster 1 9080 "resp_bench"
                    run_redis_benchmark 8080 "single_node_resp" 10000 50
                    stop_cluster "resp_bench"
                    
                    # HTTP benchmarks
                    start_cluster 1 9080 "http_bench"
                    run_custom_load "http://localhost:9080" "single_node_http" 1000 10
                    stop_cluster "http_bench"
                    
                    # Cluster benchmarks
                    start_cluster 3 9080 "cluster_bench"
                    for i in {1..3}; do
                        run_redis_benchmark $((8079 + i)) "cluster_node${i}_resp" 5000 25
                    done
                    for i in {1..3}; do
                        run_custom_load "http://localhost:$((9079 + i))" "cluster_node${i}_http" 500 10
                    done
                    stop_cluster "cluster_bench"
                    ;;
                    
                *)
                    echo -e "${RED}Unknown benchmark type: $test_type${NC}"
                    echo -e "Valid benchmark types: resp, http, cluster, all"
                    exit 1
                    ;;
            esac
            ;;
            
        *)
            echo -e "${RED}Unknown mode: $mode${NC}"
            echo -e "Usage: $0 [test|benchmark] [type]"
            echo -e ""
            echo -e "Test types: api, replication, persistence, all"
            echo -e "Benchmark types: resp, http, cluster, all"
            exit 1
            ;;
    esac
}

# Check for required binaries
check_binary $HYPERCACHE_BIN

# Process command line arguments
if [ $# -lt 2 ]; then
    echo -e "${RED}Insufficient arguments${NC}"
    echo -e "Usage: $0 [test|benchmark] [type]"
    echo -e ""
    echo -e "Test types: api, replication, persistence, all"
    echo -e "Benchmark types: resp, http, cluster, all"
    exit 1
fi

# Run main function with provided arguments
main "$1" "$2"

echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│       Test/Benchmark Run Complete       │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"
