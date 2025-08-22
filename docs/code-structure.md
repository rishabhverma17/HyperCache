# Code Structure

This document provides an overview of HyperCache's codebase architecture and organization.

## Project Layout

HyperCache follows Go project layout standards with clear separation of concerns:

```
HyperCache/
├── cmd/                    # Application entry points
│   └── hypercache/
│       └── main.go         # Main server application
├── internal/               # Private application code
│   ├── cache/              # Core cache operations
│   ├── cluster/            # Distributed clustering
│   ├── filter/             # Probabilistic data structures
│   ├── logging/            # Logging infrastructure
│   ├── network/            # Network protocols
│   ├── persistence/        # Data persistence
│   └── storage/            # Storage engines
├── pkg/                    # Public library code
│   └── config/             # Configuration management
├── configs/                # Configuration files
├── scripts/                # Automation scripts
├── docs/                   # Documentation
├── examples/               # Usage examples
└── tests/                  # Integration tests
```

## Core Modules

### 1. Cache Module (`internal/cache/`)

The heart of HyperCache, implementing core caching operations and policies.

**Key Files:**
- `interfaces.go` - Core cache interfaces and contracts
- `session_eviction_policy.go` - LRU and other eviction strategies
- `operations.go` - GET, SET, DEL operations
- `expiration.go` - TTL and expiration handling

**Key Interfaces:**
```go
type Cache interface {
    Get(key string) ([]byte, bool)
    Set(key string, value []byte, ttl time.Duration) error
    Delete(key string) error
    Exists(key string) bool
}

type EvictionPolicy interface {
    OnAccess(key string)
    OnSet(key string)
    OnDelete(key string)
    GetEvictionCandidate() (string, bool)
}
```

### 2. Storage Module (`internal/storage/`)

Multiple storage engines with different characteristics:

- **In-Memory Store** - Fast, volatile storage
- **Hybrid Store** - Memory + disk backing
- **Persistent Store** - Disk-based storage

**Key Components:**
- Storage interface abstraction
- Memory management and limits
- Data serialization/deserialization
- Storage-specific optimizations

### 3. Network Module (`internal/network/`)

Implements Redis-compatible RESP protocol for client communication.

**Key Files:**
- `resp_parser.go` - RESP protocol parsing
- `commands.go` - Command handlers
- `connection.go` - Client connection management
- `server.go` - TCP server implementation

**Supported Commands:**
- Basic: GET, SET, DEL, EXISTS
- Advanced: EXPIRE, TTL, KEYS, FLUSHALL
- Info: PING, INFO, CONFIG

### 4. Persistence Module (`internal/persistence/`)

Ensures data durability through multiple strategies:

**AOF (Append-Only File):**
- `aof_writer.go` - Append-only file operations
- `aof_reader.go` - Recovery from AOF files
- `aof_compaction.go` - Log compaction

**WAL (Write-Ahead Log):**
- `wal_writer.go` - Write-ahead logging
- `wal_recovery.go` - Crash recovery
- `checkpoint.go` - Periodic checkpointing

**Key Features:**
- Configurable sync policies (always, everysec, no)
- Background compaction
- Crash-safe recovery
- File rotation and cleanup

### 5. Cluster Module (`internal/cluster/`)

Distributed caching with consistent hashing and gossip protocol.

**Key Components:**
- **Hash Ring** (`hash_ring.go`) - Consistent hashing implementation
- **Gossip** (`gossip.go`) - Node discovery and membership
- **Replication** (`replication.go`) - Data replication strategies
- **Consensus** (`raft.go`) - Raft consensus for configuration

**Architecture:**
```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Node 1   │    │    Node 2   │    │    Node 3   │
│             │    │             │    │             │
│ Hash: 0-340 │◄──►│ Hash:341-680│◄──►│ Hash:681-1023│
│ Replicas: 2 │    │ Replicas: 3 │    │ Replicas: 1 │
└─────────────┘    └─────────────┘    └─────────────┘
```

### 6. Filter Module (`internal/filter/`)

Implements Cuckoo Filter for membership testing with deletion support.

**Key Features:**
- False positive rate < 3%
- Supports deletions (unlike Bloom filters)
- Memory efficient
- Fast lookups (O(1))

**Use Cases:**
- Cache admission control
- Duplicate request filtering
- Memory optimization

### 7. Logging Module (`internal/logging/`)

Structured logging with multiple backends:

- **Console Logging** - Development and debugging
- **File Logging** - Production log files
- **Elastic Stack** - Centralized logging via Filebeat

**Features:**
- Configurable log levels (debug, info, warn, error)
- Context-aware logging
- Performance monitoring
- Request tracing

## Data Flow

### Write Path
```
Client Request
     ↓
RESP Parser
     ↓
Command Handler
     ↓
Cache Interface
     ↓
Storage Engine ←→ Persistence Layer
     ↓
Cluster Replication (if enabled)
```

### Read Path
```
Client Request
     ↓
RESP Parser
     ↓
Command Handler
     ↓
Cuckoo Filter (membership test)
     ↓
Cache Interface
     ↓
Storage Engine
     ↓
Response
```

## Configuration Management

Configuration is handled through `pkg/config/`:

```go
type Config struct {
    Server      ServerConfig      `yaml:"server"`
    Storage     StorageConfig     `yaml:"storage"`
    Persistence PersistenceConfig `yaml:"persistence"`
    Cluster     ClusterConfig     `yaml:"cluster"`
    Logging     LoggingConfig     `yaml:"logging"`
}
```

**Configuration Sources:**
1. Default values (embedded)
2. Configuration files (YAML)
3. Environment variables
4. Command-line flags

## Error Handling

HyperCache uses Go's standard error handling with custom error types:

```go
type CacheError struct {
    Op      string    // Operation that failed
    Key     string    // Key involved (if applicable)
    Err     error     // Underlying error
    Code    ErrorCode // Error classification
}
```

**Error Categories:**
- `ErrKeyNotFound` - Key doesn't exist
- `ErrStorageFull` - Storage capacity exceeded
- `ErrInvalidCommand` - Malformed client request
- `ErrClusterDown` - Cluster connectivity issues

## Testing Strategy

### Unit Tests
- Individual module testing
- Mock interfaces for dependencies
- Coverage > 80% for critical paths

### Integration Tests
- End-to-end scenarios
- Multi-node cluster testing
- Persistence and recovery testing

### Benchmark Tests
- Performance regression detection
- Memory usage profiling
- Throughput and latency measurement

## Concurrency Model

HyperCache uses several concurrency patterns:

### 1. Goroutine Pools
- Connection handling
- Background persistence
- Cluster communication

### 2. Channel Communication
- Command queuing
- Event notifications
- Graceful shutdown

### 3. Sync Primitives
- RWMutex for cache operations
- WaitGroups for coordination
- Atomic operations for counters

### 4. Context Usage
- Request timeouts
- Cancellation propagation
- Deadline management

## Memory Management

### Memory Layout
```
┌─────────────────────────────────────────────┐
│              HyperCache Memory              │
├─────────────────┬───────────────────────────┤
│   Cache Data    │      Metadata            │
│   (80%)         │      (20%)               │
├─────────────────┼───────────────────────────┤
│ Key-Value Store │ • Expiration timers      │
│ Hash tables     │ • LRU lists             │
│ String storage  │ • Cluster membership    │
│                 │ • Connection pools       │
└─────────────────┴───────────────────────────┘
```

### Memory Optimization
- Object pooling for frequent allocations
- String interning for repeated keys
- Efficient serialization formats
- Background garbage collection tuning

## Extension Points

HyperCache is designed for extensibility:

### 1. Custom Storage Engines
```go
type StorageEngine interface {
    Get(key string) ([]byte, bool)
    Set(key string, value []byte) error
    Delete(key string) error
    // ... other methods
}
```

### 2. Custom Eviction Policies
```go
type EvictionPolicy interface {
    OnAccess(key string)
    GetEvictionCandidate() (string, bool)
}
```

### 3. Custom Persistence Backends
```go
type PersistenceBackend interface {
    Write(operation Operation) error
    Recover() ([]Operation, error)
}
```

## Performance Characteristics

### Complexity Analysis
- **GET Operation**: O(1) average, O(log n) worst case
- **SET Operation**: O(1) average, O(log n) worst case  
- **DELETE Operation**: O(1) average, O(log n) worst case
- **Eviction**: O(1) for LRU, O(log n) for advanced policies

### Benchmarks
- **Single Node**: 100K+ ops/sec
- **Cluster**: 300K+ ops/sec (3 nodes)
- **Memory Usage**: ~50 bytes overhead per key
- **Persistence**: 10K+ writes/sec to SSD

## Security Considerations

### Authentication
- Optional password authentication
- Connection limits per client
- Rate limiting support

### Network Security
- TLS/SSL support for client connections
- Cluster communication encryption
- IP allowlist/denylist

### Data Protection
- Memory encryption at rest (planned)
- Secure deletion of sensitive data
- Audit logging for security events
