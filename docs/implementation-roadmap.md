# Multi-Node Persistence Test Plan

## Phase 1: Foundation Testing (Week 1)

### Step 1A: Single-Node Persistence Validation
**Goal**: Ensure current persistence works reliably

**Tasks**:
1. ✅ Test persistence demo: `cd examples/persistence-demo && go run main.go`
2. ✅ Validate AOF creation and replay
3. ✅ Validate snapshot creation and loading
4. ✅ Test recovery after simulated crash
5. ✅ Benchmark persistence performance

**Expected Output**:
```
=== HyperCache Persistence Demo ===
✅ Created cache with persistence
✅ Added 5 test items
✅ Created manual snapshot  
✅ Simulated server restart
✅ Recovered all 5 items successfully
📊 AOF: 15KB, Snapshot: 8KB, Recovery: 45ms
```

### Step 1B: Multi-Node Foundation
**Goal**: Test clustering without distributed persistence

**Tasks**:
1. Create 3-node cluster demo
2. Test hash ring distribution
3. Test node failure detection
4. Validate key routing

**Implementation**:
```go
// examples/multi-node-demo/main.go
func main() {
    // Start 3 nodes with different ports
    nodes := []string{"node-1:6379", "node-2:6380", "node-3:6381"}
    
    for _, node := range nodes {
        go startHyperCacheNode(node)
    }
    
    // Test data distribution
    testKeyDistribution(nodes)
    
    // Test node failure
    testNodeFailure(nodes[0])
}
```

## Phase 2: Replication-Aware Persistence (Week 2-3)

### Step 2A: Enhanced Log Entries
**Goal**: Add distributed metadata to AOF entries

**Implementation**:
```go
// internal/persistence/distributed_aof.go
type DistributedLogEntry struct {
    // Standard fields
    LogEntry
    
    // Distributed metadata
    NodeID       string   `json:"node_id"`
    ReplicaNodes []string `json:"replica_nodes"`
    Term         int64    `json:"term"`
    Index        int64    `json:"index"`
}

func (dao *DistributedAOF) WriteDistributedEntry(entry *DistributedLogEntry) error {
    // 1. Write to local AOF
    if err := dao.localAOF.WriteEntry(&entry.LogEntry); err != nil {
        return err
    }
    
    // 2. Replicate to replica nodes
    return dao.replicateToNodes(entry)
}
```

### Step 2B: Basic Replication
**Goal**: Replicate writes to multiple nodes

**Tasks**:
1. Implement ReplicationManager interface
2. Add replication target calculation  
3. Implement async replication
4. Test 2-replica setup

## Phase 3: Consensus Integration (Week 4-5)

### Step 3A: Raft Integration
**Goal**: Add consensus for coordination

**Libraries to Consider**:
- `github.com/hashicorp/raft` (Most popular)
- `github.com/etcd-io/raft` (etcd's implementation)
- Custom implementation (more control)

**Implementation**:
```go
// internal/consensus/raft_engine.go
type RaftConsensusEngine struct {
    raft     *raft.Raft
    store    *raft.InmemStore
    snapshot *raft.FileSnapshotStore
    
    // Persistence integration
    persistence *DistributedPersistenceEngine
}

func (rce *RaftConsensusEngine) ProposeOperation(op *Operation) error {
    // Use Raft to get consensus on operation
    future := rce.raft.Apply(op.Serialize(), raftTimeout)
    return future.Error()
}
```

### Step 3B: Leader-Coordinated Snapshots
**Goal**: Cluster-wide consistent snapshots

**Implementation**:
```go
func (dpe *DistributedPersistenceEngine) CreateClusterSnapshot() error {
    // Only leader can initiate
    if !dpe.consensus.IsLeader() {
        return ErrNotLeader
    }
    
    // Propose snapshot to cluster
    proposal := &SnapshotProposal{
        Epoch: dpe.currentEpoch + 1,
        Timestamp: time.Now(),
    }
    
    return dpe.consensus.ProposeSnapshot(proposal)
}
```

## Phase 4: Conflict Resolution (Week 6)

### Step 4A: Vector Clocks
**Goal**: Track causality for conflict resolution

**Implementation**:
```go
// internal/consensus/vector_clock.go
type VectorClock map[string]int64

func (vc VectorClock) Increment(nodeID string) {
    vc[nodeID]++
}

func (vc VectorClock) IsAfter(other VectorClock) bool {
    // Implement vector clock comparison
}
```

### Step 4B: Conflict Resolution Strategies
**Goal**: Handle concurrent writes

**Strategies**:
1. Last-Write-Wins (simple)
2. Vector Clock Merge (causal)
3. Application-Defined (flexible)

## Phase 5: Testing & Validation (Week 7-8)

### Step 5A: Comprehensive Testing
**Tests Needed**:
1. **Basic Replication**: Write to primary, verify replicas
2. **Node Failure**: Kill node, verify failover
3. **Network Partition**: Split cluster, verify handling
4. **Conflict Resolution**: Concurrent writes, verify merge
5. **Performance**: Latency/throughput under load

### Step 5B: Demo Applications
**Demos to Create**:
1. **Multi-Node Persistence**: Show replication working
2. **Failure Recovery**: Show node failure handling
3. **Partition Tolerance**: Show split-brain resolution
4. **Performance Comparison**: Single vs distributed

---

## 🎯 **Immediate Next Steps (This Week)**

### **Priority 1: Test Current Persistence** ⭐
Let me help you test the current persistence demo:

```bash
# Method 1: Direct execution
cd /Users/rishabhverma/Documents/HobbyProjects/Cache/examples/persistence-demo
go build -o persistence-demo main.go
./persistence-demo

# Method 2: Check for build errors
go build main.go
echo "Build status: $?"

# Method 3: Check dependencies
go mod verify
go mod download
```

### **Priority 2: Create Multi-Node Foundation**
```bash
# Create basic 3-node demo
mkdir examples/multi-node-basic
# Implement simple clustering without persistence
# Test hash ring distribution
# Validate node discovery
```

### **Priority 3: Plan Distributed Persistence Architecture**
```bash
# Design session
# 1. Choose consensus library (Raft recommended)
# 2. Design replication strategy
# 3. Plan conflict resolution approach
# 4. Define consistency guarantees
```

---

## 🤔 **Key Decision Points**

### **Consensus Library Choice**
| Option | Pros | Cons | Recommendation |
|--------|------|------|----------------|
| **HashiCorp Raft** | Production-proven, good docs | Heavy dependency | ✅ **Recommended** |
| **etcd Raft** | Lightweight, fast | Less documentation | ⚠️ Consider |
| **Custom** | Full control | More development time | ❌ Not recommended |

### **Consistency Model**
| Model | Use Case | Trade-offs |
|-------|----------|------------|
| **Strong** | Financial, critical data | Higher latency, lower availability |
| **Eventual** | Social media, analytics | Lower latency, eventual consistency |
| **Causal** | Collaborative apps | Balance of both | ✅ **Recommended**

### **Replication Strategy**
```yaml
# Recommended starting configuration
replication:
  factor: 3                    # Tolerate 1 node failure
  consistency: "majority"      # 2/3 nodes must ACK
  async_replication: false     # Start with sync for safety
  conflict_resolution: "vector_clock"  # Preserve causality
```

---

## 📊 **Success Metrics**

### **Phase 1 Success**
- ✅ Persistence demo works flawlessly
- ✅ Multi-node cluster forms correctly
- ✅ Key distribution works properly

### **Phase 2 Success** 
- ✅ Writes replicate to 2+ nodes
- ✅ Node failure doesn't lose data
- ✅ Recovery rebuilds from replicas

### **Phase 3 Success**
- ✅ Leader election works
- ✅ Cluster-wide snapshots succeed
- ✅ Consensus handles failures gracefully

Let me help you test the persistence demo first, then we can proceed with the multi-node implementation! Would you like me to help debug the current demo or start implementing the multi-node foundation?
