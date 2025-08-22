#!/bin/bash

# HyperCache Load Generator & Dashboard Test Script
# This script generates realistic load against your HyperCache cluster

set -e

echo "🚀 HyperCache Load Generator & Dashboard Test"
echo "============================================"

# Configuration
NODES=("localhost:9080" "localhost:9081" "localhost:9082")
DURATION=300  # 5 minutes
CONCURRENT_USERS=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test data generation
generate_test_data() {
    local operation=$1
    local key=$2
    local value=$3
    local node=${NODES[$RANDOM % ${#NODES[@]}]}
    
    case $operation in
        "PUT")
            curl -s -X PUT "http://$node/api/cache/$key" \
                -H "Content-Type: application/json" \
                -d "{\"value\":\"$value\",\"ttl_hours\":1}" > /dev/null
            ;;
        "GET")
            curl -s "http://$node/api/cache/$key" > /dev/null
            ;;
        "DELETE")
            curl -s -X DELETE "http://$node/api/cache/$key" > /dev/null
            ;;
    esac
}

# Generate realistic workload patterns
workload_pattern() {
    local user_id=$1
    local end_time=$((SECONDS + DURATION))
    
    echo -e "${GREEN}User $user_id started${NC}"
    
    while [ $SECONDS -lt $end_time ]; do
        # Generate different types of operations with realistic distribution
        local operation_type=$((RANDOM % 100))
        local key="user${user_id}_key${RANDOM}"
        local value="data_${RANDOM}_$(date +%s)"
        
        if [ $operation_type -lt 70 ]; then
            # 70% GET operations
            generate_test_data "GET" "$key" ""
        elif [ $operation_type -lt 90 ]; then
            # 20% PUT operations
            generate_test_data "PUT" "$key" "$value"
        else
            # 10% DELETE operations
            generate_test_data "DELETE" "$key" ""
        fi
        
        # Variable delay to simulate real usage
        sleep $(echo "scale=2; $RANDOM/32767 * 2" | bc -l)
    done
    
    echo -e "${GREEN}User $user_id finished${NC}"
}

# Health check function
check_cluster_health() {
    echo -e "${YELLOW}Checking cluster health...${NC}"
    
    for node in "${NODES[@]}"; do
        if curl -s "http://$node/health" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ $node is healthy${NC}"
        else
            echo -e "${RED}✗ $node is not responding${NC}"
            return 1
        fi
    done
}

# Pre-populate some data for consistent testing
populate_initial_data() {
    echo -e "${YELLOW}Populating initial test data...${NC}"
    
    local categories=("user" "session" "config" "temp" "cache")
    local data_types=("profile" "settings" "metrics" "logs" "state")
    
    for i in {1..50}; do
        local category=${categories[$RANDOM % ${#categories[@]}]}
        local data_type=${data_types[$RANDOM % ${#data_types[@]}]}
        local key="${category}_${data_type}_${i}"
        local value="{\"id\":$i,\"type\":\"$data_type\",\"category\":\"$category\",\"timestamp\":\"$(date -Iseconds)\"}"
        
        generate_test_data "PUT" "$key" "$value"
    done
    
    echo -e "${GREEN}Initial data populated${NC}"
}

# Main execution
main() {
    echo "Starting load test for $DURATION seconds with $CONCURRENT_USERS concurrent users"
    
    # Check if cluster is healthy
    if ! check_cluster_health; then
        echo -e "${RED}Cluster health check failed. Please ensure all nodes are running.${NC}"
        exit 1
    fi
    
    # Populate initial data
    populate_initial_data
    
    echo -e "${YELLOW}Starting concurrent workload...${NC}"
    
    # Start concurrent users
    for i in $(seq 1 $CONCURRENT_USERS); do
        workload_pattern $i &
    done
    
    # Wait for all background processes
    wait
    
    echo -e "${GREEN}Load test completed!${NC}"
    echo -e "${YELLOW}Check your Grafana dashboards at: http://localhost:3000${NC}"
    echo ""
    echo "Dashboard URLs:"
    echo "- Health Dashboard: http://localhost:3000/d/health"
    echo "- Performance Metrics: http://localhost:3000/d/performance" 
    echo "- System Components: http://localhost:3000/d/components"
    echo "- Operational Dashboard: http://localhost:3000/d/operational"
    echo "- Log Stream: http://localhost:3000/d/logs"
}

# Check dependencies
check_dependencies() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}curl is required but not installed.${NC}"
        exit 1
    fi
    
    if ! command -v bc &> /dev/null; then
        echo -e "${RED}bc is required but not installed.${NC}"
        exit 1
    fi
}

# Script execution
check_dependencies
main

echo -e "${GREEN}🎉 Load generation complete! Your dashboards should now show meaningful data.${NC}"
