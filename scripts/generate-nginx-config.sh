#!/bin/bash

# Nginx Load Balancer Configuration Generator for HyperCache
# Automatically discovers running nodes and generates nginx config

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
NGINX_DIR="$PROJECT_ROOT/nginx"

# Default configuration
DEFAULT_LISTEN_PORT=6379
HEALTH_CHECK_TIMEOUT=3

# Parse command line arguments
LISTEN_PORT=$DEFAULT_LISTEN_PORT
OUTPUT_FILE=""
ENABLE_STICKY_SESSIONS=false

show_help() {
    echo "HyperCache Nginx Configuration Generator"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --listen-port=PORT     Port for nginx to listen on (default: 6379)"
    echo "  --output-file=FILE     Output file path (default: nginx/hypercache.conf)"
    echo "  --sticky-sessions      Enable sticky sessions (default: disabled)"
    echo "  --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Generate with defaults"
    echo "  $0 --listen-port=7000                 # Custom listen port"
    echo "  $0 --output-file=/etc/nginx/hypercache.conf"
    echo "  $0 --sticky-sessions                  # Enable sticky sessions"
    echo ""
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --listen-port=*)
            LISTEN_PORT="${1#*=}"
            shift
            ;;
        --output-file=*)
            OUTPUT_FILE="${1#*=}"
            shift
            ;;
        --sticky-sessions)
            ENABLE_STICKY_SESSIONS=true
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

# Set default output file if not provided
if [ -z "$OUTPUT_FILE" ]; then
    OUTPUT_FILE="$NGINX_DIR/hypercache.conf"
fi

# Function to check if a port has a service responding
check_service_health() {
    local port=$1
    
    # Try Redis PING command
    if timeout $HEALTH_CHECK_TIMEOUT redis-cli -h localhost -p $port ping >/dev/null 2>&1; then
        return 0
    fi
    
    # Try basic TCP connection
    if timeout $HEALTH_CHECK_TIMEOUT bash -c "echo > /dev/tcp/localhost/$port" >/dev/null 2>&1; then
        return 0
    fi
    
    return 1
}

# Function to discover active HyperCache nodes
discover_nodes() {
    local nodes=()
    local node_info=()
    
    echo "🔍 Scanning for active HyperCache nodes..."
    
    # Scan common port ranges
    local port_ranges=(
        "8080 8100"   # Standard RESP ports
        "7000 7020"   # Alternative range
        "6379 6399"   # Redis standard range
        "9000 9020"   # High port range
    )
    
    for range in "${port_ranges[@]}"; do
        read start_port end_port <<< "$range"
        
        for port in $(seq $start_port $end_port); do
            if check_service_health $port; then
                nodes+=("127.0.0.1:$port")
                echo "  ✅ Found active node at localhost:$port"
                
                # Try to get node info via HTTP API
                local http_port=$((port + 1000))
                if curl -s -m 2 "http://localhost:$http_port/health" >/dev/null 2>&1; then
                    local node_id=$(curl -s -m 2 "http://localhost:$http_port/health" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "node-$port")
                    node_info+=("# Node: $node_id (HTTP: $http_port)")
                else
                    node_info+=("# Node: unknown (RESP: $port)")
                fi
            fi
        done
    done
    
    if [ ${#nodes[@]} -eq 0 ]; then
        echo "⚠️  No active HyperCache nodes found!"
        echo "Please start your cluster first using:"
        echo "  ./scripts/start-cluster-enhanced.sh"
        exit 1
    fi
    
    echo "📊 Discovered ${#nodes[@]} active nodes"
    echo ""
}

# Function to generate nginx configuration
generate_nginx_config() {
    local config_file=$1
    
    # Ensure nginx directory exists
    mkdir -p "$(dirname "$config_file")"
    
    echo "📝 Generating nginx configuration..."
    
    cat > "$config_file" << EOF
# HyperCache Load Balancer Configuration
# Generated on: $(date)
# Nodes discovered: ${#nodes[@]}
# Listen port: $LISTEN_PORT

# Stream module configuration for Redis protocol
stream {
    # Logging
    error_log /var/log/nginx/hypercache_error.log;
    access_log /var/log/nginx/hypercache_access.log;

    # Upstream backend pool
    upstream hypercache_backend {
EOF

    # Add load balancing method
    if [ "$ENABLE_STICKY_SESSIONS" = true ]; then
        echo "        # Sticky sessions based on client IP" >> "$config_file"
        echo "        ip_hash;" >> "$config_file"
    else
        echo "        # Round-robin load balancing (default)" >> "$config_file"
        echo "        least_conn;" >> "$config_file"
    fi

    echo "" >> "$config_file"

    # Add each discovered node
    for i in "${!nodes[@]}"; do
        local node="${nodes[i]}"
        local info="${node_info[i]}"
        
        echo "        $info" >> "$config_file"
        echo "        server $node max_fails=3 fail_timeout=30s;" >> "$config_file"
        echo "" >> "$config_file"
    done

    cat >> "$config_file" << EOF
    }

    # Main server block
    server {
        listen $LISTEN_PORT;
        
        # Proxy settings
        proxy_pass hypercache_backend;
        proxy_timeout 1s;
        proxy_responses 1;
        proxy_connect_timeout 1s;
        
        # Connection settings
        proxy_bind \$remote_addr transparent;
    }
}

# HTTP module for health checks and management
http {
    # Upstream for HTTP API health checks
    upstream hypercache_http_backend {
EOF

    # Add HTTP upstream servers
    for i in "${!nodes[@]}"; do
        local node="${nodes[i]}"
        local resp_port=$(echo "$node" | cut -d':' -f2)
        local http_port=$((resp_port + 1000))
        local info="${node_info[i]}"
        
        echo "        $info" >> "$config_file"
        echo "        server 127.0.0.1:$http_port max_fails=3 fail_timeout=30s;" >> "$config_file"
    done

    cat >> "$config_file" << EOF
    }

    # Health check endpoint
    server {
        listen $((LISTEN_PORT + 1000));
        location /health {
            proxy_pass http://hypercache_http_backend/health;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
        }
        
        # Nginx stats
        location /nginx_status {
            stub_status on;
            access_log off;
            allow 127.0.0.1;
            deny all;
        }
    }
}
EOF

    echo "✅ Configuration generated: $config_file"
}

# Function to generate startup script
generate_startup_script() {
    local startup_script="$NGINX_DIR/start-nginx.sh"
    
    cat > "$startup_script" << EOF
#!/bin/bash

# HyperCache Nginx Startup Script
# Auto-generated on: $(date)

set -e

NGINX_CONF="$OUTPUT_FILE"
NGINX_PID_FILE="/tmp/hypercache-nginx.pid"

echo "🚀 Starting HyperCache Load Balancer"
echo "=================================="
echo "Config: \$NGINX_CONF"
echo "Listen Port: $LISTEN_PORT"
echo "Health Check Port: $((LISTEN_PORT + 1000))"
echo ""

# Validate configuration
if nginx -t -c "\$NGINX_CONF" 2>/dev/null; then
    echo "✅ Nginx configuration is valid"
else
    echo "❌ Nginx configuration validation failed:"
    nginx -t -c "\$NGINX_CONF"
    exit 1
fi

# Start nginx
echo "🌐 Starting nginx..."
nginx -c "\$NGINX_CONF" -g "pid \$NGINX_PID_FILE;"

echo "✅ Load balancer started successfully!"
echo ""
echo "🔗 Connection Information:"
echo "========================="
echo "Redis Client: redis-cli -h localhost -p $LISTEN_PORT"
echo "Health Check: curl http://localhost:$((LISTEN_PORT + 1000))/health"
echo "Nginx Status: curl http://localhost:$((LISTEN_PORT + 1000))/nginx_status"
echo ""
echo "🛑 To stop: nginx -s quit -c \$NGINX_CONF"
echo "PID file: \$NGINX_PID_FILE"
EOF

    chmod +x "$startup_script"
    echo "📋 Startup script created: $startup_script"
}

# Main execution
echo "🔧 HyperCache Nginx Configuration Generator"
echo "=========================================="
echo "Listen Port: $LISTEN_PORT"
echo "Output File: $OUTPUT_FILE"
echo "Sticky Sessions: $ENABLE_STICKY_SESSIONS"
echo ""

# Discover active nodes
discover_nodes

# Generate configuration
generate_nginx_config "$OUTPUT_FILE"

# Generate startup script
generate_startup_script

echo ""
echo "🎉 Nginx configuration generated successfully!"
echo ""
echo "📋 Next Steps:"
echo "=============="
echo "1. Review the configuration: cat $OUTPUT_FILE"
echo "2. Start the load balancer: $NGINX_DIR/start-nginx.sh"
echo "3. Test the setup: redis-cli -h localhost -p $LISTEN_PORT"
echo ""
echo "🔄 To regenerate after adding/removing nodes:"
echo "  $0"
echo ""
