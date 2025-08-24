// Detailed Explanation: Go Method with Context Checking

//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("=== Method Receiver Examples ===")
	
	// Example usage
	ExampleUsage()
	
	// More examples
	fmt.Println("\n=== Additional Examples ===")
	
	// Test different patterns
	ctx := context.Background()
	err := checkContextNonBlocking(ctx)
	if err != nil {
		fmt.Printf("Context error: %v\n", err)
	} else {
		fmt.Println("✅ Context is active")
	}
}

// WriteAheadLog is a struct (like a class)
type WriteAheadLog struct {
	file     *os.File
	entries  []LogEntry
	filePath string
}

// LogEntry represents a single log entry
type LogEntry struct {
	Key   []byte
	Value []byte
	Op    string // "PUT", "DELETE"
}

// serialize converts LogEntry to bytes for disk storage
func (entry LogEntry) serialize() []byte {
	// Simple serialization format:
	// [OpLen][Op][KeyLen][Key][ValueLen][Value]
	
	result := make([]byte, 0)
	
	// Write operation length and operation
	opBytes := []byte(entry.Op)
	result = append(result, byte(len(opBytes)))
	result = append(result, opBytes...)
	
	// Write key length and key
	result = append(result, byte(len(entry.Key)))
	result = append(result, entry.Key...)
	
	// Write value length and value  
	valueLen := make([]byte, 4) // 4 bytes for length (up to 4GB values)
	valueLen[0] = byte(len(entry.Value) >> 24)
	valueLen[1] = byte(len(entry.Value) >> 16) 
	valueLen[2] = byte(len(entry.Value) >> 8)
	valueLen[3] = byte(len(entry.Value))
	result = append(result, valueLen...)
	result = append(result, entry.Value...)
	
	return result
}

// deserialize creates LogEntry from bytes (for reading from disk)
func deserializeLogEntry(data []byte) (LogEntry, error) {
	if len(data) < 2 {
		return LogEntry{}, fmt.Errorf("invalid data: too short")
	}
	
	pos := 0
	
	// Read operation
	opLen := int(data[pos])
	pos++
	op := string(data[pos : pos+opLen])
	pos += opLen
	
	// Read key
	keyLen := int(data[pos])
	pos++
	key := data[pos : pos+keyLen]
	pos += keyLen
	
	// Read value length (4 bytes)
	valueLen := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
	pos += 4
	
	// Read value
	value := data[pos : pos+valueLen]
	
	return LogEntry{
		Key:   key,
		Value: value,
		Op:    op,
	}, nil
}

// sync is a METHOD that belongs to WriteAheadLog struct
// Think of it like: WriteAheadLog.sync() in other languages
func (wal *WriteAheadLog) sync(ctx context.Context) error {
	// STEP 1: Check if operation should be cancelled
	// This is non-blocking - it checks immediately and continues
	select {
	case <-ctx.Done():
		// Context was cancelled (timeout/client disconnect/shutdown)
		return ctx.Err() // Returns specific error: context.Canceled or context.DeadlineExceeded
	default:
		// Context is still active, continue with operation
		// This doesn't block - it immediately goes to next step
	}

	// STEP 2: Perform expensive disk operation
	// This is where the actual work happens
	fmt.Printf("💾 Syncing WAL to disk: %s\n", wal.filePath)
	return wal.file.Sync() // Force write to physical disk
}

// Example of how this method would be called
func ExampleUsage() {
	// Create a WriteAheadLog instance
	wal := &WriteAheadLog{
		filePath: "/tmp/hypercache/wal.log",
	}

	// Scenario 1: Normal operation
	ctx1 := context.Background()
	err := wal.sync(ctx1) // Calls the method
	if err != nil {
		fmt.Printf("Sync failed: %v\n", err)
	}

	// Scenario 2: With timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = wal.sync(ctx2) // Will abort if takes longer than 1 second
	if err == context.DeadlineExceeded {
		fmt.Println("⏰ Sync took too long, aborted")
	}

	// Scenario 3: Cancelled operation
	ctx3, cancel3 := context.WithCancel(context.Background())
	
	// Cancel immediately (simulate client disconnect)
	cancel3()
	
	err = wal.sync(ctx3) // Will return immediately without syncing
	if err == context.Canceled {
		fmt.Println("🚫 Sync was cancelled")
	}
}

// More examples of method receiver patterns in HyperCache

// Value receiver (copies the struct)
func (entry LogEntry) String() string {
	return fmt.Sprintf("LogEntry{Op: %s, Key: %s}", entry.Op, string(entry.Key))
}

// Pointer receiver (modifies original struct)
func (wal *WriteAheadLog) addEntry(entry LogEntry) error {
	// This modifies the original wal struct
	wal.entries = append(wal.entries, entry)
	return nil
}

// Interface implementation example
type Syncer interface {
	sync(ctx context.Context) error
}

// WriteAheadLog automatically implements Syncer interface
// because it has a sync method with matching signature
var _ Syncer = &WriteAheadLog{} // Compile-time check

// Different select patterns you'll see in Go

// Pattern 1: Non-blocking check (what we used above)
func checkContextNonBlocking(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil // Context is still active
	}
}

// Pattern 2: Blocking wait with timeout
func waitWithTimeout(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("operation timed out after 5 seconds")
	}
}

// Pattern 3: Wait for multiple signals
func waitForMultipleSignals(ctx context.Context, done chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil // Work completed successfully
	case <-time.After(30 * time.Second):
		return fmt.Errorf("deadline exceeded")
	}
}

// Real-world example: How this fits in HyperCache
func (wal *WriteAheadLog) writeEntry(ctx context.Context, entry LogEntry) error {
	// Check context before expensive operation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Add to memory buffer (fast)
	wal.entries = append(wal.entries, entry)

	// Check context again before disk I/O
	select {
	case <-ctx.Done():
		// Remove from buffer since we're cancelling
		wal.entries = wal.entries[:len(wal.entries)-1]
		return ctx.Err()
	default:
	}

	// Write to disk (slow operation)
	_, err := wal.file.Write(entry.serialize())
	if err != nil {
		return err
	}

	// Sync to ensure durability
	return wal.sync(ctx) // Calls our sync method
}
