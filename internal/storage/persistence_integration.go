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
func (s *BasicStore) getSnapshotData() map[string]interface{} {
	data := make(map[string]interface{})
	s.data.RangeAll(func(key string, item *CacheItem) bool {
		value, err := item.GetValue()
		if err != nil {
			return true // continue
		}
		data[key] = value
		return true
	})
	return data
}

// StopPersistence gracefully stops the persistence engine.
// Closes the AOF channel, waits for the background writer to drain, then flushes.
func (s *BasicStore) StopPersistence() error {
	if s.persistEngine == nil {
		return nil
	}

	// Close AOF channel so backgroundAOFWriter drains remaining entries and exits
	s.aofCloseOnce.Do(func() { close(s.aofChan) })
	<-s.aofDone

	// Flush buffered writes to disk
	_ = s.persistEngine.Flush()

	return s.persistEngine.Stop()
}

// CreateSnapshot creates a persistence snapshot of current cache state
func (s *BasicStore) CreateSnapshot() error {
	if s.persistEngine == nil {
		return fmt.Errorf("persistence not enabled")
	}

	data := s.getSnapshotData()
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

	recoveredCount := 0
	errorCount := 0

	for _, entry := range entries {
		switch entry.Operation {
		case "SET":
			value := string(entry.Value)

			var ttl time.Duration
			if entry.TTL > 0 {
				createdAt := entry.Timestamp
				expiresAt := createdAt.Add(time.Duration(entry.TTL) * time.Second)
				if time.Now().After(expiresAt) {
					continue
				}
				ttl = time.Until(expiresAt)
			}

			if err := s.setInternal(entry.Key, value, entry.SessionID, ttl); err != nil {
				logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore, "Failed to recover SET", map[string]interface{}{"key": entry.Key, "error": err.Error()})
				errorCount++
				continue
			}
			recoveredCount++

		case "DEL":
			if s.data.Exists(entry.Key) {
				if err := s.deleteInternal(entry.Key); err != nil {
					logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore, "Failed to recover DEL", map[string]interface{}{"key": entry.Key, "error": err.Error()})
					errorCount++
					continue
				}
				recoveredCount++
			}

		case "CLEAR":
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

	serializedData, valueType, err := serializeValue(value)
	if err != nil {
		return fmt.Errorf("failed to serialize value: %w", err)
	}

	size := uint64(len(serializedData))

	allocatedMemory, err := s.memPool.Allocate(int64(size))
	if err != nil {
		return fmt.Errorf("failed to allocate memory: %w", err)
	}
	copy(allocatedMemory, serializedData)

	// Handle existing item via ShardedMap
	s.data.LockShard(key)
	sh := s.data.getShard(key)
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

	sh.items[key] = item
	sh.allocatedPtrs[key] = allocatedMemory
	s.data.UnlockShard(key)

	s.updateStats(func() {
		s.stats.TotalItems++
		s.stats.TotalMemory += size
	})

	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnInsert(entry)

	if s.filter != nil {
		_ = s.filter.Add([]byte(key))
	}

	return nil
}

// deleteInternal is like Delete but without persistence logging (used for recovery)
func (s *BasicStore) deleteInternal(key string) error {
	item, allocPtr, existed := s.data.Delete(key)
	if !existed {
		return fmt.Errorf("key not found: %s", key)
	}

	if allocPtr != nil {
		_ = s.memPool.Free(allocPtr)
	}

	entry := s.itemToEntry(key, item)
	s.evictPolicy.OnDelete(entry)

	s.updateStats(func() {
		s.stats.TotalItems--
		s.stats.TotalMemory -= item.Size
	})

	if s.filter != nil {
		s.filter.Delete([]byte(key))
	}

	return nil
}

// clearInternal is like Clear but without persistence logging (used for recovery)
func (s *BasicStore) clearInternal() {
	s.data.RangeAll(func(key string, item *CacheItem) bool {
		if ptr, ok := s.data.GetAllocatedPtr(key); ok {
			_ = s.memPool.Free(ptr)
		}
		return true
	})

	s.data.Clear()

	s.mutex.Lock()
	s.stats.TotalItems = 0
	s.stats.TotalMemory = 0
	s.mutex.Unlock()

	s.evictPolicy = cache.NewSessionEvictionPolicy()
}
