// Package filter provides probabilistic data structures for efficient negative lookups.
// Cuckoo filters offer superior performance and functionality compared to traditional Bloom filters,
// including support for deletions and lower false positive rates.
package filter

import (
	"fmt"
	"time"
)

// ProbabilisticFilter defines the interface for probabilistic membership testing.
// Implementations guarantee no false negatives but may have false positives.
type ProbabilisticFilter interface {
	// Add inserts a key into the filter.
	// Returns error if the filter is full and cannot accommodate the key.
	Add(key []byte) error

	// Contains checks if a key might exist in the filter.
	// Returns true if the key might exist (could be false positive).
	// Returns false if the key definitely does not exist (guaranteed accurate).
	Contains(key []byte) bool

	// Delete removes a key from the filter if it exists.
	// Returns true if the key was found and removed, false otherwise.
	// Note: This is a key advantage of Cuckoo filters over Bloom filters.
	Delete(key []byte) bool

	// Clear removes all items from the filter, resetting it to empty state.
	Clear() error

	// Size returns the current number of items in the filter.
	Size() uint64

	// Capacity returns the maximum number of items the filter can hold.
	Capacity() uint64

	// LoadFactor returns the current load factor (size/capacity).
	LoadFactor() float64

	// GetStats returns detailed statistics about the filter's performance.
	GetStats() *FilterStats

	// EstimatedMemoryUsage returns the approximate memory usage in bytes.
	EstimatedMemoryUsage() uint64

	// FalsePositiveRate returns the theoretical false positive rate.
	FalsePositiveRate() float64
}

// FilterStats contains detailed statistics about filter performance and state.
type FilterStats struct {
	// Basic metrics
	Size               uint64    `json:"size"`                // Current number of items
	Capacity           uint64    `json:"capacity"`            // Maximum capacity
	LoadFactor         float64   `json:"load_factor"`         // Current load factor
	MemoryUsage        uint64    `json:"memory_usage"`        // Memory usage in bytes
	FalsePositiveRate  float64   `json:"false_positive_rate"` // Theoretical FP rate

	// Operational metrics
	AddOperations      uint64    `json:"add_operations"`      // Total add operations
	LookupOperations   uint64    `json:"lookup_operations"`   // Total lookup operations
	DeleteOperations   uint64    `json:"delete_operations"`   // Total delete operations
	ClearOperations    uint64    `json:"clear_operations"`    // Total clear operations

	// Performance metrics
	SuccessfulAdds     uint64    `json:"successful_adds"`     // Successful add operations
	FailedAdds         uint64    `json:"failed_adds"`         // Failed add operations (filter full)
	SuccessfulDeletes  uint64    `json:"successful_deletes"`  // Successful delete operations
	FailedDeletes      uint64    `json:"failed_deletes"`      // Failed delete operations (not found)

	// Cuckoo filter specific metrics
	EvictionChains     uint64    `json:"eviction_chains"`     // Number of eviction chains triggered
	MaxEvictionLength  uint32    `json:"max_eviction_length"` // Longest eviction chain
	ResizeOperations   uint64    `json:"resize_operations"`   // Number of filter resizes

	// Timing
	CreatedAt          time.Time `json:"created_at"`          // Filter creation time
	LastModified       time.Time `json:"last_modified"`       // Last modification time
	LastStatsReset     time.Time `json:"last_stats_reset"`    // Last time stats were reset
}

// FilterConfig contains configuration parameters for filter creation.
type FilterConfig struct {
	// Core configuration
	Name                 string  `yaml:"name"`                   // Filter name for identification
	FilterType           string  `yaml:"type"`                   // "cuckoo" or "bloom"
	ExpectedItems        uint64  `yaml:"expected_items"`         // Expected number of items
	FalsePositiveRate    float64 `yaml:"false_positive_rate"`    // Target false positive rate
	
	// Memory configuration  
	MemoryBudgetBytes    uint64  `yaml:"memory_budget_bytes"`    // Maximum memory usage
	MemoryBudgetPercent  float64 `yaml:"memory_budget_percent"`  // Percentage of store memory

	// Cuckoo filter specific configuration
	FingerprintSize      uint8   `yaml:"fingerprint_size"`       // Bits per fingerprint (8, 12, 16)
	BucketSize           uint8   `yaml:"bucket_size"`            // Slots per bucket (typically 4)
	MaxEvictionAttempts  uint32  `yaml:"max_eviction_attempts"`  // Max attempts before resize (500)
	EnableAutoResize     bool    `yaml:"enable_auto_resize"`     // Allow automatic resizing

	// Performance tuning
	EnableStatistics     bool    `yaml:"enable_statistics"`      // Collect detailed statistics
	HashFunction         string  `yaml:"hash_function"`          // "xxhash", "sha1", "murmur3"
}

// DefaultCuckooConfig returns a default configuration for Cuckoo filters optimized
// for general-purpose use with GUID keys and 0.1% false positive rate.
func DefaultCuckooConfig(name string, expectedItems uint64) *FilterConfig {
	return &FilterConfig{
		Name:                 name,
		FilterType:           "cuckoo",
		ExpectedItems:        expectedItems,
		FalsePositiveRate:    0.001, // 0.1%
		MemoryBudgetPercent:  5.0,   // 5% of store memory
		FingerprintSize:      12,    // 12 bits for 0.1% FP rate
		BucketSize:           4,     // 4 slots per bucket (optimal)
		MaxEvictionAttempts:  500,   // Before resize
		EnableAutoResize:     true,
		EnableStatistics:     true,
		HashFunction:         "xxhash", // Fast and high-quality
	}
}

// OptimizedForGUIDs returns a configuration specifically optimized for GUID keys.
// This configuration is ideal for the user's Cosmos DB use case with billions of GUID keys.
func OptimizedForGUIDs(name string, expectedItems uint64, memoryBudgetPercent float64) *FilterConfig {
	return &FilterConfig{
		Name:                 name,
		FilterType:           "cuckoo",
		ExpectedItems:        expectedItems,
		FalsePositiveRate:    0.001, // 0.1% - excellent for cost reduction
		MemoryBudgetPercent:  memoryBudgetPercent,
		FingerprintSize:      12,    // Optimal for GUIDs
		BucketSize:           4,     // Cache-friendly
		MaxEvictionAttempts:  500,   // Handle high load factors
		EnableAutoResize:     true,  // Handle growth
		EnableStatistics:     true,  // Monitor performance
		HashFunction:         "xxhash", // Excellent GUID distribution
	}
}

// FilterError represents errors that can occur during filter operations.
type FilterError struct {
	Operation string // The operation that failed
	Message   string // Error description
	Cause     error  // Underlying error, if any
}

func (e *FilterError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("filter %s failed: %s (caused by: %v)", e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("filter %s failed: %s", e.Operation, e.Message)
}

// Common error types
var (
	ErrFilterFull         = &FilterError{Operation: "add", Message: "filter is full, cannot add more items"}
	ErrFilterEmpty        = &FilterError{Operation: "delete", Message: "filter is empty"}
	ErrInvalidKey         = &FilterError{Operation: "key", Message: "key cannot be empty"}
	ErrConfigInvalid      = &FilterError{Operation: "config", Message: "filter configuration is invalid"}
	ErrResizeFailed       = &FilterError{Operation: "resize", Message: "filter resize operation failed"}
	ErrMemoryExceeded     = &FilterError{Operation: "memory", Message: "operation would exceed memory budget"}
)
