package filter

import (
	"crypto/rand"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

// CuckooFilter implements a Cuckoo filter for efficient probabilistic membership testing.
// It provides better space efficiency and supports deletions compared to Bloom filters.
//
// Key advantages:
// - Lower false positive rates (0.1% vs 1% for Bloom)
// - Supports deletions (Bloom filters cannot)
// - Better cache locality
// - Constant-time operations
type CuckooFilter struct {
	// Configuration
	config *FilterConfig
	name   string

	// Core data structure
	buckets           []bucket    // Array of buckets containing fingerprints
	numBuckets        uint64      // Number of buckets
	bucketSize        uint8       // Slots per bucket (typically 4)
	fingerprintSize   uint8       // Bits per fingerprint
	fingerprintMask   uint32      // Mask for fingerprint extraction
	maxEvictionLength uint32      // Maximum eviction chain length

	// Statistics (atomic for thread safety)
	size              uint64 // Current number of items
	capacity          uint64 // Maximum capacity
	addOps            uint64 // Add operations counter
	lookupOps         uint64 // Lookup operations counter  
	deleteOps         uint64 // Delete operations counter
	clearOps          uint64 // Clear operations counter
	successfulAdds    uint64 // Successful add operations
	failedAdds        uint64 // Failed add operations
	successfulDeletes uint64 // Successful delete operations
	failedDeletes     uint64 // Failed delete operations
	evictionChains    uint64 // Eviction chains triggered
	maxEvictionLen    uint32 // Longest eviction chain
	resizeOps         uint64 // Resize operations

	// Timing
	createdAt     time.Time
	lastModified  time.Time
	lastStatsReset time.Time

	// Thread safety
	mutex sync.RWMutex
}

// bucket represents a collection of fingerprint slots in the Cuckoo filter.
// Each bucket contains multiple slots to reduce collision probability.
type bucket struct {
	fingerprints [4]uint16 // Fixed size for cache efficiency (4 slots × 16 bits each)
	occupied     uint8     // Bitmask indicating which slots are occupied
}

// NewCuckooFilter creates a new Cuckoo filter with the specified configuration.
func NewCuckooFilter(config *FilterConfig) (*CuckooFilter, error) {
	if config == nil {
		return nil, ErrConfigInvalid
	}

	// Validate configuration
	if config.ExpectedItems == 0 {
		return nil, &FilterError{Operation: "create", Message: "expected_items must be greater than 0"}
	}
	if config.FalsePositiveRate <= 0 || config.FalsePositiveRate >= 1 {
		return nil, &FilterError{Operation: "create", Message: "false_positive_rate must be between 0 and 1"}
	}
	if config.BucketSize == 0 || config.BucketSize > 8 {
		return nil, &FilterError{Operation: "create", Message: "bucket_size must be between 1 and 8"}
	}

	// Calculate optimal parameters
	fingerprintSize := calculateOptimalFingerprintSize(config.FalsePositiveRate, config.BucketSize)
	if config.FingerprintSize != 0 {
		fingerprintSize = config.FingerprintSize
	}

	// Calculate number of buckets needed
	loadFactor := 0.85 // 85% load factor to reduce collisions and improve FPR
	numBuckets := uint64(math.Ceil(float64(config.ExpectedItems) / (float64(config.BucketSize) * loadFactor)))

	// Ensure power of 2 for efficient modulo operations
	numBuckets = nextPowerOfTwo(numBuckets)

	// Create the filter
	now := time.Now()
	cf := &CuckooFilter{
		config:            config,
		name:              config.Name,
		buckets:           make([]bucket, numBuckets),
		numBuckets:        numBuckets,
		bucketSize:        config.BucketSize,
		fingerprintSize:   fingerprintSize,
		fingerprintMask:   (1 << fingerprintSize) - 1,
		maxEvictionLength: config.MaxEvictionAttempts,
		capacity:          uint64(float64(numBuckets) * float64(config.BucketSize) * loadFactor),
		createdAt:         now,
		lastModified:      now,
		lastStatsReset:    now,
	}

	return cf, nil
}

// Add inserts a key into the Cuckoo filter.
func (cf *CuckooFilter) Add(key []byte) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}

	atomic.AddUint64(&cf.addOps, 1)

	// Check capacity
	currentSize := atomic.LoadUint64(&cf.size)
	if currentSize >= cf.capacity {
		atomic.AddUint64(&cf.failedAdds, 1)
		return ErrFilterFull
	}

	// Calculate hash and fingerprint
	hash := cf.hash(key)
	fingerprint := cf.fingerprint(hash)
	if fingerprint == 0 {
		fingerprint = 1 // Avoid zero fingerprints
	}

	// Calculate two possible bucket positions
	bucket1 := cf.bucketIndex(hash)
	bucket2 := cf.altBucketIndex(bucket1, fingerprint)

	cf.mutex.Lock()
	defer cf.mutex.Unlock()

	// Try to insert in bucket1
	if cf.insertToBucket(bucket1, fingerprint) {
		atomic.AddUint64(&cf.size, 1)
		atomic.AddUint64(&cf.successfulAdds, 1)
		cf.lastModified = time.Now()
		return nil
	}

	// Try to insert in bucket2  
	if cf.insertToBucket(bucket2, fingerprint) {
		atomic.AddUint64(&cf.size, 1)
		atomic.AddUint64(&cf.successfulAdds, 1)
		cf.lastModified = time.Now()
		return nil
	}

	// Both buckets are full, start eviction process
	success := cf.evictAndInsert(bucket1, fingerprint)
	if success {
		atomic.AddUint64(&cf.size, 1)
		atomic.AddUint64(&cf.successfulAdds, 1)
		cf.lastModified = time.Now()
		return nil
	}

	atomic.AddUint64(&cf.failedAdds, 1)
	return ErrFilterFull
}

// Contains checks if a key might exist in the filter.
func (cf *CuckooFilter) Contains(key []byte) bool {
	if len(key) == 0 {
		return false
	}

	atomic.AddUint64(&cf.lookupOps, 1)

	// Calculate hash and fingerprint
	hash := cf.hash(key)
	fingerprint := cf.fingerprint(hash)
	if fingerprint == 0 {
		fingerprint = 1
	}

	// Calculate two possible bucket positions
	bucket1 := cf.bucketIndex(hash)
	bucket2 := cf.altBucketIndex(bucket1, fingerprint)

	cf.mutex.RLock()
	defer cf.mutex.RUnlock()

	// Check both buckets
	return cf.bucketContains(bucket1, fingerprint) || cf.bucketContains(bucket2, fingerprint)
}

// Delete removes a key from the filter if it exists.
func (cf *CuckooFilter) Delete(key []byte) bool {
	if len(key) == 0 {
		atomic.AddUint64(&cf.failedDeletes, 1)
		return false
	}

	atomic.AddUint64(&cf.deleteOps, 1)

	// Calculate hash and fingerprint
	hash := cf.hash(key)
	fingerprint := cf.fingerprint(hash)
	if fingerprint == 0 {
		fingerprint = 1
	}

	// Calculate two possible bucket positions
	bucket1 := cf.bucketIndex(hash)
	bucket2 := cf.altBucketIndex(bucket1, fingerprint)

	cf.mutex.Lock()
	defer cf.mutex.Unlock()

	// Try to delete from bucket1
	if cf.deleteFromBucket(bucket1, fingerprint) {
		atomic.AddUint64(&cf.size, ^uint64(0)) // Atomic decrement
		atomic.AddUint64(&cf.successfulDeletes, 1)
		cf.lastModified = time.Now()
		return true
	}

	// Try to delete from bucket2
	if cf.deleteFromBucket(bucket2, fingerprint) {
		atomic.AddUint64(&cf.size, ^uint64(0)) // Atomic decrement
		atomic.AddUint64(&cf.successfulDeletes, 1)
		cf.lastModified = time.Now()
		return true
	}

	atomic.AddUint64(&cf.failedDeletes, 1)
	return false
}

// Clear removes all items from the filter.
func (cf *CuckooFilter) Clear() error {
	atomic.AddUint64(&cf.clearOps, 1)

	cf.mutex.Lock()
	defer cf.mutex.Unlock()

	// Reset all buckets
	for i := range cf.buckets {
		cf.buckets[i] = bucket{}
	}

	// Reset counters
	atomic.StoreUint64(&cf.size, 0)
	cf.lastModified = time.Now()

	return nil
}

// Size returns the current number of items in the filter.
func (cf *CuckooFilter) Size() uint64 {
	return atomic.LoadUint64(&cf.size)
}

// Capacity returns the maximum number of items the filter can hold.
func (cf *CuckooFilter) Capacity() uint64 {
	return cf.capacity
}

// LoadFactor returns the current load factor.
func (cf *CuckooFilter) LoadFactor() float64 {
	size := atomic.LoadUint64(&cf.size)
	return float64(size) / float64(cf.capacity)
}

// GetStats returns detailed statistics about the filter.
func (cf *CuckooFilter) GetStats() *FilterStats {
	cf.mutex.RLock()
	defer cf.mutex.RUnlock()

	return &FilterStats{
		Size:               atomic.LoadUint64(&cf.size),
		Capacity:           cf.capacity,
		LoadFactor:         cf.LoadFactor(),
		MemoryUsage:        cf.EstimatedMemoryUsage(),
		FalsePositiveRate:  cf.FalsePositiveRate(),
		AddOperations:      atomic.LoadUint64(&cf.addOps),
		LookupOperations:   atomic.LoadUint64(&cf.lookupOps),
		DeleteOperations:   atomic.LoadUint64(&cf.deleteOps),
		ClearOperations:    atomic.LoadUint64(&cf.clearOps),
		SuccessfulAdds:     atomic.LoadUint64(&cf.successfulAdds),
		FailedAdds:         atomic.LoadUint64(&cf.failedAdds),
		SuccessfulDeletes:  atomic.LoadUint64(&cf.successfulDeletes),
		FailedDeletes:      atomic.LoadUint64(&cf.failedDeletes),
		EvictionChains:     atomic.LoadUint64(&cf.evictionChains),
		MaxEvictionLength:  atomic.LoadUint32(&cf.maxEvictionLen),
		ResizeOperations:   atomic.LoadUint64(&cf.resizeOps),
		CreatedAt:          cf.createdAt,
		LastModified:       cf.lastModified,
		LastStatsReset:     cf.lastStatsReset,
	}
}

// EstimatedMemoryUsage returns the approximate memory usage in bytes.
func (cf *CuckooFilter) EstimatedMemoryUsage() uint64 {
	// Bucket storage: numBuckets × (4 fingerprints × 2 bytes + 1 occupied byte)
	bucketStorage := cf.numBuckets * (4*2 + 1)
	
	// Struct overhead
	structOverhead := uint64(unsafe.Sizeof(*cf))
	
	// Slice header overhead
	sliceOverhead := uint64(unsafe.Sizeof(cf.buckets))
	
	return bucketStorage + structOverhead + sliceOverhead
}

// FalsePositiveRate returns the theoretical false positive rate.
func (cf *CuckooFilter) FalsePositiveRate() float64 {
	// For Cuckoo filters: FPR ≈ 2^(-fingerprintSize) × bucketSize
	return float64(cf.bucketSize) / math.Pow(2, float64(cf.fingerprintSize))
}

// Internal helper methods

// hash computes the hash of a key using xxHash
func (cf *CuckooFilter) hash(key []byte) uint64 {
	return xxhash.Sum64(key)
}

// fingerprint extracts a fingerprint from the hash
func (cf *CuckooFilter) fingerprint(hash uint64) uint32 {
	// Use upper bits for fingerprint to avoid correlation with bucket index
	// Apply additional mixing to improve hash quality
	fp := uint32(hash >> 32) // Use upper 32 bits
	fp ^= uint32(hash)       // XOR with lower 32 bits for mixing
	return fp & cf.fingerprintMask
}

// bucketIndex calculates the bucket index from hash
func (cf *CuckooFilter) bucketIndex(hash uint64) uint64 {
	return hash % cf.numBuckets
}

// altBucketIndex calculates the alternative bucket index
func (cf *CuckooFilter) altBucketIndex(bucketIdx uint64, fingerprint uint32) uint64 {
	// Improved hash mixing to reduce clustering
	// Use a different hash function for alternative bucket calculation
	altHash := uint64(fingerprint)
	altHash ^= altHash >> 16
	altHash *= 0x85ebca6b  // Different constant than MurmurHash
	altHash ^= altHash >> 13
	altHash *= 0xc2b2ae35  // Second mixing constant
	altHash ^= altHash >> 16
	
	return (bucketIdx ^ altHash) % cf.numBuckets
}

// insertToBucket tries to insert a fingerprint into a bucket
func (cf *CuckooFilter) insertToBucket(bucketIdx uint64, fingerprint uint32) bool {
	bucket := &cf.buckets[bucketIdx]
	
	// Find first empty slot
	for i := uint8(0); i < cf.bucketSize && i < 4; i++ {
		if (bucket.occupied & (1 << i)) == 0 {
			bucket.fingerprints[i] = uint16(fingerprint)
			bucket.occupied |= (1 << i)
			return true
		}
	}
	return false
}

// bucketContains checks if a bucket contains a fingerprint
func (cf *CuckooFilter) bucketContains(bucketIdx uint64, fingerprint uint32) bool {
	bucket := &cf.buckets[bucketIdx]
	fp16 := uint16(fingerprint)
	
	// Check all occupied slots
	for i := uint8(0); i < cf.bucketSize && i < 4; i++ {
		if (bucket.occupied & (1 << i)) != 0 {
			if bucket.fingerprints[i] == fp16 {
				return true
			}
		}
	}
	return false
}

// deleteFromBucket removes a fingerprint from a bucket
func (cf *CuckooFilter) deleteFromBucket(bucketIdx uint64, fingerprint uint32) bool {
	bucket := &cf.buckets[bucketIdx]
	fp16 := uint16(fingerprint)
	
	// Find and remove the fingerprint
	for i := uint8(0); i < cf.bucketSize && i < 4; i++ {
		if (bucket.occupied & (1 << i)) != 0 {
			if bucket.fingerprints[i] == fp16 {
				bucket.fingerprints[i] = 0
				bucket.occupied &= ^(1 << i)
				return true
			}
		}
	}
	return false
}

// evictAndInsert performs the cuckoo eviction process
func (cf *CuckooFilter) evictAndInsert(bucketIdx uint64, fingerprint uint32) bool {
	atomic.AddUint64(&cf.evictionChains, 1)
	
	currentBucket := bucketIdx
	currentFingerprint := fingerprint
	evictionLength := uint32(0)
	
	for evictionLength < cf.maxEvictionLength {
		// Pick a random slot to evict
		bucket := &cf.buckets[currentBucket]
		slotIdx := cf.randomSlot(bucket)
		
		// Swap fingerprints
		evictedFingerprint := uint32(bucket.fingerprints[slotIdx])
		bucket.fingerprints[slotIdx] = uint16(currentFingerprint)
		
		// Try to place evicted fingerprint in its alternative bucket
		altBucket := cf.altBucketIndex(currentBucket, evictedFingerprint)
		if cf.insertToBucket(altBucket, evictedFingerprint) {
			// Success! Update max eviction length
			if evictionLength > atomic.LoadUint32(&cf.maxEvictionLen) {
				atomic.StoreUint32(&cf.maxEvictionLen, evictionLength+1)
			}
			return true
		}
		
		// Continue eviction chain
		currentBucket = altBucket
		currentFingerprint = evictedFingerprint
		evictionLength++
	}
	
	return false // Eviction failed
}

// randomSlot returns a random occupied slot index in a bucket
func (cf *CuckooFilter) randomSlot(bucket *bucket) uint8 {
	// Simple random selection among occupied slots
	var occupiedSlots []uint8
	for i := uint8(0); i < cf.bucketSize && i < 4; i++ {
		if (bucket.occupied & (1 << i)) != 0 {
			occupiedSlots = append(occupiedSlots, i)
		}
	}
	
	if len(occupiedSlots) == 0 {
		return 0 // Shouldn't happen in normal operation
	}
	
	// Use crypto/rand for better randomness
	randBytes := make([]byte, 1)
	rand.Read(randBytes)
	return occupiedSlots[int(randBytes[0])%len(occupiedSlots)]
}

// Utility functions

// calculateOptimalFingerprintSize calculates the optimal fingerprint size for given FPR and bucket size
func calculateOptimalFingerprintSize(fpr float64, bucketSize uint8) uint8 {
	// FPR ≈ bucketSize / 2^fingerprintSize
	// fingerprintSize ≈ log2(bucketSize / FPR)
	size := math.Log2(float64(bucketSize) / fpr)
	return uint8(math.Ceil(size))
}

// nextPowerOfTwo returns the next power of two greater than or equal to n
func nextPowerOfTwo(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}
