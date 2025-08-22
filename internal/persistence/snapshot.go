package persistence

import (
	"compress/gzip"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SnapshotManager handles snapshot creation and loading
type SnapshotManager struct {
	config    PersistenceConfig
	dataDir   string
}

// SnapshotHeader contains metadata about the snapshot
type SnapshotHeader struct {
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	NodeID      string    `json:"node_id"`
	EntryCount  int64     `json:"entry_count"`
	Compressed  bool      `json:"compressed"`
	Checksum    string    `json:"checksum"`
}

// SnapshotEntry represents a single cache entry in the snapshot
type SnapshotEntry struct {
	Key          string    `json:"key"`
	Value        []byte    `json:"value"`
	ValueType    string    `json:"value_type"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	AccessCount  uint64    `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
	Size         uint64    `json:"size"`
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(config PersistenceConfig) *SnapshotManager {
	return &SnapshotManager{
		config:  config,
		dataDir: config.DataDirectory,
	}
}

// CreateSnapshot creates a point-in-time snapshot of cache data
func (sm *SnapshotManager) CreateSnapshot(ctx context.Context, data map[string]interface{}, nodeID string) error {
	start := time.Now()
	
	// Generate snapshot filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("hypercache-snapshot-%s.rdb", timestamp)
	filepath := filepath.Join(sm.dataDir, filename)
	
	// Create temporary file first
	tempFile := filepath + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()
	
	var writer interface{}
	
	// Setup compression if enabled
	if sm.config.CompressionLevel > 0 {
		gzWriter, err := gzip.NewWriterLevel(file, sm.config.CompressionLevel)
		if err != nil {
			return fmt.Errorf("failed to create gzip writer: %w", err)
		}
		defer gzWriter.Close()
		writer = gzWriter
	} else {
		writer = file
	}
	
	// Create header
	header := SnapshotHeader{
		Version:    1,
		CreatedAt:  time.Now(),
		NodeID:     nodeID,
		EntryCount: int64(len(data)),
		Compressed: sm.config.CompressionLevel > 0,
	}
	
	// Write header
	encoder := gob.NewEncoder(writer.(interface{ Write([]byte) (int, error) }))
	if err := encoder.Encode(header); err != nil {
		return fmt.Errorf("failed to encode snapshot header: %w", err)
	}
	
	// Write entries
	entriesWritten := int64(0)
	for key, value := range data {
		// Convert to snapshot entry format
		entry := sm.convertToSnapshotEntry(key, value)
		
		// Check context cancellation
		select {
		case <-ctx.Done():
			os.Remove(tempFile)
			return ctx.Err()
		default:
		}
		
		if err := encoder.Encode(entry); err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("failed to encode entry %s: %w", key, err)
		}
		
		entriesWritten++
	}
	
	// Ensure all data is written
	if gzWriter, ok := writer.(*gzip.Writer); ok {
		gzWriter.Close()
	}
	file.Sync()
	file.Close()
	
	// Atomically move temp file to final location
	if err := os.Rename(tempFile, filepath); err != nil {
		return fmt.Errorf("failed to finalize snapshot: %w", err)
	}
	
	// Clean up old snapshots
	if err := sm.cleanupOldSnapshots(); err != nil {
		// Log error but don't fail the snapshot creation
		fmt.Printf("Warning: failed to cleanup old snapshots: %v\n", err)
	}
	
	fmt.Printf("Snapshot created: %s (%d entries in %v)\n", 
		filename, entriesWritten, time.Since(start))
	
	return nil
}

// LoadSnapshot loads cache data from the most recent snapshot
func (sm *SnapshotManager) LoadSnapshot(ctx context.Context) (map[string]interface{}, *SnapshotHeader, error) {
	// Find the most recent snapshot
	snapshotFile, err := sm.findLatestSnapshot()
	if err != nil {
		return nil, nil, err
	}
	
	if snapshotFile == "" {
		return make(map[string]interface{}), nil, nil // No snapshot exists
	}
	
	start := time.Now()
	filepath := filepath.Join(sm.dataDir, snapshotFile)
	
	file, err := os.Open(filepath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()
	
	var reader interface{}
	
	// Check if file is compressed (read magic bytes)
	magic := make([]byte, 2)
	file.Read(magic)
	file.Seek(0, 0) // Reset to beginning
	
	if magic[0] == 0x1f && magic[1] == 0x8b { // gzip magic bytes
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	} else {
		reader = file
	}
	
	decoder := gob.NewDecoder(reader.(interface{ Read([]byte) (int, error) }))
	
	// Read header
	var header SnapshotHeader
	if err := decoder.Decode(&header); err != nil {
		return nil, nil, fmt.Errorf("failed to decode snapshot header: %w", err)
	}
	
	// Read entries
	data := make(map[string]interface{})
	entriesLoaded := int64(0)
	
	for entriesLoaded < header.EntryCount {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		
		var entry SnapshotEntry
		if err := decoder.Decode(&entry); err != nil {
			return nil, nil, fmt.Errorf("failed to decode entry at position %d: %w", entriesLoaded, err)
		}
		
		// Convert back to cache format
		cacheEntry := sm.convertFromSnapshotEntry(entry)
		data[entry.Key] = cacheEntry
		
		entriesLoaded++
	}
	
	fmt.Printf("Snapshot loaded: %s (%d entries in %v)\n", 
		snapshotFile, entriesLoaded, time.Since(start))
	
	return data, &header, nil
}

// Helper methods

func (sm *SnapshotManager) convertToSnapshotEntry(key string, value interface{}) SnapshotEntry {
	// This is a simplified conversion - in real implementation,
	// we'd need to properly handle the CacheItem structure
	entry := SnapshotEntry{
		Key:          key,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  1,
	}
	
	// Convert value to bytes (simplified)
	if str, ok := value.(string); ok {
		entry.Value = []byte(str)
		entry.ValueType = "string"
		entry.Size = uint64(len(str))
	} else {
		// Handle other types as needed
		entry.Value = []byte(fmt.Sprintf("%v", value))
		entry.ValueType = "unknown"
		entry.Size = uint64(len(entry.Value))
	}
	
	return entry
}

func (sm *SnapshotManager) convertFromSnapshotEntry(entry SnapshotEntry) interface{} {
	// Convert back to appropriate format
	// This would need to match your BasicStore's CacheItem structure
	return map[string]interface{}{
		"key":           entry.Key,
		"value":         entry.Value,
		"value_type":    entry.ValueType,
		"created_at":    entry.CreatedAt,
		"expires_at":    entry.ExpiresAt,
		"session_id":    entry.SessionID,
		"access_count":  entry.AccessCount,
		"last_accessed": entry.LastAccessed,
		"size":         entry.Size,
	}
}

func (sm *SnapshotManager) findLatestSnapshot() (string, error) {
	files, err := filepath.Glob(filepath.Join(sm.dataDir, "hypercache-snapshot-*.rdb"))
	if err != nil {
		return "", fmt.Errorf("failed to search for snapshots: %w", err)
	}
	
	if len(files) == 0 {
		return "", nil
	}
	
	// Find the most recent file (assuming timestamp naming)
	var latest string
	var latestTime time.Time
	
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Base(file)
		}
	}
	
	return latest, nil
}

func (sm *SnapshotManager) cleanupOldSnapshots() error {
	if sm.config.RetainLogs <= 0 {
		return nil // No cleanup needed
	}
	
	files, err := filepath.Glob(filepath.Join(sm.dataDir, "hypercache-snapshot-*.rdb"))
	if err != nil {
		return err
	}
	
	if len(files) <= sm.config.RetainLogs {
		return nil // Not enough files to clean up
	}
	
	// Sort files by modification time (oldest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	
	var fileInfos []fileInfo
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, fileInfo{file, info.ModTime()})
	}
	
	// Sort by modification time
	for i := 0; i < len(fileInfos)-1; i++ {
		for j := i + 1; j < len(fileInfos); j++ {
			if fileInfos[i].modTime.After(fileInfos[j].modTime) {
				fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
			}
		}
	}
	
	// Remove oldest files
	toRemove := len(fileInfos) - sm.config.RetainLogs
	for i := 0; i < toRemove; i++ {
		if err := os.Remove(fileInfos[i].path); err != nil {
			fmt.Printf("Warning: failed to remove old snapshot %s: %v\n", fileInfos[i].path, err)
		}
	}
	
	return nil
}
