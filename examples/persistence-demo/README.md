# HyperCache Persistence Demo

This demo showcases HyperCache's persistence capabilities including:

- **AOF (Append-Only File)**: Write-ahead logging for durability
- **Snapshots**: Point-in-time backups
- **Hybrid Engine**: Combines both persistence strategies

## Running the Demo

```bash
go run main.go
```

## Features Demonstrated

1. **Configuration**: Setting up persistence parameters
2. **Storage Operations**: Basic key-value operations with persistence
3. **File Management**: Automatic persistence file creation and management
4. **Recovery**: How data persists across restarts

## Files Created

The demo creates temporary files in your system's temp directory to demonstrate:
- AOF log files (`.aof`)
- Snapshot files (`.snapshot`)
- Configuration and metadata
