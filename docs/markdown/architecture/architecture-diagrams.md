# HyperCache Architecture Diagrams

## 1. System Overview - Component Architecture

```mermaid
graph TB
    Client[Client Applications] --> LB[Load Balancer]
    
    subgraph "HyperCache Cluster"
        LB --> Node1[HyperCache Node 1]
        LB --> Node2[HyperCache Node 2] 
        LB --> Node3[HyperCache Node 3]
        
        Node1 <--> Node2
        Node2 <--> Node3
        Node3 <--> Node1
    end
    
    subgraph "Node Architecture"
        direction TB
        API[API Layer<br/>Custom Protocol<br/>Redis Compat Future] 
        
        subgraph "Cache Engine"
            Router[Request Router]
            CuckooFilter[Cuckoo Filter Engine]
            
            subgraph "Multi-Store Manager"
                StoreA[Sessions Store<br/>TTL Policy<br/>1GB Pool]
                StoreB[Hot Data Store<br/>LRU Policy<br/>4GB Pool]
                StoreC[Analytics Store<br/>LFU Policy<br/>2GB Pool]
            end
        end
        
        subgraph "Distribution Layer"
            Hash[Consistent Hash Ring]
            Replication[Replication Manager]
            Gossip[Gossip Protocol]
        end
        
        subgraph "Storage Layer"
            WAL[Write-Ahead Log]
            LSM[LSM Tree Storage]
            Compaction[Compaction Engine]
        end
        
        API --> Router
        Router --> CuckooFilter
        CuckooFilter --> StoreA
        CuckooFilter --> StoreB
        CuckooFilter --> StoreC
        
        Router --> Hash
        Hash --> Replication
        Replication --> Gossip
        
        StoreA --> WAL
        StoreB --> WAL  
        StoreC --> WAL
        WAL --> LSM
        LSM --> Compaction
    end
```

## 2. Request Flow - Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant API as API Layer
    participant Router as Request Router
    participant CF as Cuckoo Filter
    participant Store as Target Store
    participant Evict as Eviction Queue
    participant WAL as Write-Ahead Log
    participant Rep as Replication

    Note over Client, Rep: PUT Operation Flow
    
    Client->>API: PUT(store="sessions", key="user123", value="data", ttl=30m)
    API->>Router: Route request to store
    
    Router->>CF: Check if key might exist
    CF-->>Router: false (new key)
    
    Router->>Store: Get target store "sessions"
    Store->>Store: Check memory pressure
    
    alt Memory pressure > threshold
        Store->>Evict: Get next eviction candidate (O(1))
        Evict-->>Store: Return TTL-expired entry
        Store->>Store: Evict expired entry
        Store->>WAL: Log eviction
    end
    
    Store->>WAL: Log PUT operation
    WAL-->>Store: Confirm write
    
    Store->>Store: Insert entry into memory
    Store->>Evict: Update TTL queue (O(1))
    Store->>CF: Add key to filter
    
    par Async Replication
        Store->>Rep: Replicate to followers
        Rep->>Rep: Send to replica nodes
    end
    
    Store-->>Router: Success
    Router-->>API: Success
    API-->>Client: 200 OK
```

## 3. Multi-Store Memory Management

```mermaid
graph LR
    subgraph "Global Memory Manager"
        Total[Total Memory: 16GB]
        Allocator[Memory Allocator]
    end
    
    Total --> Allocator
    
    Allocator --> Pool1[Sessions Pool<br/>1GB<br/>Current: 800MB]
    Allocator --> Pool2[Hot Data Pool<br/>4GB<br/>Current: 3.8GB]
    Allocator --> Pool3[Analytics Pool<br/>2GB<br/>Current: 1.2GB]
    Allocator --> Pool4[Bulk Cache Pool<br/>8GB<br/>Current: 5.1GB]
    
    subgraph "Store 1: Sessions"
        Pool1 --> TTL_Queue[TTL Priority Queue]
        TTL_Queue --> TTL_Data[Session Data<br/>Auto-expire 30m]
    end
    
    subgraph "Store 2: Hot Data"
        Pool2 --> LRU_List[LRU Doubly Linked List]
        LRU_List --> LRU_Data[Frequently Accessed Data]
    end
    
    subgraph "Store 3: Analytics"  
        Pool3 --> LFU_Heap[LFU Min Heap]
        LFU_Heap --> LFU_Data[Report Cache Data]
    end
    
    subgraph "Store 4: Bulk Cache"
        Pool4 --> FIFO_Queue[FIFO Simple Queue]
        FIFO_Queue --> FIFO_Data[Large Dataset Cache]
    end
```

## 4. Eviction Policy Data Structures (O(1) Operations)

```mermaid
graph TB
    subgraph "LRU Store - O(1) Operations"
        LRU_Hash[HashMap<br/>Key → Node*]
        LRU_List[Doubly Linked List<br/>MRU ← → ← → LRU]
        
        LRU_Hash <--> LRU_List
        
        note1[GET: Move to head O(1)<br/>PUT: Add to head O(1)<br/>EVICT: Remove tail O(1)]
    end
    
    subgraph "LFU Store - O(1) Amortized"
        LFU_Hash[HashMap<br/>Key → FreqNode*]
        LFU_Freq[Frequency Buckets<br/>Min Heap by Frequency]
        
        LFU_Hash <--> LFU_Freq
        
        note2[GET: Increment freq O(1)<br/>PUT: Add with freq=1 O(1)<br/>EVICT: Remove min freq O(1)]
    end
    
    subgraph "TTL Store - O(1) for Expired"
        TTL_Hash[HashMap<br/>Key → Entry*]
        TTL_Pqueue[Priority Queue<br/>Sorted by Expiration]
        
        TTL_Hash <--> TTL_Pqueue
        
        note3[GET: Check expiration O(1)<br/>PUT: Insert with TTL O(log n)<br/>EVICT: Remove expired O(1)]
    end
```

## 5. Distribution & Replication Strategy

```mermaid
graph TB
    subgraph "Consistent Hash Ring"
        Ring((Hash Ring<br/>0 to 2^32))
        
        VN1[Virtual Node 1<br/>Node A]
        VN2[Virtual Node 2<br/>Node A]
        VN3[Virtual Node 3<br/>Node B]
        VN4[Virtual Node 4<br/>Node B]
        VN5[Virtual Node 5<br/>Node C]
        VN6[Virtual Node 6<br/>Node C]
        
        Ring --- VN1
        Ring --- VN2
        Ring --- VN3
        Ring --- VN4
        Ring --- VN5
        Ring --- VN6
    end
    
    subgraph "Replication Strategy"
        Primary[Primary Node<br/>Hash(key) → Node]
        Replica1[Replica 1<br/>Next node clockwise]
        Replica2[Replica 2<br/>Next+1 node clockwise]
        
        Primary --> Replica1
        Primary --> Replica2
        
        note4[Replication Factor: 3<br/>Consistency: Eventual<br/>Read: Any replica<br/>Write: Primary + async replicas]
    end
    
    subgraph "Gossip Protocol"
        NodeA[Node A<br/>Heartbeat every 1s]
        NodeB[Node B<br/>Failure detection]
        NodeC[Node C<br/>Metadata sync]
        
        NodeA <--> NodeB
        NodeB <--> NodeC  
        NodeC <--> NodeA
        
        note5[φ-accrual failure detector<br/>Node join/leave events<br/>Hash ring updates]
    end
```

## 6. Storage Engine Architecture

```mermaid
graph TB
    subgraph "Write Path"
        WriteReq[Write Request]
        WriteReq --> WAL[Write-Ahead Log<br/>Immediate durability]
        WAL --> MemTable[MemTable<br/>In-memory sorted]
        
        MemTable --> Flush{MemTable full?}
        Flush -->|Yes| SST0[SSTable L0<br/>Immutable file]
        Flush -->|No| MemTable
    end
    
    subgraph "Read Path"
        ReadReq[Read Request]
        ReadReq --> MemCheck[Check MemTable]
        MemCheck --> L0Check[Check L0 SSTables]
        L0Check --> L1Check[Check L1 SSTables]
        L1Check --> BloomCheck[Bloom Filter Check]
        BloomCheck --> DiskRead[Disk Read if needed]
    end
    
    subgraph "Compaction Engine"
        Compact[Background Compaction]
        SST0 --> Compact
        Compact --> SST1[SSTable L1<br/>Merged & sorted]
        SST1 --> Compact2[L1→L2 Compaction]
        Compact2 --> SST2[SSTable L2<br/>Long-term storage]
        
        BloomBuild[Build Bloom Filters<br/>Per SSTable Level]
        Compact --> BloomBuild
        Compact2 --> BloomBuild
    end
```

## Implementation Notes:
- ✅ Multi-store with per-store eviction policies
- ✅ O(1) operations with optimized data structures
- ✅ Basic networking with custom protocol
- ✅ WAL + simple LSM storage
- ✅ Cuckoo filter integration
