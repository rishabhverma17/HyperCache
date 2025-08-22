#!/bin/bash

# HyperCache Persistence Cleanup Script
# This script safely removes persistence data while preserving the directory structure

# Set color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}┌─────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│     HyperCache Persistence Cleanup      │${NC}"
echo -e "${CYAN}└─────────────────────────────────────────┘${NC}"

# Default data directory
DATA_DIR="./data"

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

# Function to cleanup a specific node's data
cleanup_node_data() {
    local node_dir=$1
    
    if [ -d "$node_dir" ]; then
        echo -e "${YELLOW}Cleaning up persistence data in ${BOLD}$node_dir${NC}"
        
        # Remove all database and persistence files
        find "$node_dir" -name "*.db" -type f -delete 2>/dev/null
        find "$node_dir" -name "*.wal" -type f -delete 2>/dev/null
        find "$node_dir" -name "*.log" -type f -delete 2>/dev/null
        find "$node_dir" -name "LOCK" -type f -delete 2>/dev/null
        find "$node_dir" -name "MANIFEST-*" -type f -delete 2>/dev/null
        find "$node_dir" -name "CURRENT" -type f -delete 2>/dev/null
        find "$node_dir" -name "*.sst" -type f -delete 2>/dev/null
        find "$node_dir" -name "*.ldb" -type f -delete 2>/dev/null
        find "$node_dir" -name "OPTIONS-*" -type f -delete 2>/dev/null
        find "$node_dir" -name "LOG.old.*" -type f -delete 2>/dev/null
        
        # Remove any temporary or backup files
        find "$node_dir" -name "*.tmp" -type f -delete 2>/dev/null
        find "$node_dir" -name "*.bak" -type f -delete 2>/dev/null
        find "$node_dir" -name "*~" -type f -delete 2>/dev/null
        
        # Count remaining files
        local remaining=$(find "$node_dir" -type f 2>/dev/null | wc -l | tr -d ' ')
        
        if [ "$remaining" -gt 0 ]; then
            echo -e "${YELLOW}Found $remaining other files in $node_dir${NC}"
            
            if confirm "Do you want to see these files?"; then
                find "$node_dir" -type f -exec ls -la {} \; 2>/dev/null
                
                if confirm "Do you want to delete ALL files in $node_dir?"; then
                    # Safely remove all files but keep directory structure
                    find "$node_dir" -type f -delete 2>/dev/null
                    find "$node_dir" -type d -empty -delete 2>/dev/null
                    mkdir -p "$node_dir" # Recreate the node directory
                    echo -e "${GREEN}✓ Completely cleaned $node_dir${NC}"
                else
                    echo -e "${BLUE}Keeping remaining files in $node_dir${NC}"
                fi
            fi
        else
            echo -e "${GREEN}✓ Cleaned all persistence files in $node_dir${NC}"
        fi
    else
        echo -e "${BLUE}Creating data directory $node_dir${NC}"
        mkdir -p "$node_dir"
        echo -e "${GREEN}✓ Created $node_dir${NC}"
    fi
}

# Show help
show_help() {
    echo -e "${YELLOW}Usage:${NC}"
    echo -e "  $0 [options]"
    echo -e ""
    echo -e "${YELLOW}Options:${NC}"
    echo -e "  -h, --help       Show this help message"
    echo -e "  -a, --all        Clean all nodes"
    echo -e "  -d, --dir DIR    Specify data directory (default: $DATA_DIR)"
    echo -e "  -n, --node ID    Clean specific node (e.g., node-1, node-2)"
    echo -e ""
    echo -e "${YELLOW}Examples:${NC}"
    echo -e "  $0 --all                  # Clean all node data"
    echo -e "  $0 --node node-1          # Clean only node-1 data"
    echo -e "  $0 --dir ./custom/data    # Use custom data directory"
}

# Parse command line arguments
CLEAN_ALL=false
NODE_ID=""

while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -h|--help)
            show_help
            exit 0
            ;;
        -a|--all)
            CLEAN_ALL=true
            shift
            ;;
        -d|--dir)
            DATA_DIR="$2"
            shift
            shift
            ;;
        -n|--node)
            NODE_ID="$2"
            shift
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# Main logic
if [ "$CLEAN_ALL" = true ]; then
    echo -e "${YELLOW}Cleaning all persistence data...${NC}"
    
    # Check if data directory exists
    if [ ! -d "$DATA_DIR" ]; then
        echo -e "${BLUE}Creating data directory $DATA_DIR${NC}"
        mkdir -p "$DATA_DIR"
    fi
    
    # Find all node directories
    node_dirs=$(find "$DATA_DIR" -maxdepth 1 -type d -name "node-*" 2>/dev/null)
    
    if [ -z "$node_dirs" ]; then
        echo -e "${YELLOW}No node directories found. Creating default structure...${NC}"
        mkdir -p "$DATA_DIR/node-1" "$DATA_DIR/node-2" "$DATA_DIR/node-3"
        echo -e "${GREEN}✓ Created default node directories${NC}"
    else
        # Clean each node directory
        for dir in $node_dirs; do
            cleanup_node_data "$dir"
        done
    fi
elif [ -n "$NODE_ID" ]; then
    # Clean specific node
    node_dir="$DATA_DIR/$NODE_ID"
    
    if [ ! -d "$DATA_DIR" ]; then
        echo -e "${BLUE}Creating data directory $DATA_DIR${NC}"
        mkdir -p "$DATA_DIR"
    fi
    
    cleanup_node_data "$node_dir"
else
    # No specific options provided, ask for confirmation
    echo -e "${YELLOW}No specific cleanup option selected.${NC}"
    
    if confirm "Do you want to clean all persistence data?"; then
        echo -e "${YELLOW}Cleaning all persistence data...${NC}"
        
        # Check if data directory exists
        if [ ! -d "$DATA_DIR" ]; then
            echo -e "${BLUE}Creating data directory $DATA_DIR${NC}"
            mkdir -p "$DATA_DIR"
        fi
        
        # Find all node directories
        node_dirs=$(find "$DATA_DIR" -maxdepth 1 -type d -name "node-*" 2>/dev/null)
        
        if [ -z "$node_dirs" ]; then
            echo -e "${YELLOW}No node directories found. Creating default structure...${NC}"
            mkdir -p "$DATA_DIR/node-1" "$DATA_DIR/node-2" "$DATA_DIR/node-3"
            echo -e "${GREEN}✓ Created default node directories${NC}"
        else
            # Clean each node directory
            for dir in $node_dirs; do
                cleanup_node_data "$dir"
            done
        fi
    else
        show_help
        exit 0
    fi
fi

echo -e "${GREEN}Persistence cleanup complete!${NC}"
echo -e "${BLUE}Persistence directories are ready for fresh data.${NC}"
