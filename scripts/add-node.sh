#!/bin/bash

# Dynamic Node Addition Script for HyperCache Cluster
# Adds a single node to an existing cluster

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATES_DIR="$PROJECT_ROOT/templates"
CONFIGS_DIR="$PROJECT_ROOT/configs"

# Default configuration
DEFAULT_BASE_PORT=8080
DEFAULT_LOG_LEVEL="debug"

# Parse command line arguments
BASE_PORT=$DEFAULT_BASE_PORT
LOG_LEVEL=$DEFAULT_LOG_LEVEL
AUTO_PORTS=true
NODE_NAME=""

show_help() {
    echo "HyperCache Dynamic Node Addition"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --node-name=NAME   Custom node name (default: auto-generated)"
    echo "  --base-port=PORT   Preferred starting port for scanning (default: 8080)"
    echo "  --log-level=LEVEL  Log level: debug, info, warn, error (default: debug)"
    echo "  --help             Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                              # Add node with auto-generated name"
    echo "  $0 --node-name=node-api-1       # Add specific node"
    echo "  $0 --base-port=9000 --log-level=info"
    echo ""
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --node-name=*)
            NODE_NAME="${1#*=}"
            shift
            ;;
        --base-port=*)
            BASE_PORT="${1#*=}"
            shift
            ;;
        --log-level=*)
            LOG_LEVEL="${1#*=}"
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate log level
if [[ ! "$LOG_LEVEL" =~ ^(debug|info|warn|error)$ ]]; then
    echo "Error: Invalid log level '$LOG_LEVEL'. Must be one of: debug, info, warn, error"
    exit 1
fi

# Function to check if a port is available
is_port_available() {
    local port=$1
    ! lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1
}

# Function to find next available port
find_next_available_port() {
    local start_port=$1
    local current_port=$start_port
    
    while ! is_port_available $current_port; do
        ((current_port++))
        # Safety check
        if [ $current_port -gt $((start_port + 1000)) ]; then
            echo "Error: Could not find available port starting from $start_port"
            exit 1
        fi
    done
    
    echo $current_port
}

# Function to discover existing cluster seeds
discover_cluster_seeds() {
    local seeds=()
    
    # Look for existing gossip ports
    for port in $(seq $((BASE_PORT - 2000)) $((BASE_PORT - 1000))); do
        if ! is_port_available $port; then
            seeds+=("127.0.0.1:$port")
        fi
    done
    
    # Also check standard range
    for port in $(seq 7946 7950); do
        if ! is_port_available $port; then
            seeds+=("127.0.0.1:$port")
        fi
    done
    
    if [ ${#seeds[@]} -eq 0 ]; then
        echo "Warning: No existing cluster nodes found. Starting standalone node."
        echo "127.0.0.1:$((BASE_PORT - 2000))"
    else
        echo "$(IFS=', '; echo "\"${seeds[*]}\"")"
    fi
}

# Function to generate config from template
generate_config() {
    local node_id=$1
    local resp_port=$2
    local http_port=$3
    local gossip_port=$4
    local seeds=$5
    local config_file="$CONFIGS_DIR/${node_id}-config.yaml"
    
    # Ensure configs directory exists
    mkdir -p "$CONFIGS_DIR"
    
    # Replace template variables
    sed -e "s/\${NODE_ID}/$node_id/g" \
        -e "s/\${RESP_PORT}/$resp_port/g" \
        -e "s/\${HTTP_PORT}/$http_port/g" \
        -e "s/\${GOSSIP_PORT}/$gossip_port/g" \
        -e "s/\${CLUSTER_SEEDS}/$seeds/g" \
        -e "s/\${LOG_LEVEL}/$LOG_LEVEL/g" \
        "$TEMPLATES_DIR/node-config.yaml.template" > "$config_file"
    
    echo "$config_file"
}

echo "🔗 HyperCache Dynamic Node Addition"
echo "===================================="
echo ""

# Generate node name if not provided
if [ -z "$NODE_NAME" ]; then
    TIMESTAMP=$(date +%s)
    NODE_NAME="node-$(echo $HOSTNAME | tr '[:upper:]' '[:lower:]')-$TIMESTAMP"
    echo "🏷️  Auto-generated node name: $NODE_NAME"
else
    echo "🏷️  Using node name: $NODE_NAME"
fi

# Discover existing cluster
echo "🔍 Discovering existing cluster..."
CLUSTER_SEEDS=$(discover_cluster_seeds)
echo "🌐 Cluster seeds: $CLUSTER_SEEDS"
echo ""

# Find available ports
echo "📊 Finding available ports..."
RESP_PORT=$(find_next_available_port $BASE_PORT)
HTTP_PORT=$(find_next_available_port $((BASE_PORT + 1000)))
GOSSIP_PORT=$(find_next_available_port $((BASE_PORT - 2000)))

echo "🔧 Port assignments:"
echo "  RESP: $RESP_PORT"
echo "  HTTP: $HTTP_PORT"
echo "  Gossip: $GOSSIP_PORT"
echo ""

# Check if template exists
if [ ! -f "$TEMPLATES_DIR/node-config.yaml.template" ]; then
    echo "❌ Error: Template file not found at $TEMPLATES_DIR/node-config.yaml.template"
    echo "Please run the enhanced start-cluster script first to create the template."
    exit 1
fi

# Generate configuration
echo "📝 Generating configuration..."
CONFIG_FILE=$(generate_config "$NODE_NAME" "$RESP_PORT" "$HTTP_PORT" "$GOSSIP_PORT" "$CLUSTER_SEEDS")
echo "📄 Config generated: $CONFIG_FILE"

# Build binary if needed
if [ ! -f "$PROJECT_ROOT/bin/hypercache" ]; then
    echo "🔨 Building hypercache binary..."
    cd "$PROJECT_ROOT" && make build
fi

# Start the node
echo "🚀 Starting node $NODE_NAME..."
"$PROJECT_ROOT/bin/hypercache" --config "$CONFIG_FILE" --node-id "$NODE_NAME" --protocol resp &
NODE_PID=$!

echo "✅ Node started successfully!"
echo "📋 Node Details:"
echo "  Name: $NODE_NAME"
echo "  PID: $NODE_PID"
echo "  Config: $CONFIG_FILE"
echo "  RESP: redis-cli -h localhost -p $RESP_PORT"
echo "  HTTP: curl http://localhost:$HTTP_PORT/health"
echo ""

# Wait a moment and check health
sleep 3

echo "🏥 Health Check:"
echo "================"

# HTTP health check
if curl -s -m 5 "http://localhost:$HTTP_PORT/health" >/dev/null 2>&1; then
    echo "✅ HTTP (port $HTTP_PORT) - OK"
else
    echo "❌ HTTP (port $HTTP_PORT) - Failed"
fi

# RESP health check
if redis-cli -h localhost -p $RESP_PORT ping 2>/dev/null | grep -q "PONG"; then
    echo "✅ RESP (port $RESP_PORT) - OK"
else
    echo "❌ RESP (port $RESP_PORT) - Failed"
fi

echo ""
echo "🔧 Integration Steps:"
echo "===================="
echo "1. Add to nginx load balancer (choose one):"
echo ""
echo "   Option A - Manual edit nginx/hypercache.conf:"
echo "   Add this line to hypercache_backend:"
echo "   server 127.0.0.1:$RESP_PORT max_fails=3 fail_timeout=30s;"
echo ""
echo "   Option B - Use management script:"
echo "   ./scripts/manage-nginx.sh add-node --port=$RESP_PORT"
echo ""
echo "2. Reload nginx: ./scripts/manage-nginx.sh reload-nginx"
echo ""
echo "3. To stop this node: kill $NODE_PID"
echo ""
echo "4. Generated config saved at: $CONFIG_FILE"
echo ""

# Offer to automatically add to nginx if management script exists
if [[ -f "$SCRIPT_DIR/manage-nginx.sh" ]]; then
    echo "🤖 Auto-integration available!"
    echo "Run this to add node to nginx automatically:"
    echo "./scripts/manage-nginx.sh add-node --port=$RESP_PORT"
    echo ""
fi

echo "🎉 Node $NODE_NAME is ready and joining the cluster!"
