package cluster

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestNewHashRing(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	if ring == nil {
		t.Fatal("NewHashRing returned nil")
	}

	if len(ring.nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(ring.nodes))
	}

	if len(ring.vnodes) != 0 {
		t.Errorf("Expected 0 virtual nodes, got %d", len(ring.vnodes))
	}

	if ring.IsEmpty() != true {
		t.Error("New ring should be empty")
	}
}

func TestAddNode(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add first node
	err := ring.AddNode("node1", "192.168.1.1", 6379)
	if err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}

	if ring.NodeCount() != 1 {
		t.Errorf("Expected 1 node, got %d", ring.NodeCount())
	}

	if ring.VNodeCount() != config.VirtualNodeCount {
		t.Errorf("Expected %d virtual nodes, got %d", config.VirtualNodeCount, ring.VNodeCount())
	}

	// Verify node details
	nodes := ring.GetNodes()
	node1, exists := nodes["node1"]
	if !exists {
		t.Fatal("node1 not found in ring")
	}

	if node1.Address != "192.168.1.1" {
		t.Errorf("Expected address 192.168.1.1, got %s", node1.Address)
	}

	if node1.Port != 6379 {
		t.Errorf("Expected port 6379, got %d", node1.Port)
	}

	if node1.Status != NodeAlive {
		t.Errorf("Expected status NodeAlive, got %v", node1.Status)
	}

	// Try to add duplicate node
	err = ring.AddNode("node1", "192.168.1.2", 6380)
	if err == nil {
		t.Error("Expected error when adding duplicate node")
	}
}

func TestRemoveNode(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add nodes
	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)

	if ring.NodeCount() != 2 {
		t.Fatalf("Expected 2 nodes, got %d", ring.NodeCount())
	}

	// Remove node1
	err := ring.RemoveNode("node1")
	if err != nil {
		t.Fatalf("Failed to remove node1: %v", err)
	}

	if ring.NodeCount() != 1 {
		t.Errorf("Expected 1 node after removal, got %d", ring.NodeCount())
	}

	if ring.VNodeCount() != config.VirtualNodeCount {
		t.Errorf("Expected %d virtual nodes after removal, got %d", config.VirtualNodeCount, ring.VNodeCount())
	}

	// Try to remove non-existent node
	err = ring.RemoveNode("node3")
	if err == nil {
		t.Error("Expected error when removing non-existent node")
	}

	// Verify node1 is gone
	nodes := ring.GetNodes()
	if _, exists := nodes["node1"]; exists {
		t.Error("node1 should be removed from ring")
	}
}

func TestGetNode(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Empty ring
	node := ring.GetNode("testkey")
	if node != "" {
		t.Errorf("Expected empty string for empty ring, got %s", node)
	}

	// Add nodes
	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)
	ring.AddNode("node3", "192.168.1.3", 6379)

	// Test key routing
	testKeys := []string{
		"user:1", "user:2", "user:3",
		"session:abc", "session:def", "session:ghi",
		"product:123", "product:456", "product:789",
	}

	keyToNode := make(map[string]string)
	for _, key := range testKeys {
		node := ring.GetNode(key)
		if node == "" {
			t.Errorf("No node returned for key %s", key)
		}
		keyToNode[key] = node
		t.Logf("Key %s -> Node %s", key, node)
	}

	// Same key should always map to same node
	for _, key := range testKeys {
		for i := 0; i < 10; i++ {
			node := ring.GetNode(key)
			if node != keyToNode[key] {
				t.Errorf("Inconsistent mapping for key %s: expected %s, got %s",
					key, keyToNode[key], node)
			}
		}
	}
}

func TestGetReplicas(t *testing.T) {
	config := DefaultHashRingConfig()
	config.ReplicationFactor = 3
	ring := NewHashRing(config)

	// Add nodes
	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)
	ring.AddNode("node3", "192.168.1.3", 6379)
	ring.AddNode("node4", "192.168.1.4", 6379)

	// Test replica selection
	replicas := ring.GetReplicas("testkey", 3)
	if len(replicas) != 3 {
		t.Errorf("Expected 3 replicas, got %d", len(replicas))
	}

	// Check for uniqueness
	seen := make(map[string]bool)
	for _, replica := range replicas {
		if seen[replica] {
			t.Errorf("Duplicate replica: %s", replica)
		}
		seen[replica] = true
	}

	// Primary node should be first
	primary := ring.GetNode("testkey")
	if len(replicas) > 0 && replicas[0] != primary {
		t.Errorf("Primary node mismatch: expected %s, got %s", primary, replicas[0])
	}

	// Test with fewer requested replicas
	oneReplica := ring.GetReplicas("testkey", 1)
	if len(oneReplica) != 1 {
		t.Errorf("Expected 1 replica, got %d", len(oneReplica))
	}

	if oneReplica[0] != replicas[0] {
		t.Error("First replica should match primary node")
	}
}

func TestConsistentHashing(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add initial nodes
	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)
	ring.AddNode("node3", "192.168.1.3", 6379)

	// Generate test keys and record initial mapping
	testKeys := make([]string, 1000)
	initialMapping := make(map[string]string)

	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key:%d", i)
		testKeys[i] = key
		initialMapping[key] = ring.GetNode(key)
	}

	// Add a new node
	ring.AddNode("node4", "192.168.1.4", 6379)

	// Check how many keys moved
	movedKeys := 0
	for _, key := range testKeys {
		newNode := ring.GetNode(key)
		if newNode != initialMapping[key] {
			movedKeys++
		}
	}

	// With consistent hashing, we expect approximately 1/4 of keys to move
	// (since we went from 3 to 4 nodes)
	expectedMoved := len(testKeys) / 4
	tolerance := int(float64(expectedMoved) * 0.3) // 30% tolerance

	if movedKeys < expectedMoved-tolerance || movedKeys > expectedMoved+tolerance {
		t.Errorf("Too many keys moved: expected ~%d (±%d), got %d",
			expectedMoved, tolerance, movedKeys)
	}

	t.Logf("Keys moved when adding node4: %d/%d (%.1f%%)",
		movedKeys, len(testKeys), float64(movedKeys)/float64(len(testKeys))*100)
}

func TestNodeStatus(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)

	// Test setting node status
	err := ring.SetNodeStatus("node1", NodeSuspected)
	if err != nil {
		t.Fatalf("Failed to set node status: %v", err)
	}

	nodes := ring.GetNodes()
	if nodes["node1"].Status != NodeSuspected {
		t.Error("Node status not updated")
	}

	// Test with non-existent node
	err = ring.SetNodeStatus("node3", NodeDead)
	if err == nil {
		t.Error("Expected error for non-existent node")
	}

	// Mark node as dead and verify it's not returned in replicas
	ring.SetNodeStatus("node1", NodeDead)

	// With node1 dead, all keys should go to node2
	node := ring.GetNode("testkey")
	if node != "node2" {
		t.Errorf("Expected node2 for dead node1 scenario, got %s", node)
	}
}

func TestLoadUpdates(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	ring.AddNode("node1", "192.168.1.1", 6379)

	// Test updating load
	err := ring.UpdateNodeLoad("node1", 0.75)
	if err != nil {
		t.Fatalf("Failed to update node load: %v", err)
	}

	nodes := ring.GetNodes()
	if nodes["node1"].Load != 0.75 {
		t.Errorf("Expected load 0.75, got %f", nodes["node1"].Load)
	}

	// Test load clamping
	ring.UpdateNodeLoad("node1", 1.5) // Should clamp to 1.0
	nodes = ring.GetNodes()
	if nodes["node1"].Load != 1.0 {
		t.Errorf("Expected load clamped to 1.0, got %f", nodes["node1"].Load)
	}

	ring.UpdateNodeLoad("node1", -0.5) // Should clamp to 0.0
	nodes = ring.GetNodes()
	if nodes["node1"].Load != 0.0 {
		t.Errorf("Expected load clamped to 0.0, got %f", nodes["node1"].Load)
	}
}

func TestDistributionAnalysis(t *testing.T) {
	config := DefaultHashRingConfig()
	config.VirtualNodeCount = 256 // Good distribution
	ring := NewHashRing(config)

	// Add nodes
	nodeNames := []string{"node1", "node2", "node3", "node4", "node5"}
	for i, name := range nodeNames {
		ring.AddNode(name, fmt.Sprintf("192.168.1.%d", i+1), 6379)
	}

	// Generate test keys
	testKeys := make([]string, 10000)
	for i := 0; i < 10000; i++ {
		testKeys[i] = fmt.Sprintf("key:%d", i)
	}

	// Analyze distribution
	stats := ring.AnalyzeDistribution(testKeys)

	t.Logf("Distribution Analysis:")
	t.Logf("  Total Keys: %d", stats.TotalKeys)
	t.Logf("  Average Load: %.2f", stats.AvgLoad)
	t.Logf("  Min Load: %d", stats.MinLoad)
	t.Logf("  Max Load: %d", stats.MaxLoad)
	t.Logf("  Load Factor: %.2f", stats.LoadFactor)
	t.Logf("  Std Deviation: %.2f", stats.StdDeviation)

	for node, load := range stats.NodeLoads {
		t.Logf("  %s: %d keys (%.1f%%)", node, load,
			float64(load)/float64(stats.TotalKeys)*100)
	}

	// Check that distribution is reasonably uniform
	expectedAvg := float64(len(testKeys)) / float64(len(nodeNames))
	if math.Abs(stats.AvgLoad-expectedAvg) > 1.0 {
		t.Errorf("Average load significantly off: expected %.2f, got %.2f",
			expectedAvg, stats.AvgLoad)
	}

	// Load factor should be reasonable (< 2.0 indicates good distribution)
	if stats.LoadFactor > 2.0 {
		t.Errorf("Load factor too high: %f (indicates poor distribution)", stats.LoadFactor)
	}

	// Standard deviation should be reasonable
	maxStdDev := expectedAvg * 0.2 // 20% of average
	if stats.StdDeviation > maxStdDev {
		t.Errorf("Standard deviation too high: %.2f (max expected: %.2f)",
			stats.StdDeviation, maxStdDev)
	}
}

func TestLookupCache(t *testing.T) {
	config := DefaultHashRingConfig()
	config.LookupCacheSize = 5 // Small cache for testing
	ring := NewHashRing(config)

	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)

	// First lookup should miss cache
	initialMetrics := ring.GetMetrics()

	replicas1 := ring.GetReplicas("key1", 2)
	if len(replicas1) != 2 {
		t.Errorf("Expected 2 replicas, got %d", len(replicas1))
	}

	// Second lookup should hit cache
	replicas2 := ring.GetReplicas("key1", 2)
	if len(replicas2) != 2 {
		t.Errorf("Expected 2 replicas from cache, got %d", len(replicas2))
	}

	// Verify cache hit
	metrics := ring.GetMetrics()
	expectedLookups := initialMetrics.LookupCount + 2
	expectedHits := initialMetrics.CacheHitCount + 1

	if metrics.LookupCount != expectedLookups {
		t.Errorf("Expected %d lookups, got %d", expectedLookups, metrics.LookupCount)
	}

	if metrics.CacheHitCount != expectedHits {
		t.Errorf("Expected %d cache hits, got %d", expectedHits, metrics.CacheHitCount)
	}

	// Test cache eviction by filling it beyond capacity
	for i := 0; i < config.LookupCacheSize+2; i++ {
		ring.GetReplicas(fmt.Sprintf("key%d", i), 1)
	}

	finalMetrics := ring.GetMetrics()
	if finalMetrics.CacheSize > config.LookupCacheSize {
		t.Errorf("Cache size exceeded capacity: %d > %d",
			finalMetrics.CacheSize, config.LookupCacheSize)
	}
}

func TestHashFunctions(t *testing.T) {
	// Test different hash functions
	hashFunctions := []string{"xxhash64", "sha256", "invalid"}

	for _, hashFunc := range hashFunctions {
		config := DefaultHashRingConfig()
		config.HashFunction = hashFunc

		ring := NewHashRing(config)
		ring.AddNode("node1", "192.168.1.1", 6379)

		// Should work even with invalid hash function (falls back to xxhash64)
		node := ring.GetNode("testkey")
		if node != "node1" {
			t.Errorf("Hash function %s failed: got node %s", hashFunc, node)
		}

		t.Logf("Hash function %s: working", hashFunc)
	}
}

func TestConcurrentOperations(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add initial nodes
	for i := 1; i <= 3; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i), fmt.Sprintf("192.168.1.%d", i), 6379)
	}

	// Run concurrent lookups and modifications
	done := make(chan bool, 1)

	// Lookup goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("key:%d", i%100)
			ring.GetNode(key)
			ring.GetReplicas(key, 2)
		}
		done <- true
	}()

	// Status update goroutine
	go func() {
		for i := 0; i < 100; i++ {
			nodeID := fmt.Sprintf("node%d", (i%3)+1)
			status := NodeAlive
			if i%10 == 0 {
				status = NodeSuspected
			}
			ring.SetNodeStatus(nodeID, status)
			ring.UpdateNodeLoad(nodeID, float64(i%100)/100.0)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify ring is still functional
	metrics := ring.GetMetrics()
	if metrics.TotalNodes != 3 {
		t.Errorf("Expected 3 nodes after concurrent operations, got %d", metrics.TotalNodes)
	}

	// All lookups should still work
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("testkey%d", i)
		node := ring.GetNode(key)
		if node == "" {
			t.Errorf("No node returned for key %s after concurrent operations", key)
		}
	}
}

func TestMetrics(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	ring.AddNode("node1", "192.168.1.1", 6379)
	ring.AddNode("node2", "192.168.1.2", 6379)
	ring.SetNodeStatus("node1", NodeSuspected)

	metrics := ring.GetMetrics()

	if metrics.TotalNodes != 2 {
		t.Errorf("Expected 2 total nodes, got %d", metrics.TotalNodes)
	}

	if metrics.AliveNodes != 1 {
		t.Errorf("Expected 1 alive node, got %d", metrics.AliveNodes)
	}

	if metrics.SuspectedNodes != 1 {
		t.Errorf("Expected 1 suspected node, got %d", metrics.SuspectedNodes)
	}

	if metrics.TotalVNodes != config.VirtualNodeCount*2 {
		t.Errorf("Expected %d virtual nodes, got %d",
			config.VirtualNodeCount*2, metrics.TotalVNodes)
	}

	// Generate some cache activity using GetReplicas directly
	for i := 0; i < 10; i++ {
		ring.GetReplicas(fmt.Sprintf("key%d", i), 1) // First call - cache miss
		ring.GetReplicas(fmt.Sprintf("key%d", i), 1) // Second call - cache hit
	}

	metrics = ring.GetMetrics()
	if metrics.LookupCount != 20 {
		t.Errorf("Expected 20 lookups, got %d", metrics.LookupCount)
	}

	if metrics.CacheHitCount != 10 {
		t.Errorf("Expected 10 cache hits, got %d", metrics.CacheHitCount)
	}

	if metrics.CacheHitRate != 0.5 {
		t.Errorf("Expected 0.5 cache hit rate, got %f", metrics.CacheHitRate)
	}
}

// Benchmark tests
func BenchmarkGetNode(b *testing.B) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add nodes
	for i := 1; i <= 10; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i), fmt.Sprintf("192.168.1.%d", i), 6379)
	}

	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ring.GetNode(keys[i%len(keys)])
			i++
		}
	})
}

func BenchmarkGetReplicas(b *testing.B) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add nodes
	for i := 1; i <= 10; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i), fmt.Sprintf("192.168.1.%d", i), 6379)
	}

	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ring.GetReplicas(keys[i%len(keys)], 3)
			i++
		}
	})
}

func BenchmarkAddRemoveNode(b *testing.B) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		ring.AddNode(nodeID, "192.168.1.1", 6379)
		ring.RemoveNode(nodeID)
	}
}

// Example usage
func ExampleHashRing() {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add nodes to cluster
	ring.AddNode("redis-1", "10.0.1.1", 6379)
	ring.AddNode("redis-2", "10.0.1.2", 6379)
	ring.AddNode("redis-3", "10.0.1.3", 6379)

	// Route keys to nodes
	userKey := "user:12345"
	primaryNode := ring.GetNode(userKey)
	replicas := ring.GetReplicas(userKey, 2) // Primary + 1 backup

	fmt.Printf("Key %s -> Primary: %s, Replicas: %v\n", userKey, primaryNode, replicas)

	// Add new node (triggers rebalancing)
	ring.AddNode("redis-4", "10.0.1.4", 6379)

	// Check if key moved
	newPrimary := ring.GetNode(userKey)
	fmt.Printf("After adding node, key %s -> Primary: %s\n", userKey, newPrimary)

	// Analyze distribution
	testKeys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		testKeys[i] = fmt.Sprintf("key:%d", i)
	}

	stats := ring.AnalyzeDistribution(testKeys)
	fmt.Printf("Distribution stats: avg=%.1f, load_factor=%.2f\n",
		stats.AvgLoad, stats.LoadFactor)
}
