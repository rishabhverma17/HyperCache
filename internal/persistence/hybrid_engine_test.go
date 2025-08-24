package persistence

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHybridEngine_BasicOperations(t *testing.T) {
	// Create temp directory for testing
	tempDir := "/tmp/hypercache-test"
	defer os.RemoveAll(tempDir)

	config := DefaultPersistenceConfig()
	config.Enabled = true
	config.EnableAOF = true
	config.DataDirectory = tempDir
	config.SnapshotInterval = 0 // Disable automatic snapshots for testing

	engine := NewHybridEngine(config)

	ctx := context.Background()

	// Test Start
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// Test WriteEntry
	entry := &LogEntry{
		Timestamp: time.Now(),
		Operation: "SET",
		Key:       "test_key",
		Value:     []byte("test_value"),
		TTL:       300,
		SessionID: "session_123",
	}

	if err := engine.WriteEntry(entry); err != nil {
		t.Fatalf("Failed to write entry: %v", err)
	}

	// Test another entry
	delEntry := &LogEntry{
		Timestamp: time.Now(),
		Operation: "DEL",
		Key:       "old_key",
	}

	if err := engine.WriteEntry(delEntry); err != nil {
		t.Fatalf("Failed to write delete entry: %v", err)
	}

	// Test Stop
	if err := engine.Stop(); err != nil {
		t.Fatalf("Failed to stop engine: %v", err)
	}

	// Create new engine to test recovery
	engine2 := NewHybridEngine(config)
	if err := engine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine2: %v", err)
	}

	// Test ReadEntries (recovery)
	entries, err := engine2.ReadEntries()
	if err != nil {
		t.Fatalf("Failed to read entries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Verify the entries
	if entries[0].Key != "test_key" || entries[0].Operation != "SET" {
		t.Errorf("First entry mismatch: %+v", entries[0])
	}

	if entries[1].Key != "old_key" || entries[1].Operation != "DEL" {
		t.Errorf("Second entry mismatch: %+v", entries[1])
	}

	// Test stats
	stats := engine2.GetStats()
	if stats.EntriesRecovered != 2 {
		t.Errorf("Expected 2 recovered entries, got %d", stats.EntriesRecovered)
	}

	// Stop engine2
	if err := engine2.Stop(); err != nil {
		t.Fatalf("Failed to stop engine2: %v", err)
	}
}

func TestHybridEngine_Snapshot(t *testing.T) {
	tempDir := "/tmp/hypercache-snapshot-test"
	defer os.RemoveAll(tempDir)

	config := DefaultPersistenceConfig()
	config.Enabled = true
	config.DataDirectory = tempDir

	engine := NewHybridEngine(config)

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// Test data
	testData := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": map[string]interface{}{"nested": true},
	}

	// Create snapshot
	if err := engine.CreateSnapshot(testData); err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Load snapshot
	loadedData, err := engine.LoadSnapshot()
	if err != nil {
		t.Fatalf("Failed to load snapshot: %v", err)
	}

	if len(loadedData) != len(testData) {
		t.Errorf("Expected %d items in snapshot, got %d", len(testData), len(loadedData))
	}

	// Verify at least one key exists (actual conversion might differ)
	if _, exists := loadedData["key1"]; !exists {
		t.Error("Expected key1 to exist in loaded snapshot")
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Failed to stop engine: %v", err)
	}
}

func TestHybridEngine_Disabled(t *testing.T) {
	config := DefaultPersistenceConfig()
	config.Enabled = false // Disabled

	engine := NewHybridEngine(config)

	ctx := context.Background()

	// Should work without errors when disabled
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start disabled engine: %v", err)
	}

	entry := &LogEntry{
		Operation: "SET",
		Key:       "test",
		Value:     []byte("value"),
	}

	// Should be no-op when disabled
	if err := engine.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed for disabled engine: %v", err)
	}

	// Should return empty when disabled
	entries, err := engine.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries failed for disabled engine: %v", err)
	}

	if entries != nil {
		t.Error("Expected nil entries for disabled engine")
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Failed to stop disabled engine: %v", err)
	}
}
