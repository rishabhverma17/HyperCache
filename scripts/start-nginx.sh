#!/bin/bash

# Simple Nginx Startup Script for HyperCache Load Balancer

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
NGINX_CONF="$PROJECT_ROOT/nginx/hypercache.conf"

echo "🚀 Starting HyperCache Nginx Load Balancer"
echo "=========================================="
echo ""

# Check if nginx config exists
if [[ ! -f "$NGINX_CONF" ]]; then
    echo "❌ Nginx config not found: $NGINX_CONF"
    echo ""
    echo "Please ensure you have the nginx configuration file."
    echo "You can copy from: nginx/hypercache.conf"
    exit 1
fi

# Validate configuration
echo "🔍 Validating nginx configuration..."
if nginx -t -c "$NGINX_CONF" 2>/dev/null; then
    echo "✅ Nginx configuration is valid"
else
    echo "❌ Nginx configuration validation failed:"
    nginx -t -c "$NGINX_CONF"
    exit 1
fi

# Check if nginx is already running
if pgrep nginx >/dev/null; then
    echo "⚠️  Nginx is already running"
    echo "To reload: nginx -s reload -c $NGINX_CONF"
    echo "To stop: nginx -s quit"
    exit 1
fi

# Start nginx
echo "🌐 Starting nginx with HyperCache configuration..."
nginx -c "$NGINX_CONF"

if pgrep nginx >/dev/null; then
    echo "✅ Nginx started successfully!"
    echo ""
    echo "🔗 Connection Information:"
    echo "========================="
    echo "Redis Client: redis-cli -h localhost -p 6379"
    echo "Health Check: curl http://localhost:8080/health"
    echo "Nginx Status: curl http://localhost:8080/nginx_status"
    echo ""
    echo "🛑 To stop nginx:"
    echo "nginx -s quit"
    echo ""
    echo "📋 To manage nodes:"
    echo "./scripts/manage-nginx.sh --help"
else
    echo "❌ Failed to start nginx"
    exit 1
fi
