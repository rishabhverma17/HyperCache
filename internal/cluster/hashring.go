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

// Redis-compatible hash slot constants
const (
	RedisHashSlots = 16384 // Redis cluster uses 16384 hash slots
)

// CRC16 table for Redis-compatible hash slot calculation
var crc16Table = [256]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50a5, 0x60c6, 0x70e7,
	0x8108, 0x9129, 0xa14a, 0xb16b, 0xc18c, 0xd1ad, 0xe1ce, 0xf1ef,
	0x1231, 0x0210, 0x3273, 0x2252, 0x52b5, 0x4294, 0x72f7, 0x62d6,
	0x9339, 0x8318, 0xb37b, 0xa35a, 0xd3bd, 0xc39c, 0xf3ff, 0xe3de,
	0x2462, 0x3443, 0x0420, 0x1401, 0x64e6, 0x74c7, 0x44a4, 0x5485,
	0xa56a, 0xb54b, 0x8528, 0x9509, 0xe5ee, 0xf5cf, 0xc5ac, 0xd58d,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76d7, 0x66f6, 0x5695, 0x46b4,
	0xb75b, 0xa77a, 0x9719, 0x8738, 0xf7df, 0xe7fe, 0xd79d, 0xc7bc,
	0x48c4, 0x58e5, 0x6886, 0x78a7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xc9cc, 0xd9ed, 0xe98e, 0xf9af, 0x8948, 0x9969, 0xa90a, 0xb92b,
	0x5af5, 0x4ad4, 0x7ab7, 0x6a96, 0x1a71, 0x0a50, 0x3a33, 0x2a12,
	0xdbfd, 0xcbdc, 0xfbbf, 0xeb9e, 0x9b79, 0x8b58, 0xbb3b, 0xab1a,
	0x6ca6, 0x7c87, 0x4ce4, 0x5cc5, 0x2c22, 0x3c03, 0x0c60, 0x1c41,
	0xedae, 0xfd8f, 0xcdec, 0xddcd, 0xad2a, 0xbd0b, 0x8d68, 0x9d49,
	0x7e97, 0x6eb6, 0x5ed5, 0x4ef4, 0x3e13, 0x2e32, 0x1e51, 0x0e70,
	0xff9f, 0xefbe, 0xdfdd, 0xcffc, 0xbf1b, 0xaf3a, 0x9f59, 0x8f78,
	0x9188, 0x81a9, 0xb1ca, 0xa1eb, 0xd10c, 0xc12d, 0xf14e, 0xe16f,
	0x1080, 0x00a1, 0x30c2, 0x20e3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83b9, 0x9398, 0xa3fb, 0xb3da, 0xc33d, 0xd31c, 0xe37f, 0xf35e,
	0x02b1, 0x1290, 0x22f3, 0x32d2, 0x4235, 0x5214, 0x6277, 0x7256,
	0xb5ea, 0xa5cb, 0x95a8, 0x8589, 0xf56e, 0xe54f, 0xd52c, 0xc50d,
	0x34e2, 0x24c3, 0x14a0, 0x0481, 0x7466, 0x6447, 0x5424, 0x4405,
	0xa7db, 0xb7fa, 0x8799, 0x97b8, 0xe75f, 0xf77e, 0xc71d, 0xd73c,
	0x26d3, 0x36f2, 0x0691, 0x16b0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xd94c, 0xc96d, 0xf90e, 0xe92f, 0x99c8, 0x89e9, 0xb98a, 0xa9ab,
	0x5844, 0x4865, 0x7806, 0x6827, 0x18c0, 0x08e1, 0x3882, 0x28a3,
	0xcb7d, 0xdb5c, 0xeb3f, 0xfb1e, 0x8bf9, 0x9bd8, 0xabbb, 0xbb9a,
	0x4a75, 0x5a54, 0x6a37, 0x7a16, 0x0af1, 0x1ad0, 0x2ab3, 0x3a92,
	0xfd2e, 0xed0f, 0xdd6c, 0xcd4d, 0xbdaa, 0xad8b, 0x9de8, 0x8dc9,
	0x7c26, 0x6c07, 0x5c64, 0x4c45, 0x3ca2, 0x2c83, 0x1ce0, 0x0cc1,
	0xef1f, 0xff3e, 0xcf5d, 0xdf7c, 0xaf9b, 0xbfba, 0x8fd9, 0x9ff8,
	0x6e17, 0x7e36, 0x4e55, 0x5e74, 0x2e93, 0x3eb2, 0x0ed1, 0x1ef0,
}

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
	Load     float64 // Current load metric (0.0 - 1.0)
	LastSeen time.Time

	// Node capabilities
	SupportsFilters     bool
	SupportsCompression bool
	SupportsTLS         bool
}

// VirtualNode represents a virtual node on the hash ring
type VirtualNode struct {
	Hash    uint64 // Position on the ring
	NodeID  string // Physical node identifier
	VNodeID int    // Virtual node number (0, 1, 2, ...)
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
		VirtualNodeCount:     256, // Good balance of distribution and memory usage
		ReplicationFactor:    3,   // Standard fault tolerance
		HashFunction:         "xxhash64",
		LookupCacheSize:      10000, // Cache for hot keys
		HealthCheckInterval:  30,    // 30 seconds
		NodeTimeoutThreshold: 60,    // 60 seconds
	}
}

// HashRing implements consistent hashing with virtual nodes
type HashRing struct {
	// Core ring state
	vnodes []VirtualNode    // Sorted by hash value
	nodes  map[string]*Node // Physical nodes in cluster
	config HashRingConfig

	// Redis-compatible hash slots
	slotMap [RedisHashSlots]string // Maps hash slots to node IDs

	// Performance optimizations
	lookupCache map[string][]string // Cache: key -> replica nodes
	cacheKeys   []string            // LRU cache keys
	cacheIndex  int                 // Current cache position for LRU

	// Thread safety
	mu sync.RWMutex

	// Metrics
	lookupCount    int64
	cacheHitCount  int64
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

// crc16 computes CRC16 checksum using the XMODEM polynomial (Redis-compatible)
func crc16(data []byte) uint16 {
	var crc uint16 = 0
	for _, b := range data {
		crc = (crc<<8 ^ crc16Table[((crc>>8)^uint16(b))&0xFF])
	}
	return crc
}

// GetHashSlot calculates the Redis-compatible hash slot for a key
func GetHashSlot(key string) uint16 {
	// Redis hash slot calculation: CRC16(key) % 16384
	// Look for hash tags {...} in the key
	keyBytes := []byte(key)
	
	// Check for hash tags in the format {tag}
	start := -1
	for i, b := range keyBytes {
		if b == '{' {
			start = i + 1
			break
		}
	}
	
	if start != -1 {
		// Found opening brace, look for closing brace
		for i := start; i < len(keyBytes); i++ {
			if keyBytes[i] == '}' {
				// Found hash tag, use content between braces
				if i > start {
					keyBytes = keyBytes[start:i]
				}
				break
			}
		}
	}
	
	return crc16(keyBytes) % RedisHashSlots
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
		SupportsFilters:     true,
		SupportsCompression: true,
		SupportsTLS:         false,
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
	
	// Redistribute hash slots for Redis-compatible routing
	ring.redistributeSlots()

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
	
	// Redistribute hash slots for Redis-compatible routing
	ring.redistributeSlots()

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

			SupportsFilters:     node.SupportsFilters,
			SupportsCompression: node.SupportsCompression,
			SupportsTLS:         node.SupportsTLS,
		}
	}

	return nodesCopy
}

// GetNodeBySlot returns the node responsible for a given hash slot
func (ring *HashRing) GetNodeBySlot(slot uint16) string {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	
	if slot >= RedisHashSlots {
		return ""
	}
	
	return ring.slotMap[slot]
}

// GetNodeByKey returns the node responsible for a given key using hash slots
func (ring *HashRing) GetNodeByKey(key string) string {
	slot := GetHashSlot(key)
	return ring.GetNodeBySlot(slot)
}

// GetSlotsByNode returns all slots assigned to a given node
func (ring *HashRing) GetSlotsByNode(nodeID string) []uint16 {
	ring.mu.RLock()
	defer ring.mu.RUnlock()
	
	var slots []uint16
	for slot, assignedNodeID := range ring.slotMap {
		if assignedNodeID == nodeID {
			slots = append(slots, uint16(slot))
		}
	}
	
	return slots
}

// redistributeSlots redistributes hash slots evenly among all alive nodes
func (ring *HashRing) redistributeSlots() {
	// This method should be called with ring.mu locked
	
	// Get all alive nodes
	var aliveNodes []string
	for nodeID, node := range ring.nodes {
		if node.Status == NodeAlive {
			aliveNodes = append(aliveNodes, nodeID)
		}
	}
	
	if len(aliveNodes) == 0 {
		// No alive nodes, clear all slots
		for i := range ring.slotMap {
			ring.slotMap[i] = ""
		}
		return
	}
	
	// Distribute slots evenly among alive nodes
	slotsPerNode := RedisHashSlots / len(aliveNodes)
	remainder := RedisHashSlots % len(aliveNodes)
	
	slot := 0
	for i, nodeID := range aliveNodes {
		// Calculate number of slots for this node
		nodeSlots := slotsPerNode
		if i < remainder {
			nodeSlots++
		}
		
		// Assign slots to this node
		for j := 0; j < nodeSlots && slot < RedisHashSlots; j++ {
			ring.slotMap[slot] = nodeID
			slot++
		}
	}
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
	NodeLoads    map[string]int // Keys per node
	TotalKeys    int            // Total number of keys analyzed
	MinLoad      int            // Minimum keys on any node
	MaxLoad      int            // Maximum keys on any node
	AvgLoad      float64        // Average keys per node
	StdDeviation float64        // Standard deviation of loads
	LoadFactor   float64        // MaxLoad / AvgLoad
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
	LookupCount   int64   `json:"lookup_count"`
	CacheHitCount int64   `json:"cache_hit_count"`
	CacheHitRate  float64 `json:"cache_hit_rate"`

	// Ring state metrics
	TotalNodes     int `json:"total_nodes"`
	AliveNodes     int `json:"alive_nodes"`
	SuspectedNodes int `json:"suspected_nodes"`
	DeadNodes      int `json:"dead_nodes"`
	TotalVNodes    int `json:"total_vnodes"`

	// Rebalancing metrics
	RebalanceCount int64 `json:"rebalance_count"`

	// Memory metrics
	CacheSize     int `json:"cache_size"`
	CacheCapacity int `json:"cache_capacity"`
}

// GetMetrics returns current operational metrics
func (ring *HashRing) GetMetrics() HashRingMetrics {
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	metrics := HashRingMetrics{
		LookupCount:    ring.lookupCount,
		CacheHitCount:  ring.cacheHitCount,
		TotalVNodes:    len(ring.vnodes),
		CacheSize:      len(ring.lookupCache),
		CacheCapacity:  ring.config.LookupCacheSize,
		RebalanceCount: ring.rebalanceCount,
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
