# Development Setup

This guide will help you set up a development environment for HyperCache.

## Prerequisites

- **Go 1.23.2** or later
- **Git** for version control
- **Make** (optional, for build automation)
- **Docker & Docker Compose** (for monitoring stack)

## Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/rishabhverma17/HyperCache.git
cd HyperCache
```

### 2. Install Dependencies

```bash
go mod download
go mod verify
```

### 3. Build the Project

```bash
# Build the main binary
go build -o bin/hypercache cmd/hypercache/main.go

# Or use the build script
chmod +x scripts/build-hypercache.sh
./scripts/build-hypercache.sh
```

### 4. Run Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/cache/
```

### 5. Start Development Server

```bash
# Start with default configuration
./bin/hypercache

# Start with custom config
./bin/hypercache -config configs/test-config.yaml
```

## Development Workflow

### Directory Structure

```
├── cmd/                 # Application entry points
├── internal/           # Private application code
│   ├── cache/          # Core cache implementation
│   ├── cluster/        # Clustering and gossip
│   ├── filter/         # Cuckoo filter implementation
│   ├── logging/        # Logging utilities
│   ├── network/        # Network protocols (RESP)
│   ├── persistence/    # Data persistence (AOF, WAL)
│   └── storage/        # Storage engines
├── pkg/                # Public library code
├── configs/            # Configuration files
├── scripts/            # Build and deployment scripts
└── docs/               # Documentation
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `go vet` for static analysis
- Use meaningful variable and function names
- Add comments for exported functions

### Configuration

HyperCache uses YAML configuration files. Key configuration sections:

- **Server**: Port, timeouts, limits
- **Cluster**: Node discovery, gossip settings
- **Persistence**: AOF/WAL configuration
- **Logging**: Log levels and outputs
- **Storage**: Memory limits, eviction policies

Example development config:

```yaml
server:
  host: "127.0.0.1"
  port: 6379
  timeout: "30s"

cluster:
  enabled: false
  node_id: "dev-node"

persistence:
  enabled: true
  type: "aof"
  sync_policy: "everysec"

logging:
  level: "debug"
  output: "console"
```

### Monitoring Stack (Optional)

For development with monitoring:

```bash
# Start Elasticsearch and Kibana
docker-compose -f docker-compose.logging.yml up -d

# Start Grafana (if configured)
# Check grafana/ directory for provisioning
```

### Common Development Tasks

#### Adding New Cache Operations

1. Define interface in `internal/cache/interfaces.go`
2. Implement in storage engine (`internal/storage/`)
3. Add RESP protocol handler (`internal/network/`)
4. Write tests

#### Adding New Persistence Features

1. Extend persistence interface (`internal/persistence/`)
2. Implement for AOF/WAL handlers
3. Add configuration options
4. Test with different sync policies

#### Debugging

- Use `go run -race` to detect race conditions
- Enable debug logging in config
- Use Go's built-in profiler: `go tool pprof`
- Monitor memory usage: `go tool trace`

### Environment Variables

```bash
# Development environment
export HYPERCACHE_ENV=development
export HYPERCACHE_LOG_LEVEL=debug
export HYPERCACHE_CONFIG_PATH=configs/test-config.yaml

# Testing
export HYPERCACHE_TEST_PERSISTENCE_DIR=/tmp/hypercache-test
export HYPERCACHE_TEST_CLEANUP=true
```

## IDE Setup

### VS Code

Recommended extensions:
- Go (official)
- gopls (Go language server)
- Go Test Explorer

Settings:
```json
{
  "go.useLanguageServer": true,
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint"
}
```

### GoLand/IntelliJ

- Enable Go modules support
- Configure code style to use gofmt
- Set up run configurations for different node configs

## Troubleshooting

### Common Issues

1. **Build Failures**: Ensure Go 1.23.2+ is installed
2. **Module Issues**: Run `go mod tidy` to clean dependencies
3. **Port Conflicts**: Change server.port in config file
4. **Permission Errors**: Check file permissions for persistence directory

### Getting Help

- Review existing [GitHub Issues](https://github.com/rishabhverma17/HyperCache/issues)
- Join discussions in GitHub Discussions

## Next Steps

- Review [Code Structure](code-structure.md) to understand the architecture
- Read [Contribution Guidelines](contribution-guidelines.md) before submitting PRs
- Explore the `/examples` directory for usage patterns
