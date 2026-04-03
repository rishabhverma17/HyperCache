# HyperCache Implementation Guide
## Understanding Classes, Configs, and Architecture

### 🏗️ Core Components Breakdown

## 1. **Entry Struct - The Data Container**
```go
type Entry struct {
    Key       []byte        // The cache key (binary data)
    Value     []byte        // The cached value (binary data) 
    TTL       time.Duration // Time to live (0 = no expiration)
    Version   uint64        // For conflict resolution in distributed setup
    Timestamp int64         // Unix timestamp when created
    StoreID   string        // Which store this entry belongs to
}
```
**Purpose**: Represents a single cached item with all its metadata.
**Example**: User session, product data, API response, etc.

## 2. **Cache Interface - The Main API**
```go
type Cache interface {
    // Basic CRUD operations
    Get(ctx, store, key) → value, error
    Put(ctx, store, key, value, ttl) → error
    Delete(ctx, store, key) → error
    Exists(ctx, store, key) → bool
    
    // Batch operations (more efficient)
    BatchGet(ctx, store, []keys) → map[key]value, error
    
    // Probabilistic filtering
    MightContain(ctx, store, key) → bool  // Cuckoo filter check
    
    // Store management
    CreateStore(name, config) → error     // Create new store
    StoreStats(name) → *StoreStats, error // Get metrics
}
```

**Usage Example**:
```go
cache := NewHyperCache(config)

// Put user session in sessions store (TTL policy)
cache.Put(ctx, "sessions", []byte("user:123"), sessionData, 30*time.Minute)

// Put product data in hot_data store (LRU policy)  
cache.Put(ctx, "hot_data", []byte("product:456"), productData, 0)

// Get from specific store
value, err := cache.Get(ctx, "sessions", []byte("user:123"))
```

## 3. **Store Interface - Individual Storage Unit**
```go
type Store interface {
    Get(key) → value, error       // O(1) retrieval
    Put(key, value, ttl) → error  // O(1) insertion
    Delete(key) → error           // O(1) removal
    
    NeedsEviction() → bool        // Check memory pressure
    Evict() → *Entry, error       // Remove one item (O(1))
    
    Stats() → *StoreStats         // Performance metrics
}
```

**Real-world analogy**: Think of stores as different warehouses:
- **Sessions Store**: Quick pickup/dropoff (TTL-based)
- **Hot Data Store**: Most popular items at front (LRU)
- **Analytics Store**: Organized by frequency (LFU)

## 4. **EvictionPolicy Interface - The Brain**
```go
type EvictionPolicy interface {
    // When memory is full, what to remove?
    NextEvictionCandidate() → *Entry     // MUST be O(1)
    
    // Update tracking when items are used
    OnAccess(entry)                      // Item was read
    OnInsert(entry)                      // Item was added
    OnDelete(entry)                      // Item was removed
    
    PolicyName() → string                // "lru", "lfu", etc.
}
```

**Policy Implementations**:

### **LRU (Least Recently Used) Policy**
```go
type LRUPolicy struct {
    accessOrder *list.List              // Doubly linked list
    entryMap    map[string]*list.Element // For O(1) lookup
}

func (lru *LRUPolicy) OnAccess(entry *Entry) {
    // Move accessed item to front = O(1)
    element := lru.entryMap[string(entry.Key)]
    lru.accessOrder.MoveToFront(element)
}

func (lru *LRUPolicy) NextEvictionCandidate() *Entry {
    // Return item at back of list = O(1)
    return lru.accessOrder.Back().Value.(*Entry)
}
```

### **LFU (Least Frequently Used) Policy**
```go
type LFUPolicy struct {
    frequencies map[string]int64      // How often each key is accessed
    freqBuckets map[int64]*list.List  // Group keys by frequency
    minFreq     int64                 // Current minimum frequency
}

func (lfu *LFUPolicy) OnAccess(entry *Entry) {
    // Increment frequency and update buckets = O(1)
    key := string(entry.Key)
    oldFreq := lfu.frequencies[key]
    newFreq := oldFreq + 1
    lfu.frequencies[key] = newFreq
    
    // Move to higher frequency bucket
    lfu.moveToFrequencyBucket(key, oldFreq, newFreq)
}
```

### **TTL (Time To Live) Policy**  
```go
type TTLPolicy struct {
    expirationQueue *heap.Heap  // Min heap sorted by expiration time
}

func (ttl *TTLPolicy) NextEvictionCandidate() *Entry {
    // Return most expired item = O(1)
    if ttl.expirationQueue.Len() == 0 {
        return nil
    }
    
    candidate := ttl.expirationQueue.Peek().(*Entry)
    if time.Now().Unix() > candidate.Timestamp + int64(candidate.TTL.Seconds()) {
        return candidate // This item has expired
    }
    return nil // Nothing to evict yet
}
```

## 5. **MemoryPool Interface - Resource Manager**
```go
type MemoryPool interface {
    CurrentUsage() int64      // How much memory used
    MaxSize() int64           // Maximum allowed memory
    MemoryPressure() float64  // 0.0 (empty) to 1.0 (full)
}
```

**Example Implementation**:
```go
type SimpleMemoryPool struct {
    maxSize     int64
    currentSize int64
    mutex       sync.RWMutex
}

func (pool *SimpleMemoryPool) MemoryPressure() float64 {
    pool.mutex.RLock()
    defer pool.mutex.RUnlock()
    
    return float64(pool.currentSize) / float64(pool.maxSize)
}
```

## 6. **Configuration System**

### **Main Config Structure**
```yaml
# configs/hypercache.yaml
node:
  id: "hypercache-node-1"
  bind_addr: "127.0.0.1:7000"
  data_dir: "/tmp/hypercache"

stores:
  - name: "sessions"           # User sessions
    eviction_policy: "ttl"     # Expire automatically  
    max_memory: "1GB"
    default_ttl: "30m"
    
  - name: "hot_data"          # Frequently accessed data
    eviction_policy: "lru"     # Keep recent items
    max_memory: "4GB"
    default_ttl: "2h"
    
  - name: "analytics"         # Reporting data
    eviction_policy: "lfu"     # Keep frequent items
    max_memory: "2GB" 
    default_ttl: "24h"
```

### **Config Loading Process**
```go
func Load(path string) (*Config, error) {
    // 1. Set sensible defaults
    config := &Config{
        Node: NodeConfig{
            ID: "hypercache-node-1",
            BindAddr: "127.0.0.1:7000",
        },
        // ... other defaults
    }
    
    // 2. Try to read YAML file
    data, err := os.ReadFile(path)
    if err != nil && !os.IsNotExist(err) {
        return nil, err
    }
    
    // 3. Parse YAML and override defaults
    if len(data) > 0 {
        yaml.Unmarshal(data, config)
    }
    
    // 4. Validate configuration
    return config, config.Validate()
}
```

## 7. **How It All Works Together**

### **Initialization Flow**
```go
// 1. Load configuration
config, err := config.Load("configs/hypercache.yaml")

// 2. Create cache instance
cache, err := cache.NewInstance(config)

// 3. For each store in config, create:
for _, storeConfig := range config.Stores {
    // a. Memory pool for the store
    memPool := NewMemoryPool(storeConfig.MaxMemory)
    
    // b. Eviction policy based on config
    var policy EvictionPolicy
    switch storeConfig.EvictionPolicy {
    case "lru":
        policy = NewLRUPolicy()
    case "lfu": 
        policy = NewLFUPolicy()
    case "ttl":
        policy = NewTTLPolicy()
    }
    
    // c. Create the store
    store := NewStore(storeConfig.Name, policy, memPool)
    cache.RegisterStore(store)
}
```

### **Request Processing Flow**
```go
// Client request: PUT sessions user:123 sessionData 30m
func (cache *Cache) Put(ctx, storeName, key, value, ttl) error {
    // 1. Find the target store
    store := cache.stores[storeName]  // O(1) lookup
    
    // 2. Check if store needs eviction
    if store.NeedsEviction() {       // O(1) memory check
        victim := store.policy.NextEvictionCandidate()  // O(1)
        store.Delete(victim.Key)     // O(1) removal
    }
    
    // 3. Create entry and insert
    entry := &Entry{
        Key: key,
        Value: value, 
        TTL: ttl,
        Timestamp: time.Now().Unix(),
        StoreID: storeName,
    }
    
    // 4. Store the entry
    store.Put(key, value, ttl)       // O(1) insertion
    
    // 5. Update policy tracking
    store.policy.OnInsert(entry)     // O(1) policy update
    
    return nil
}
```

## 8. **Key Innovations**

### **Per-Store Eviction Policies**
- **Problem**: Redis has one policy for all data
- **Solution**: Each store optimized for its data pattern
- **Benefit**: Better hit rates, lower memory usage

### **O(1) Performance Guarantee**
- **Challenge**: Multiple stores could mean O(n) operations
- **Solution**: Pre-computed eviction candidates per store
- **Result**: Cache speed maintained regardless of store count

### **Memory Isolation**
- **Design**: Each store has dedicated memory pool
- **Benefit**: One store can't starve others
- **Control**: Fine-grained memory management

This architecture gives you the flexibility of multiple caches with the efficiency of a single system!
