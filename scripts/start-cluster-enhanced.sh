#!/bin/bash

# Enhanced HyperCache Cluster Start Script
# Supports backward compatibility and dynamic scaling

set -e

# Default configuration
DEFAULT_NODES=3
DEFAULT_BASE_PORT=8080
DEFAULT_LOG_LEVEL="debug"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATES_DIR="$PROJECT_ROOT/templates"
CONFIGS_DIR="$PROJECT_ROOT/configs"

# Parse command line arguments
NODES=$DEFAULT_NODES
BASE_PORT=$DEFAULT_BASE_PORT
LOG_LEVEL=$DEFAULT_LOG_LEVEL
AUTO_PORTS=false

show_help() {
    echo "Enhanced HyperCache Cluster Start Script"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --nodes=N          Number of nodes to start (default: 3)"
    echo "  --base-port=PORT   Starting port number (default: 8080)"
    echo "  --auto-ports       Auto-scan for available ports (default: increment)"
    echo "  --log-level=LEVEL  Log level: debug, info, warn, error (default: debug)"
    echo "  --help             Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                           # Start 3 nodes (backward compatible)"
    echo "  $0 --nodes=5                 # Start 5 nodes on ports 8080-8084"
    echo "  $0 --nodes=10 --base-port=9000  # Start 10 nodes on ports 9000-9009"
    echo "  $0 --auto-ports --log-level=info # Auto-scan ports, info logging"
    echo ""
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --nodes=*)
            NODES="${1#*=}"
            shift
            ;;
        --base-port=*)
            BASE_PORT="${1#*=}"
            shift
            ;;
        --auto-ports)
            AUTO_PORTS=true
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

# Function to find N available ports
find_available_ports() {
    local start_port=$1
    local num_ports=$2
    local ports=()
    local current_port=$start_port
    
    while [ ${#ports[@]} -lt $num_ports ]; do
        if is_port_available $current_port; then
            ports+=($current_port)
        fi
        ((current_port++))
        
        # Safety check to prevent infinite loop
        if [ $current_port -gt $((start_port + 1000)) ]; then
            echo "Error: Could not find $num_ports available ports starting from $start_port"
            exit 1
        fi
    done
    
    echo "${ports[@]}"
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

echo "🚀 Enhanced HyperCache Cluster Startup"
echo "======================================"
echo "Nodes: $NODES"
echo "Base Port: $BASE_PORT"
echo "Auto Ports: $AUTO_PORTS"
echo "Log Level: $LOG_LEVEL"
echo ""

# Clean up any existing processes
echo "🧹 Cleaning up existing processes..."
pkill -f hypercache 2>/dev/null || true
sleep 2

# Determine ports to use
if [ "$AUTO_PORTS" = true ]; then
    echo "🔍 Scanning for available ports starting from $BASE_PORT..."
    RESP_PORTS=($(find_available_ports $BASE_PORT $NODES))
    HTTP_PORTS=($(find_available_ports $((BASE_PORT + 1000)) $NODES))
    GOSSIP_PORTS=($(find_available_ports $((BASE_PORT - 2000)) $NODES))
else
    echo "📊 Using incremental port assignment..."
    RESP_PORTS=()
    HTTP_PORTS=()
    GOSSIP_PORTS=()
    
    for ((i=0; i<NODES; i++)); do
        RESP_PORTS+=($((BASE_PORT + i)))
        HTTP_PORTS+=($((BASE_PORT + 1000 + i)))
        GOSSIP_PORTS+=($((BASE_PORT - 2000 + i)))
    done
fi

# Build gossip seeds list
SEEDS_LIST=""
for ((i=0; i<NODES; i++)); do
    if [ -n "$SEEDS_LIST" ]; then
        SEEDS_LIST="$SEEDS_LIST, "
    fi
    SEEDS_LIST="$SEEDS_LIST\"127.0.0.1:${GOSSIP_PORTS[i]}\""
done

echo "🔧 Port assignments:"
for ((i=0; i<NODES; i++)); do
    echo "  Node $((i+1)): RESP=${RESP_PORTS[i]}, HTTP=${HTTP_PORTS[i]}, Gossip=${GOSSIP_PORTS[i]}"
done
echo ""

# Generate configurations and start nodes
echo "📝 Generating configurations from template..."
NODE_PIDS=()

for ((i=0; i<NODES; i++)); do
    NODE_ID="node-$((i+1))"
    RESP_PORT=${RESP_PORTS[i]}
    HTTP_PORT=${HTTP_PORTS[i]}
    GOSSIP_PORT=${GOSSIP_PORTS[i]}
    
    echo "  Generating config for $NODE_ID..."
    CONFIG_FILE=$(generate_config "$NODE_ID" "$RESP_PORT" "$HTTP_PORT" "$GOSSIP_PORT" "$SEEDS_LIST")
    
    echo "  Starting $NODE_ID (RESP: $RESP_PORT, HTTP: $HTTP_PORT, Gossip: $GOSSIP_PORT)..."
    
    # Build binary if it doesn't exist
    if [ ! -f "$PROJECT_ROOT/bin/hypercache" ]; then
        echo "  Building hypercache binary..."
        cd "$PROJECT_ROOT" && make build
    fi
    
    # Start the node
    "$PROJECT_ROOT/bin/hypercache" --config "$CONFIG_FILE" --node-id "$NODE_ID" --protocol resp &
    NODE_PID=$!
    NODE_PIDS+=($NODE_PID)
    
    echo "  $NODE_ID started with PID: $NODE_PID"
    sleep 2  # Give time for startup
done

echo ""
echo "✅ All $NODES nodes started successfully!"
echo "PIDs: ${NODE_PIDS[@]}"
echo ""

# Display connection information
echo "🌐 Connection Information:"
echo "========================="
for ((i=0; i<NODES; i++)); do
    echo "Node $((i+1)) - node-$((i+1)):"
    echo "  RESP (Redis): redis-cli -h localhost -p ${RESP_PORTS[i]}"
    echo "  HTTP API:     curl http://localhost:${HTTP_PORTS[i]}/health"
    echo ""
done

echo "🔄 Load Balancer Setup:"
echo "======================"
echo "To generate Nginx configuration for load balancing:"
echo "  ./scripts/generate-nginx-config.sh"
echo ""

echo "📊 Health Checks:"
echo "================"
sleep 3  # Wait for nodes to fully start

for ((i=0; i<NODES; i++)); do
    NODE_NAME="node-$((i+1))"
    HTTP_PORT=${HTTP_PORTS[i]}
    RESP_PORT=${RESP_PORTS[i]}
    
    # HTTP health check
    if curl -s -m 5 "http://localhost:$HTTP_PORT/health" >/dev/null 2>&1; then
        echo "✅ $NODE_NAME HTTP (port $HTTP_PORT) - OK"
    else
        echo "❌ $NODE_NAME HTTP (port $HTTP_PORT) - Failed"
    fi
    
    # RESP health check
    if redis-cli -h localhost -p $RESP_PORT ping 2>/dev/null | grep -q "PONG"; then
        echo "✅ $NODE_NAME RESP (port $RESP_PORT) - OK"
    else
        echo "❌ $NODE_NAME RESP (port $RESP_PORT) - Failed"
    fi
done

echo ""
echo "📋 Cluster Status:"
echo "=================="
echo "Generated configs in: $CONFIGS_DIR/"
echo "Template used: $TEMPLATES_DIR/node-config.yaml.template"
echo "Log Level: $LOG_LEVEL"
echo "Logs location: $PROJECT_ROOT/logs/"
echo ""

echo "🛑 To stop the cluster:"
echo "======================"
echo "pkill -f hypercache"
echo "# Or kill individual PIDs: ${NODE_PIDS[@]}"
echo ""

echo "🎉 HyperCache cluster is ready!"
echo "You can now test with: redis-cli -h localhost -p ${RESP_PORTS[0]}"
