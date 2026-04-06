package storage

import (
	"context"
	"fmt"
	"time"

	"hypercache/internal/cache"
	"hypercache/internal/logging"
	"hypercache/internal/persistence"
)

// StartPersistence initializes and starts the persistence engine
func (s *BasicStore) StartPersistence(ctx context.Context) error {
	if s.persistEngine == nil {
		return nil // No persistence configured
	}

	// Wire snapshot data callback so background workers can access cache data
	if he, ok := s.persistEngine.(*persistence.HybridEngine); ok {
		he.SetSnapshotDataFunc(s.getSnapshotData)
	}

	if err := s.persistEngine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start persistence engine: %w", err)
	}

	// Attempt recovery from persistence
	if err := s.recoverFromPersistence(); err != nil {
		return fmt.Errorf("failed to recover from persistence: %w", err)
	}

	return nil
}

// getSnapshotData returns current cache data for snapshot/compaction use.
// Thread-safe: acquires read lock.
func (s *BasicStore) getSnapshotData() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := make(map[string]interface{}, len(s.items))
	for key, item := range s.items {
		value, err := item.GetValue()
		if err != nil {
			continue
		}
		data[key] = value
	}
	return data
}

// StopPersistence gracefully stops the persistence engine
func (s *BasicStore) StopPersistence() error {
	if s.persistEngine == nil {
		return nil
	}

	return s.persistEngine.Stop()
}

// CreateSnapshot creates a persistence snapshot of current cache state
func (s *BasicStore) CreateSnapshot() error {
	if s.persistEngine == nil {
		return fmt.Errorf("persistence not enabled")
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Convert cache items to snapshot data
	data := make(map[string]interface{})
	for key, item := range s.items {
		// For simplicity, convert back to original value
		value, err := item.GetValue()
		if err != nil {
			logging.Warn(nil, logging.ComponentStorage, logging.ActionSnapshot, "Failed to get value for key during snapshot", map[string]interface{}{"key": key, "error": err.Error()})
			continue
		}
		data[key] = value
	}

	return s.persistEngine.CreateSnapshot(data)
}

// GetPersistenceStats returns persistence statistics
func (s *BasicStore) GetPersistenceStats() interface{} {
	if s.persistEngine == nil {
		return nil
	}
	return s.persistEngine.GetStats()
}

// recoverFromPersistence replays persistence logs to restore cache state
func (s *BasicStore) recoverFromPersistence() error {
	if s.persistEngine == nil {
		return nil
	}

	// Read all persistence entries
	entries, err := s.persistEngine.ReadEntries()
	if err != nil {
		return fmt.Errorf("failed to read persistence entries: %w", err)
	}

	if len(entries) == 0 {
		logging.Info(nil, logging.ComponentStorage, logging.ActionRestore, "No persistence entries to recover")
		return nil
	}

	logging.Info(nil, logging.ComponentStorage, logging.ActionRestore, "Recovering entries from persistence", map[string]interface{}{"entry_count": len(entries)})

	s.mutex.Lock()
	defer s.mutex.Unlock()

	recoveredCount := 0
	errorCount := 0

	for _, entry := range entries {
		switch entry.Operation {
		case "SET":
			// AOF stores values as raw bytes without type info.
			// Convert to string for recovery — this matches the original write path
			// where most values come in as strings from RESP/HTTP.
			value := string(entry.Value)

			// Calculate TTL from entry
			var ttl time.Duration
			if entry.TTL > 0 {
				// Check if item should have expired by now
				createdAt := entry.Timestamp
				expiresAt := createdAt.Add(time.Duration(entry.TTL) * time.Second)
				if time.Now().After(expiresAt) {
					// Skip expired items
					continue
				}
				ttl = time.Until(expiresAt)
			}

			// Use internal set without persistence logging to avoid recursion
			if err := s.setInternal(entry.Key, value, entry.SessionID, ttl); err != nil {
				logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore, "Failed to recover SET", map[string]interface{}{"key": entry.Key, "error": err.Error()})
				errorCount++
				continue
			}
			recoveredCount++

		case "DEL":
			// Delete the key if it exists
			if _, exists := s.items[entry.Key]; exists {
				if err := s.deleteInternal(entry.Key); err != nil {
					logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore, "Failed to recover DEL", map[string]interface{}{"key": entry.Key, "error": err.Error()})
					errorCount++
					continue
				}
				recoveredCount++
			}

		case "CLEAR":
			// Clear all items
			s.clearInternal()
			recoveredCount++
		}
	}

	logging.Info(nil, logging.ComponentStorage, logging.ActionRestore, "Recovery complete", map[string]interface{}{"recovered": recoveredCount, "errors": errorCount})
	return nil
}

// setInternal is like Set but without persistence logging (used for recovery)
func (s *BasicStore) setInternal(key string, value interface{}, sessionID string, ttl time.Duration) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// Serialize the value
	serializedData, valueType, err := serializeValue(value)
	if err != nil {
		return fmt.Errorf("failed to serialize value: %w", err)
	}

	size := uint64(len(serializedData))

	// Allocate memory
	allocatedMemory, err := s.memPool.Allocate(int64(size))
	if err != nil {
		return fmt.Errorf("failed to allocate memory: %w", err)
	}

	copy(allocatedMemory, serializedData)

	// Handle existing item
	if existingItem, exists := s.items[key]; exists {
		if oldPtr, ptrExists := s.allocatedPtrs[key]; ptrExists {
			_ = s.memPool.Free(oldPtr)
			delete(s.allocatedPtrs, key)
		}
		oldEntry := s.itemToEntry(key, existingItem)
		s.evictPolicy.OnDelete(oldEntry)
		s.stats.TotalItems--
		s.stats.TotalMemory -= existingItem.Size
	}

	// Create new item
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
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
		LamportTimestamp: 0,
	}

	// Store the item
	s.items[key] = item
	s.allocatedPtrs[key] = allocatedMemory
	s.stats.TotalItems++
	s.stats.TotalMemory += size

	// Add to eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnInsert(entry)

	// Add to filter
	if s.filter != nil {
		_ = s.filter.Add([]byte(key))
	}

	return nil
}

// deleteInternal is like Delete but without persistence logging (used for recovery)
func (s *BasicStore) deleteInternal(key string) error {
	item, exists := s.items[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	// Free memory
	if ptr, ptrExists := s.allocatedPtrs[key]; ptrExists {
		_ = s.memPool.Free(ptr)
		delete(s.allocatedPtrs, key)
	}

	// Remove from eviction policy
	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnDelete(entry)

	// Remove from items
	delete(s.items, key)
	s.stats.TotalItems--
	s.stats.TotalMemory -= item.Size

	// Remove from filter
	if s.filter != nil {
		s.filter.Delete([]byte(key))
	}

	return nil
}

// clearInternal is like Clear but without persistence logging (used for recovery)
func (s *BasicStore) clearInternal() {
	// Free all allocated memory
	for _, ptr := range s.allocatedPtrs {
		_ = s.memPool.Free(ptr)
	}

	// Clear all data structures
	s.items = make(map[string]*CacheItem)
	s.allocatedPtrs = make(map[string][]byte)
	s.stats.TotalItems = 0
	s.stats.TotalMemory = 0
	s.evictPolicy = cache.NewSessionEvictionPolicy() // Reset eviction policy
}
