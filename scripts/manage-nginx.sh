#!/bin/bash

# Simple HyperCache Node Management
# Add/remove nodes and optionally update nginx config

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
NGINX_CONF="$PROJECT_ROOT/nginx/hypercache.conf"

show_help() {
    echo "HyperCache Node Management"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  add-node --port=8083               Add node to nginx config"
    echo "  remove-node --port=8083            Remove node from nginx config"
    echo "  list-nodes                         Show all configured nodes"
    echo "  reload-nginx                       Reload nginx configuration"
    echo ""
    echo "Examples:"
    echo "  $0 add-node --port=8083           # Add node at port 8083"
    echo "  $0 remove-node --port=8082        # Remove node at port 8082"
    echo "  $0 list-nodes                     # Show current nodes"
    echo ""
}

list_nodes() {
    echo "📋 Current HyperCache Nodes in Nginx Config:"
    echo "============================================"
    
    if [[ ! -f "$NGINX_CONF" ]]; then
        echo "❌ Nginx config not found: $NGINX_CONF"
        return 1
    fi
    
    echo ""
    echo "RESP (Redis Protocol) Nodes:"
    grep -n "server 127.0.0.1:" "$NGINX_CONF" | grep -v "# server" | head -10
    
    echo ""
    echo "HTTP API Nodes:"
    grep -n "server 127.0.0.1:" "$NGINX_CONF" | grep "90" | grep -v "# server"
    echo ""
}

add_node() {
    local resp_port=$1
    local http_port=$((resp_port + 1000))
    
    if [[ -z "$resp_port" ]]; then
        echo "❌ Error: Port not specified"
        echo "Usage: $0 add-node --port=8083"
        return 1
    fi
    
    echo "➕ Adding HyperCache node at port $resp_port..."
    
    # Add to RESP upstream (look for the comment line)
    sed -i.bak "/# Add more nodes by uncommenting:/a\\
        server 127.0.0.1:$resp_port max_fails=3 fail_timeout=30s;" "$NGINX_CONF"
    
    # Add to HTTP upstream (simpler match)
    sed -i.bak "/server 127.0.0.1:9082;/a\\
        server 127.0.0.1:$http_port;" "$NGINX_CONF"
    
    echo "✅ Added node: RESP=$resp_port, HTTP=$http_port"
    echo "🔄 Reload nginx to apply changes: $0 reload-nginx"
}

remove_node() {
    local resp_port=$1
    local http_port=$((resp_port + 1000))
    
    if [[ -z "$resp_port" ]]; then
        echo "❌ Error: Port not specified"
        echo "Usage: $0 remove-node --port=8083"
        return 1
    fi
    
    echo "➖ Removing HyperCache node at port $resp_port..."
    
    # Remove from config
    sed -i.bak "/server 127.0.0.1:$resp_port/d" "$NGINX_CONF"
    sed -i.bak "/server 127.0.0.1:$http_port/d" "$NGINX_CONF"
    
    echo "✅ Removed node: RESP=$resp_port, HTTP=$http_port"
    echo "🔄 Reload nginx to apply changes: $0 reload-nginx"
}

reload_nginx() {
    echo "🔄 Reloading Nginx configuration..."
    
    # Test configuration first
    if nginx -t -c "$NGINX_CONF" 2>/dev/null; then
        echo "✅ Nginx configuration is valid"
        
        # Reload if nginx is running
        if pgrep nginx >/dev/null; then
            nginx -s reload -c "$NGINX_CONF"
            echo "✅ Nginx reloaded successfully"
        else
            echo "⚠️  Nginx not running. Start it with:"
            echo "   nginx -c $NGINX_CONF"
        fi
    else
        echo "❌ Nginx configuration validation failed:"
        nginx -t -c "$NGINX_CONF"
        return 1
    fi
}

# Parse command line
if [[ $# -eq 0 ]]; then
    show_help
    exit 1
fi

COMMAND=$1
shift

case $COMMAND in
    add-node)
        PORT=""
        while [[ $# -gt 0 ]]; do
            case $1 in
                --port=*)
                    PORT="${1#*=}"
                    shift
                    ;;
                *)
                    echo "Unknown option: $1"
                    exit 1
                    ;;
            esac
        done
        add_node "$PORT"
        ;;
    remove-node)
        PORT=""
        while [[ $# -gt 0 ]]; do
            case $1 in
                --port=*)
                    PORT="${1#*=}"
                    shift
                    ;;
                *)
                    echo "Unknown option: $1"
                    exit 1
                    ;;
            esac
        done
        remove_node "$PORT"
        ;;
    list-nodes)
        list_nodes
        ;;
    reload-nginx)
        reload_nginx
        ;;
    *)
        echo "Unknown command: $COMMAND"
        show_help
        exit 1
        ;;
esac
