package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"hypercache/internal/persistence"
)

func TestBasicStore_PersistenceIntegration(t *testing.T) {
	// Create temp directory for testing
	tempDir := "/tmp/hypercache-integration-test"
	defer os.RemoveAll(tempDir)

	// Create persistence config
	persistConfig := persistence.DefaultPersistenceConfig()
	persistConfig.Enabled = true
	persistConfig.EnableAOF = true
	persistConfig.DataDirectory = tempDir

	// Create store config with persistence
	config := BasicStoreConfig{
		Name:              "test-store-with-persistence",
		MaxMemory:         1024 * 1024, // 1MB
		DefaultTTL:        time.Minute,
		CleanupInterval:   time.Second,
		PersistenceConfig: &persistConfig,
	}

	// Create store
	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()

	// Start persistence
	if err := store.StartPersistence(ctx); err != nil {
		t.Fatalf("Failed to start persistence: %v", err)
	}

	// Add some test data
	testData := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	for key, value := range testData {
		if err := store.Set(key, value, "session1", 0); err != nil {
			t.Fatalf("Failed to set %s: %v", key, err)
		}
	}

	// Verify data is in cache
	for key, expectedValue := range testData {
		value, err := store.Get(key)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Expected %s, got %s", expectedValue, value)
		}
	}

	// Stop persistence to flush data
	if err := store.StopPersistence(); err != nil {
		t.Fatalf("Failed to stop persistence: %v", err)
	}

	// Create a new store instance to test recovery
	store2, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}

	// Start persistence on new store (should recover data)
	if err := store2.StartPersistence(ctx); err != nil {
		t.Fatalf("Failed to start persistence on second store: %v", err)
	}

	// Verify recovered data
	for key, expectedValue := range testData {
		value, err := store2.Get(key)
		if err != nil {
			t.Fatalf("Failed to get %s from recovered store: %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Recovery failed: expected %s, got %s", expectedValue, value)
		}
	}

	// Test snapshot functionality
	if err := store2.CreateSnapshot(); err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Check persistence stats
	stats := store2.GetPersistenceStats()
	if stats == nil {
		t.Error("Expected persistence stats, got nil")
	}

	// Clean up
	if err := store2.StopPersistence(); err != nil {
		t.Fatalf("Failed to stop second store persistence: %v", err)
	}

	t.Logf("Persistence integration test completed successfully")
}

func TestBasicStore_PersistenceDisabled(t *testing.T) {
	// Create store config without persistence
	config := BasicStoreConfig{
		Name:              "test-store-no-persistence",
		MaxMemory:         1024 * 1024,
		DefaultTTL:        time.Minute,
		CleanupInterval:   time.Second,
		PersistenceConfig: nil, // No persistence
	}

	store, err := NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()

	// Start persistence should be a no-op
	if err := store.StartPersistence(ctx); err != nil {
		t.Fatalf("StartPersistence should not fail when disabled: %v", err)
	}

	// Set some data
	if err := store.Set("key1", "value1", "session1", 0); err != nil {
		t.Fatalf("Failed to set data: %v", err)
	}

	// Stop persistence should be a no-op
	if err := store.StopPersistence(); err != nil {
		t.Fatalf("StopPersistence should not fail when disabled: %v", err)
	}

	// Persistence stats should be nil
	stats := store.GetPersistenceStats()
	if stats != nil {
		t.Error("Expected nil persistence stats when disabled")
	}

	// Snapshot should fail
	if err := store.CreateSnapshot(); err == nil {
		t.Error("Expected CreateSnapshot to fail when persistence disabled")
	}

	t.Logf("Persistence disabled test completed successfully")
}
