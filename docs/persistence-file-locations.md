# HyperCache Persistence File Locations

## 📍 WAL and AOF File Paths

### **Primary Locations**
Based on cluster startup logs and actual file discovery:

```
Node 1: /tmp/hypercache/node-1/node-1/hypercache.aof
Node 2: /tmp/hypercache/node-2/node-2/hypercache.aof  
Node 3: /tmp/hypercache/node-3/node-3/hypercache.aof
```

### **macOS Actual Locations**
On macOS, `/tmp` is often mapped to `/private/tmp`:

```
Node 1: /private/tmp/hypercache/node-1/node-1/hypercache.aof
Node 2: /private/tmp/hypercache/node-2/node-2/hypercache.aof
Node 3: /private/tmp/hypercache/node-3/node-3/hypercache.aof
```

## 📁 Directory Structure

### **Configuration vs Reality**
- **Config Setting**: `data_dir: "/tmp/hypercache/node-X"` (in `configs/nodeX-config.yaml`)
- **Actual Structure**: Creates nested `node-X/node-X/` subdirectories
- **Full Path**: `/tmp/hypercache/node-X/node-X/hypercache.aof`

### **Directory Tree**
```
/tmp/hypercache/
├── node-1/
│   └── node-1/
│       ├── hypercache.aof
│       └── hypercache.aof.tmp (during compaction)
├── node-2/
│   └── node-2/
│       ├── hypercache.aof
│       └── hypercache.aof.tmp (during compaction)
└── node-3/
    └── node-3/
        ├── hypercache.aof
        └── hypercache.aof.tmp (during compaction)
```

## 📋 File Types

### **AOF Files**
- **Primary**: `hypercache.aof` - Main append-only file
- **Temporary**: `hypercache.aof.tmp` - Created during compaction process

### **Other Persistence Files**
- `*.snapshot` - Point-in-time snapshots (if using hybrid persistence)
- `*.log` - Additional log files
- `*.wal` - Write-ahead log files (if WAL is implemented)

## 🔍 How to Access Files

### **Terminal Commands**
```bash
# List all persistence files
ls -la /tmp/hypercache/*/node-*/*.aof*

# On macOS, try:
ls -la /private/tmp/hypercache/*/node-*/*.aof*

# Check file sizes
du -sh /tmp/hypercache/*/node-*/*.aof*
```

### **Finder (macOS)**
1. Press `Cmd + Shift + G` (Go to folder)
2. Enter `/tmp/hypercache` or `/private/tmp/hypercache`
3. Press `Cmd + Shift + .` to show hidden files
4. Navigate to `node-X/node-X/` subdirectories

## 📊 Cluster Recovery Information

From latest startup:
- **Node 1**: 1649 AOF entries → 1049 cache entries recovered
- **Node 2**: 1673 AOF entries → 1052 cache entries recovered  
- **Node 3**: 1695 AOF entries → 1059 cache entries recovered

## 🧹 Cleanup Script

Use the enhanced `clean-persistence.sh` script:
```bash
# Clean all nodes
./scripts/clean-persistence.sh --all

# Clean specific node
./scripts/clean-persistence.sh --node node-1

# Show where files would be stored
./scripts/clean-persistence.sh --show

# Find existing files
./scripts/clean-persistence.sh --find
```

## 🔧 Configuration Sources

- **Node 1**: `configs/node1-config.yaml`
- **Node 2**: `configs/node2-config.yaml`
- **Node 3**: `configs/node3-config.yaml`

Each config contains:
```yaml
node:
  data_dir: "/tmp/hypercache/node-X"
```

## ⚠️ Important Notes

1. **Double Directory Nesting**: Files are in `node-X/node-X/` not just `node-X/`
2. **Hidden Files**: Use appropriate commands/settings to view hidden files
3. **macOS /tmp Mapping**: `/tmp` → `/private/tmp`
4. **File Permissions**: Files may have restricted permissions
5. **Process Dependencies**: Files are created only when HyperCache processes are running

---
*Last Updated: August 22, 2025*
*Cluster Version: HyperCache 2.0.0*
