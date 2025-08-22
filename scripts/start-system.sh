#!/bin/bash

# HyperCache Complete System Launcher
# This script starts both the cluster and monitoring stack

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│        HyperCache System Launcher       │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"

# Function to check if a port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null ; then
        return 0
    else
        return 1
    fi
}

# Function to wait for service
wait_for_service() {
    local url=$1
    local name=$2
    local max_attempts=${3:-30}
    
    echo -e "${YELLOW}Waiting for $name to be ready...${NC}"
    
    for i in $(seq 1 $max_attempts); do
        if curl -s -f "$url" >/dev/null 2>&1; then
            echo -e "${GREEN}✅ $name is ready${NC}"
            return 0
        fi
        echo -e "   Attempt $i/$max_attempts - waiting for $name..."
        sleep 2
    done
    
    echo -e "${RED}❌ $name failed to start after $max_attempts attempts${NC}"
    return 1
}

# Function to show help
show_help() {
    echo -e "${YELLOW}Usage:${NC} $0 [options]"
    echo -e ""
    echo -e "${YELLOW}Options:${NC}"
    echo -e "  -h, --help       Show this help message"
    echo -e "  -c, --cluster    Start only the HyperCache cluster"
    echo -e "  -m, --monitor    Start only the monitoring stack"
    echo -e "  -a, --all        Start both cluster and monitoring (default)"
    echo -e "  --clean          Clean persistence data before starting"
    echo -e "  --rebuild        Rebuild binaries before starting"
    echo -e ""
    echo -e "${YELLOW}Examples:${NC}"
    echo -e "  $0                    # Start both cluster and monitoring"
    echo -e "  $0 --cluster          # Start only cluster"
    echo -e "  $0 --monitor          # Start only monitoring"
    echo -e "  $0 --clean --all      # Clean data and start everything"
}

# Default options
START_CLUSTER=false
START_MONITOR=false
CLEAN_DATA=false
REBUILD=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -c|--cluster)
            START_CLUSTER=true
            shift
            ;;
        -m|--monitor)
            START_MONITOR=true
            shift
            ;;
        -a|--all)
            START_CLUSTER=true
            START_MONITOR=true
            shift
            ;;
        --clean)
            CLEAN_DATA=true
            shift
            ;;
        --rebuild)
            REBUILD=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# If no specific option, start both
if [ "$START_CLUSTER" = false ] && [ "$START_MONITOR" = false ]; then
    START_CLUSTER=true
    START_MONITOR=true
fi

echo -e "${BLUE}Starting HyperCache system with:${NC}"
[ "$START_CLUSTER" = true ] && echo -e "  • HyperCache Cluster"
[ "$START_MONITOR" = true ] && echo -e "  • Monitoring Stack (Elasticsearch, Grafana, Filebeat)"
[ "$CLEAN_DATA" = true ] && echo -e "  • Clean persistence data"
[ "$REBUILD" = true ] && echo -e "  • Rebuild binaries"
echo ""

# Step 1: Clean data if requested
if [ "$CLEAN_DATA" = true ]; then
    echo -e "${YELLOW}🧹 Cleaning persistence data...${NC}"
    ./scripts/clean-persistence.sh --all
    echo ""
fi

# Step 2: Rebuild if requested
if [ "$REBUILD" = true ]; then
    echo -e "${YELLOW}🔨 Rebuilding HyperCache...${NC}"
    ./scripts/build-hypercache.sh
    echo ""
fi

# Step 3: Start monitoring stack
if [ "$START_MONITOR" = true ]; then
    echo -e "${YELLOW}📊 Starting monitoring stack...${NC}"
    
    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        echo -e "${RED}❌ Docker is not running. Please start Docker first.${NC}"
        exit 1
    fi
    
    # Start monitoring services
    docker-compose -f docker-compose.logging.yml up -d
    
    # Wait for services to be ready
    wait_for_service "http://localhost:9200/_cluster/health" "Elasticsearch" 30
    wait_for_service "http://localhost:3000" "Grafana" 20
    
    echo -e "${GREEN}✅ Monitoring stack is ready${NC}"
    echo -e "   • Elasticsearch: http://localhost:9200"
    echo -e "   • Grafana: http://localhost:3000 (admin/admin123)"
    echo ""
fi

# Step 4: Start HyperCache cluster
if [ "$START_CLUSTER" = true ]; then
    echo -e "${YELLOW}🚀 Starting HyperCache cluster...${NC}"
    
    # Check for port conflicts
    for port in 8080 8081 8082 9080 9081 9082; do
        if check_port $port; then
            echo -e "${RED}❌ Port $port is already in use${NC}"
            echo -e "   Stop existing services or use: lsof -ti:$port | xargs kill"
            exit 1
        fi
    done
    
    # Start cluster in background
    ./scripts/build-and-run.sh cluster &
    CLUSTER_PID=$!
    
    # Wait for cluster to be ready
    echo -e "${YELLOW}Waiting for cluster nodes to start...${NC}"
    sleep 10
    
    # Check if nodes are responding
    for port in 9080 9081 9082; do
        if wait_for_service "http://localhost:$port/health" "Node on port $port" 10; then
            echo -e "${GREEN}✅ Node on port $port is ready${NC}"
        else
            echo -e "${YELLOW}⚠️ Node on port $port may still be starting${NC}"
        fi
    done
    
    echo -e "${GREEN}✅ HyperCache cluster started${NC}"
    echo -e "   • Node 1: http://localhost:9080"
    echo -e "   • Node 2: http://localhost:9081" 
    echo -e "   • Node 3: http://localhost:9082"
    echo ""
fi

# Step 5: Show system status
echo -e "${CYAN}🎯 System Status:${NC}"
echo -e "=================="

if [ "$START_MONITOR" = true ]; then
    echo -e "${BOLD}Monitoring:${NC}"
    curl -s "http://localhost:9200/_cluster/health" | jq -r '"  • Elasticsearch: " + .status' 2>/dev/null || echo "  • Elasticsearch: Checking..."
    
    if curl -s "http://localhost:3000" >/dev/null 2>&1; then
        echo -e "  • Grafana: ${GREEN}Available${NC}"
    else
        echo -e "  • Grafana: ${YELLOW}Starting...${NC}"
    fi
    
    # Show log indices
    echo -e "  • Log indices:"
    curl -s "http://localhost:9200/_cat/indices/logs-*?h=index,docs.count" 2>/dev/null | sed 's/^/    /' || echo "    None yet"
fi

if [ "$START_CLUSTER" = true ]; then
    echo -e "${BOLD}Cluster Nodes:${NC}"
    for port in 9080 9081 9082; do
        if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
            echo -e "  • Port $port: ${GREEN}Running${NC}"
        else
            echo -e "  • Port $port: ${YELLOW}Starting...${NC}"
        fi
    done
fi

echo ""
echo -e "${GREEN}🎉 HyperCache system is ready!${NC}"

if [ "$START_MONITOR" = true ]; then
    echo -e "${BLUE}📊 Access monitoring:${NC}"
    echo -e "   • Grafana: http://localhost:3000"
    echo -e "   • Login: admin / admin123"
    echo -e "   • HyperCache Logs datasource is pre-configured"
fi

if [ "$START_CLUSTER" = true ]; then
    echo -e "${BLUE}🔧 Test the cluster:${NC}"
    echo -e "   • curl -X PUT http://localhost:9080/api/cache/test -d '{\"value\":\"hello\",\"ttl_hours\":1}'"
    echo -e "   • curl http://localhost:9080/api/cache/test"
fi

echo ""
echo -e "${YELLOW}💡 Useful commands:${NC}"
echo -e "   • Stop cluster: pkill -f hypercache"
echo -e "   • Stop monitoring: docker-compose -f docker-compose.logging.yml down"
echo -e "   • Clean data: ./scripts/clean-persistence.sh --all"
echo -e "   • Clean Elasticsearch: ./scripts/clean-elasticsearch.sh"
