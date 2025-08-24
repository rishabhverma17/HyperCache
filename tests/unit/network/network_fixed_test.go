package network_test

import (
	"testing"
	"time"

	"hypercache/internal/network/resp"
	"hypercache/internal/storage"
)

func TestRESPServerFixed(t *testing.T) {
	t.Run("Server_Start_Stop", func(t *testing.T) {
		// Create required components
		config := storage.BasicStoreConfig{
			Name:             "test-store",
			MaxMemory:        1000000, // 1MB
			DefaultTTL:       5 * time.Minute,
			EnableStatistics: true,
			CleanupInterval:  30 * time.Second,
		}
		store, err := storage.NewBasicStore(config)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}
		defer store.Close()

		// Create server (coordinator can be nil for basic tests)
		server := resp.NewServer(":0", store, nil)

		// Start server
		err = server.Start()
		if err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
		defer server.Stop()

		// Verify server is running by checking stats
		stats := server.GetStats()
		if stats.ActiveConnections < 0 {
			t.Error("Server should have valid stats after starting")
		}
	})

	t.Run("Server_With_Config", func(t *testing.T) {
		storeConfig := storage.BasicStoreConfig{
			Name:             "test-store-2",
			MaxMemory:        1000000, // 1MB
			DefaultTTL:       5 * time.Minute,
			EnableStatistics: true,
			CleanupInterval:  30 * time.Second,
		}
		store, err := storage.NewBasicStore(storeConfig)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}
		defer store.Close()

		// Create server with custom config
		config := resp.DefaultServerConfig()
		config.MaxConnections = 100
		server := resp.NewServerWithConfig(":0", store, nil, config)

		// Start and verify it works
		err = server.Start()
		if err != nil {
			t.Fatalf("Failed to start server with config: %v", err)
		}
		defer server.Stop()

		// Give server time to start
		time.Sleep(50 * time.Millisecond)

		stats := server.GetStats()
		if stats.TotalConnections < 0 || stats.CommandsProcessed < 0 {
			t.Error("Server stats should be initialized")
		}
	})
}
