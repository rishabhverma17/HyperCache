#!/bin/bash

# HyperCache Elasticsearch Cleanup Script
# Provides various options to clean Elasticsearch data

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│      Elasticsearch Cleanup Utility      │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"

# Configuration
ES_URL="http://localhost:9200"

# Function to check if Elasticsearch is accessible
check_elasticsearch() {
    if curl -s -f "$ES_URL/_cluster/health" >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

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

# Function to show current indices
show_indices() {
    echo -e "${BLUE}Current indices:${NC}"
    if check_elasticsearch; then
        curl -s "$ES_URL/_cat/indices?v" | head -20
        
        local total_docs=$(curl -s "$ES_URL/_cat/indices?h=docs.count" | awk '{sum += $1} END {print sum}')
        echo -e "${YELLOW}Total documents: $total_docs${NC}"
    else
        echo -e "${RED}❌ Elasticsearch is not accessible${NC}"
        return 1
    fi
}

# Function to delete all HyperCache logs
clean_logs() {
    echo -e "${YELLOW}🧹 Cleaning HyperCache log indices...${NC}"
    
    if ! check_elasticsearch; then
        echo -e "${RED}❌ Elasticsearch is not accessible at $ES_URL${NC}"
        exit 1
    fi
    
    # Delete log indices
    curl -X DELETE "$ES_URL/logs-*" 2>/dev/null || echo "No logs-* indices found"
    curl -X DELETE "$ES_URL/hypercache-*" 2>/dev/null || echo "No hypercache-* indices found"
    curl -X DELETE "$ES_URL/test-*" 2>/dev/null || echo "No test-* indices found"
    curl -X DELETE "$ES_URL/logs-grafana" 2>/dev/null || echo "No logs-grafana index found"
    
    # Delete data streams if any
    curl -X DELETE "$ES_URL/_data_stream/logs-*" 2>/dev/null || true
    curl -X DELETE "$ES_URL/_data_stream/hypercache-*" 2>/dev/null || true
    curl -X DELETE "$ES_URL/.ds-*" 2>/dev/null || true
    
    echo -e "${GREEN}✅ Cleaned HyperCache log indices${NC}"
}

# Function to delete ALL indices
clean_all() {
    echo -e "${RED}⚠️  WARNING: This will delete ALL indices in Elasticsearch!${NC}"
    
    if ! confirm "Are you sure you want to delete ALL data?"; then
        echo -e "${BLUE}Operation cancelled${NC}"
        return 0
    fi
    
    if ! check_elasticsearch; then
        echo -e "${RED}❌ Elasticsearch is not accessible at $ES_URL${NC}"
        exit 1
    fi
    
    # Delete all indices
    curl -X DELETE "$ES_URL/_all" 2>/dev/null
    
    # Delete all data streams
    curl -X DELETE "$ES_URL/_data_stream/*" 2>/dev/null || true
    
    echo -e "${GREEN}✅ Deleted all indices and data streams${NC}"
}

# Function to restart Filebeat
restart_filebeat() {
    echo -e "${YELLOW}🔄 Restarting Filebeat...${NC}"
    
    if docker ps --format "{{.Names}}" | grep -q "hypercache-filebeat"; then
        docker restart hypercache-filebeat
        sleep 5
        echo -e "${GREEN}✅ Filebeat restarted${NC}"
    else
        echo -e "${YELLOW}⚠️  Filebeat container not found${NC}"
    fi
}

# Function to show help
show_help() {
    echo -e "${YELLOW}Usage:${NC} $0 [options]"
    echo -e ""
    echo -e "${YELLOW}Options:${NC}"
    echo -e "  -h, --help       Show this help message"
    echo -e "  -s, --status     Show current Elasticsearch status"
    echo -e "  -l, --logs       Delete only HyperCache log indices"
    echo -e "  -a, --all        Delete ALL indices (dangerous!)"
    echo -e "  -r, --restart    Restart Filebeat after cleanup"
    echo -e "  --url URL        Elasticsearch URL (default: $ES_URL)"
    echo -e ""
    echo -e "${YELLOW}Examples:${NC}"
    echo -e "  $0 --status              # Show current indices"
    echo -e "  $0 --logs                # Clean only log indices"
    echo -e "  $0 --logs --restart      # Clean logs and restart Filebeat"
    echo -e "  $0 --all                 # Clean everything (use with caution!)"
    echo -e ""
    echo -e "${YELLOW}Quick Commands:${NC}"
    echo -e "  $0 -s    # Status"
    echo -e "  $0 -l    # Clean logs"
    echo -e "  $0 -lr   # Clean logs + restart Filebeat"
}

# Parse command line arguments
SHOW_STATUS=false
CLEAN_LOGS=false
CLEAN_ALL=false
RESTART_FILEBEAT=false

if [ $# -eq 0 ]; then
    show_help
    exit 0
fi

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -s|--status)
            SHOW_STATUS=true
            shift
            ;;
        -l|--logs)
            CLEAN_LOGS=true
            shift
            ;;
        -a|--all)
            CLEAN_ALL=true
            shift
            ;;
        -r|--restart)
            RESTART_FILEBEAT=true
            shift
            ;;
        --url)
            ES_URL="$2"
            shift
            shift
            ;;
        -lr)
            CLEAN_LOGS=true
            RESTART_FILEBEAT=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# Execute requested actions
if [ "$SHOW_STATUS" = true ]; then
    show_indices
fi

if [ "$CLEAN_ALL" = true ]; then
    clean_all
elif [ "$CLEAN_LOGS" = true ]; then
    clean_logs
fi

if [ "$RESTART_FILEBEAT" = true ]; then
    restart_filebeat
fi

# Show final status if any cleanup was performed
if [ "$CLEAN_LOGS" = true ] || [ "$CLEAN_ALL" = true ]; then
    echo ""
    echo -e "${BLUE}Final status:${NC}"
    show_indices
fi

echo -e "${GREEN}🎯 Cleanup complete!${NC}"
