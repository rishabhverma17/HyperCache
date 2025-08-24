//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rishabhverma17/hypercache/internal/persistence"
	"github.com/rishabhverma17/hypercache/internal/storage"
	"github.com/rishabhverma17/hypercache/pkg/config"
)

func main() {
	fmt.Println("🔄 HyperCache Persistence Demo")
	fmt.Println("==============================")

	// Create a temporary directory for demo
	tempDir := filepath.Join(os.TempDir(), "hypercache-persistence-demo")
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	// Create basic configuration
	cfg := &config.Config{
		Persistence: config.PersistenceConfig{
			Enabled:     true,
			Directory:   tempDir,
			AOFEnabled:  true,
			AOFSyncMode: "everysec",
			SnapshotConfig: config.SnapshotConfig{
				Enabled:        true,
				IntervalMinutes: 1,
			},
		},
	}

	// Initialize storage
	store := storage.NewBasicStore()

	// Initialize persistence engine
	engine, err := persistence.NewHybridEngine(cfg, store)
	if err != nil {
		log.Fatalf("Failed to create persistence engine: %v", err)
	}

	fmt.Println("✅ Persistence engine initialized")

	// Demo operations
	demoOperations(store, engine)

	// Close engine
	engine.Close()
	fmt.Println("✅ Demo completed successfully!")
}

func demoOperations(store storage.Store, engine persistence.Engine) {
	fmt.Println("\n📝 Performing demo operations...")

	// Set some key-value pairs
	testData := map[string]interface{}{
		"user:1":    "John Doe",
		"user:2":    "Jane Smith",
		"counter:1": 42,
		"config:db": "redis://localhost:6379",
	}

	for key, value := range testData {
		store.Set(key, value, time.Hour)
		fmt.Printf("SET %s = %v\n", key, value)
	}

	fmt.Println("\n📖 Reading back values...")
	for key := range testData {
		value, exists := store.Get(key)
		if exists {
			fmt.Printf("GET %s = %v\n", key, value)
		} else {
			fmt.Printf("GET %s = <not found>\n", key)
		}
	}

	fmt.Println("\n💾 Persistence files should be created in:", engine.GetDataDirectory())
}
