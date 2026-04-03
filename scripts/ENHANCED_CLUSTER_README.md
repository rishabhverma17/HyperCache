# Enhanced HyperCache Cluster Management

This directory contains enhanced cluster management scripts that support dynamic scaling, load balancing, and template-based configuration generation.

## Overview

The enhanced cluster management system provides:
- **Dynamic Node Scaling**: Start N nodes with a single command
- **Auto Port Assignment**: Automatic port scanning and assignment
- **Template-Based Configs**: Consistent configuration generation
- **Load Balancer Integration**: Nginx configuration generation
- **Single-Node Addition**: Add nodes to running clusters
- **Health Monitoring**: Built-in health checks and status reporting

## Quick Start

### 1. Start a Basic Cluster
```bash
# Start 3 nodes (backward compatible)
./scripts/start-cluster-enhanced.sh

# Start 5 nodes with custom configuration
./scripts/start-cluster-enhanced.sh --nodes=5 --log-level=info

# Start 10 nodes with auto port scanning
./scripts/start-cluster-enhanced.sh --nodes=10 --auto-ports --base-port=9000
```

### 2. Add a Node to Running Cluster
```bash
# Add a node with auto-discovery
./scripts/add-node.sh

# Add a named node
./scripts/add-node.sh --node-name=api-server-1 --log-level=info
```

### 3. Setup Load Balancer (Simple Approach)
```bash
# Edit nginx config to match your nodes
vim nginx/hypercache.conf

# Start nginx load balancer
./scripts/start-nginx.sh

# Or use management script to add nodes
./scripts/manage-nginx.sh add-node --port=8083
./scripts/manage-nginx.sh reload-nginx
```

## File Structure

```
templates/
├── node-config.yaml.template     # Template for node configurations

scripts/
├── start-cluster-enhanced.sh     # Enhanced cluster startup
├── add-node.sh                   # Dynamic node addition
├── manage-nginx.sh               # Simple nginx node management
├── start-nginx.sh                # Nginx startup script
└── start-cluster.sh             # Legacy script (backward compatible)

nginx/
├── hypercache.conf               # Static nginx configuration
├── hypercache.conf.template      # Environment-based template
└── README.md                     # Nginx configuration guide

configs/
├── node-1-config.yaml           # Generated node configurations
├── node-2-config.yaml
└── ...
```

## Script Details

### start-cluster-enhanced.sh

Enhanced cluster startup with template-based configuration generation.

**Features:**
- Dynamic node count (1-N nodes)
- Auto port scanning or incremental assignment
- Template-based configuration generation
- Health check verification
- Backward compatibility with existing scripts

**Options:**
- `--nodes=N`: Number of nodes to start (default: 3)
- `--base-port=PORT`: Starting port number (default: 8080)
- `--auto-ports`: Enable automatic port scanning
- `--log-level=LEVEL`: Set log level (debug, info, warn, error)

**Examples:**
```bash
# Standard 3-node cluster
./scripts/start-cluster-enhanced.sh

# Large cluster with auto port scanning
./scripts/start-cluster-enhanced.sh --nodes=20 --auto-ports

# Production setup with info logging
./scripts/start-cluster-enhanced.sh --nodes=5 --log-level=info --base-port=8000
```

### add-node.sh

Dynamic node addition to running clusters.

**Features:**
- Automatic cluster discovery
- Port conflict resolution
- Template-based configuration
- Health check verification
- Integration guidance

**Options:**
- `--node-name=NAME`: Custom node identifier
- `--base-port=PORT`: Preferred port range for scanning
- `--log-level=LEVEL`: Set log level

**Examples:**
```bash
# Add node with auto-generated name
./scripts/add-node.sh

# Add named node for specific purpose
./scripts/add-node.sh --node-name=worker-01

# Add node with custom port preference
./scripts/add-node.sh --base-port=9000 --log-level=warn
```

### manage-nginx.sh

Simple nginx configuration management for adding/removing nodes.

**Features:**
- Add/remove nodes from nginx config
- Configuration validation
- Nginx reload automation
- Node listing and status

**Options:**
- `add-node --port=PORT`: Add node to load balancer
- `remove-node --port=PORT`: Remove node from load balancer
- `list-nodes`: Show configured nodes
- `reload-nginx`: Reload nginx configuration

**Examples:**
```bash
# Add a new node
./scripts/manage-nginx.sh add-node --port=8083

# Remove a node
./scripts/manage-nginx.sh remove-node --port=8082

# List all configured nodes
./scripts/manage-nginx.sh list-nodes

# Reload nginx after changes
./scripts/manage-nginx.sh reload-nginx
```

## Configuration Template

The `templates/node-config.yaml.template` file contains environment variable placeholders:

- `${NODE_ID}`: Unique node identifier
- `${RESP_PORT}`: Redis protocol port
- `${HTTP_PORT}`: HTTP API port
- `${GOSSIP_PORT}`: Serf gossip protocol port
- `${CLUSTER_SEEDS}`: Comma-separated list of cluster seeds
- `${LOG_LEVEL}`: Logging level

## Port Management

The scripts use intelligent port assignment:

**Port Ranges:**
- RESP Ports: Base port + offset (e.g., 8080, 8081, 8082...)
- HTTP Ports: Base port + 1000 + offset (e.g., 9080, 9081, 9082...)
- Gossip Ports: Base port - 2000 + offset (e.g., 6080, 6081, 6082...)

**Auto Port Scanning:**
- Scans for available ports starting from base port
- Prevents conflicts with existing services
- Safety limits prevent infinite loops
- Reports all assigned ports clearly

## Load Balancing

The nginx configuration provides:

**Stream Module (Redis Protocol):**
- TCP load balancing for Redis clients
- Health checks and failover
- Connection pooling
- Configurable load balancing algorithms

**HTTP Module (API Access):**
- HTTP API load balancing
- Health check endpoints
- Nginx status monitoring
- Management interface

## Health Monitoring

Built-in health checks verify:
- **RESP Protocol**: Redis PING command responses
- **HTTP API**: Health endpoint availability
- **Process Status**: PID tracking and management
- **Port Availability**: Conflict detection

## Migration Guide

### From Legacy Scripts

The enhanced scripts are backward compatible:

```bash
# Legacy way (still works)
./scripts/start-cluster.sh

# Enhanced way (same result)
./scripts/start-cluster-enhanced.sh

# Enhanced with more features
./scripts/start-cluster-enhanced.sh --nodes=3 --log-level=debug
```

### Configuration Files

Generated configurations maintain compatibility:
- Same YAML structure as manual configs
- Environment-based customization
- Preserves all existing functionality

## Troubleshooting

### Common Issues

**Port Conflicts:**
```bash
# Check for conflicting processes
lsof -i :8080

# Use auto port scanning
./scripts/start-cluster-enhanced.sh --auto-ports
```

**Template Not Found:**
```bash
# Ensure templates directory exists
mkdir -p templates

# Regenerate template if needed
./scripts/start-cluster-enhanced.sh --help
```

**Nginx Configuration Issues:**
```bash
# Test nginx configuration
nginx -t -c nginx/hypercache.conf

# Check nginx error logs
tail -f /var/log/nginx/hypercache_error.log
```

### Debug Mode

Enable debug logging for troubleshooting:
```bash
# Start cluster with debug logging
./scripts/start-cluster-enhanced.sh --log-level=debug

# Add node with debug logging
./scripts/add-node.sh --log-level=debug
```

### Health Check Scripts

Manual health verification:
```bash
# Check all nodes
for port in 8080 8081 8082; do
  echo "Node $port: $(redis-cli -h localhost -p $port ping)"
done

# Check HTTP APIs
for port in 9080 9081 9082; do
  curl -s http://localhost:$port/health | jq .
done
```

## Performance Considerations

### Scaling Guidelines

**Small Clusters (1-5 nodes):**
- Use incremental port assignment
- Standard logging levels
- Basic load balancing

**Medium Clusters (5-20 nodes):**
- Enable auto port scanning
- Use info/warn logging
- Consider sticky sessions

**Large Clusters (20+ nodes):**
- Use dedicated port ranges
- Optimize logging levels
- Implement monitoring

### Resource Management

**Memory Usage:**
- Each node uses ~50MB base memory
- Template generation is lightweight
- Configuration files are minimal

**Network Ports:**
- 3 ports per node (RESP, HTTP, Gossip)
- Auto scanning prevents conflicts
- Configurable port ranges

## Integration Examples

### Docker Deployment
```bash
# Generate configurations
./scripts/start-cluster-enhanced.sh --nodes=5

# Build Docker images with generated configs
docker-compose up --scale hypercache=5
```

### Kubernetes Deployment
```bash
# Generate configs for K8s
./scripts/start-cluster-enhanced.sh --nodes=3 --base-port=8080

# Apply as ConfigMaps
kubectl create configmap hypercache-configs --from-file=configs/
```

### CI/CD Pipeline
```bash
# Automated testing
./scripts/start-cluster-enhanced.sh --nodes=3 --log-level=warn
./scripts/generate-nginx-config.sh
# Run tests
pkill -f hypercache
```

## Contributing

When extending these scripts:

1. **Maintain Backward Compatibility**: Ensure existing usage patterns continue to work
2. **Add Comprehensive Help**: Include detailed help text and examples
3. **Test All Options**: Verify all parameter combinations work correctly
4. **Update Documentation**: Keep this README current with new features
5. **Error Handling**: Provide clear error messages and recovery suggestions

## Support

For issues with the enhanced cluster scripts:

1. Check the troubleshooting section above
2. Enable debug logging to identify the issue
3. Review generated configuration files for correctness
4. Test individual components (nginx, node health, port availability)
5. Create an issue with detailed logs and reproduction steps
