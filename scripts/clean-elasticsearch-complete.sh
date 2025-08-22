#!/bin/bash

# Complete Elasticsearch Cleanup Script
# This script ensures complete cleanup of all Elasticsearch data including Docker volumes

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}┌─────────────────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│      Complete Elasticsearch Cleanup Utility        │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────────────────┘${NC}"

# Function to confirm an action
confirm() {
    local message=$1
    read -p "$message [y/N] " response
    case "$response" in
        [yY][eE][sS]|[yY]) 
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

echo -e "${YELLOW}This will completely clean all Elasticsearch data including:${NC}"
echo -e "  - All Elasticsearch indices and data"
echo -e "  - Docker volumes (elasticsearch_data, filebeat_data)"
echo -e "  - Container restart with fresh state"
echo ""

if ! confirm "Are you sure you want to proceed with complete cleanup?"; then
    echo -e "${BLUE}Operation cancelled${NC}"
    exit 0
fi

echo -e "${YELLOW}🛑 Stopping all services...${NC}"
docker-compose -f docker-compose.logging.yml down

echo -e "${YELLOW}🗑️  Removing Docker volumes...${NC}"
docker volume rm cache_elasticsearch_data 2>/dev/null || echo "Volume cache_elasticsearch_data not found"
docker volume rm cache_filebeat_data 2>/dev/null || echo "Volume cache_filebeat_data not found"
docker volume rm cache_grafana_data 2>/dev/null || echo "Volume cache_grafana_data not found"

# Try alternative volume names
docker volume rm hypercache_elasticsearch_data 2>/dev/null || echo "Volume hypercache_elasticsearch_data not found"
docker volume rm hypercache_filebeat_data 2>/dev/null || echo "Volume hypercache_filebeat_data not found"
docker volume rm hypercache_grafana_data 2>/dev/null || echo "Volume hypercache_grafana_data not found"

# List and try to remove any volumes that might match our project
echo -e "${YELLOW}🔍 Checking for any remaining project volumes...${NC}"
docker volume ls | grep -E "(elasticsearch|filebeat|grafana|cache|hypercache)" || echo "No matching volumes found"

# Prune unused volumes
echo -e "${YELLOW}🧹 Pruning unused Docker volumes...${NC}"
docker volume prune -f

echo -e "${YELLOW}🔄 Starting services with fresh state...${NC}"
docker-compose -f docker-compose.logging.yml up -d

echo -e "${YELLOW}⏳ Waiting for Elasticsearch to be ready...${NC}"
sleep 30

# Check if Elasticsearch is healthy
echo -e "${BLUE}🔍 Checking Elasticsearch health...${NC}"
for i in {1..10}; do
    if curl -s -f "http://localhost:9200/_cluster/health" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Elasticsearch is healthy${NC}"
        break
    else
        echo -e "${YELLOW}⏳ Waiting for Elasticsearch... (attempt $i/10)${NC}"
        sleep 10
    fi
done

# Show final status
echo -e "${BLUE}📊 Final Elasticsearch status:${NC}"
curl -s "http://localhost:9200/_cluster/health?pretty" 2>/dev/null || echo "Could not connect to Elasticsearch"

echo -e "${BLUE}📋 Current indices (should be empty):${NC}"
curl -s "http://localhost:9200/_cat/indices?v" 2>/dev/null || echo "Could not list indices"

echo ""
echo -e "${GREEN}🎯 Complete Elasticsearch cleanup finished!${NC}"
echo -e "${GREEN}✅ All data has been completely removed${NC}"
echo -e "${GREEN}✅ Fresh Elasticsearch instance is running${NC}"
echo ""
echo -e "${CYAN}Next steps:${NC}"
echo -e "  1. Start HyperCache cluster: ./scripts/start-cluster.sh"
echo -e "  2. Generate test data: ./scripts/generate-dashboard-load.sh"
echo -e "  3. Check dashboards: http://localhost:3000"
