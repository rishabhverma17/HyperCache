package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
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
	Key          string
	ValuePtr     []byte    // Points to actual allocated memory containing serialized value
	ValueType    string    // Type information for deserialization  
	Size         uint64    // Size of allocated memory
	CreatedAt    time.Time
	ExpiresAt    time.Time
	SessionID    string
	AccessCount  uint64
	LastAccessed time.Time
}

// GetValue deserializes and returns the actual value from allocated memory
func (item *CacheItem) GetValue() (interface{}, error) {
	return deserializeValue(item.ValuePtr, item.ValueType)
}

// IsExpired checks if the item has expired
func (item *CacheItem) IsExpired() bool {
	return !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt)
}

// BasicStoreConfig holds configuration for BasicStore
type BasicStoreConfig struct {
	Name               string
	MaxMemory          uint64
	DefaultTTL         time.Duration
	EnableStatistics   bool
	CleanupInterval    time.Duration
	FilterConfig       *filter.FilterConfig       // Optional filter configuration (nil = no filter)
	PersistenceConfig  *persistence.PersistenceConfig // Optional persistence configuration (nil = no persistence)
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
	config         BasicStoreConfig
	items          map[string]*CacheItem
	memPool        *MemoryPool
	evictPolicy    cache.EvictionPolicy
	filter         filter.ProbabilisticFilter  // Optional Cuckoo/Bloom filter for negative lookups
	persistEngine  persistence.PersistenceEngine // Optional persistence layer
	mutex          sync.RWMutex
	stats          BasicStoreStats
	stopCleanup    chan bool
	allocatedPtrs  map[string][]byte // Track allocated pointers for proper cleanup
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
		// Use gob encoding for complex types
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		if err := encoder.Encode(v); err != nil {
			return nil, "", fmt.Errorf("failed to encode value: %w", err)
		}
		return buf.Bytes(), valueType, nil
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
		// Use gob decoding for complex types
		buf := bytes.NewBuffer(data)
		decoder := gob.NewDecoder(buf)
		var result interface{}
		if err := decoder.Decode(&result); err != nil {
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
		config:        config,
		items:         make(map[string]*CacheItem),
		memPool:       memPool,
		stopCleanup:   make(chan bool),
		allocatedPtrs: make(map[string][]byte),
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

	// Set up memory pressure callbacks
	memPool.SetPressureHandlers(
		func(usage float64) { store.handleMemoryPressure("low", usage) },    // Warning
		func(usage float64) { store.handleMemoryPressure("medium", usage) }, // Critical
		func(usage float64) { store.handleMemoryPressure("high", usage) },   // Panic
	)

	// Start cleanup goroutine for expired items
	if config.CleanupInterval > 0 {
		go store.cleanupExpiredItems()
	}

	return store, nil
}

// SetWithContext adds or updates an item in the cache with correlation context
func (s *BasicStore) SetWithContext(ctx context.Context, key string, value interface{}, sessionID string, ttl time.Duration) error {
	// Store the current context for logging
	originalLoggingContext := ctx
	defer func() {
		// Restore context if needed
		_ = originalLoggingContext
	}()
	
	return s.setWithContextInternal(ctx, key, value, sessionID, ttl)
}

// Set adds or updates an item in the cache with true memory integration
func (s *BasicStore) Set(key string, value interface{}, sessionID string, ttl time.Duration) error {
	return s.setWithContextInternal(nil, key, value, sessionID, ttl)
}

// setWithContextInternal is the internal implementation that accepts an optional context
func (s *BasicStore) setWithContextInternal(ctx context.Context, key string, value interface{}, sessionID string, ttl time.Duration) error {
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

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if we can allocate memory
	if s.memPool.AvailableSpace() < int64(size) {
		// Try to evict some items to make space
		if evictErr := s.evictForSpace(size); evictErr != nil {
			s.incrementErrorCount()
			return fmt.Errorf("insufficient memory: need %d bytes, available %d", size, s.memPool.AvailableSpace())
		}
	}

	// Allocate memory for the serialized data
	allocatedMemory, err := s.memPool.Allocate(int64(size))
	if err != nil {
		s.incrementErrorCount()
		return fmt.Errorf("failed to allocate memory: %w", err)
	}

	// Copy serialized data into allocated memory
	copy(allocatedMemory, serializedData)

	// Handle existing item
	if existingItem, exists := s.items[key]; exists {
		// Free old item's memory
		if oldPtr, ptrExists := s.allocatedPtrs[key]; ptrExists {
			s.memPool.Free(oldPtr)
			delete(s.allocatedPtrs, key)
		}
		// Remove from eviction policy
		oldEntry := s.itemToEntry(key, existingItem)
		s.evictPolicy.OnDelete(oldEntry)
		s.stats.TotalItems--
		s.stats.TotalMemory -= existingItem.Size
	}

	// Create new item with memory integration
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else if s.config.DefaultTTL > 0 {
		expiresAt = time.Now().Add(s.config.DefaultTTL)
	}

	item := &CacheItem{
		Key:          key,
		ValuePtr:     allocatedMemory, // Points to actual allocated memory!
		ValueType:    valueType,       // Store type for deserialization
		Size:         size,            // Actual size of serialized data
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
		SessionID:    sessionID,
		AccessCount:  0,
		LastAccessed: time.Now(),
	}

	// Store the item and track allocation
	s.items[key] = item
	s.allocatedPtrs[key] = allocatedMemory
	s.stats.TotalItems++
	s.stats.TotalMemory += size
	s.stats.LastAccess = time.Now()

	// Add to eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnInsert(entry)

	// Add to filter if available
	if s.filter != nil {
		if err := s.filter.Add([]byte(key)); err != nil {
			// Log error but don't fail the Set operation
			logging.Warn(nil, logging.ComponentFilter, "add_error", "Failed to add key to cuckoo filter", map[string]interface{}{
				"key":   key,
				"store": s.config.Name,
				"filter_type": "cuckoo",
				"error": err.Error(),
			})
		} else {
			logging.Debug(ctx, logging.ComponentFilter, "add_success", "Key added to cuckoo filter", map[string]interface{}{
				"key":   key,
				"store": s.config.Name,
				"filter_type": "cuckoo",
			})
		}
	}

	// Log to persistence if enabled
	if s.persistEngine != nil {
		logEntry := &persistence.LogEntry{
			Timestamp: time.Now(),
			Operation: "SET",
			Key:       key,
			Value:     serializedData,
			TTL:       int64(ttl.Seconds()),
			SessionID: sessionID,
		}
		if err := s.persistEngine.WriteEntry(logEntry); err != nil {
			// Log error but don't fail the Set operation
			// In production, you might want to handle this more gracefully
			fmt.Printf("Warning: failed to log SET operation: %v\n", err)
		}
	}

	return nil
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

// getInternal is the internal implementation that accepts an optional context
func (s *BasicStore) getInternal(ctx context.Context, key string) (interface{}, error) {
	if key == "" {
		s.incrementErrorCount()
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Check filter first for early negative lookup (if filter is enabled)
	if s.filter != nil {
		if !s.filter.Contains([]byte(key)) {
			// Key definitely not in cache - early return
			logging.Debug(ctx, logging.ComponentFilter, "negative_lookup", "Cuckoo filter early rejection", map[string]interface{}{
				"key":   key,
				"store": s.config.Name,
				"filter_type": "cuckoo",
			})
			s.incrementMissCount()
			return nil, fmt.Errorf("key not found: %s", key)
		}
		// Key might be in cache (filter says possibly present)
		logging.Debug(ctx, logging.ComponentFilter, "positive_lookup", "Cuckoo filter possible match", map[string]interface{}{
			"key":   key,
			"store": s.config.Name,
			"filter_type": "cuckoo",
		})
		// Continue with actual lookup
	}

	s.mutex.RLock()
	item, exists := s.items[key]
	s.mutex.RUnlock()

	if !exists {
		s.incrementMissCount()
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check expiration
	if item.IsExpired() {
		// Remove expired item
		s.Delete(key)
		s.incrementMissCount()
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access statistics
	s.mutex.Lock()
	item.AccessCount++
	item.LastAccessed = time.Now()
	s.stats.LastAccess = time.Now()
	s.mutex.Unlock()

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

	s.mutex.Lock()
	defer s.mutex.Unlock()

	item, exists := s.items[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	// Free memory
	if ptr, ptrExists := s.allocatedPtrs[key]; ptrExists {
		s.memPool.Free(ptr)
		delete(s.allocatedPtrs, key)
	}

	// Remove from eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnDelete(entry)

	// Remove from items
	delete(s.items, key)
	s.stats.TotalItems--
	s.stats.TotalMemory -= item.Size
	s.stats.LastAccess = time.Now()

	// Remove from filter if available
	if s.filter != nil {
		s.filter.Delete([]byte(key))
		// Note: Delete returns bool, but we don't need to handle failure here
	}

	// Log to persistence if enabled
	if s.persistEngine != nil {
		logEntry := &persistence.LogEntry{
			Timestamp: time.Now(),
			Operation: "DEL",
			Key:       key,
		}
		if err := s.persistEngine.WriteEntry(logEntry); err != nil {
			fmt.Printf("Warning: failed to log DELETE operation: %v\n", err)
		}
	}

	return nil
}

// Clear removes all items from the cache
func (s *BasicStore) Clear() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Free all memory
	for key, ptr := range s.allocatedPtrs {
		s.memPool.Free(ptr)
		// Notify eviction policy
		if item, exists := s.items[key]; exists {
			entry := s.itemToEntry(key, item)
			s.evictPolicy.OnDelete(entry)
		}
	}

	// Clear eviction policy (already cleared through OnDelete calls above)

	// Clear items and pointers
	s.items = make(map[string]*CacheItem)
	s.allocatedPtrs = make(map[string][]byte)
	s.stats.TotalItems = 0
	s.stats.TotalMemory = 0
	s.stats.LastAccess = time.Now()

	// Clear filter if available
	if s.filter != nil {
		s.filter.Clear()
	}

	return nil
}

// Size returns the number of items in the cache
func (s *BasicStore) Size() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.stats.TotalItems
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

// Close shuts down the store and cleans up resources
func (s *BasicStore) Close() error {
	// Stop cleanup goroutine
	select {
	case s.stopCleanup <- true:
	default:
	}

	// Clear all items
	s.Clear()

	// Close resources - no specific close method needed for MemoryPool
	return nil
}

// handleMemoryPressure handles memory pressure events by triggering evictions
// This method needs to be thread-safe since it's called from memory pool callbacks
func (s *BasicStore) handleMemoryPressure(level string, usage float64) {
	// Use a separate goroutine to avoid blocking the allocation path
	go func() {
		switch level {
		case "low":
			// Light eviction - remove expired items
			s.evictExpiredItemsSafe()
		case "medium":
			// Medium eviction - remove expired + least accessed
			s.evictExpiredItemsSafe()
			s.evictLeastAccessedSafe(10) // Evict 10 items
		case "high":
			// Aggressive eviction - remove expired + more least accessed
			s.evictExpiredItemsSafe()
			s.evictLeastAccessedSafe(50) // Evict 50 items
		}
	}()
}

// evictForSpace tries to evict items to make space for new allocation
func (s *BasicStore) evictForSpace(neededSize uint64) error {
	// First try expired items
	evicted := s.evictExpiredItems()
	if s.memPool.AvailableSpace() >= int64(neededSize) {
		return nil
	}

	// Use eviction policy to find candidates
	for i := 0; i < 10 && s.memPool.AvailableSpace() < int64(neededSize); i++ {
		candidate := s.evictPolicy.NextEvictionCandidate()
		if candidate == nil {
			break
		}

		key := string(candidate.Key)
		if item, exists := s.items[key]; exists {
			// Free memory
			if ptr, ptrExists := s.allocatedPtrs[key]; ptrExists {
				s.memPool.Free(ptr)
				delete(s.allocatedPtrs, key)
			}
			// Remove from eviction policy
			s.evictPolicy.OnDelete(candidate)
			// Remove from items
			delete(s.items, key)
			s.stats.TotalItems--
			s.stats.TotalMemory -= item.Size
			s.stats.EvictionCount++
			evicted++

			// Remove from filter if available
			if s.filter != nil {
				s.filter.Delete([]byte(key))
			}
		}
	}

	if evicted == 0 {
		return fmt.Errorf("unable to evict any items to make space")
	}

	return nil
}

// evictExpiredItems removes all expired items (assumes lock is held)
func (s *BasicStore) evictExpiredItems() uint64 {
	return s.evictExpiredItemsUnsafe()
}

// evictLeastAccessed removes the least accessed items (assumes lock is held)
func (s *BasicStore) evictLeastAccessed(count uint64) uint64 {
	return s.evictLeastAccessedUnsafe(count)
}

// cleanupExpiredItems runs periodic cleanup of expired items
func (s *BasicStore) cleanupExpiredItems() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.evictExpiredItemsSafe() // Use safe version for background cleanup
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

// evictExpiredItemsSafe is a thread-safe version of evictExpiredItems for async callbacks
func (s *BasicStore) evictExpiredItemsSafe() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.evictExpiredItemsUnsafe()
}

// evictExpiredItemsUnsafe removes expired items without locking (caller must hold lock)
func (s *BasicStore) evictExpiredItemsUnsafe() uint64 {
	now := time.Now()
	evicted := uint64(0)

	for key, item := range s.items {
		if !item.ExpiresAt.IsZero() && now.After(item.ExpiresAt) {
			// Free memory
			if ptr, ptrExists := s.allocatedPtrs[key]; ptrExists {
				s.memPool.Free(ptr)
				delete(s.allocatedPtrs, key)
			}
			// Remove from eviction policy
			entry := s.itemToEntry(key, item)
			s.evictPolicy.OnDelete(entry)
			// Remove from items
			delete(s.items, key)
			s.stats.TotalItems--
			s.stats.TotalMemory -= item.Size
			s.stats.EvictionCount++
			evicted++

			// Remove from filter if available
			if s.filter != nil {
				s.filter.Delete([]byte(key))
			}
		}
	}

	return evicted
}

// evictLeastAccessedSafe is a thread-safe version of evictLeastAccessed for async callbacks
func (s *BasicStore) evictLeastAccessedSafe(count uint64) uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.evictLeastAccessedUnsafe(count)
}

// evictLeastAccessedUnsafe removes least accessed items without locking (caller must hold lock)
func (s *BasicStore) evictLeastAccessedUnsafe(count uint64) uint64 {
	if count == 0 {
		return 0
	}

	evicted := uint64(0)
	for i := uint64(0); i < count; i++ {
		candidate := s.evictPolicy.NextEvictionCandidate()
		if candidate == nil {
			break // No more candidates
		}

		key := string(candidate.Key)
		if item, exists := s.items[key]; exists {
			// Free memory
			if ptr, ptrExists := s.allocatedPtrs[key]; ptrExists {
				s.memPool.Free(ptr)
				delete(s.allocatedPtrs, key)
			}
			// Remove from eviction policy
			s.evictPolicy.OnDelete(candidate)
			// Remove from items
			delete(s.items, key)
			s.stats.TotalItems--
			s.stats.TotalMemory -= item.Size
			s.stats.EvictionCount++
			evicted++

			// Remove from filter if available
			if s.filter != nil {
				s.filter.Delete([]byte(key))
			}
		}
	}

	return evicted
}
