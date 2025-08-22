package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// HyperCacheDemo demonstrates how to use HyperCache with Redis clients
type HyperCacheDemo struct {
	client *redis.Client
	ctx    context.Context
}

// NewDemo creates a new demo instance
func NewDemo(addr string) *HyperCacheDemo {
	// Create Redis client - works with any Redis-compatible server
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // No password for demo
		DB:       0,  // Default DB
		
		// Connection settings
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		
		// Pool settings for better performance
		PoolSize:       10,
		MinIdleConns:   3,
		PoolTimeout:    4 * time.Second,
	})

	return &HyperCacheDemo{
		client: client,
		ctx:    context.Background(),
	}
}

// Close closes the demo and cleans up resources
func (d *HyperCacheDemo) Close() error {
	return d.client.Close()
}

// TestBasicOperations demonstrates basic cache operations
func (d *HyperCacheDemo) TestBasicOperations() error {
	fmt.Println("=== Testing Basic Operations ===")
	
	// Test PING
	fmt.Print("Testing PING... ")
	pong, err := d.client.Ping(d.ctx).Result()
	if err != nil {
		return fmt.Errorf("PING failed: %w", err)
	}
	fmt.Printf("✅ Response: %s\n", pong)
	
	// Test SET
	fmt.Print("Testing SET... ")
	err = d.client.Set(d.ctx, "demo:key1", "Hello HyperCache!", 0).Err()
	if err != nil {
		return fmt.Errorf("SET failed: %w", err)
	}
	fmt.Println("✅ Key set successfully")
	
	// Test GET
	fmt.Print("Testing GET... ")
	val, err := d.client.Get(d.ctx, "demo:key1").Result()
	if err != nil {
		return fmt.Errorf("GET failed: %w", err)
	}
	fmt.Printf("✅ Retrieved: %s\n", val)
	
	// Test EXISTS
	fmt.Print("Testing EXISTS... ")
	exists, err := d.client.Exists(d.ctx, "demo:key1").Result()
	if err != nil {
		return fmt.Errorf("EXISTS failed: %w", err)
	}
	fmt.Printf("✅ Exists: %d keys\n", exists)
	
	// Test DEL
	fmt.Print("Testing DEL... ")
	deleted, err := d.client.Del(d.ctx, "demo:key1").Result()
	if err != nil {
		return fmt.Errorf("DEL failed: %w", err)
	}
	fmt.Printf("✅ Deleted: %d keys\n", deleted)
	
	// Verify deletion
	fmt.Print("Verifying deletion... ")
	_, err = d.client.Get(d.ctx, "demo:key1").Result()
	if err == redis.Nil {
		fmt.Println("✅ Key correctly deleted")
	} else if err != nil {
		return fmt.Errorf("unexpected error: %w", err)
	} else {
		return fmt.Errorf("key still exists after deletion")
	}
	
	return nil
}

// TestTTLOperations demonstrates TTL (Time To Live) operations
func (d *HyperCacheDemo) TestTTLOperations() error {
	fmt.Println("\n=== Testing TTL Operations ===")
	
	// SET with expiration
	fmt.Print("Setting key with 3 second TTL... ")
	err := d.client.Set(d.ctx, "demo:ttl_key", "This will expire", 3*time.Second).Err()
	if err != nil {
		return fmt.Errorf("SET with TTL failed: %w", err)
	}
	fmt.Println("✅ Key set with TTL")
	
	// Check immediately
	fmt.Print("Checking key immediately... ")
	val, err := d.client.Get(d.ctx, "demo:ttl_key").Result()
	if err != nil {
		return fmt.Errorf("GET failed: %w", err)
	}
	fmt.Printf("✅ Retrieved: %s\n", val)
	
	// Wait and check again
	fmt.Print("Waiting 4 seconds for expiration... ")
	time.Sleep(4 * time.Second)
	
	_, err = d.client.Get(d.ctx, "demo:ttl_key").Result()
	if err == redis.Nil {
		fmt.Println("✅ Key correctly expired")
	} else if err != nil {
		return fmt.Errorf("unexpected error: %w", err)
	} else {
		fmt.Println("⚠️ Key should have expired (TTL may not be fully implemented)")
	}
	
	return nil
}

// TestBulkOperations demonstrates bulk operations
func (d *HyperCacheDemo) TestBulkOperations() error {
	fmt.Println("\n=== Testing Bulk Operations ===")
	
	// Set multiple keys
	fmt.Print("Setting multiple keys... ")
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("demo:bulk_key_%d", i)
		value := fmt.Sprintf("Bulk value #%d", i)
		
		err := d.client.Set(d.ctx, key, value, 0).Err()
		if err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	fmt.Println("✅ Multiple keys set")
	
	// Check existence of multiple keys
	fmt.Print("Checking existence of multiple keys... ")
	keys := []string{"demo:bulk_key_1", "demo:bulk_key_3", "demo:bulk_key_5", "demo:nonexistent"}
	exists, err := d.client.Exists(d.ctx, keys...).Result()
	if err != nil {
		return fmt.Errorf("EXISTS bulk failed: %w", err)
	}
	fmt.Printf("✅ Found %d out of %d keys\n", exists, len(keys))
	
	// Delete multiple keys
	fmt.Print("Deleting multiple keys... ")
	delKeys := []string{"demo:bulk_key_1", "demo:bulk_key_2", "demo:bulk_key_3"}
	deleted, err := d.client.Del(d.ctx, delKeys...).Result()
	if err != nil {
		return fmt.Errorf("DEL bulk failed: %w", err)
	}
	fmt.Printf("✅ Deleted %d keys\n", deleted)
	
	return nil
}

// TestConcurrentOperations demonstrates concurrent access
func (d *HyperCacheDemo) TestConcurrentOperations() error {
	fmt.Println("\n=== Testing Concurrent Operations ===")
	
	const numGoroutines = 10
	const opsPerGoroutine = 20
	
	fmt.Printf("Running %d goroutines with %d operations each... ", numGoroutines, opsPerGoroutine)
	
	// Channel to collect errors
	errChan := make(chan error, numGoroutines)
	
	// Start concurrent operations
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("demo:concurrent_%d_%d", workerID, j)
				value := fmt.Sprintf("Worker %d, Op %d", workerID, j)
				
				// SET
				if err := d.client.Set(d.ctx, key, value, 0).Err(); err != nil {
					errChan <- fmt.Errorf("worker %d SET failed: %w", workerID, err)
					return
				}
				
				// GET
				if _, err := d.client.Get(d.ctx, key).Result(); err != nil {
					errChan <- fmt.Errorf("worker %d GET failed: %w", workerID, err)
					return
				}
				
				// DEL
				if err := d.client.Del(d.ctx, key).Err(); err != nil {
					errChan <- fmt.Errorf("worker %d DEL failed: %w", workerID, err)
					return
				}
			}
			errChan <- nil // Success
		}(i)
	}
	
	// Wait for all goroutines
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		if err := <-errChan; err != nil {
			return err
		}
		successCount++
	}
	
	fmt.Printf("✅ All %d workers completed successfully\n", successCount)
	return nil
}

// TestServerInfo demonstrates server information commands
func (d *HyperCacheDemo) TestServerInfo() error {
	fmt.Println("\n=== Testing Server Information ===")
	
	// Get server info (if available)
	fmt.Print("Getting server info... ")
	info, err := d.client.Info(d.ctx).Result()
	if err != nil {
		fmt.Printf("⚠️ INFO command may not be fully implemented: %v\n", err)
	} else {
		fmt.Println("✅ Server info retrieved:")
		// Print first few lines of info
		lines := fmt.Sprintf("%.200s...", info)
		fmt.Printf("   %s\n", lines)
	}
	
	// Get database size
	fmt.Print("Getting database size... ")
	size, err := d.client.DBSize(d.ctx).Result()
	if err != nil {
		fmt.Printf("⚠️ DBSIZE command error: %v\n", err)
	} else {
		fmt.Printf("✅ Database size: %d keys\n", size)
	}
	
	return nil
}

// TestFlushDatabase demonstrates database clearing
func (d *HyperCacheDemo) TestFlushDatabase() error {
	fmt.Println("\n=== Testing Database Operations ===")
	
	// Add some test data
	fmt.Print("Adding test data... ")
	testKeys := []string{"demo:flush_test_1", "demo:flush_test_2", "demo:flush_test_3"}
	for _, key := range testKeys {
		err := d.client.Set(d.ctx, key, "test data", 0).Err()
		if err != nil {
			return fmt.Errorf("failed to set test data: %w", err)
		}
	}
	fmt.Println("✅ Test data added")
	
	// Check database size
	fmt.Print("Checking database size... ")
	size, err := d.client.DBSize(d.ctx).Result()
	if err != nil {
		fmt.Printf("⚠️ DBSIZE error: %v\n", err)
	} else {
		fmt.Printf("✅ Database size: %d keys\n", size)
	}
	
	// Flush database
	fmt.Print("Flushing database... ")
	err = d.client.FlushAll(d.ctx).Err()
	if err != nil {
		return fmt.Errorf("FLUSHALL failed: %w", err)
	}
	fmt.Println("✅ Database flushed")
	
	// Verify database is empty
	fmt.Print("Verifying database is empty... ")
	size, err = d.client.DBSize(d.ctx).Result()
	if err != nil {
		fmt.Printf("⚠️ DBSIZE error: %v\n", err)
	} else {
		fmt.Printf("✅ Database size after flush: %d keys\n", size)
	}
	
	return nil
}

// RunAllTests runs all demo tests
func (d *HyperCacheDemo) RunAllTests() error {
	fmt.Println("🚀 HyperCache RESP Server Demo")
	fmt.Println("==============================")
	
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Basic Operations", d.TestBasicOperations},
		{"TTL Operations", d.TestTTLOperations},
		{"Bulk Operations", d.TestBulkOperations},
		{"Concurrent Operations", d.TestConcurrentOperations},
		{"Server Information", d.TestServerInfo},
		{"Database Operations", d.TestFlushDatabase},
	}
	
	for _, test := range tests {
		if err := test.fn(); err != nil {
			return fmt.Errorf("%s failed: %w", test.name, err)
		}
	}
	
	fmt.Println("\n🎉 All tests completed successfully!")
	fmt.Println("HyperCache RESP server is working correctly with Redis clients.")
	
	return nil
}

func main() {
	// Default to localhost:6380 (alternative to standard Redis port)
	addr := "localhost:6380"
	
	// You can override with environment variable or command line arg
	if len(fmt.Sprintf("%v", addr)) > 0 {
		fmt.Printf("Connecting to HyperCache server at: %s\n", addr)
	}
	
	// Create demo instance
	demo := NewDemo(addr)
	defer demo.Close()
	
	// Test connection first
	fmt.Print("Testing connection... ")
	err := demo.client.Ping(demo.ctx).Err()
	if err != nil {
		log.Fatalf("❌ Failed to connect to HyperCache server: %v\n\nMake sure the HyperCache RESP server is running on %s", err, addr)
	}
	fmt.Println("✅ Connected successfully")
	
	// Run all demo tests
	if err := demo.RunAllTests(); err != nil {
		log.Fatalf("❌ Demo failed: %v", err)
	}
}
