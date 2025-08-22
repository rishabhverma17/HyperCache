package persistence

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// PersistenceEngine handles data persistence operations
type PersistenceEngine interface {
	// Core operations
	WriteEntry(entry *LogEntry) error
	ReadEntries() ([]*LogEntry, error)
	CreateSnapshot(data map[string]interface{}) error
	LoadSnapshot() (map[string]interface{}, error)
	
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
	
	// Maintenance
	Compact() error
	GetStats() *PersistenceStats
}

// PersistenceConfig defines persistence behavior
type PersistenceConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	Strategy          string        `yaml:"strategy" json:"strategy"`                   // "aof", "snapshot", "hybrid"
	DataDirectory     string        `yaml:"data_directory" json:"data_directory"`
	EnableAOF         bool          `yaml:"enable_aof" json:"enable_aof"`
	SyncPolicy        string        `yaml:"sync_policy" json:"sync_policy"`             // "always", "everysec", "no"
	SyncInterval      time.Duration `yaml:"sync_interval" json:"sync_interval"`
	SnapshotInterval  time.Duration `yaml:"snapshot_interval" json:"snapshot_interval"`
	MaxLogSize        int64         `yaml:"max_log_size" json:"max_log_size"`           // Bytes
	CompressionLevel  int           `yaml:"compression_level" json:"compression_level"` // 0-9
	RetainLogs        int           `yaml:"retain_logs" json:"retain_logs"`             // Number of old logs to keep
}

// DefaultPersistenceConfig returns production-ready defaults
func DefaultPersistenceConfig() PersistenceConfig {
	return PersistenceConfig{
		Enabled:           false, // Opt-in for safety
		Strategy:          "hybrid",
		DataDirectory:     "./data",
		EnableAOF:         true,
		SyncPolicy:        "everysec",
		SyncInterval:      time.Second,
		SnapshotInterval:  15 * time.Minute,
		MaxLogSize:        100 * 1024 * 1024, // 100MB
		CompressionLevel:  1,                  // Light compression
		RetainLogs:        3,
	}
}

// PersistenceStats provides metrics about persistence operations
type PersistenceStats struct {
	// File stats
	AOFSize          int64     `json:"aof_size"`
	SnapshotSize     int64     `json:"snapshot_size"`
	LastSnapshot     time.Time `json:"last_snapshot"`
	LastSync         time.Time `json:"last_sync"`
	
	// Operation stats  
	EntriesWritten   int64     `json:"entries_written"`
	EntriesRecovered int64     `json:"entries_recovered"`
	SyncOperations   int64     `json:"sync_operations"`
	CompactionRuns   int64     `json:"compaction_runs"`
	
	// Performance stats
	WriteLatency     time.Duration `json:"write_latency"`
	RecoveryTime     time.Duration `json:"recovery_time"`
	CompactionTime   time.Duration `json:"compaction_time"`
	
	// Error tracking
	WriteErrors      int64     `json:"write_errors"`
	ReadErrors       int64     `json:"read_errors"`
}

// HybridEngine implements both AOF and snapshot persistence strategies
type HybridEngine struct {
	config  PersistenceConfig
	running bool
	
	// Managers
	aofManager      *AOFManager
	snapshotManager *SnapshotManager
	
	// Background workers
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	
	// Statistics
	stats      PersistenceStats
	statsMutex sync.RWMutex
}

// NewHybridEngine creates a new hybrid persistence engine
func NewHybridEngine(config PersistenceConfig) *HybridEngine {
	engine := &HybridEngine{
		config: config,
		stats:  PersistenceStats{}, // Initialize with zero values
	}
	
	// Initialize components
	engine.aofManager = NewAOFManager(config)
	engine.snapshotManager = NewSnapshotManager(config)
	
	return engine
}

// Start initializes the persistence engine and starts background workers
func (he *HybridEngine) Start(ctx context.Context) error {
	he.mu.Lock()
	defer he.mu.Unlock()
	
	if he.running {
		return fmt.Errorf("persistence engine already running")
	}
	
	if !he.config.Enabled {
		return nil // No-op when disabled
	}
	
	// Ensure data directory exists
	if err := os.MkdirAll(he.config.DataDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	
	// Initialize AOF if enabled
	if he.config.EnableAOF {
		if err := he.aofManager.Open(); err != nil {
			return fmt.Errorf("failed to initialize AOF: %w", err)
		}
		fmt.Printf("AOF initialized: %s\n", he.config.DataDirectory)
	}
	
	he.running = true
	he.ctx, he.cancel = context.WithCancel(ctx)
	
	// Start background workers
	if he.config.SnapshotInterval > 0 {
		he.wg.Add(1)
		go he.snapshotWorker()
	}
	
	if he.config.MaxLogSize > 0 {
		he.wg.Add(1)
		go he.compactionWorker()
	}
	
	fmt.Printf("Persistence engine started (AOF: %v, Snapshots: %v)\n", 
		he.config.EnableAOF, he.config.SnapshotInterval > 0)
	
	return nil
}

// Stop gracefully shuts down the persistence engine
func (he *HybridEngine) Stop() error {
	he.mu.Lock()
	defer he.mu.Unlock()
	
	if !he.running {
		return nil
	}
	
	// Cancel context to stop workers
	if he.cancel != nil {
		he.cancel()
	}
	
	// Wait for workers to finish
	he.wg.Wait()
	
	// Close AOF
	if he.aofManager != nil {
		if err := he.aofManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close AOF: %v\n", err)
		}
	}
	
	he.running = false
	fmt.Printf("Persistence engine stopped\n")
	
	return nil
}

// WriteEntry writes a single operation to persistence
func (he *HybridEngine) WriteEntry(entry *LogEntry) error {
	if !he.config.Enabled {
		return nil
	}
	
	start := time.Now()
	defer func() {
		he.updateStats(func(stats *PersistenceStats) {
			stats.WriteLatency = time.Since(start)
			stats.EntriesWritten++
		})
	}()
	
	// Write to AOF if enabled
	if he.config.EnableAOF && he.aofManager != nil {
		var err error
		switch entry.Operation {
		case "SET":
			ttl := time.Duration(entry.TTL) * time.Second
			err = he.aofManager.LogSet(entry.Key, entry.Value, ttl, entry.SessionID)
		case "DEL":
			err = he.aofManager.LogDelete(entry.Key)
		case "EXPIRE":
			ttl := time.Duration(entry.TTL) * time.Second
			err = he.aofManager.LogExpire(entry.Key, ttl)
		case "CLEAR":
			err = he.aofManager.LogClear()
		default:
			return fmt.Errorf("unsupported operation: %s", entry.Operation)
		}
		
		if err != nil {
			he.updateStats(func(stats *PersistenceStats) {
				stats.WriteErrors++
			})
			return fmt.Errorf("failed to write AOF entry: %w", err)
		}
	}
	
	return nil
}

// ReadEntries reads all entries for recovery
func (he *HybridEngine) ReadEntries() ([]*LogEntry, error) {
	if !he.config.Enabled {
		return nil, nil
	}
	
	start := time.Now()
	defer func() {
		he.updateStats(func(stats *PersistenceStats) {
			stats.RecoveryTime = time.Since(start)
		})
	}()
	
	var allEntries []*LogEntry
	
	// First, try to load from snapshot
	if he.snapshotManager != nil {
		data, header, err := he.snapshotManager.LoadSnapshot(he.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load snapshot: %w", err)
		}
		
		if header != nil {
			fmt.Printf("Loaded snapshot: %d entries from %v\n", 
				header.EntryCount, header.CreatedAt.Format("2006-01-02 15:04:05"))
			
			// Convert snapshot data to log entries
			for key, value := range data {
				entry := &LogEntry{
					Timestamp: time.Now(),
					Operation: "SET",
					Key:       key,
					Value:     []byte(fmt.Sprintf("%v", value)),
				}
				allEntries = append(allEntries, entry)
			}
		}
	}
	
	// Then, replay AOF entries that came after the snapshot
	if he.config.EnableAOF && he.aofManager != nil {
		aofEntries, err := he.aofManager.Replay(he.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to replay AOF: %w", err)
		}
		
		// Convert AOF entries to LogEntry pointers
		for _, entry := range aofEntries {
			logEntry := &LogEntry{
				Timestamp: entry.Timestamp,
				Operation: entry.Operation,
				Key:       entry.Key,
				Value:     entry.Value,
				TTL:       entry.TTL,
				SessionID: entry.SessionID,
			}
			allEntries = append(allEntries, logEntry)
		}
	}
	
	he.updateStats(func(stats *PersistenceStats) {
		stats.EntriesRecovered = int64(len(allEntries))
	})
	
	return allEntries, nil
}

// Helper methods

// snapshotWorker runs periodic snapshots
func (he *HybridEngine) snapshotWorker() {
	defer he.wg.Done()
	
	ticker := time.NewTicker(he.config.SnapshotInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-he.ctx.Done():
			return
		case <-ticker.C:
			// This will be called from the main cache when data is available
			// For now, just log that the worker is running
			fmt.Printf("Snapshot worker tick (waiting for cache integration)\n")
		}
	}
}

// compactionWorker runs AOF compaction when needed
func (he *HybridEngine) compactionWorker() {
	defer he.wg.Done()
	
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()
	
	for {
		select {
		case <-he.ctx.Done():
			return
		case <-ticker.C:
			if he.shouldCompact() {
				fmt.Printf("AOF compaction needed (waiting for cache integration)\n")
				// This will be implemented when integrated with cache
			}
		}
	}
}

// shouldCompact determines if AOF compaction is needed
func (he *HybridEngine) shouldCompact() bool {
	if !he.config.EnableAOF || he.aofManager == nil {
		return false
	}
	
	stats := he.aofManager.GetStats()
	logSize, ok := stats["log_size"].(int64)
	if !ok {
		return false
	}
	
	return logSize > he.config.MaxLogSize
}

// updateStats safely updates persistence statistics
func (he *HybridEngine) updateStats(fn func(*PersistenceStats)) {
	he.statsMutex.Lock()
	defer he.statsMutex.Unlock()
	fn(&he.stats)
}

// GetStats returns current persistence statistics
func (he *HybridEngine) GetStats() *PersistenceStats {
	he.statsMutex.RLock()
	defer he.statsMutex.RUnlock()
	
	// Create a copy to avoid race conditions
	statsCopy := he.stats
	return &statsCopy
}

// CreateSnapshot creates a point-in-time snapshot
func (he *HybridEngine) CreateSnapshot(data map[string]interface{}) error {
	if !he.config.Enabled || he.snapshotManager == nil {
		return nil
	}
	
	start := time.Now()
	nodeID := "hypercache-node" // This could be configurable
	
	if err := he.snapshotManager.CreateSnapshot(he.ctx, data, nodeID); err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}
	
	he.updateStats(func(stats *PersistenceStats) {
		stats.LastSnapshot = time.Now()
		stats.CompactionTime = time.Since(start)
	})
	
	return nil
}

// LoadSnapshot loads data from the most recent snapshot
func (he *HybridEngine) LoadSnapshot() (map[string]interface{}, error) {
	if !he.config.Enabled || he.snapshotManager == nil {
		return make(map[string]interface{}), nil
	}
	
	data, header, err := he.snapshotManager.LoadSnapshot(he.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}
	
	if header != nil {
		he.updateStats(func(stats *PersistenceStats) {
			stats.EntriesRecovered = header.EntryCount
		})
	}
	
	return data, nil
}

// Compact performs AOF compaction
func (he *HybridEngine) Compact() error {
	// This will be implemented when we have cache data access
	return fmt.Errorf("compaction requires cache integration")
}

func (he *HybridEngine) shouldRotateLog() bool {
	if he.aofManager == nil {
		return false
	}
	
	// Simple size-based rotation logic
	// This will be enhanced when we implement proper rotation
	return false // TODO: Implement proper rotation logic
}

func (he *HybridEngine) rotateLog() {
	// TODO: Implement log rotation
	// 1. Close current AOF
	// 2. Rename to .old
	// 3. Create new AOF
	// 4. Trigger compaction if needed
}

func (he *HybridEngine) calculateChecksum(entry *LogEntry) uint32 {
	// Simple checksum - in production, use CRC32 or similar
	sum := uint32(0)
	for _, b := range []byte(entry.Key) {
		sum += uint32(b)
	}
	for _, b := range entry.Value {
		sum += uint32(b)
	}
	return sum
}


