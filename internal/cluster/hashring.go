package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

// NodeStatus represents the health status of a node
type NodeStatus int

const (
	NodeAlive NodeStatus = iota
	NodeSuspected
	NodeDead
	NodeLeaving
	NodeUpdating
)

// String returns the string representation of NodeStatus
func (s NodeStatus) String() string {
	switch s {
	case NodeAlive:
		return "alive"
	case NodeSuspected:
		return "suspected"
	case NodeDead:
		return "dead"
	case NodeLeaving:
		return "leaving"
	case NodeUpdating:
		return "updating"
	default:
		return "unknown"
	}
}

// Node represents a physical node in the cluster
type Node struct {
	ID       string
	Address  string
	Port     int
	Status   NodeStatus
	Load     float64   // Current load metric (0.0 - 1.0)
	LastSeen time.Time
	
	// Node capabilities
	SupportsFilters    bool
	SupportsCompression bool
	SupportsTLS        bool
}

// VirtualNode represents a virtual node on the hash ring
type VirtualNode struct {
	Hash     uint64 // Position on the ring
	NodeID   string // Physical node identifier
	VNodeID  int    // Virtual node number (0, 1, 2, ...)
}

// HashRingConfig holds configuration for the hash ring
type HashRingConfig struct {
	VirtualNodeCount     int    `yaml:"virtual_node_count" json:"virtual_node_count"`
	ReplicationFactor    int    `yaml:"replication_factor" json:"replication_factor"`
	HashFunction         string `yaml:"hash_function" json:"hash_function"`
	LookupCacheSize      int    `yaml:"lookup_cache_size" json:"lookup_cache_size"`
	HealthCheckInterval  int    `yaml:"health_check_interval_seconds" json:"health_check_interval_seconds"`
	NodeTimeoutThreshold int    `yaml:"node_timeout_threshold_seconds" json:"node_timeout_threshold_seconds"`
}

// DefaultHashRingConfig returns a production-ready default configuration
func DefaultHashRingConfig() HashRingConfig {
	return HashRingConfig{
		VirtualNodeCount:     256,  // Good balance of distribution and memory usage
		ReplicationFactor:    3,    // Standard fault tolerance
		HashFunction:         "xxhash64",
		LookupCacheSize:      10000, // Cache for hot keys
		HealthCheckInterval:  30,    // 30 seconds
		NodeTimeoutThreshold: 60,    // 60 seconds
	}
}

// HashRing implements consistent hashing with virtual nodes
type HashRing struct {
	// Core ring state
	vnodes        []VirtualNode     // Sorted by hash value
	nodes         map[string]*Node  // Physical nodes in cluster
	config        HashRingConfig
	
	// Performance optimizations
	lookupCache   map[string][]string // Cache: key -> replica nodes
	cacheKeys     []string            // LRU cache keys
	cacheIndex    int                 // Current cache position for LRU
	
	// Thread safety
	mu sync.RWMutex
	
	// Metrics
	lookupCount   int64
	cacheHitCount int64
	rebalanceCount int64
}

// NewHashRing creates a new hash ring with the given configuration
func NewHashRing(config HashRingConfig) *HashRing {
	return &HashRing{
		vnodes:      make([]VirtualNode, 0),
		nodes:       make(map[string]*Node),
		config:      config,
		lookupCache: make(map[string][]string),
		cacheKeys:   make([]string, config.LookupCacheSize),
	}
}

// hashFunction computes hash based on configured algorithm
func (ring *HashRing) hashFunction(data []byte) uint64 {
	switch ring.config.HashFunction {
	case "xxhash64":
		return xxhash.Sum64(data)
	case "sha256":
		hash := sha256.Sum256(data)
		return binary.BigEndian.Uint64(hash[:8])
	default:
		// Fallback to xxhash64
		return xxhash.Sum64(data)
	}
}

// AddNode adds a new physical node to the ring
func (ring *HashRing) AddNode(nodeID, address string, port int) error {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	
	// Check if node already exists
	if _, exists := ring.nodes[nodeID]; exists {
		return fmt.Errorf("node %s already exists", nodeID)
	}
	
	// Add physical node
	ring.nodes[nodeID] = &Node{
		ID:       nodeID,
		Address:  address,
		Port:     port,
		Status:   NodeAlive,
		Load:     0.0,
		LastSeen: time.Now(),
		
		// Default capabilities
		SupportsFilters:    true,
		SupportsCompression: true,
		SupportsTLS:        false,
	}
	
	// Create virtual nodes
	newVNodes := make([]VirtualNode, ring.config.VirtualNodeCount)
	for i := 0; i < ring.config.VirtualNodeCount; i++ {
		vNodeKey := fmt.Sprintf("%s:%d", nodeID, i)
		hash := ring.hashFunction([]byte(vNodeKey))
		
		newVNodes[i] = VirtualNode{
			Hash:    hash,
			NodeID:  nodeID,
			VNodeID: i,
		}
	}
	
	// Insert virtual nodes and maintain sorted order
	ring.vnodes = append(ring.vnodes, newVNodes...)
	sort.Slice(ring.vnodes, func(i, j int) bool {
		return ring.vnodes[i].Hash < ring.vnodes[j].Hash
	})
	
	// Clear lookup cache (ring topology changed)
	ring.clearLookupCache()
	
	return nil
}

// RemoveNode removes a physical node from the ring
func (ring *HashRing) RemoveNode(nodeID string) error {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	
	// Check if node exists
	if _, exists := ring.nodes[nodeID]; !exists {
		return fmt.Errorf("node %s does not exist", nodeID)
	}
	
	// Remove physical node
	delete(ring.nodes, nodeID)
	
	// Remove virtual nodes
	filteredVNodes := make([]VirtualNode, 0, len(ring.vnodes))
	for _, vnode := range ring.vnodes {
		if vnode.NodeID != nodeID {
			filteredVNodes = append(filteredVNodes, vnode)
		}
	}
	ring.vnodes = filteredVNodes
	
	// Clear lookup cache
	ring.clearLookupCache()
	
	return nil
}

// GetNode returns the primary node for a given key
func (ring *HashRing) GetNode(key string) string {
	ring.mu.Lock()
	ring.lookupCount++
	ring.mu.Unlock()
	
	replicas := ring.GetReplicas(key, 1)
	if len(replicas) == 0 {
		return ""
	}
	return replicas[0]
}

// GetReplicas returns the ordered list of replica nodes for a key
func (ring *HashRing) GetReplicas(key string, count int) []string {
	ring.mu.Lock()
	ring.lookupCount++
	
	// Check cache first
	if cached, exists := ring.lookupCache[key]; exists {
		ring.cacheHitCount++
		ring.mu.Unlock()
		
		// Return requested number of replicas
		if count >= len(cached) {
			return cached
		}
		return cached[:count]
	}
	ring.mu.Unlock()
	
	// Compute replicas
	replicas := ring.computeReplicas(key, ring.config.ReplicationFactor)
	
	// Cache the result
	ring.mu.Lock()
	ring.cacheLookupResult(key, replicas)
	ring.mu.Unlock()
	
	// Return requested number
	if count >= len(replicas) {
		return replicas
	}
	return replicas[:count]
}

// computeReplicas computes replica nodes without holding locks
func (ring *HashRing) computeReplicas(key string, count int) []string {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	
	if len(ring.vnodes) == 0 || count == 0 {
		return nil
	}
	
	// Hash the key to find position on ring
	keyHash := ring.hashFunction([]byte(key))
	
	// Find starting position using binary search
	startIdx := sort.Search(len(ring.vnodes), func(i int) bool {
		return ring.vnodes[i].Hash >= keyHash
	})
	
	// Wrap around if necessary
	if startIdx == len(ring.vnodes) {
		startIdx = 0
	}
	
	// Collect unique physical nodes
	seen := make(map[string]bool)
	replicas := make([]string, 0, count)
	
	for i := 0; i < len(ring.vnodes) && len(replicas) < count; i++ {
		idx := (startIdx + i) % len(ring.vnodes)
		nodeID := ring.vnodes[idx].NodeID
		
		// Only include alive nodes and avoid duplicates
		if node, exists := ring.nodes[nodeID]; exists && node.Status == NodeAlive && !seen[nodeID] {
			seen[nodeID] = true
			replicas = append(replicas, nodeID)
		}
	}
	
	return replicas
}

// cacheLookupResult caches a lookup result using simple LRU
func (ring *HashRing) cacheLookupResult(key string, replicas []string) {
	// Simple LRU: overwrite oldest entry
	if len(ring.cacheKeys) > 0 {
		oldKey := ring.cacheKeys[ring.cacheIndex]
		if oldKey != "" {
			delete(ring.lookupCache, oldKey)
		}
		
		ring.cacheKeys[ring.cacheIndex] = key
		ring.cacheIndex = (ring.cacheIndex + 1) % len(ring.cacheKeys)
	}
	
	ring.lookupCache[key] = replicas
}

// clearLookupCache clears the entire lookup cache
func (ring *HashRing) clearLookupCache() {
	ring.lookupCache = make(map[string][]string)
	for i := range ring.cacheKeys {
		ring.cacheKeys[i] = ""
	}
	ring.cacheIndex = 0
}

// GetNodes returns all nodes in the ring
func (ring *HashRing) GetNodes() map[string]*Node {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	
	// Return a copy to prevent external modifications
	nodesCopy := make(map[string]*Node)
	for id, node := range ring.nodes {
		nodesCopy[id] = &Node{
			ID:       node.ID,
			Address:  node.Address,
			Port:     node.Port,
			Status:   node.Status,
			Load:     node.Load,
			LastSeen: node.LastSeen,
			
			SupportsFilters:    node.SupportsFilters,
			SupportsCompression: node.SupportsCompression,
			SupportsTLS:        node.SupportsTLS,
		}
	}
	
	return nodesCopy
}

// SetNodeStatus updates the status of a node
func (ring *HashRing) SetNodeStatus(nodeID string, status NodeStatus) error {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	
	node, exists := ring.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s does not exist", nodeID)
	}
	
	oldStatus := node.Status
	node.Status = status
	node.LastSeen = time.Now()
	
	// Clear cache if node status affects availability
	if (oldStatus == NodeAlive && status != NodeAlive) || 
	   (oldStatus != NodeAlive && status == NodeAlive) {
		ring.clearLookupCache()
	}
	
	return nil
}

// UpdateNodeLoad updates the load metric for a node
func (ring *HashRing) UpdateNodeLoad(nodeID string, load float64) error {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	
	node, exists := ring.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s does not exist", nodeID)
	}
	
	node.Load = math.Max(0.0, math.Min(1.0, load)) // Clamp to [0, 1]
	node.LastSeen = time.Now()
	
	return nil
}

// DistributionStats provides statistics about key distribution
type DistributionStats struct {
	NodeLoads     map[string]int // Keys per node
	TotalKeys     int            // Total number of keys analyzed
	MinLoad       int            // Minimum keys on any node
	MaxLoad       int            // Maximum keys on any node
	AvgLoad       float64        // Average keys per node
	StdDeviation  float64        // Standard deviation of loads
	LoadFactor    float64        // MaxLoad / AvgLoad
}

// AnalyzeDistribution analyzes the distribution quality for a set of test keys
func (ring *HashRing) AnalyzeDistribution(testKeys []string) DistributionStats {
	nodeLoads := make(map[string]int)
	
	// Count keys per node
	for _, key := range testKeys {
		node := ring.GetNode(key)
		if node != "" {
			nodeLoads[node]++
		}
	}
	
	// Calculate statistics
	stats := DistributionStats{
		NodeLoads: nodeLoads,
		TotalKeys: len(testKeys),
		MinLoad:   math.MaxInt,
	}
	
	totalLoad := 0
	for _, load := range nodeLoads {
		totalLoad += load
		if load < stats.MinLoad {
			stats.MinLoad = load
		}
		if load > stats.MaxLoad {
			stats.MaxLoad = load
		}
	}
	
	if len(nodeLoads) > 0 {
		stats.AvgLoad = float64(totalLoad) / float64(len(nodeLoads))
		
		if stats.AvgLoad > 0 {
			stats.LoadFactor = float64(stats.MaxLoad) / stats.AvgLoad
		}
		
		// Calculate standard deviation
		variance := 0.0
		for _, load := range nodeLoads {
			diff := float64(load) - stats.AvgLoad
			variance += diff * diff
		}
		stats.StdDeviation = math.Sqrt(variance / float64(len(nodeLoads)))
	}
	
	if stats.MinLoad == math.MaxInt {
		stats.MinLoad = 0
	}
	
	return stats
}

// HashRingMetrics provides operational metrics for the hash ring
type HashRingMetrics struct {
	// Performance metrics
	LookupCount     int64   `json:"lookup_count"`
	CacheHitCount   int64   `json:"cache_hit_count"`
	CacheHitRate    float64 `json:"cache_hit_rate"`
	
	// Ring state metrics
	TotalNodes      int `json:"total_nodes"`
	AliveNodes      int `json:"alive_nodes"`
	SuspectedNodes  int `json:"suspected_nodes"`
	DeadNodes       int `json:"dead_nodes"`
	TotalVNodes     int `json:"total_vnodes"`
	
	// Rebalancing metrics
	RebalanceCount  int64 `json:"rebalance_count"`
	
	// Memory metrics
	CacheSize       int `json:"cache_size"`
	CacheCapacity   int `json:"cache_capacity"`
}

// GetMetrics returns current operational metrics
func (ring *HashRing) GetMetrics() HashRingMetrics {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	
	metrics := HashRingMetrics{
		LookupCount:     ring.lookupCount,
		CacheHitCount:   ring.cacheHitCount,
		TotalVNodes:     len(ring.vnodes),
		CacheSize:       len(ring.lookupCache),
		CacheCapacity:   ring.config.LookupCacheSize,
		RebalanceCount:  ring.rebalanceCount,
	}
	
	// Calculate cache hit rate
	if ring.lookupCount > 0 {
		metrics.CacheHitRate = float64(ring.cacheHitCount) / float64(ring.lookupCount)
	}
	
	// Count nodes by status
	for _, node := range ring.nodes {
		metrics.TotalNodes++
		switch node.Status {
		case NodeAlive:
			metrics.AliveNodes++
		case NodeSuspected:
			metrics.SuspectedNodes++
		case NodeDead:
			metrics.DeadNodes++
		}
	}
	
	return metrics
}

// IsEmpty returns true if the ring has no nodes
func (ring *HashRing) IsEmpty() bool {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	return len(ring.nodes) == 0
}

// NodeCount returns the number of physical nodes
func (ring *HashRing) NodeCount() int {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	return len(ring.nodes)
}

// VNodeCount returns the number of virtual nodes
func (ring *HashRing) VNodeCount() int {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	return len(ring.vnodes)
}
