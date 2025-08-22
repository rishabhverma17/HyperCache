# Distributed Persistence Architecture for HyperCache

## 🔍 **Current State vs Distributed Challenges**

### **Current Single-Node Persistence**
- **Local AOF**: Each node logs to `./data/hypercache.aof`
- **Local Snapshots**: Point-in-time snapshots stored locally  
- **Simple Recovery**: Replay local AOF + load local snapshot
- **No Coordination**: Each node operates independently

### **🚨 Distributed Environment Challenges**

#### **1. Data Consistency Issues**
```
Node A: SET user:123 → {name: "Alice"}     [AOF: operation logged locally]
Node B: SET user:123 → {name: "Bob"}       [AOF: operation logged locally]

❌ PROBLEM: Two different values for same key in separate AOF files!
❌ RECOVERY: Which value is correct? Node A or Node B?
```

#### **2. Partial Failure Scenarios**
```
Cluster: [Node-A, Node-B, Node-C]
Client: SET session:abc → value (should replicate to 2 nodes)

✅ Node-A: Writes AOF + primary storage
✅ Node-B: Writes AOF + replica storage  
❌ Node-C: NETWORK FAILURE - no replication

RESULT: Inconsistent persistence state across nodes
```

#### **3. Split-Brain During Recovery**
```
Network Partition:
  Group 1: [Node-A, Node-B] 
  Group 2: [Node-C]

Each group continues accepting writes and logging to AOF
When partition heals: CONFLICTING AOF FILES!
```

#### **4. Key Distribution Problems**
```
Hash Ring Changes:
  Before: key "user:123" → Node-A
  After: key "user:123" → Node-B

❌ PROBLEM: Node-A has AOF entries for "user:123"
❌ BUT: Node-B should own this key now!
❌ RECOVERY: Data in wrong place!
```

---

## 🛠️ **Distributed Persistence Solutions**

### **Phase 1: Distributed AOF with Replication**

#### **1. Replicated Write Logging**
```go
// Enhanced AOF with replication awareness
type DistributedAOFEntry struct {
    // Standard fields
    Timestamp time.Time
    Operation string
    Key       string
    Value     []byte
    
    // NEW: Distributed fields
    NodeID      string   // Which node initiated
    ReplicaNodes []string // Which nodes should have this
    VectorClock  map[string]int64 // For conflict resolution
    Term         int64    // Raft term (for consistency)
}

// Enhanced persistence engine
type DistributedPersistenceEngine struct {
    localEngine   *HybridEngine
    replication   ReplicationManager
    coordinator   cluster.CoordinatorService
    
    // Distributed state
    currentTerm   int64
    isLeader      bool
    followers     []string
}
```

#### **2. Replication-Aware Logging**
```go
func (dpe *DistributedPersistenceEngine) WriteEntry(entry *LogEntry) error {
    // 1. Determine replication targets
    replicaNodes := dpe.coordinator.GetRouting().GetReplicas(entry.Key, 2)
    
    // 2. Create distributed entry
    distEntry := &DistributedAOFEntry{
        LogEntry:     *entry,
        NodeID:       dpe.coordinator.GetLocalNodeID(),
        ReplicaNodes: replicaNodes,
        VectorClock:  dpe.generateVectorClock(),
        Term:         dpe.currentTerm,
    }
    
    // 3. Write locally first
    if err := dpe.localEngine.WriteEntry(entry); err != nil {
        return err
    }
    
    // 4. Replicate to other nodes
    return dpe.replicateToNodes(distEntry, replicaNodes)
}
```

### **Phase 2: Consensus-Based Snapshots**

#### **1. Coordinated Snapshot Creation**
```go
type DistributedSnapshotManager struct {
    localSnapshots *SnapshotManager
    consensus      ConsensusEngine
    
    // Snapshot coordination
    leaderNode     string
    snapshotEpoch  int64
    participants   []string
}

func (dsm *DistributedSnapshotManager) CreateClusterSnapshot() error {
    // 1. Leader initiates cluster-wide snapshot
    if !dsm.isLeader() {
        return fmt.Errorf("only leader can initiate cluster snapshots")
    }
    
    // 2. Send snapshot proposal to all nodes
    proposal := &SnapshotProposal{
        Epoch:        dsm.snapshotEpoch + 1,
        InitiatedBy:  dsm.leaderNode,
        Participants: dsm.participants,
        Timestamp:    time.Now(),
    }
    
    // 3. Wait for consensus from majority
    if !dsm.consensus.ProposeSnapshot(proposal) {
        return fmt.Errorf("snapshot consensus failed")
    }
    
    // 4. Create coordinated snapshots
    return dsm.executeCoordinatedSnapshot(proposal)
}
```

#### **2. Key-Consistent Snapshots**
```go
// Snapshot that includes routing information
type DistributedSnapshot struct {
    // Standard snapshot data
    Data      map[string]interface{}
    Header    SnapshotHeader
    
    // NEW: Distributed metadata  
    ClusterState struct {
        Nodes         []cluster.ClusterMember
        HashRing      cluster.HashRingState
        ReplicationMap map[string][]string // key → replica nodes
        Epoch         int64
    }
    
    // Consistency info
    VectorClocks  map[string]map[string]int64 // node → key → version
    LastLogIndex  int64 // Last applied AOF entry
}
```

### **Phase 3: Distributed Recovery**

#### **1. Consensus-Based Recovery**
```go
func (dpe *DistributedPersistenceEngine) DistributedRecovery() error {
    // 1. Load local snapshot and AOF
    localData, err := dpe.localEngine.LoadSnapshot()
    if err != nil {
        return fmt.Errorf("local recovery failed: %w", err)
    }
    
    localEntries, err := dpe.localEngine.ReadEntries()
    if err != nil {
        return fmt.Errorf("AOF recovery failed: %w", err)
    }
    
    // 2. Communicate with other nodes to resolve conflicts
    clusterState, err := dpe.gatherClusterState()
    if err != nil {
        return fmt.Errorf("cluster state gathering failed: %w", err)
    }
    
    // 3. Use vector clocks to resolve conflicts
    resolvedData := dpe.resolveConflicts(localData, localEntries, clusterState)
    
    // 4. Apply resolved state
    return dpe.applyResolvedState(resolvedData)
}

func (dpe *DistributedPersistenceEngine) resolveConflicts(
    localData map[string]interface{},
    localEntries []*LogEntry,
    clusterState ClusterRecoveryState) map[string]interface{} {
    
    resolved := make(map[string]interface{})
    
    // For each key, determine the authoritative value
    for key, localValue := range localData {
        // Check if this node should own this key
        ownerNode := dpe.coordinator.GetRouting().RouteKey(key)
        
        if ownerNode == dpe.coordinator.GetLocalNodeID() {
            // This node owns the key - use local value but check replicas
            resolved[key] = dpe.resolveWithReplicas(key, localValue, clusterState)
        } else {
            // This key belongs to another node - remove it
            log.Printf("Key %s belongs to %s, removing from local storage", key, ownerNode)
        }
    }
    
    return resolved
}
```

### **Phase 4: Network Partition Handling**

#### **1. Partition-Tolerant AOF**
```go
type PartitionManager struct {
    persistenceEngine *DistributedPersistenceEngine
    partitionDetector PartitionDetector
    
    // Partition state
    inPartition      bool
    partitionStarted time.Time
    partitionPeers   []string
}

func (pm *PartitionManager) HandlePartition(partition PartitionEvent) error {
    switch partition.Type {
    case PartitionDetected:
        return pm.enterPartitionMode(partition)
    case PartitionHealed:
        return pm.exitPartitionMode(partition)
    }
    return nil
}

func (pm *PartitionManager) enterPartitionMode(partition PartitionEvent) error {
    pm.inPartition = true
    pm.partitionStarted = time.Now()
    pm.partitionPeers = partition.ConnectedNodes
    
    // Switch to partition-safe logging
    config := pm.persistenceEngine.config
    config.Strategy = "partition-aof"  // Special partition mode
    config.SyncPolicy = "always"      // Maximum durability
    
    log.Printf("Entering partition mode with nodes: %v", pm.partitionPeers)
    return pm.persistenceEngine.ReconfigureForPartition(config)
}
```

#### **2. Merge Conflict Resolution**
```go
type ConflictResolver struct {
    strategy ConflictResolutionStrategy
}

type ConflictResolutionStrategy int

const (
    LastWriteWins ConflictResolutionStrategy = iota
    VectorClockMerge
    ApplicationDefined
)

func (cr *ConflictResolver) ResolveConflict(
    key string, 
    values []ConflictValue) (interface{}, error) {
    
    switch cr.strategy {
    case LastWriteWins:
        return cr.lastWriteWins(values)
    case VectorClockMerge:
        return cr.vectorClockMerge(values)
    case ApplicationDefined:
        return cr.applicationDefinedMerge(key, values)
    }
    
    return nil, fmt.Errorf("unknown conflict resolution strategy")
}

func (cr *ConflictResolver) vectorClockMerge(values []ConflictValue) (interface{}, error) {
    // Use vector clocks to determine causal ordering
    var latest ConflictValue
    var latestClock VectorClock
    
    for _, value := range values {
        if latestClock.IsConcurrentWith(value.VectorClock) {
            // Concurrent updates - need merge strategy
            return cr.mergeConcurrentValues(latest.Value, value.Value)
        } else if value.VectorClock.IsAfter(latestClock) {
            latest = value
            latestClock = value.VectorClock
        }
    }
    
    return latest.Value, nil
}
```

---

## 📊 **Implementation Roadmap**

### **Week 1: Replication-Aware AOF**
- [ ] Extend LogEntry with distributed fields (NodeID, ReplicaNodes, VectorClock)
- [ ] Implement DistributedPersistenceEngine wrapper
- [ ] Add replication target calculation
- [ ] Basic replication to secondary nodes

### **Week 2: Consensus Integration** 
- [ ] Integrate with Raft consensus (or similar)
- [ ] Leader election for persistence coordination
- [ ] Term-based operation ordering
- [ ] Conflict detection and resolution

### **Week 3: Distributed Snapshots**
- [ ] Cluster-wide snapshot coordination
- [ ] Consistent hash ring state capture
- [ ] Cross-node snapshot synchronization
- [ ] Recovery with cluster awareness

### **Week 4: Partition Tolerance**
- [ ] Partition detection integration
- [ ] Split-brain prevention
- [ ] Merge conflict resolution
- [ ] Testing with network failures

---

## 🎯 **Key Design Decisions**

### **Consistency Model**: **Eventually Consistent with Strong Consensus**
- **Writes**: Use Raft for strongly consistent replication
- **Reads**: Local reads for performance, optional strong reads
- **Conflicts**: Vector clock + application-defined resolution

### **Replication Strategy**: **Configurable N-Factor**
```yaml
persistence:
  replication_factor: 3      # Each key replicated to 3 nodes
  consistency_level: "majority"  # Require majority ACK for writes
  read_preference: "local"   # Read from local node when possible
```

### **Partition Handling**: **Availability over Consistency**
- Accept writes during partitions
- Use vector clocks to track causality
- Merge conflicts when partition heals
- Configurable per application needs

---

## ⚡ **Performance Implications**

### **Write Latency**: **2-3x increase** (due to replication)
```
Single Node:  ~1ms per write
Multi-Node:   ~3ms per write (2 replicas + network)
```

### **Storage Requirements**: **N-factor increase**
```
Single Node:  100MB AOF
3-Node Rep:   300MB total (100MB per replica node)
```

### **Recovery Time**: **Coordination overhead**
```
Single Node:  Fast local recovery
Multi-Node:   Consensus + conflict resolution overhead
```

### **Optimization Strategies**:
- **Async Replication**: Fire-and-forget to replicas (faster writes, eventual consistency)
- **Batched Operations**: Group multiple writes into single consensus round
- **Local Snapshots**: Each node maintains local snapshots, coordinate periodically

This distributed persistence design ensures **data durability across node failures** while maintaining **Redis compatibility** and **performance characteristics** suitable for production distributed environments!
