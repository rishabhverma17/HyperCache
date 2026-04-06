package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hypercache/internal/filter"
	"hypercache/internal/logging"
	"hypercache/internal/persistence"
	"hypercache/pkg/config"
)

// StoreManager manages multiple named BasicStore instances.
// It handles creation, lookup, deletion, and persistence of store metadata
// (stores.json) so that runtime-created stores survive restarts.
type StoreManager struct {
	stores    map[string]*BasicStore
	mu        sync.RWMutex
	dataDir   string
	maxStores int

	// Global config used as defaults for new stores
	globalPersistence config.PersistenceConfig
	globalCacheConfig config.CacheConfig
}

// StoreManagerConfig holds configuration for the StoreManager.
type StoreManagerConfig struct {
	DataDir           string
	MaxStores         int
	GlobalPersistence config.PersistenceConfig
	GlobalCacheConfig config.CacheConfig
}

// storeRegistryEntry is persisted to stores.json for runtime-created stores.
type storeRegistryEntry struct {
	Name           string `json:"name"`
	EvictionPolicy string `json:"eviction_policy"`
	MaxMemory      string `json:"max_memory"`
	DefaultTTL     string `json:"default_ttl"`
	CuckooFilter   bool   `json:"cuckoo_filter"`
	Persistence    string `json:"persistence"`
	CreatedAt      string `json:"created_at"`
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(cfg StoreManagerConfig) *StoreManager {
	return &StoreManager{
		stores:            make(map[string]*BasicStore),
		dataDir:           cfg.DataDir,
		maxStores:         cfg.MaxStores,
		globalPersistence: cfg.GlobalPersistence,
		globalCacheConfig: cfg.GlobalCacheConfig,
	}
}

// CreateStore creates a new named store with the given config.
// Returns error if the store already exists or max stores is reached.
// Store config is immutable — to change settings, drop and recreate.
func (sm *StoreManager) CreateStore(storeCfg config.StoreConfig, ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.stores[storeCfg.Name]; exists {
		return fmt.Errorf("store '%s' already exists", storeCfg.Name)
	}

	if len(sm.stores) >= sm.maxStores {
		return fmt.Errorf("maximum stores (%d) reached", sm.maxStores)
	}

	store, err := sm.createStoreInternal(storeCfg)
	if err != nil {
		return fmt.Errorf("failed to create store '%s': %w", storeCfg.Name, err)
	}

	// Start persistence
	if err := store.StartPersistence(ctx); err != nil {
		logging.Warn(nil, logging.ComponentPersistence, logging.ActionStart,
			"Failed to start persistence for store", map[string]interface{}{
				"store": storeCfg.Name,
				"error": err.Error(),
			})
	}

	sm.stores[storeCfg.Name] = store

	logging.Info(nil, logging.ComponentStorage, logging.ActionStart, "Store created", map[string]interface{}{
		"store":           storeCfg.Name,
		"eviction_policy": storeCfg.EvictionPolicy,
		"max_memory":      storeCfg.MaxMemory,
		"persistence":     storeCfg.GetPersistence(sm.globalPersistence.Strategy),
		"cuckoo_filter":   storeCfg.IsCuckooFilterEnabled(),
	})

	return nil
}

// GetStore returns a store by name. Returns nil if not found.
func (sm *StoreManager) GetStore(name string) *BasicStore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.stores[name]
}

// GetDefaultStore returns the "default" store. Panics if it doesn't exist.
func (sm *StoreManager) GetDefaultStore() *BasicStore {
	store := sm.GetStore("default")
	if store == nil {
		panic("default store does not exist — this is a bug")
	}
	return store
}

// ListStores returns the names of all stores.
func (sm *StoreManager) ListStores() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	names := make([]string, 0, len(sm.stores))
	for name := range sm.stores {
		names = append(names, name)
	}
	return names
}

// DropStore removes a store by name. Cannot drop "default".
func (sm *StoreManager) DropStore(name string) error {
	if name == "default" {
		return fmt.Errorf("cannot drop the default store")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	store, exists := sm.stores[name]
	if !exists {
		return fmt.Errorf("store '%s' not found", name)
	}

	store.Close()
	delete(sm.stores, name)

	// Remove persisted data directory
	storeDir := filepath.Join(sm.dataDir, "stores", name)
	_ = os.RemoveAll(storeDir)

	// Update registry
	sm.saveRegistryLocked()

	logging.Info(nil, logging.ComponentStorage, logging.ActionStop, "Store dropped", map[string]interface{}{
		"store": name,
	})

	return nil
}

// GetStoreConfig returns the effective config for a store (for API responses).
func (sm *StoreManager) GetStoreConfig(name string) (*config.StoreConfig, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	store, exists := sm.stores[name]
	if !exists {
		return nil, fmt.Errorf("store '%s' not found", name)
	}

	cfg := &config.StoreConfig{
		Name:           store.config.Name,
		EvictionPolicy: "lru", // BasicStoreConfig doesn't store this, but we can infer
		MaxMemory:      fmt.Sprintf("%dB", store.config.MaxMemory),
	}
	return cfg, nil
}

// StoreCount returns the number of stores.
func (sm *StoreManager) StoreCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.stores)
}

// Close shuts down all stores gracefully.
func (sm *StoreManager) Close() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for name, store := range sm.stores {
		store.Close()
		logging.Info(nil, logging.ComponentStorage, logging.ActionStop, "Store closed", map[string]interface{}{
			"store": name,
		})
	}
}

// LoadRegistry loads runtime-created stores from stores.json.
// Called at startup after config-defined stores are created.
func (sm *StoreManager) LoadRegistry(ctx context.Context) error {
	registryPath := filepath.Join(sm.dataDir, "stores.json")

	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No registry file — first run
		}
		return fmt.Errorf("failed to read store registry: %w", err)
	}

	var entries map[string]storeRegistryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore,
			"Corrupt stores.json, skipping", map[string]interface{}{"error": err.Error()})
		return nil
	}

	for _, entry := range entries {
		// Skip stores already created from config
		if sm.GetStore(entry.Name) != nil {
			continue
		}

		cuckoo := entry.CuckooFilter
		storeCfg := config.StoreConfig{
			Name:           entry.Name,
			EvictionPolicy: entry.EvictionPolicy,
			MaxMemory:      entry.MaxMemory,
			DefaultTTL:     entry.DefaultTTL,
			CuckooFilter:   &cuckoo,
			Persistence:    entry.Persistence,
		}

		if err := sm.CreateStore(storeCfg, ctx); err != nil {
			logging.Warn(nil, logging.ComponentStorage, logging.ActionRestore,
				"Failed to restore store from registry", map[string]interface{}{
					"store": entry.Name,
					"error": err.Error(),
				})
		}
	}

	return nil
}

// SaveRegistry persists the current store metadata to stores.json.
func (sm *StoreManager) SaveRegistry() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sm.saveRegistryLocked()
}

func (sm *StoreManager) saveRegistryLocked() {
	entries := make(map[string]storeRegistryEntry)

	for name, store := range sm.stores {
		entries[name] = storeRegistryEntry{
			Name:           name,
			EvictionPolicy: "lru", // default; in future, store eviction policy name in BasicStoreConfig
			MaxMemory:      fmt.Sprintf("%dB", store.config.MaxMemory),
			DefaultTTL:     store.config.DefaultTTL.String(),
			CuckooFilter:   store.filter != nil,
			Persistence:    sm.getPersistenceMode(store),
			CreatedAt:      store.stats.CreatedAt.Format(time.RFC3339),
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		logging.Warn(nil, logging.ComponentStorage, logging.ActionSnapshot,
			"Failed to marshal store registry", map[string]interface{}{"error": err.Error()})
		return
	}

	registryPath := filepath.Join(sm.dataDir, "stores.json")
	if err := os.MkdirAll(sm.dataDir, 0755); err != nil {
		logging.Warn(nil, logging.ComponentStorage, logging.ActionSnapshot,
			"Failed to create data directory for registry", map[string]interface{}{"error": err.Error()})
		return
	}

	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		logging.Warn(nil, logging.ComponentStorage, logging.ActionSnapshot,
			"Failed to write store registry", map[string]interface{}{"error": err.Error()})
	}
}

func (sm *StoreManager) getPersistenceMode(store *BasicStore) string {
	if store.persistEngine == nil {
		return "disabled"
	}
	return sm.globalPersistence.Strategy
}

// createStoreInternal creates a BasicStore from a StoreConfig. Caller must hold sm.mu.
func (sm *StoreManager) createStoreInternal(storeCfg config.StoreConfig) (*BasicStore, error) {
	maxMemory := parseMemorySize(storeCfg.MaxMemory)
	if maxMemory == 0 {
		maxMemory = 8 * 1024 * 1024 * 1024 // 8GB fallback
	}

	defaultTTL := parseTTL(storeCfg.DefaultTTL)

	// Per-store data directory
	storeDataDir := filepath.Join(sm.dataDir, "stores", storeCfg.Name)

	// Build persistence config for this store
	var persistCfg *persistence.PersistenceConfig
	effectivePersistence := storeCfg.GetPersistence(sm.globalPersistence.Strategy)
	if effectivePersistence != "disabled" && sm.globalPersistence.Enabled {
		persistCfg = &persistence.PersistenceConfig{
			Enabled:          true,
			Strategy:         effectivePersistence,
			DataDirectory:    storeDataDir,
			EnableAOF:        effectivePersistence == "aof" || effectivePersistence == "hybrid",
			SyncPolicy:       sm.globalPersistence.SyncPolicy,
			SyncInterval:     sm.globalPersistence.SyncInterval,
			SnapshotInterval: sm.globalPersistence.SnapshotInterval,
			MaxLogSize:       int64(parseMemorySize(sm.globalPersistence.MaxLogSize)),
			CompressionLevel: sm.globalPersistence.CompressionLevel,
			RetainLogs:       sm.globalPersistence.RetainLogs,
		}
	}

	// Build filter config
	var filterCfg *filter.FilterConfig
	if storeCfg.IsCuckooFilterEnabled() {
		filterCfg = &filter.FilterConfig{
			Name:              storeCfg.Name,
			FilterType:        "cuckoo",
			ExpectedItems:     1000000,
			FalsePositiveRate: sm.globalCacheConfig.CuckooFilterFPP,
			FingerprintSize:   12,
			BucketSize:        4,
			EnableAutoResize:  true,
			EnableStatistics:  true,
			HashFunction:      "xxhash",
		}
	}

	bsCfg := BasicStoreConfig{
		Name:              storeCfg.Name,
		MaxMemory:         maxMemory,
		DefaultTTL:        defaultTTL,
		EnableStatistics:  true,
		CleanupInterval:   time.Minute,
		PersistenceConfig: persistCfg,
		FilterConfig:      filterCfg,
	}

	return NewBasicStore(bsCfg)
}

// parseMemorySize parses a size string like "4GB", "512MB", "100" into uint64 bytes.
func parseMemorySize(s string) uint64 {
	if s == "" || s == "0" {
		return 0
	}

	multipliers := map[string]uint64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	var size uint64
	var unit string
	n, err := fmt.Sscanf(s, "%d%s", &size, &unit)
	if err != nil || n == 0 {
		return 0
	}
	if n == 1 {
		return size // raw bytes
	}

	if m, ok := multipliers[unit]; ok {
		return size * m
	}
	return size
}

// parseTTL parses a duration string or "0" into time.Duration.
// "0" and "" both mean no TTL (infinite).
func parseTTL(s string) time.Duration {
	if s == "" || s == "0" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
