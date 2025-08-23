#!/bin/bash

# HyperCache Docker Volume Initialization Script
# This script ensures proper permissions for Docker volumes

set -e

echo "🐳 Initializing HyperCache Docker volumes..."

# Create volumes if they don't exist
echo "Creating Docker volumes..."
docker volume create hypercache_logs 2>/dev/null || true
docker volume create hypercache_node1_data 2>/dev/null || true  
docker volume create hypercache_node2_data 2>/dev/null || true
docker volume create hypercache_node3_data 2>/dev/null || true
docker volume create elasticsearch_data 2>/dev/null || true
docker volume create grafana_data 2>/dev/null || true

# Set proper permissions for log volume
echo "Setting permissions for log volume..."
docker run --rm \
    -v hypercache_logs:/app/logs \
    alpine:3.18 \
    sh -c "
        mkdir -p /app/logs && 
        chmod 755 /app/logs && 
        chown 1000:1000 /app/logs &&
        echo 'Log volume permissions set: $(ls -la /app/)'
    "

# Set permissions for data volumes
echo "Setting permissions for data volumes..."
for i in 1 2 3; do
    docker run --rm \
        -v hypercache_node${i}_data:/data \
        alpine:3.18 \
        sh -c "
            mkdir -p /data && 
            chmod 755 /data && 
            chown 1000:1000 /data &&
            echo 'Data volume ${i} permissions set'
        "
done

echo "✅ Volume initialization complete!"
echo ""
echo "You can now run:"
echo "  docker-compose -f docker-compose.cluster.yml up -d"
echo ""
echo "Or use the deployment script:"
echo "  ./scripts/docker-deploy.sh start"
