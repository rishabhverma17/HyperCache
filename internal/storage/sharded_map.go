package storage

import (
	"math/rand/v2"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const numShards = 32

// shard is a single partition of the sharded map
type shard struct {
	items         map[string]*CacheItem
	allocatedPtrs map[string][]byte
	tombstones    map[string]struct{} // lightweight tombstone set (expiry managed externally)
	mu            sync.RWMutex
}

// ShardedMap is a concurrent map split into numShards independent partitions.
// Each shard has its own lock, eliminating the global mutex bottleneck.
type ShardedMap struct {
	shards [numShards]shard
}

// NewShardedMap creates a new sharded map
func NewShardedMap() *ShardedMap {
	sm := &ShardedMap{}
	for i := range sm.shards {
		sm.shards[i].items = make(map[string]*CacheItem)
		sm.shards[i].allocatedPtrs = make(map[string][]byte)
		sm.shards[i].tombstones = make(map[string]struct{})
	}
	return sm
}

// getShard returns the shard for a given key
func (sm *ShardedMap) getShard(key string) *shard {
	h := xxhash.Sum64String(key)
	return &sm.shards[h%numShards]
}

// Get retrieves an item by key
func (sm *ShardedMap) Get(key string) (*CacheItem, bool) {
	s := sm.getShard(key)
	s.mu.RLock()
	item, ok := s.items[key]
	s.mu.RUnlock()
	return item, ok
}

// Set stores an item by key (caller must handle old item cleanup before calling)
func (sm *ShardedMap) Set(key string, item *CacheItem, allocPtr []byte) {
	s := sm.getShard(key)
	s.mu.Lock()
	s.items[key] = item
	s.allocatedPtrs[key] = allocPtr
	delete(s.tombstones, key) // clear tombstone on re-creation
	s.mu.Unlock()
}

// Delete removes an item by key. Returns the old item and its allocated pointer, or nil if not found.
func (sm *ShardedMap) Delete(key string) (*CacheItem, []byte, bool) {
	s := sm.getShard(key)
	s.mu.Lock()
	item, exists := s.items[key]
	if !exists {
		s.mu.Unlock()
		return nil, nil, false
	}
	ptr := s.allocatedPtrs[key]
	delete(s.items, key)
	delete(s.allocatedPtrs, key)
	s.tombstones[key] = struct{}{}
	s.mu.Unlock()
	return item, ptr, true
}

// GetOldForReplace retrieves and deletes the old item for a key, used during SET to free old memory.
// Returns old item, old allocated pointer, and whether old existed.
func (sm *ShardedMap) GetOldForReplace(key string) (*CacheItem, []byte, bool) {
	s := sm.getShard(key)
	s.mu.RLock()
	item, exists := s.items[key]
	var ptr []byte
	if exists {
		ptr = s.allocatedPtrs[key]
	}
	s.mu.RUnlock()
	return item, ptr, exists
}

// Exists checks if a key exists
func (sm *ShardedMap) Exists(key string) bool {
	s := sm.getShard(key)
	s.mu.RLock()
	_, ok := s.items[key]
	s.mu.RUnlock()
	return ok
}

// IsTombstoned checks if a key was recently deleted
func (sm *ShardedMap) IsTombstoned(key string) bool {
	s := sm.getShard(key)
	s.mu.RLock()
	_, ok := s.tombstones[key]
	s.mu.RUnlock()
	return ok
}

// ClearTombstone removes a tombstone for a key
func (sm *ShardedMap) ClearTombstone(key string) {
	s := sm.getShard(key)
	s.mu.Lock()
	delete(s.tombstones, key)
	s.mu.Unlock()
}

// Size returns total number of items across all shards
func (sm *ShardedMap) Size() int {
	total := 0
	for i := range sm.shards {
		sm.shards[i].mu.RLock()
		total += len(sm.shards[i].items)
		sm.shards[i].mu.RUnlock()
	}
	return total
}

// RangeAll calls fn for every item across all shards. fn must NOT modify the map.
// If fn returns false, iteration stops.
func (sm *ShardedMap) RangeAll(fn func(key string, item *CacheItem) bool) {
	for i := range sm.shards {
		sm.shards[i].mu.RLock()
		for k, v := range sm.shards[i].items {
			if !fn(k, v) {
				sm.shards[i].mu.RUnlock()
				return
			}
		}
		sm.shards[i].mu.RUnlock()
	}
}

// DeleteFromShard deletes a key while the shard is already locked (used during eviction).
// Caller must hold the shard write lock.
func (sm *ShardedMap) DeleteUnsafe(key string) (*CacheItem, []byte, bool) {
	s := sm.getShard(key)
	item, exists := s.items[key]
	if !exists {
		return nil, nil, false
	}
	ptr := s.allocatedPtrs[key]
	delete(s.items, key)
	delete(s.allocatedPtrs, key)
	return item, ptr, true
}

// LockShard locks the shard for a given key (write lock)
func (sm *ShardedMap) LockShard(key string) {
	sm.getShard(key).mu.Lock()
}

// UnlockShard unlocks the shard for a given key (write lock)
func (sm *ShardedMap) UnlockShard(key string) {
	sm.getShard(key).mu.Unlock()
}

// Clear removes all items from all shards
func (sm *ShardedMap) Clear() {
	for i := range sm.shards {
		sm.shards[i].mu.Lock()
		sm.shards[i].items = make(map[string]*CacheItem)
		sm.shards[i].allocatedPtrs = make(map[string][]byte)
		sm.shards[i].tombstones = make(map[string]struct{})
		sm.shards[i].mu.Unlock()
	}
}

// CollectExpired returns a list of (key, item) that are expired. Used by background evictor.
func (sm *ShardedMap) CollectExpired(isExpired func(item *CacheItem) bool) []string {
	var expired []string
	for i := range sm.shards {
		sm.shards[i].mu.RLock()
		for k, v := range sm.shards[i].items {
			if isExpired(v) {
				expired = append(expired, k)
			}
		}
		sm.shards[i].mu.RUnlock()
	}
	return expired
}

// GetAllocatedPtr returns the allocated pointer for a key
func (sm *ShardedMap) GetAllocatedPtr(key string) ([]byte, bool) {
	s := sm.getShard(key)
	s.mu.RLock()
	ptr, ok := s.allocatedPtrs[key]
	s.mu.RUnlock()
	return ptr, ok
}

// SampleKeys returns up to n random keys from across all shards.
// Uses random sampling (Redis-style) for O(n) eviction candidate selection.
func (sm *ShardedMap) SampleKeys(n int) []string {
	if n <= 0 {
		return nil
	}
	result := make([]string, 0, n)
	startShard := rand.IntN(numShards)
	for attempt := 0; attempt < numShards && len(result) < n; attempt++ {
		idx := (startShard + attempt) % numShards
		s := &sm.shards[idx]
		s.mu.RLock()
		need := n - len(result)
		collected := 0
		for k := range s.items {
			result = append(result, k)
			collected++
			if collected >= need {
				break
			}
		}
		s.mu.RUnlock()
	}
	return result
}

// SnapshotItem holds a point-in-time copy of a single cached entry's raw data.
type SnapshotItem struct {
	Key       string
	RawBytes  []byte
	ValueType string
	Size      uint64
}

// SnapshotRawData returns a copy of all keys with their raw bytes and value types.
// Each shard is briefly RLocked then released, so writes to other shards proceed concurrently.
func (sm *ShardedMap) SnapshotRawData() []SnapshotItem {
	total := 0
	for i := range sm.shards {
		sm.shards[i].mu.RLock()
		total += len(sm.shards[i].items)
		sm.shards[i].mu.RUnlock()
	}
	result := make([]SnapshotItem, 0, total)
	for i := range sm.shards {
		sm.shards[i].mu.RLock()
		for _, item := range sm.shards[i].items {
			raw := make([]byte, len(item.ValuePtr))
			copy(raw, item.ValuePtr)
			result = append(result, SnapshotItem{
				Key:       item.Key,
				RawBytes:  raw,
				ValueType: item.ValueType,
				Size:      item.Size,
			})
		}
		sm.shards[i].mu.RUnlock()
	}
	return result
}
