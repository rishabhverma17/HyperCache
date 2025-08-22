package cache

import (
	"context"
	"time"
)

// Entry represents a cache entry with metadata
type Entry struct {
	Key       []byte
	Value     []byte
	TTL       time.Duration
	Version   uint64
	Timestamp int64
	StoreID   string
}

// Cache defines the core cache interface
type Cache interface {
	// Basic operations - must be O(1)
	Get(ctx context.Context, store string, key []byte) ([]byte, error)
	Put(ctx context.Context, store string, key []byte, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, store string, key []byte) error
	Exists(ctx context.Context, store string, key []byte) bool

	// Batch operations for efficiency
	BatchGet(ctx context.Context, store string, keys [][]byte) (map[string][]byte, error)
	BatchPut(ctx context.Context, store string, entries map[string]Entry) error

	// Probabilistic operations
	MightContain(ctx context.Context, store string, key []byte) bool

	// Store management
	CreateStore(name string, config StoreConfig) error
	DropStore(name string) error
	ListStores() []string
	StoreStats(name string) (*StoreStats, error)  // Returns pointer to avoid copying large struct

	// Cleanup
	Close() error
}

// Store represents an individual cache store with its own eviction policy
type Store interface {
	// Basic operations - O(1) guaranteed
	Get(key []byte) ([]byte, error)
	Put(key []byte, value []byte, ttl time.Duration) error
	Delete(key []byte) error
	Exists(key []byte) bool

	// Memory management
	NeedsEviction() bool
	Evict() (*Entry, error)
	
	// Metadata
	Name() string
	Size() int64
	Stats() *StoreStats
	
	// Cleanup
	Close() error
}

// EvictionPolicy defines the interface for eviction policies
type EvictionPolicy interface {
	// Must be O(1) for cache performance
	ShouldEvict(entry *Entry, memoryPressure float64) bool
	OnAccess(entry *Entry)  // Update access patterns
	OnInsert(entry *Entry)  // Handle new entries  
	OnDelete(entry *Entry)  // Clean up tracking
	
	// Get next eviction candidate - MUST be O(1)
	NextEvictionCandidate() *Entry
	
	// Policy metadata
	PolicyName() string
}

// MemoryPool manages memory allocation for a store
type MemoryPool interface {
	// Memory management - O(1) operations
	Allocate(size int64) ([]byte, error)
	Free(ptr []byte) error
	
	// Memory status - O(1) operations
	CurrentUsage() int64
	MaxSize() int64
	AvailableSpace() int64
	
	// Memory pressure calculation
	MemoryPressure() float64 // 0.0 to 1.0
}

// StoreConfig defines configuration for individual stores
type StoreConfig struct {
	Name           string
	EvictionPolicy string
	MaxMemory      int64
	DefaultTTL     time.Duration
}

// StoreStats provides metrics about store performance
type StoreStats struct {
	Name             string    `json:"name"`
	EvictionPolicy   string    `json:"eviction_policy"`
	
	// Memory metrics
	MaxMemory        int64     `json:"max_memory"`
	CurrentMemory    int64     `json:"current_memory"`
	MemoryPressure   float64   `json:"memory_pressure"`
	
	// Operation metrics
	TotalEntries     int64     `json:"total_entries"`
	Hits             int64     `json:"hits"`
	Misses           int64     `json:"misses"`
	Evictions        int64     `json:"evictions"`
	
	// Performance metrics
	AvgGetLatency    time.Duration `json:"avg_get_latency"`
	AvgPutLatency    time.Duration `json:"avg_put_latency"`
	
	// Time metrics
	LastAccess       time.Time `json:"last_access"`
	CreatedAt        time.Time `json:"created_at"`
}
