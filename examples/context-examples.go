// Example: Context Usage in HyperCache Implementation

//go:build ignore

package main

import (
	"context"
	"fmt"
	"time"
)

// Mock types for demonstration
type HyperCache struct{}

func NewHyperCache() *HyperCache {
	return &HyperCache{}
}

func (hc *HyperCache) Get(ctx context.Context, store string, key []byte) ([]byte, error) {
	// Simulate cache operation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Millisecond): // Simulate work
		return []byte("session_data"), nil
	}
}

// Example of how context flows through our cache operations
func ExampleContextUsage() {
	cache := NewHyperCache()

	// 1. HTTP request arrives with context
	ctx := context.Background()
	
	// 2. Add request timeout (cache operations should be fast!)
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	
	// 3. Add request metadata
	ctx = context.WithValue(ctx, "request_id", "req_12345")
	ctx = context.WithValue(ctx, "operation", "user_session_get")

	// 4. Call cache with context
	sessionData, err := cache.Get(ctx, "sessions", []byte("user:123"))
	if err != nil {
		if err == context.DeadlineExceeded {
			// Cache took too long - maybe there's a problem
			fmt.Println("⚠️  Cache timeout - falling back to database")
			return
		}
		if err == context.Canceled {
			// Client cancelled request (closed browser, etc.)
			fmt.Println("🚫 Request cancelled by client")
			return
		}
	}

	fmt.Printf("✅ Got session data: %v\n", sessionData)
}

// Mock types for demonstration (continued)
type HyperCacheInstance struct {
	logger *Logger
	stores map[string]*MockStore
}

type Logger struct{}
func (l *Logger) Printf(format string, args ...interface{}) {
	fmt.Printf("[LOG] "+format+"\n", args...)
}

type MockStore struct{}
func (m *MockStore) Get(key []byte) ([]byte, error) {
	return []byte("mock_value"), nil
}

type NetworkServer struct {
	cache *HyperCacheInstance
}

func (s *NetworkServer) sendResponse(response []byte) {
	fmt.Printf("Sending response: %s\n", string(response))
}

type Request struct {
	Store []byte
	Key   []byte
}

func main() {
	fmt.Println("=== Context Usage Examples ===")
	ExampleContextUsage()
}

// Implementation inside our cache
func (c *HyperCacheInstance) Get(ctx context.Context, storeName string, key []byte) ([]byte, error) {
	// 1. Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err() // Return cancellation error immediately
	default:
		// Continue with operation
	}

	// 2. Log request with context metadata
	requestID := ctx.Value("request_id")
	if requestID != nil {
		c.logger.Printf("Cache GET [%s] store=%s key=%s", requestID, storeName, string(key))
	}

	// 3. Get the target store
	store, exists := c.stores[storeName]
	if !exists {
		return nil, fmt.Errorf("store %s not found", storeName)
	}

	// 4. Perform operation with context checking
	// For fast operations like cache GET, we might not need to check ctx
	// But for slower operations (disk reads, network calls), we would:
	
	value, err := store.Get(key)
	
	// 5. Check context again before returning (for longer operations)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return value, err
	}
}

// For network operations, context is crucial
func (s *NetworkServer) handleRequest(ctx context.Context, req *Request) {
	// Client might disconnect, so we need to check context
	select {
	case <-ctx.Done():
		return // Don't waste time on cancelled requests
	default:
	}

	// Process cache operation
	result, err := s.cache.Get(ctx, string(req.Store), req.Key)
	
	// Check again before sending response
	select {
	case <-ctx.Done():
		return // Client gone, don't send response
	default:
		if err != nil {
			s.sendResponse([]byte("ERROR: " + err.Error()))
		} else {
			s.sendResponse(result)
		}
	}
}
