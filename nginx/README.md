# Nginx Load Balancer Configuration Guide

This guide covers multiple approaches for configuring nginx as a load balancer for HyperCache, from simple static configs to dynamic service discovery.

## 🎯 Quick Start (Recommended for Most Users)

### Step 1: Start Your HyperCache Cluster
```bash
# Start 3 nodes
./scripts/start-cluster-enhanced.sh --nodes=3

# Or start 5 nodes with custom ports
./scripts/start-cluster-enhanced.sh --nodes=5 --base-port=8000
```

### Step 2: Use the Static Nginx Config
```bash
# Edit the nginx config to match your nodes
vim nginx/hypercache.conf

# Start nginx
./scripts/start-nginx.sh

# Connect via load balancer
redis-cli -h localhost -p 6379
```

## 📋 Available Approaches

### Approach 1: Static Configuration (Simplest)

**Best for:** Small, stable clusters with known node counts.

**File:** `nginx/hypercache.conf`

**How it works:**
- Manually edit the config file to add/remove nodes
- Simple, predictable, no external dependencies
- Perfect for development and small production deployments

**Adding a node:**
```bash
# Method A: Edit config manually
vim nginx/hypercache.conf
# Add: server 127.0.0.1:8083 max_fails=3 fail_timeout=30s;

# Method B: Use management script
./scripts/manage-nginx.sh add-node --port=8083
./scripts/manage-nginx.sh reload-nginx
```

**Pros:**
- ✅ Simple and reliable
- ✅ No external dependencies
- ✅ Easy to understand and debug
- ✅ Works with any nginx installation

**Cons:**
- ❌ Manual configuration required
- ❌ Requires nginx reload for changes

### Approach 2: Management Script (Recommended)

**Best for:** Clusters where you occasionally add/remove nodes.

**Files:** `scripts/manage-nginx.sh`

**How it works:**
- Script automatically edits nginx config
- Validates configuration before applying
- Handles both RESP and HTTP upstreams

**Usage:**
```bash
# Add a node
./scripts/manage-nginx.sh add-node --port=8083

# Remove a node
./scripts/manage-nginx.sh remove-node --port=8083

# List current nodes
./scripts/manage-nginx.sh list-nodes

# Reload nginx
./scripts/manage-nginx.sh reload-nginx
```

**Pros:**
- ✅ Automated config management
- ✅ Built-in validation
- ✅ Error handling
- ✅ Easy to use

**Cons:**
- ❌ Still requires nginx reload
- ❌ Limited to simple use cases

### Approach 3: Docker Compose (Service Discovery)

**Best for:** Containerized deployments with automatic scaling.

**File:** `docker-compose.loadbalancer.yml`

**How it works:**
- Docker handles service discovery
- Nginx uses service names instead of IP:port
- Automatically scales with `docker-compose scale`

**Usage:**
```bash
# Start the stack
docker-compose -f docker-compose.loadbalancer.yml up -d

# Scale nodes
docker-compose -f docker-compose.loadbalancer.yml up -d --scale hypercache-1=3

# Connect
redis-cli -h localhost -p 6379
```

**Pros:**
- ✅ True service discovery
- ✅ Automatic scaling
- ✅ Container isolation
- ✅ Production ready

**Cons:**
- ❌ Requires Docker
- ❌ More complex setup
- ❌ Learning curve for Docker Compose

### Approach 4: Environment-Based Template

**Best for:** Multiple environments (dev/staging/prod) with different configurations.

**File:** `nginx/hypercache.conf.template`

**How it works:**
- Uses environment variables in template
- Generate actual config with `envsubst`
- Different configs for different environments

**Usage:**
```bash
# Set environment variables
export NODE1_RESP_PORT=8080
export NODE2_RESP_PORT=8081
export NODE3_RESP_PORT=8082
export NGINX_RESP_PORT=6379

# Generate config
envsubst < nginx/hypercache.conf.template > nginx/hypercache.conf

# Start nginx
./scripts/start-nginx.sh
```

**Pros:**
- ✅ Environment-specific configs
- ✅ Template-based consistency
- ✅ Good for CI/CD pipelines
- ✅ Version controlled templates

**Cons:**
- ❌ Requires envsubst utility
- ❌ More complex setup
- ❌ Environment variable management

## 🔧 Configuration Details

### Load Balancing Algorithms

Edit the `upstream` block to change algorithms:

```nginx
upstream hypercache_backend {
    # Round-robin (default)
    server 127.0.0.1:8080;
    
    # OR least connections
    least_conn;
    server 127.0.0.1:8080;
    
    # OR IP hash (sticky sessions)
    ip_hash;
    server 127.0.0.1:8080;
}
```

### Health Check Configuration

Adjust health check parameters:

```nginx
server 127.0.0.1:8080 max_fails=3 fail_timeout=30s weight=5;
#                     ^^^^^^^^^^ ^^^^^^^^^^^^^  ^^^^^^^^
#                     Max failed  Timeout      Load weight
#                     attempts    before retry
```

### Port Configuration

**Default Ports:**
- Nginx Redis Proxy: `6379`
- Nginx HTTP Management: `8080`
- HyperCache RESP: `8080, 8081, 8082...`
- HyperCache HTTP: `9080, 9081, 9082...`

**Custom Ports:**
```nginx
server {
    listen 7000;  # Custom Redis port
    proxy_pass hypercache_backend;
}
```

## 🚀 Integration with HyperCache Scripts

### Adding Nodes Workflow

**Step 1:** Add the node to your cluster
```bash
./scripts/add-node.sh --node-name=worker-01
```

**Step 2:** Add to nginx (choose method)
```bash
# Method A: Management script
./scripts/manage-nginx.sh add-node --port=8083

# Method B: Manual edit
vim nginx/hypercache.conf
# Add server line

# Method C: Environment update (for template approach)
export NODE4_RESP_PORT=8083
envsubst < nginx/hypercache.conf.template > nginx/hypercache.conf
```

**Step 3:** Reload nginx
```bash
./scripts/manage-nginx.sh reload-nginx
# OR
nginx -s reload
```

### Automation with Scripts

You can integrate nginx management into your node management:

```bash
# In add-node.sh (already included)
if [[ -f "$SCRIPT_DIR/manage-nginx.sh" ]]; then
    echo "Auto-adding to nginx..."
    ./scripts/manage-nginx.sh add-node --port=$RESP_PORT
    ./scripts/manage-nginx.sh reload-nginx
fi
```

## 🔍 Monitoring and Debugging

### Check Nginx Status
```bash
# Nginx status
curl http://localhost:8080/nginx_status

# HyperCache health through load balancer
curl http://localhost:8080/health

# Test Redis connection
redis-cli -h localhost -p 6379 ping
```

### Debug Connection Issues
```bash
# Check nginx error logs
tail -f /var/log/nginx/hypercache_error.log

# Check which backend is being used
redis-cli -h localhost -p 6379 info server | grep tcp_port

# Test direct connection to nodes
for port in 8080 8081 8082; do
  echo "Testing port $port: $(redis-cli -h localhost -p $port ping)"
done
```

## 💡 Best Practices

### Development
- Use **Static Configuration** for simplicity
- Keep 2-3 nodes for testing
- Use management script for occasional changes

### Staging
- Use **Environment-Based Template** for consistency
- Test scaling scenarios
- Validate health checks work properly

### Production
- Use **Docker Compose** for service discovery
- Implement proper logging and monitoring
- Use health checks and failover settings
- Monitor nginx metrics

### Security
- Restrict nginx status endpoint to localhost
- Use proper firewall rules
- Consider TLS termination at nginx
- Implement rate limiting if needed

## 📚 Examples

### Quick 3-Node Setup
```bash
# Start cluster
./scripts/start-cluster-enhanced.sh --nodes=3

# Verify nginx config matches your ports
cat nginx/hypercache.conf

# Start nginx
./scripts/start-nginx.sh

# Test
redis-cli -h localhost -p 6379 set test "hello"
redis-cli -h localhost -p 6379 get test
```

### Adding Node in Production
```bash
# Add node to cluster
./scripts/add-node.sh --node-name=prod-worker-04

# Add to nginx
./scripts/manage-nginx.sh add-node --port=8083

# Verify nginx config
./scripts/manage-nginx.sh list-nodes

# Reload nginx
./scripts/manage-nginx.sh reload-nginx

# Test load distribution
for i in {1..10}; do
  redis-cli -h localhost -p 6379 set test$i "value$i"
done
```

This approach gives you flexibility while keeping things simple and maintainable!
