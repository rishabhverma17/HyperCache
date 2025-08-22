// Memory Pressure and Eviction Explained
// Understanding NeedsEviction() in HyperCache

package main

import (
	"fmt"
	"time"
)

// MemoryPool tracks memory usage for a store
type MemoryPool struct {
	maxSize     int64  // Maximum bytes allowed
	currentSize int64  // Current bytes used
	entries     int64  // Number of entries stored
}

// NeedsEviction checks if memory pool is under pressure
func (mp *MemoryPool) NeedsEviction() bool {
	memoryPressure := float64(mp.currentSize) / float64(mp.maxSize) > 0.8
	return memoryPressure
}

// Usage returns current memory usage as percentage
func (mp *MemoryPool) Usage() float64 {
	if mp.maxSize == 0 {
		return 0
	}
	return float64(mp.currentSize) / float64(mp.maxSize) * 100
}

// EvictionPolicy defines when to evict entries
type EvictionPolicy int

const (
	EvictOnPressure EvictionPolicy = iota
	EvictOnThreshold
)

// Entry represents a cache entry
type Entry struct {
	Key          string
	Value        []byte
	Size         int64
	CreatedAt    time.Time
	LastAccessed time.Time
}

// Store represents a cache store with memory management
type Store struct {
	name       string
	memoryPool *MemoryPool
	policy     EvictionPolicy
	entries    map[string]*Entry
}

// NeedsEviction checks if store is under memory pressure
func (store *Store) NeedsEviction() bool {
	return store.memoryPool.NeedsEviction()
}

// Evict removes entries to free memory
func (store *Store) Evict(count int) int {
	evicted := 0
	for key, entry := range store.entries {
		if evicted >= count {
			break
		}
		delete(store.entries, key)
		store.memoryPool.currentSize -= entry.Size
		store.memoryPool.entries--
		evicted++
	}
	return evicted
}

func main() {
	fmt.Println("=== Memory Pressure Examples ===")
	
	// Create demo store
	store := &Store{
		name:       "demo-store", 
		memoryPool: &MemoryPool{maxSize: 1024, currentSize: 900, entries: 100},
		policy:     EvictOnPressure,
		entries:    make(map[string]*Entry),
	}
	
	fmt.Printf("Store: %s, Needs eviction: %v\n", store.name, store.NeedsEviction())
	fmt.Printf("Memory usage: %.1f%%\n", store.memoryPool.Usage())
	
	// Add test entry
	entry := &Entry{
		Key:       "test_key",
		Value:     []byte("test_value"),
		Size:      50,
		CreatedAt: time.Now(),
	}
	store.entries["test_key"] = entry
	
	if store.NeedsEviction() {
		evicted := store.Evict(1)
		fmt.Printf("Evicted %d entries\n", evicted)
	}
}
