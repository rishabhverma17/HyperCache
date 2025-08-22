package persistence

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AOFManager handles append-only file operations
type AOFManager struct {
	config     PersistenceConfig
	dataDir    string
	currentLog *os.File
	writer     *bufio.Writer
	mu         sync.RWMutex
	stats      struct {
		TotalWrites    int64
		LogSize        int64
		LastWrite      time.Time
		CompactionRuns int64
	}
}

// LogEntry represents a single operation in the AOF
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"`
	Key       string    `json:"key"`
	Value     []byte    `json:"value,omitempty"`
	TTL       int64     `json:"ttl,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
}

// NewAOFManager creates a new AOF manager
func NewAOFManager(config PersistenceConfig) *AOFManager {
	return &AOFManager{
		config:  config,
		dataDir: config.DataDirectory,
	}
}

// Open opens or creates the AOF file for writing
func (aof *AOFManager) Open() error {
	aof.mu.Lock()
	defer aof.mu.Unlock()
	
	if err := os.MkdirAll(aof.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	
	logPath := filepath.Join(aof.dataDir, "hypercache.aof")
	
	// Open file for appending, create if doesn't exist
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open AOF file: %w", err)
	}
	
	aof.currentLog = file
	aof.writer = bufio.NewWriterSize(file, 64*1024) // 64KB buffer
	
	// Get initial file size
	info, err := file.Stat()
	if err == nil {
		aof.stats.LogSize = info.Size()
	}
	
	return nil
}

// Close closes the AOF file and flushes pending writes
func (aof *AOFManager) Close() error {
	aof.mu.Lock()
	defer aof.mu.Unlock()
	
	if aof.writer != nil {
		aof.writer.Flush()
		aof.writer = nil
	}
	
	if aof.currentLog != nil {
		err := aof.currentLog.Close()
		aof.currentLog = nil
		return err
	}
	
	return nil
}

// LogSet logs a SET operation to AOF
func (aof *AOFManager) LogSet(key string, value []byte, ttl time.Duration, sessionID string) error {
	entry := LogEntry{
		Timestamp: time.Now(),
		Operation: "SET",
		Key:       key,
		Value:     value,
		SessionID: sessionID,
	}
	
	if ttl > 0 {
		entry.TTL = int64(ttl.Seconds())
	}
	
	return aof.writeEntry(entry)
}

// LogDelete logs a DELETE operation to AOF
func (aof *AOFManager) LogDelete(key string) error {
	entry := LogEntry{
		Timestamp: time.Now(),
		Operation: "DEL",
		Key:       key,
	}
	
	return aof.writeEntry(entry)
}

// LogExpire logs an EXPIRE operation to AOF
func (aof *AOFManager) LogExpire(key string, ttl time.Duration) error {
	entry := LogEntry{
		Timestamp: time.Now(),
		Operation: "EXPIRE",
		Key:       key,
		TTL:       int64(ttl.Seconds()),
	}
	
	return aof.writeEntry(entry)
}

// LogClear logs a CLEAR operation to AOF
func (aof *AOFManager) LogClear() error {
	entry := LogEntry{
		Timestamp: time.Now(),
		Operation: "CLEAR",
	}
	
	return aof.writeEntry(entry)
}

// writeEntry writes a log entry to the AOF file
func (aof *AOFManager) writeEntry(entry LogEntry) error {
	aof.mu.Lock()
	defer aof.mu.Unlock()
	
	if aof.writer == nil {
		return fmt.Errorf("AOF not initialized")
	}
	
	// Format: TIMESTAMP|OPERATION|KEY|VALUE_LENGTH|VALUE|TTL|SESSION_ID\n
	line := fmt.Sprintf("%d|%s|%s|%d|",
		entry.Timestamp.Unix(),
		entry.Operation,
		entry.Key,
		len(entry.Value))
	
	if _, err := aof.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write AOF entry: %w", err)
	}
	
	if len(entry.Value) > 0 {
		if _, err := aof.writer.Write(entry.Value); err != nil {
			return fmt.Errorf("failed to write AOF value: %w", err)
		}
	}
	
	// Add TTL and session info
	if _, err := aof.writer.WriteString(fmt.Sprintf("|%d|%s\n", entry.TTL, entry.SessionID)); err != nil {
		return fmt.Errorf("failed to write AOF metadata: %w", err)
	}
	
	// Update stats
	aof.stats.TotalWrites++
	aof.stats.LastWrite = time.Now()
	
	// Flush based on sync policy
	switch aof.config.SyncPolicy {
	case "always":
		if err := aof.writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush AOF: %w", err)
		}
		if err := aof.currentLog.Sync(); err != nil {
			return fmt.Errorf("failed to sync AOF: %w", err)
		}
	case "everysec":
		// This would be handled by a background goroutine in a real implementation
		aof.writer.Flush()
	default: // "no"
		// Buffer writes, rely on OS for flushing
	}
	
	// Update log size estimate
	aof.stats.LogSize += int64(len(line)) + int64(len(entry.Value)) + int64(len(fmt.Sprintf("|%d|%s\n", entry.TTL, entry.SessionID)))
	
	return nil
}

// Replay replays the AOF log to reconstruct cache state
func (aof *AOFManager) Replay(ctx context.Context) ([]LogEntry, error) {
	logPath := filepath.Join(aof.dataDir, "hypercache.aof")
	
	// Check if AOF file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return []LogEntry{}, nil // No AOF to replay
	}
	
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open AOF for replay: %w", err)
	}
	defer file.Close()
	
	var entries []LogEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0
	
	start := time.Now()
	
	for scanner.Scan() {
		lineNum++
		
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		line := scanner.Text()
		entry, err := aof.parseLogEntry(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AOF line %d: %w", lineNum, err)
		}
		
		entries = append(entries, entry)
		
		// Progress feedback for large logs
		if lineNum%10000 == 0 {
			fmt.Printf("AOF replay progress: %d entries processed\n", lineNum)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading AOF: %w", err)
	}
	
	fmt.Printf("AOF replay completed: %d entries in %v\n", len(entries), time.Since(start))
	
	return entries, nil
}

// parseLogEntry parses a single line from the AOF file
func (aof *AOFManager) parseLogEntry(line string) (LogEntry, error) {
	// Format: TIMESTAMP|OPERATION|KEY|VALUE_LENGTH|VALUE|TTL|SESSION_ID
	parts := strings.Split(line, "|")
	if len(parts) < 7 {
		return LogEntry{}, fmt.Errorf("invalid AOF line format: insufficient parts")
	}
	
	// Parse timestamp
	var timestamp int64
	if _, err := fmt.Sscanf(parts[0], "%d", &timestamp); err != nil {
		return LogEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	
	operation := parts[1]
	key := parts[2]
	
	// Parse value length
	var valueLen int
	if _, err := fmt.Sscanf(parts[3], "%d", &valueLen); err != nil {
		return LogEntry{}, fmt.Errorf("invalid value length: %w", err)
	}
	
	// Extract value (if present)
	var value []byte
	if valueLen > 0 {
		if len(parts[4]) < valueLen {
			return LogEntry{}, fmt.Errorf("value length mismatch")
		}
		value = []byte(parts[4][:valueLen])
	}
	
	// Parse TTL
	var ttl int64
	if _, err := fmt.Sscanf(parts[5], "%d", &ttl); err != nil {
		return LogEntry{}, fmt.Errorf("invalid TTL: %w", err)
	}
	
	sessionID := parts[6]
	
	return LogEntry{
		Timestamp: time.Unix(timestamp, 0),
		Operation: operation,
		Key:       key,
		Value:     value,
		TTL:       ttl,
		SessionID: sessionID,
	}, nil
}

// Compact performs AOF compaction by rewriting the log file
func (aof *AOFManager) Compact(ctx context.Context, currentData map[string]interface{}) error {
	start := time.Now()
	
	aof.mu.Lock()
	defer aof.mu.Unlock()
	
	// Create temporary compacted file
	tempPath := filepath.Join(aof.dataDir, "hypercache.aof.tmp")
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp AOF file: %w", err)
	}
	defer tempFile.Close()
	
	writer := bufio.NewWriterSize(tempFile, 64*1024)
	
	// Write current state to temporary file
	entriesWritten := 0
	for key, value := range currentData {
		// Check context cancellation
		select {
		case <-ctx.Done():
			os.Remove(tempPath)
			return ctx.Err()
		default:
		}
		
		// Convert current data to log entry
		entry := aof.convertToLogEntry(key, value)
		if err := aof.writeEntryToWriter(writer, entry); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("failed to write compacted entry: %w", err)
		}
		
		entriesWritten++
	}
	
	if err := writer.Flush(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to flush compacted AOF: %w", err)
	}
	
	if err := tempFile.Sync(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync compacted AOF: %w", err)
	}
	
	tempFile.Close()
	
	// Close current log file
	if aof.writer != nil {
		aof.writer.Flush()
	}
	if aof.currentLog != nil {
		aof.currentLog.Close()
	}
	
	// Atomically replace the old AOF with the new one
	originalPath := filepath.Join(aof.dataDir, "hypercache.aof")
	if err := os.Rename(tempPath, originalPath); err != nil {
		return fmt.Errorf("failed to replace AOF file: %w", err)
	}
	
	// Reopen the AOF file
	if err := aof.Open(); err != nil {
		return fmt.Errorf("failed to reopen AOF after compaction: %w", err)
	}
	
	aof.stats.CompactionRuns++
	
	fmt.Printf("AOF compaction completed: %d entries written in %v\n", 
		entriesWritten, time.Since(start))
	
	return nil
}

// Helper methods

func (aof *AOFManager) convertToLogEntry(key string, value interface{}) LogEntry {
	// This is a simplified conversion - in real implementation,
	// we'd need to properly handle the CacheItem structure
	entry := LogEntry{
		Timestamp: time.Now(),
		Operation: "SET",
		Key:       key,
	}
	
	// Convert value to bytes (simplified)
	if str, ok := value.(string); ok {
		entry.Value = []byte(str)
	} else {
		entry.Value = []byte(fmt.Sprintf("%v", value))
	}
	
	return entry
}

func (aof *AOFManager) writeEntryToWriter(writer *bufio.Writer, entry LogEntry) error {
	line := fmt.Sprintf("%d|%s|%s|%d|",
		entry.Timestamp.Unix(),
		entry.Operation,
		entry.Key,
		len(entry.Value))
	
	if _, err := writer.WriteString(line); err != nil {
		return err
	}
	
	if len(entry.Value) > 0 {
		if _, err := writer.Write(entry.Value); err != nil {
			return err
		}
	}
	
	if _, err := writer.WriteString(fmt.Sprintf("|%d|%s\n", entry.TTL, entry.SessionID)); err != nil {
		return err
	}
	
	return nil
}

// GetStats returns current AOF statistics
func (aof *AOFManager) GetStats() map[string]interface{} {
	aof.mu.RLock()
	defer aof.mu.RUnlock()
	
	return map[string]interface{}{
		"total_writes":     aof.stats.TotalWrites,
		"log_size":         aof.stats.LogSize,
		"last_write":       aof.stats.LastWrite,
		"compaction_runs":  aof.stats.CompactionRuns,
	}
}
