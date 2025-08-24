package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBasicStore_NewBasicStore(t *testing.T) {
	tests := []struct {
		name    string
		config  BasicStoreConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid configuration",
			config: BasicStoreConfig{
				Name:             "test-store",
				MaxMemory:        1024 * 1024, // 1MB
				DefaultTTL:       5 * time.Minute,
				EnableStatistics: true,
				CleanupInterval:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "Empty store name",
			config: BasicStoreConfig{
				Name:      "",
				MaxMemory: 1024 * 1024,
			},
			wantErr: true,
			errMsg:  "store name cannot be empty",
		},
		{
			name: "Zero max memory",
			config: BasicStoreConfig{
				Name:      "test-store",
				MaxMemory: 0,
			},
			wantErr: true,
			errMsg:  "max memory must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewBasicStore(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewBasicStore() expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewBasicStore() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("NewBasicStore() unexpected error = %v", err)
				return
			}

			if store == nil {
				t.Errorf("NewBasicStore() returned nil store")
				return
			}

			// Verify initialization
			if store.config.Name != tt.config.Name {
				t.Errorf("Store name = %v, want %v", store.config.Name, tt.config.Name)
			}

			if store.Size() != 0 {
				t.Errorf("New store size = %v, want 0", store.Size())
			}

			if store.Memory() != 0 {
				t.Errorf("New store memory = %v, want 0", store.Memory())
			}

			// Clean up
			store.Close()
		})
	}
}

func TestBasicStore_SetAndGet(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:             "test-store",
		MaxMemory:        1024 * 1024, // 1MB
		EnableStatistics: true,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name      string
		key       string
		value     interface{}
		sessionID string
		ttl       time.Duration
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "String value",
			key:       "key1",
			value:     "value1",
			sessionID: "session1",
			ttl:       0,
			wantErr:   false,
		},
		{
			name:      "Integer value",
			key:       "key2",
			value:     42,
			sessionID: "session1",
			ttl:       0,
			wantErr:   false,
		},
		{
			name:      "Byte slice value",
			key:       "key3",
			value:     []byte("binary data"),
			sessionID: "session2",
			ttl:       5 * time.Minute,
			wantErr:   false,
		},
		{
			name:      "Empty key",
			key:       "",
			value:     "value",
			sessionID: "session1",
			ttl:       0,
			wantErr:   true,
			errMsg:    "key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Set
			err := store.Set(tt.key, tt.value, tt.sessionID, tt.ttl)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Set() expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Set() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("Set() unexpected error = %v", err)
				return
			}

			// Test Get
			value, err := store.Get(tt.key)
			if err != nil {
				t.Errorf("Get() unexpected error = %v", err)
				return
			}

			// Check if values match (handle byte slices specially)
			valuesMatch := false
			if vBytes, ok := value.([]byte); ok {
				if ttBytes, ok := tt.value.([]byte); ok {
					valuesMatch = string(vBytes) == string(ttBytes)
				}
			} else {
				valuesMatch = value == tt.value
			}

			if !valuesMatch {
				t.Errorf("Get() value = %v, want %v", value, tt.value)
			}
		})
	}

	// Verify store size and memory
	if store.Size() != 3 { // 3 successful sets
		t.Errorf("Store size = %v, want 3", store.Size())
	}

	if store.Memory() == 0 {
		t.Errorf("Store memory should be > 0")
	}
}

func TestBasicStore_GetNonExistent(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test getting non-existent key
	value, err := store.Get("non-existent")
	if err == nil {
		t.Errorf("Get() expected error for non-existent key")
	}

	if value != nil {
		t.Errorf("Get() value = %v, want nil", value)
	}

	// Check statistics
	stats := store.Stats()
	if stats.MissCount != 1 {
		t.Errorf("Miss count = %v, want 1", stats.MissCount)
	}
}

func TestBasicStore_TTLExpiration(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set item with short TTL
	err = store.Set("expire-key", "expire-value", "session1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get immediately - should work
	value, err := store.Get("expire-key")
	if err != nil {
		t.Errorf("Get() before expiration error = %v", err)
	}
	if value != "expire-value" {
		t.Errorf("Get() value = %v, want %v", value, "expire-value")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Get after expiration - should fail
	value, err = store.Get("expire-key")
	if err == nil {
		t.Errorf("Get() after expiration should have failed")
	}
	if value != nil {
		t.Errorf("Get() after expiration value = %v, want nil", value)
	}
}

func TestBasicStore_Delete(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add an item
	err = store.Set("delete-key", "delete-value", "session1", 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify it exists
	value, err := store.Get("delete-key")
	if err != nil {
		t.Errorf("Get() before delete error = %v", err)
	}
	if value != "delete-value" {
		t.Errorf("Get() value = %v, want %v", value, "delete-value")
	}

	// Delete the item
	err = store.Delete("delete-key")
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify it's gone
	value, err = store.Get("delete-key")
	if err == nil {
		t.Errorf("Get() after delete should have failed")
	}
	if value != nil {
		t.Errorf("Get() after delete value = %v, want nil", value)
	}

	// Test deleting non-existent key
	err = store.Delete("non-existent")
	if err == nil {
		t.Errorf("Delete() non-existent key should have failed")
	}

	// Test empty key
	err = store.Delete("")
	if err == nil {
		t.Errorf("Delete() empty key should have failed")
	}
}

func TestBasicStore_Clear(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add multiple items
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		err = store.Set(key, value, "session1", 0)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Verify items exist
	if store.Size() != 10 {
		t.Errorf("Store size = %v, want 10", store.Size())
	}

	// Clear the store
	err = store.Clear()
	if err != nil {
		t.Errorf("Clear() error = %v", err)
	}

	// Verify store is empty
	if store.Size() != 0 {
		t.Errorf("Store size after clear = %v, want 0", store.Size())
	}

	if store.Memory() != 0 {
		t.Errorf("Store memory after clear = %v, want 0", store.Memory())
	}
}

func TestBasicStore_UpdateExistingKey(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set initial value
	err = store.Set("update-key", "initial-value", "session1", 0)
	if err != nil {
		t.Fatalf("Set() initial error = %v", err)
	}

	// Update value
	err = store.Set("update-key", "updated-value", "session1", 0)
	if err != nil {
		t.Errorf("Set() update error = %v", err)
	}

	// Verify updated value
	value, err := store.Get("update-key")
	if err != nil {
		t.Errorf("Get() after update error = %v", err)
	}
	if value != "updated-value" {
		t.Errorf("Get() after update value = %v, want %v", value, "updated-value")
	}

	// Store size should still be 1
	if store.Size() != 1 {
		t.Errorf("Store size after update = %v, want 1", store.Size())
	}
}

func TestBasicStore_ConcurrentOperations(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "test-store",
		MaxMemory: 10 * 1024 * 1024, // 10MB
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	const numGoroutines = 100
	const operationsPerGoroutine = 10

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*operationsPerGoroutine)

	// Concurrent Set operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", goroutineID, j)
				value := fmt.Sprintf("value-%d-%d", goroutineID, j)
				sessionID := fmt.Sprintf("session-%d", goroutineID)

				if err := store.Set(key, value, sessionID, 0); err != nil {
					errorChan <- fmt.Errorf("Set error in goroutine %d: %w", goroutineID, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Errorf("Concurrent Set error: %v", err)
	}

	// Verify all items were set
	expectedSize := uint64(numGoroutines * operationsPerGoroutine)
	if store.Size() != expectedSize {
		t.Errorf("Store size after concurrent sets = %v, want %v", store.Size(), expectedSize)
	}

	// Concurrent Get operations
	errorChan = make(chan error, numGoroutines*operationsPerGoroutine)
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", goroutineID, j)
				expectedValue := fmt.Sprintf("value-%d-%d", goroutineID, j)

				value, err := store.Get(key)
				if err != nil {
					errorChan <- fmt.Errorf("Get error in goroutine %d: %w", goroutineID, err)
				} else if value != expectedValue {
					errorChan <- fmt.Errorf("Get value mismatch in goroutine %d: got %v, want %v", goroutineID, value, expectedValue)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Errorf("Concurrent Get error: %v", err)
	}
}

func TestBasicStore_MemoryPressureHandling(t *testing.T) {
	// Create store with small memory limit
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "memory-pressure-test",
		MaxMemory: 1024, // 1KB - very small to trigger pressure
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Fill up the store until we hit memory pressure
	largeValue := string(make([]byte, 200)) // 200 bytes per value
	itemsAdded := 0

	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("large-key-%d", i)
		err := store.Set(key, largeValue, "session1", 0)
		if err != nil {
			// Expected - memory pressure should kick in
			t.Logf("Memory pressure hit at item %d: %v", i, err)
			break
		}
		itemsAdded++
		t.Logf("Added item %d, current store size: %d, memory: %d", i, store.Size(), store.Memory())
	}

	// We should have added some items before hitting memory limit
	if itemsAdded == 0 {
		t.Errorf("Should have been able to add at least some items before memory pressure")
	}

	// The store should have automatically evicted items to manage memory
	finalSize := store.Size()
	t.Logf("Added %d items, final store size: %d", itemsAdded, finalSize)

	// Check for integer underflow issues
	if finalSize > 1000000 { // Suspiciously large number indicates underflow
		t.Errorf("Detected integer underflow in size counter: %d", finalSize)
	}
}

func TestBasicStore_Statistics(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:             "stats-test",
		MaxMemory:        1024 * 1024,
		EnableStatistics: true,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test initial stats
	stats := store.Stats()
	if stats.TotalItems != 0 {
		t.Errorf("Initial total items = %v, want 0", stats.TotalItems)
	}
	if stats.HitCount != 0 {
		t.Errorf("Initial hit count = %v, want 0", stats.HitCount)
	}
	if stats.MissCount != 0 {
		t.Errorf("Initial miss count = %v, want 0", stats.MissCount)
	}

	// Add an item
	err = store.Set("stats-key", "stats-value", "session1", 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the item (hit)
	_, err = store.Get("stats-key")
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}

	// Try to get non-existent item (miss)
	_, err = store.Get("non-existent")
	if err == nil {
		t.Errorf("Get() non-existent should have failed")
	}

	// Check updated stats
	stats = store.Stats()
	if stats.TotalItems != 1 {
		t.Errorf("Total items = %v, want 1", stats.TotalItems)
	}
	if stats.HitCount != 1 {
		t.Errorf("Hit count = %v, want 1", stats.HitCount)
	}
	if stats.MissCount != 1 {
		t.Errorf("Miss count = %v, want 1", stats.MissCount)
	}

	// Test hit rate calculation
	expectedHitRate := 50.0 // 1 hit out of 2 total attempts
	if stats.HitRate() != expectedHitRate {
		t.Errorf("Hit rate = %v, want %v", stats.HitRate(), expectedHitRate)
	}
}

func TestBasicStore_CleanupExpiredItems(t *testing.T) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:            "cleanup-test",
		MaxMemory:       1024 * 1024,
		CleanupInterval: 50 * time.Millisecond, // Fast cleanup
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add items with short TTL
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("expire-key-%d", i)
		value := fmt.Sprintf("expire-value-%d", i)
		err = store.Set(key, value, "session1", 75*time.Millisecond)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Verify items exist
	if store.Size() != 5 {
		t.Errorf("Store size = %v, want 5", store.Size())
	}

	// Wait for cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Items should be cleaned up
	if store.Size() != 0 {
		t.Errorf("Store size after cleanup = %v, want 0", store.Size())
	}
}

// Benchmark tests
func BenchmarkBasicStore_Set(b *testing.B) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "benchmark-store",
		MaxMemory: 100 * 1024 * 1024, // 100MB
	})
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		store.Set(key, value, "session1", 0)
	}
}

func BenchmarkBasicStore_Get(b *testing.B) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "benchmark-store",
		MaxMemory: 100 * 1024 * 1024, // 100MB
	})
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate store
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		store.Set(key, value, "session1", 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		store.Get(key)
	}
}

func BenchmarkBasicStore_ConcurrentSetGet(b *testing.B) {
	store, err := NewBasicStore(BasicStoreConfig{
		Name:      "benchmark-store",
		MaxMemory: 100 * 1024 * 1024, // 100MB
	})
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				key := fmt.Sprintf("key-%d", i)
				value := fmt.Sprintf("value-%d", i)
				store.Set(key, value, "session1", 0)
			} else {
				key := fmt.Sprintf("key-%d", i-1)
				store.Get(key)
			}
			i++
		}
	})
}
