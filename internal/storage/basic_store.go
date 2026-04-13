package storage

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"hypercache/internal/cache"
	"hypercache/internal/filter"
	"hypercache/internal/logging"
	"hypercache/internal/persistence"
)

// CacheItem represents a single item in the cache with true memory integration
type CacheItem struct {
	Key              string
	ValuePtr         []byte // Points to actual allocated memory containing serialized value
	ValueType        string // Type information for deserialization
	Size             uint64 // Size of allocated memory
	CreatedAt        time.Time
	ExpiresAt        time.Time
	SessionID        string
	AccessCount      uint64
	LastAccessed     time.Time
	LamportTimestamp uint64 // Logical clock value when this item was last written
}

// GetValue deserializes and returns the actual value from allocated memory
func (item *CacheItem) GetValue() (interface{}, error) {
	return deserializeValue(item.ValuePtr, item.ValueType)
}

// GetRawBytes returns the raw stored bytes without deserialization.
// For string and []byte values, this avoids the string(data) allocation
// that GetValue() performs. Returns nil if the item has no data.
func (item *CacheItem) GetRawBytes() []byte {
	return item.ValuePtr
}

// IsStringType returns true if the stored value is a string or []byte,
// meaning GetRawBytes() can be used directly without deserialization.
func (item *CacheItem) IsStringType() bool {
	return item.ValueType == "string" || item.ValueType == "[]uint8"
}

// IsExpired checks if the item has expired
func (item *CacheItem) IsExpired() bool {
	return !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt)
}

// BasicStoreConfig holds configuration for BasicStore
type BasicStoreConfig struct {
	Name              string
	MaxMemory         uint64
	DefaultTTL        time.Duration
	EnableStatistics  bool
	CleanupInterval   time.Duration
	FilterConfig      *filter.FilterConfig           // Optional filter configuration (nil = no filter)
	PersistenceConfig *persistence.PersistenceConfig // Optional persistence configuration (nil = no persistence)
}

// BasicStoreStats holds statistics for the BasicStore
type BasicStoreStats struct {
	TotalItems    uint64
	TotalMemory   uint64
	HitCount      uint64
	MissCount     uint64
	EvictionCount uint64
	ErrorCount    uint64
	CreatedAt     time.Time
	LastAccess    time.Time
}

// HitRate calculates the cache hit rate
func (s *BasicStoreStats) HitRate() float64 {
	total := s.HitCount + s.MissCount
	if total == 0 {
		return 0.0
	}
	return float64(s.HitCount) / float64(total) * 100.0
}

// BasicStore implements the Store interface with integrated MemoryPool, EvictionPolicy, and optional Filter
type BasicStore struct {
	config        BasicStoreConfig
	data          *ShardedMap // Sharded concurrent map (replaces items + allocatedPtrs + tombstones)
	memPool       *MemoryPool
	evictPolicy   cache.EvictionPolicy
	filter        filter.ProbabilisticFilter    // Optional Cuckoo/Bloom filter for negative lookups
	persistEngine persistence.PersistenceEngine // Optional persistence layer
	mutex         sync.RWMutex                  // Protects stats only (not data — that's sharded)
	stats         BasicStoreStats
	stopCleanup   chan bool

	// Background eviction
	evictSignal chan struct{} // Signal background evictor to run
	evictDone   chan struct{} // Closed when background evictor exits

	// Background AOF
	aofChan chan *persistence.LogEntry // Buffered channel for async AOF writes
	aofDone chan struct{}              // Closed when AOF goroutine exits
}

// serializeValue converts interface{} values to []byte for storage in allocated memory
func serializeValue(value interface{}) ([]byte, string, error) {
	valueType := reflect.TypeOf(value).String()

	switch v := value.(type) {
	case string:
		return []byte(v), valueType, nil
	case []byte:
		// Make a copy to avoid aliasing issues
		data := make([]byte, len(v))
		copy(data, v)
		return data, valueType, nil
	case int:
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(v))
		return data, valueType, nil
	case int32:
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(v))
		return data, valueType, nil
	case int64:
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(v))
		return data, valueType, nil
	case uint32:
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, v)
		return data, valueType, nil
	case uint64:
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, v)
		return data, valueType, nil
	case float32:
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, *(*uint32)(unsafe.Pointer(&v)))
		return data, valueType, nil
	case float64:
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, *(*uint64)(unsafe.Pointer(&v)))
		return data, valueType, nil
	case bool:
		if v {
			return []byte{1}, valueType, nil
		}
		return []byte{0}, valueType, nil
	default:
		// Use JSON encoding for complex types (maps, slices, structs)
		// JSON roundtrips cleanly with interface{} unlike gob
		data, err := json.Marshal(v)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode value: %w", err)
		}
		return data, valueType, nil
	}
}

// deserializeValue converts []byte back to the original value type
func deserializeValue(data []byte, valueType string) (interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data for deserialization")
	}

	switch valueType {
	case "string":
		return string(data), nil
	case "[]uint8": // []byte shows up as []uint8 in reflection
		// Return a copy to avoid mutation issues
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	case "int":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data for int deserialization")
		}
		return int(binary.LittleEndian.Uint64(data)), nil
	case "int32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data for int32 deserialization")
		}
		return int32(binary.LittleEndian.Uint32(data)), nil
	case "int64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data for int64 deserialization")
		}
		return int64(binary.LittleEndian.Uint64(data)), nil
	case "uint32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data for uint32 deserialization")
		}
		return binary.LittleEndian.Uint32(data), nil
	case "uint64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data for uint64 deserialization")
		}
		return binary.LittleEndian.Uint64(data), nil
	case "float32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data for float32 deserialization")
		}
		bits := binary.LittleEndian.Uint32(data)
		return *(*float32)(unsafe.Pointer(&bits)), nil
	case "float64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data for float64 deserialization")
		}
		bits := binary.LittleEndian.Uint64(data)
		return *(*float64)(unsafe.Pointer(&bits)), nil
	case "bool":
		if len(data) < 1 {
			return nil, fmt.Errorf("insufficient data for bool deserialization")
		}
		return data[0] != 0, nil
	default:
		// Use JSON decoding for complex types
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		return result, nil
	}
}

// NewBasicStore creates a new BasicStore with MemoryPool and EvictionPolicy integration
func NewBasicStore(config BasicStoreConfig) (*BasicStore, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("store name cannot be empty")
	}
	if config.MaxMemory == 0 {
		return nil, fmt.Errorf("max memory must be greater than 0")
	}

	// Create MemoryPool
	memPool := NewMemoryPool(config.Name, int64(config.MaxMemory))

	store := &BasicStore{
		config:      config,
		data:        NewShardedMap(),
		memPool:     memPool,
		stopCleanup: make(chan bool),
		evictSignal: make(chan struct{}, 1),
		evictDone:   make(chan struct{}),
		aofChan:     make(chan *persistence.LogEntry, 10000),
		aofDone:     make(chan struct{}),
		stats: BasicStoreStats{
			CreatedAt: time.Now(),
		},
	}

	// Create and configure eviction policy
	evictPolicy := cache.NewSessionEvictionPolicy()
	store.evictPolicy = evictPolicy

	// Initialize filter if configured
	if config.FilterConfig != nil {
		switch config.FilterConfig.FilterType {
		case "cuckoo":
			cuckooFilter, err := filter.NewCuckooFilter(config.FilterConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create cuckoo filter: %w", err)
			}
			store.filter = cuckooFilter
		default:
			return nil, fmt.Errorf("unsupported filter type: %s", config.FilterConfig.FilterType)
		}
	}

	// Initialize persistence if configured
	if config.PersistenceConfig != nil {
		persistEngine := persistence.NewHybridEngine(*config.PersistenceConfig)
		store.persistEngine = persistEngine
	}

	// Set up memory pressure callbacks — signal background evictor
	memPool.SetPressureHandlers(
		func(usage float64) { store.signalEviction() }, // Warning
		func(usage float64) { store.signalEviction() }, // Critical
		func(usage float64) { store.signalEviction() }, // Panic
	)

	// Start background evictor goroutine
	go store.backgroundEvictor()

	// Start background AOF goroutine
	go store.backgroundAOFWriter()

	// Start cleanup goroutine for expired items
	if config.CleanupInterval > 0 {
		go store.cleanupExpiredItems()
	}

	return store, nil
}

// SetWithContext adds or updates an item in the cache with correlation context
func (s *BasicStore) SetWithContext(ctx context.Context, key string, value interface{}, sessionID string, ttl time.Duration) error {
	return s.setWithContextInternal(ctx, key, value, sessionID, ttl, 0)
}

// Set adds or updates an item in the cache with true memory integration
func (s *BasicStore) Set(key string, value interface{}, sessionID string, ttl time.Duration) error {
	return s.setWithContextInternal(nil, key, value, sessionID, ttl, 0)
}

// SetWithTimestamp writes a value only if the Lamport timestamp is newer than the existing one.
func (s *BasicStore) SetWithTimestamp(ctx context.Context, key string, value interface{}, sessionID string, ttl time.Duration, lamportTS uint64) (bool, error) {
	if existing, ok := s.data.Get(key); ok && existing.LamportTimestamp >= lamportTS {
		return false, nil // Existing value is newer or equal — skip
	}

	err := s.setWithContextInternal(ctx, key, value, sessionID, ttl, lamportTS)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetTimestamp returns the Lamport timestamp for a key, or 0 if not found.
func (s *BasicStore) GetTimestamp(key string) uint64 {
	if item, ok := s.data.Get(key); ok {
		return item.LamportTimestamp
	}
	return 0
}

// setWithContextInternal is the internal implementation that accepts an optional context
func (s *BasicStore) setWithContextInternal(ctx context.Context, key string, value interface{}, sessionID string, ttl time.Duration, lamportTS uint64) error {
	if key == "" {
		s.incrementErrorCount()
		return fmt.Errorf("key cannot be empty")
	}

	// Serialize the value first to get actual memory requirements
	serializedData, valueType, err := serializeValue(value)
	if err != nil {
		s.incrementErrorCount()
		return fmt.Errorf("failed to serialize value: %w", err)
	}

	size := uint64(len(serializedData))

	// Check if we can allocate memory (no global lock needed — memPool is atomic)
	if s.memPool.AvailableSpace() < int64(size) {
		s.signalEviction()
		time.Sleep(500 * time.Microsecond)
		if s.memPool.AvailableSpace() < int64(size) {
			s.incrementErrorCount()
			return fmt.Errorf("insufficient memory: need %d bytes, available %d", size, s.memPool.AvailableSpace())
		}
	}

	// Allocate memory
	allocatedMemory, err := s.memPool.Allocate(int64(size))
	if err != nil {
		s.incrementErrorCount()
		return fmt.Errorf("failed to allocate memory: %w", err)
	}
	copy(allocatedMemory, serializedData)

	// Lock only the shard for this key
	s.data.LockShard(key)
	sh := s.data.getShard(key)

	// Handle existing item
	if existingItem, exists := sh.items[key]; exists {
		if oldPtr, ptrExists := sh.allocatedPtrs[key]; ptrExists {
			_ = s.memPool.Free(oldPtr)
		}
		oldEntry := s.itemToEntry(key, existingItem)
		s.evictPolicy.OnDelete(oldEntry)
		s.updateStats(func() {
			s.stats.TotalItems--
			s.stats.TotalMemory -= existingItem.Size
		})
	}

	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else if s.config.DefaultTTL > 0 {
		expiresAt = time.Now().Add(s.config.DefaultTTL)
	}

	item := &CacheItem{
		Key:              key,
		ValuePtr:         allocatedMemory,
		ValueType:        valueType,
		Size:             size,
		CreatedAt:        time.Now(),
		ExpiresAt:        expiresAt,
		SessionID:        sessionID,
		AccessCount:      0,
		LastAccessed:     time.Now(),
		LamportTimestamp: lamportTS,
	}

	sh.items[key] = item
	sh.allocatedPtrs[key] = allocatedMemory
	delete(sh.tombstones, key)
	s.data.UnlockShard(key)

	s.updateStats(func() {
		s.stats.TotalItems++
		s.stats.TotalMemory += size
		s.stats.LastAccess = time.Now()
	})

	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnInsert(entry)

	if s.filter != nil {
		_ = s.filter.Add([]byte(key))
	}

	if s.persistEngine != nil {
		logEntry := &persistence.LogEntry{
			Timestamp: time.Now(),
			Operation: "SET",
			Key:       key,
			Value:     serializedData,
			TTL:       int64(ttl.Seconds()),
			SessionID: sessionID,
		}
		select {
		case s.aofChan <- logEntry:
		default:
		}
	}

	return nil
}

// updateStats safely updates store stats under the stats mutex
func (s *BasicStore) updateStats(fn func()) {
	s.mutex.Lock()
	fn()
	s.mutex.Unlock()
}

// Get retrieves an item from the cache with true memory integration
// GetWithContext retrieves an item from the cache with correlation context
func (s *BasicStore) GetWithContext(ctx context.Context, key string) (interface{}, error) {
	return s.getInternal(ctx, key)
}

// Get retrieves an item from the cache
func (s *BasicStore) Get(key string) (interface{}, error) {
	return s.getInternal(nil, key)
}

// GetRawBytes retrieves the raw stored bytes for a key without deserialization.
// Returns (bytes, valueType, error). For string/[]byte values this is zero-copy
// and avoids the string(data) allocation that Get() performs.
// The RESP handler should use this instead of Get() for maximum throughput.
func (s *BasicStore) GetRawBytes(key string) ([]byte, string, error) {
	if key == "" {
		return nil, "", fmt.Errorf("key cannot be empty")
	}

	if s.filter != nil {
		if !s.filter.Contains([]byte(key)) {
			s.incrementMissCount()
			return nil, "", fmt.Errorf("key not found: %s", key)
		}
	}

	item, exists := s.data.Get(key)
	if !exists {
		s.incrementMissCount()
		return nil, "", fmt.Errorf("key not found: %s", key)
	}

	if item.IsExpired() {
		_ = s.Delete(key)
		s.incrementMissCount()
		return nil, "", fmt.Errorf("key expired: %s", key)
	}

	// Update access stats
	s.data.LockShard(key)
	item.AccessCount++
	item.LastAccessed = time.Now()
	s.data.UnlockShard(key)
	s.updateStats(func() { s.stats.LastAccess = time.Now() })

	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnAccess(entry)

	s.incrementHitCount()
	return item.GetRawBytes(), item.ValueType, nil
}

// getInternal is the internal implementation that accepts an optional context
func (s *BasicStore) getInternal(ctx context.Context, key string) (interface{}, error) {
	if key == "" {
		s.incrementErrorCount()
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Check filter first for early negative lookup (if filter is enabled)
	if s.filter != nil {
		if !s.filter.Contains([]byte(key)) {
			s.incrementMissCount()
			return nil, fmt.Errorf("key not found: %s", key)
		}
	}

	item, exists := s.data.Get(key)

	if !exists {
		s.incrementMissCount()
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check expiration
	if item.IsExpired() {
		_ = s.Delete(key)
		s.incrementMissCount()
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access statistics (non-critical, shard-local)
	s.data.LockShard(key)
	item.AccessCount++
	item.LastAccessed = time.Now()
	s.data.UnlockShard(key)
	s.updateStats(func() { s.stats.LastAccess = time.Now() })

	// Update eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnAccess(entry)

	// Deserialize value from allocated memory - THIS IS THE MAGIC!
	value, err := item.GetValue()
	if err != nil {
		s.incrementErrorCount()
		return nil, fmt.Errorf("failed to deserialize value from memory: %w", err)
	}

	s.incrementHitCount()
	return value, nil
}

// Delete removes an item from the cache
func (s *BasicStore) Delete(key string) error {
	if key == "" {
		s.incrementErrorCount()
		return fmt.Errorf("key cannot be empty")
	}

	item, allocPtr, existed := s.data.Delete(key)
	if !existed {
		return fmt.Errorf("key not found: %s", key)
	}

	// Free memory
	if allocPtr != nil {
		_ = s.memPool.Free(allocPtr)
	}

	// Remove from eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnDelete(entry)

	s.updateStats(func() {
		s.stats.TotalItems--
		s.stats.TotalMemory -= item.Size
		s.stats.LastAccess = time.Now()
	})

	// Remove from filter if available
	if s.filter != nil {
		s.filter.Delete([]byte(key))
	}

	// Tombstone already recorded by s.data.Delete() in the shard

	// Log to persistence via background AOF channel
	if s.persistEngine != nil {
		logEntry := &persistence.LogEntry{
			Timestamp: time.Now(),
			Operation: "DEL",
			Key:       key,
		}
		select {
		case s.aofChan <- logEntry:
		default:
		}
	}

	return nil
}

// signalEviction sends a non-blocking signal to the background evictor
func (s *BasicStore) signalEviction() {
	select {
	case s.evictSignal <- struct{}{}:
	default:
		// Already signaled
	}
}

// backgroundEvictor runs in a goroutine and proactively evicts when memory pressure is high
func (s *BasicStore) backgroundEvictor() {
	defer close(s.evictDone)
	for {
		select {
		case _, ok := <-s.evictSignal:
			if !ok {
				return
			}
			targetPressure := 0.75
			for s.memPool.MemoryPressure() > targetPressure {
				// Collect expired keys first
				expired := s.data.CollectExpired(func(item *CacheItem) bool { return item.IsExpired() })
				for _, key := range expired {
					_ = s.Delete(key)
				}

				if s.memPool.MemoryPressure() <= targetPressure {
					break
				}

				// Evict via eviction policy
				evicted := uint64(0)
				for i := 0; i < 20; i++ {
					candidate := s.evictPolicy.NextEvictionCandidate()
					if candidate == nil {
						break
					}
					key := string(candidate.Key)
					_ = s.Delete(key)
					evicted++
				}
				if evicted == 0 && len(expired) == 0 {
					break
				}
			}
		case <-s.stopCleanup:
			return
		}
	}
}

// backgroundAOFWriter drains the AOF channel and writes entries to persistence
func (s *BasicStore) backgroundAOFWriter() {
	defer close(s.aofDone)
	for entry := range s.aofChan {
		if s.persistEngine != nil {
			if err := s.persistEngine.WriteEntry(entry); err != nil {
				logging.Warn(nil, logging.ComponentStorage, logging.ActionPersist, "Background AOF write failed", map[string]interface{}{"error": err.Error()})
			}
		}
	}
}

// Clear removes all items from the cache
func (s *BasicStore) Clear() error {
	// Free all memory across shards
	s.data.RangeAll(func(key string, item *CacheItem) bool {
		if ptr, ok := s.data.GetAllocatedPtr(key); ok {
			_ = s.memPool.Free(ptr)
		}
		entry := s.itemToEntry(key, item)
		s.evictPolicy.OnDelete(entry)
		return true
	})

	s.data.Clear()

	s.mutex.Lock()
	s.stats.TotalItems = 0
	s.stats.TotalMemory = 0
	s.stats.LastAccess = time.Now()
	s.mutex.Unlock()

	if s.filter != nil {
		_ = s.filter.Clear()
	}

	return nil
}

// Size returns the number of items in the cache
func (s *BasicStore) Size() uint64 {
	return uint64(s.data.Size())
}

// Memory returns the total memory usage
func (s *BasicStore) Memory() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.stats.TotalMemory
}

// Stats returns cache statistics
func (s *BasicStore) Stats() BasicStoreStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.stats
}

// FilterStats returns filter statistics if filter is enabled
func (s *BasicStore) FilterStats() *filter.FilterStats {
	if s.filter == nil {
		return nil
	}
	return s.filter.GetStats()
}

// FilterContains checks if the cuckoo filter thinks a key might exist (probabilistic)
func (s *BasicStore) FilterContains(key string) bool {
	if s.filter == nil {
		return false
	}
	return s.filter.Contains([]byte(key))
}

// FilterAdd adds a key to the cuckoo filter without storing data.
// Used to pre-populate the filter on gossip receive so that concurrent
// GET requests see "maybe here" instead of "definitely not here" during
// the gossip propagation window.
func (s *BasicStore) FilterAdd(key string) {
	if s.filter != nil {
		_ = s.filter.Add([]byte(key))
	}
}

// IsTombstoned returns true if the key was recently deleted locally.
func (s *BasicStore) IsTombstoned(key string) bool {
	return s.data.IsTombstoned(key)
}

// Close shuts down the store and cleans up resources
func (s *BasicStore) Close() error {
	// Stop cleanup goroutine and background evictor
	select {
	case s.stopCleanup <- true:
	default:
	}

	// Close eviction signal channel to stop background evictor
	close(s.evictSignal)
	<-s.evictDone

	// Close AOF channel and wait for drain
	close(s.aofChan)
	<-s.aofDone

	// Clear all items
	_ = s.Clear()

	return nil
}

// cleanupExpiredItems runs periodic cleanup of expired items
func (s *BasicStore) cleanupExpiredItems() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			expired := s.data.CollectExpired(func(item *CacheItem) bool { return item.IsExpired() })
			for _, key := range expired {
				_ = s.Delete(key)
			}
		case <-s.stopCleanup:
			return
		}
	}
}

// Helper methods for thread-safe statistics updates
func (s *BasicStore) incrementHitCount() {
	s.mutex.Lock()
	s.stats.HitCount++
	s.mutex.Unlock()
}

func (s *BasicStore) incrementMissCount() {
	s.mutex.Lock()
	s.stats.MissCount++
	s.mutex.Unlock()
}

func (s *BasicStore) incrementErrorCount() {
	s.mutex.Lock()
	s.stats.ErrorCount++
	s.mutex.Unlock()
}

// itemToEntry converts a CacheItem to an Entry for the eviction policy
func (s *BasicStore) itemToEntry(key string, item *CacheItem) *cache.Entry {
	// Get the actual value for the entry (used by eviction policy)
	value, err := item.GetValue()
	var valueBytes []byte
	if err != nil {
		// If deserialization fails, just use the raw bytes
		valueBytes = item.ValuePtr[:item.Size]
	} else {
		valueBytes = []byte(fmt.Sprintf("%v", value))
	}

	return &cache.Entry{
		Key:       []byte(key),
		Value:     valueBytes,
		TTL:       0, // TTL not used in this context
		Version:   0, // Version not used in this context
		Timestamp: item.CreatedAt.Unix(),
		StoreID:   s.config.Name,
	}
}
