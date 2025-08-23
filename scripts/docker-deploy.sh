#!/bin/bash

# HyperCache Docker Deployment Script
# Handles building, pushing to Docker Hub, and running the cluster

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

# Configuration
DOCKER_USERNAME="${DOCKER_USERNAME:-hypercache}"
IMAGE_NAME="${IMAGE_NAME:-hypercache}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
FULL_IMAGE_NAME="$DOCKER_USERNAME/$IMAGE_NAME:$IMAGE_TAG"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
    exit 1
}

# Function to check Docker Hub credentials
check_docker_credentials() {
    if [ -z "$DOCKER_USERNAME" ] || [ -z "$DOCKER_PASSWORD" ]; then
        warn "Docker Hub credentials not set. Please set DOCKER_USERNAME and DOCKER_PASSWORD environment variables"
        warn "You can also login manually: docker login"
        return 1
    fi
    
    echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin
}

# Function to build the Docker image
build_image() {
    log "Building HyperCache Docker image: $FULL_IMAGE_NAME"
    
    # Create build context
    log "Creating optimized build context..."
    
    # Build multi-stage Docker image
    docker build \
        --build-arg BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
        --build-arg VCS_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" \
        --build-arg VERSION="$(git describe --tags --always 2>/dev/null || echo 'dev')" \
        -t "$FULL_IMAGE_NAME" \
        -t "$DOCKER_USERNAME/$IMAGE_NAME:$(git rev-parse --short HEAD 2>/dev/null || echo 'dev')" \
        .
    
    log "Docker image built successfully: $FULL_IMAGE_NAME"
    
    # Show image size
    docker images "$FULL_IMAGE_NAME" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
}

# Function to push to Docker Hub
push_image() {
    log "Pushing image to Docker Hub: $FULL_IMAGE_NAME"
    
    if ! check_docker_credentials; then
        error "Docker Hub authentication failed"
    fi
    
    docker push "$FULL_IMAGE_NAME"
    
    # Also push the commit-tagged version
    COMMIT_TAG="$(git rev-parse --short HEAD 2>/dev/null || echo 'dev')"
    if [ "$COMMIT_TAG" != "dev" ]; then
        docker push "$DOCKER_USERNAME/$IMAGE_NAME:$COMMIT_TAG"
    fi
    
    log "Image pushed successfully to Docker Hub"
}

# Function to start the cluster
start_cluster() {
    log "Starting HyperCache Docker cluster..."
    
    # Ensure network exists
    docker network create hypercache-network 2>/dev/null || true
    
    # Use docker-compose to start the cluster
    docker-compose -f docker-compose.cluster.yml up -d
    
    log "Cluster started successfully"
    log "Access points:"
    log "  - Node 1: http://localhost:9080 (RESP: localhost:8080)"
    log "  - Node 2: http://localhost:9081 (RESP: localhost:8081)"
    log "  - Node 3: http://localhost:9082 (RESP: localhost:8082)"
    log "  - Grafana: http://localhost:3000 (admin/admin123)"
    log "  - Elasticsearch: http://localhost:9200"
}

# Function to stop the cluster
stop_cluster() {
    log "Stopping HyperCache Docker cluster..."
    docker-compose -f docker-compose.cluster.yml down
    log "Cluster stopped"
}

# Function to show cluster status
status_cluster() {
    log "HyperCache Docker cluster status:"
    docker-compose -f docker-compose.cluster.yml ps
    
    echo
    log "Health checks:"
    for port in 9080 9081 9082; do
        if curl -sf "http://localhost:$port/health" >/dev/null 2>&1; then
            echo -e "  ${GREEN}✓${NC} Node on port $port is healthy"
        else
            echo -e "  ${RED}✗${NC} Node on port $port is not responding"
        fi
    done
}

# Function to view logs
logs_cluster() {
    local service="$1"
    if [ -n "$service" ]; then
        docker-compose -f docker-compose.cluster.yml logs -f "$service"
    else
        docker-compose -f docker-compose.cluster.yml logs -f
    fi
}

# Function to test the cluster
test_cluster() {
    log "Testing HyperCache cluster..."
    
    # Wait for nodes to be ready
    for i in {1..30}; do
        if curl -sf "http://localhost:9080/health" >/dev/null 2>&1; then
            break
        fi
        sleep 2
    done
    
    # Test basic operations
    log "Testing basic operations..."
    
    # PUT operation
    curl -X PUT "http://localhost:9080/api/cache/test-key" \
        -H "Content-Type: application/json" \
        -d '{"value":"Hello Docker HyperCache!","ttl_hours":1}' || error "PUT operation failed"
    
    # GET operation from different node
    RESPONSE=$(curl -s "http://localhost:9081/api/cache/test-key")
    if echo "$RESPONSE" | grep -q "Hello Docker HyperCache!"; then
        log "✓ Cross-node GET operation successful"
    else
        error "Cross-node GET operation failed: $RESPONSE"
    fi
    
    # DELETE operation
    curl -X DELETE "http://localhost:9082/api/cache/test-key" || error "DELETE operation failed"
    
    log "✓ Cluster test completed successfully"
}

# Function to clean everything
clean_all() {
    log "Cleaning up HyperCache Docker resources..."
    
    # Stop and remove containers
    docker-compose -f docker-compose.cluster.yml down -v --remove-orphans
    
    # Remove images
    docker rmi "$FULL_IMAGE_NAME" 2>/dev/null || true
    docker rmi "$DOCKER_USERNAME/$IMAGE_NAME:$(git rev-parse --short HEAD 2>/dev/null || echo 'dev')" 2>/dev/null || true
    
    # Remove networks
    docker network rm hypercache-network 2>/dev/null || true
    
    # Clean Docker system
    docker system prune -f
    
    log "Cleanup completed"
}

# Function to show usage
usage() {
    echo "HyperCache Docker Deployment Script"
    echo
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo
    echo "Commands:"
    echo "  build          Build the Docker image"
    echo "  push           Push image to Docker Hub"
    echo "  start          Start the HyperCache cluster"
    echo "  stop           Stop the cluster"
    echo "  restart        Restart the cluster"
    echo "  status         Show cluster status"
    echo "  logs [service] View cluster logs"
    echo "  test           Test cluster functionality"
    echo "  clean          Clean up all resources"
    echo "  deploy         Build, push, and start (full deployment)"
    echo
    echo "Environment Variables:"
    echo "  DOCKER_USERNAME - Docker Hub username (default: hypercache)"
    echo "  DOCKER_PASSWORD - Docker Hub password"
    echo "  IMAGE_NAME      - Image name (default: hypercache)"
    echo "  IMAGE_TAG       - Image tag (default: latest)"
    echo
    echo "Examples:"
    echo "  $0 build                 # Build image locally"
    echo "  $0 deploy                # Full deployment to Docker Hub"
    echo "  $0 start                 # Start local cluster"
    echo "  $0 logs hypercache-node1 # View node 1 logs"
}

# Main script logic
case "${1:-help}" in
    "build")
        build_image
        ;;
    "push")
        push_image
        ;;
    "start")
        start_cluster
        ;;
    "stop")
        stop_cluster
        ;;
    "restart")
        stop_cluster
        build_image
        start_cluster
        ;;
    "status")
        status_cluster
        ;;
    "logs")
        logs_cluster "$2"
        ;;
    "test")
        test_cluster
        ;;
    "clean")
        clean_all
        ;;
    "deploy")
        build_image
        push_image
        start_cluster
        test_cluster
        ;;
    "help"|*)
        usage
        ;;
esac
