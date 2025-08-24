package cluster_test

import (
	"context"
	"testing"
	"time"

	"hypercache/internal/cluster"
)

func TestHashRing(t *testing.T) {
	t.Run("Basic_Hash_Ring_Operations", func(t *testing.T) {
		config := cluster.DefaultHashRingConfig()
		ring := cluster.NewHashRing(config)
		
		// Add nodes with proper parameters (nodeID, address, port)
		nodes := []struct {
			id      string
			address string
			port    int
		}{
			{"node1", "192.168.1.1", 6379},
			{"node2", "192.168.1.2", 6379},
			{"node3", "192.168.1.3", 6379},
		}
		
		for _, node := range nodes {
			err := ring.AddNode(node.id, node.address, node.port)
			if err != nil {
				t.Fatalf("Failed to add node %s: %v", node.id, err)
			}
		}
		
		// Test node count
		if ring.NodeCount() != 3 {
			t.Errorf("Expected 3 nodes, got %d", ring.NodeCount())
		}
		
		// Test key distribution
		key1 := "user:1"
		key2 := "user:2"
		key3 := "user:3"
		
		node1 := ring.GetNode(key1)
		node2 := ring.GetNode(key2)
		node3 := ring.GetNode(key3)
		
		if node1 == "" || node2 == "" || node3 == "" {
			t.Errorf("All keys should map to valid nodes")
		}
		
		// Test consistency - same key should always map to same node
		for i := 0; i < 10; i++ {
			if ring.GetNode(key1) != node1 {
				t.Errorf("Key mapping should be consistent")
			}
		}
	})

	t.Run("Node_Removal", func(t *testing.T) {
		config := cluster.DefaultHashRingConfig()
		ring := cluster.NewHashRing(config)
		
		// Add nodes
		ring.AddNode("node1", "192.168.1.1", 6379)
		ring.AddNode("node2", "192.168.1.2", 6379)
		ring.AddNode("node3", "192.168.1.3", 6379)
		
		// Map some keys
		key := "test-key"
		originalNode := ring.GetNode(key)
		
		// Remove a different node
		ring.RemoveNode("node2")
		
		// Key should still map to the same node (if it wasn't node2)
		newNode := ring.GetNode(key)
		if originalNode != "node2" && newNode != originalNode {
			t.Errorf("Key should map to same node after removing different node")
		}
		
		// Verify node count
		if ring.NodeCount() != 2 {
			t.Errorf("Expected 2 nodes after removal, got %d", ring.NodeCount())
		}
	})

	t.Run("Hash_Ring_Rebalancing", func(t *testing.T) {
		config := cluster.DefaultHashRingConfig()
		ring := cluster.NewHashRing(config)
		
		// Add initial nodes
		ring.AddNode("node1", "192.168.1.1", 6379)
		ring.AddNode("node2", "192.168.1.2", 6379)
		
		// Map keys before adding new node
		keys := []string{"key1", "key2", "key3", "key4", "key5"}
		originalMapping := make(map[string]string)
		
		for _, key := range keys {
			originalMapping[key] = ring.GetNode(key)
		}
		
		// Add new node
		ring.AddNode("node3", "192.168.1.3", 6379)
		
		// Check how many keys moved
		movedKeys := 0
		for _, key := range keys {
			if ring.GetNode(key) != originalMapping[key] {
				movedKeys++
			}
		}
		
		// Some keys should move, but not all (good hash distribution)
		if movedKeys == 0 {
			t.Errorf("Expected some keys to move to new node")
		}
		if movedKeys == len(keys) {
			t.Errorf("Expected some keys to remain on original nodes")
		}
		
		t.Logf("Rebalancing moved %d out of %d keys", movedKeys, len(keys))
	})

	t.Run("Virtual_Nodes", func(t *testing.T) {
		config := cluster.DefaultHashRingConfig()
		ring := cluster.NewHashRing(config)
		
		// Add nodes
		ring.AddNode("node1", "192.168.1.1", 6379)
		ring.AddNode("node2", "192.168.1.2", 6379)
		
		// Test virtual node distribution
		keyDistribution := make(map[string]int)
		
		// Map many keys to see distribution
		for i := 0; i < 1000; i++ {
			key := "key-" + string(rune('0'+(i%10))) + "-" + string(rune('0'+(i/10%10))) + "-" + string(rune('0'+(i/100)))
			node := ring.GetNode(key)
			keyDistribution[node]++
		}
		
		// Check that distribution is reasonably balanced
		for node, count := range keyDistribution {
			t.Logf("Node %s: %d keys", node, count)
			if count < 300 || count > 700 { // Allow 30-70% distribution
				t.Errorf("Node %s has unbalanced distribution: %d keys", node, count)
			}
		}
	})
}

func TestGossipMembership(t *testing.T) {
	t.Run("Basic_Membership", func(t *testing.T) {
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		config.BindPort = 9000
		
		membership, err := cluster.NewGossipMembership(config)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}
		
		// Start membership
		ctx := context.Background()
		err = membership.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start membership: %v", err)
		}
		defer membership.Stop(ctx)
		
		// Check initial state
		members := membership.GetMembers()
		if len(members) != 1 {
			t.Errorf("Expected 1 member initially (self), got %d", len(members))
		}
		
		if members[0].NodeID != "node1" {
			t.Errorf("Expected self node to be node1, got %s", members[0].NodeID)
		}
	})

	t.Run("Node_Join", func(t *testing.T) {
		// Create first node
		config1 := cluster.DefaultClusterConfig()
		config1.NodeID = "node1"
		config1.BindPort = 9001
		
		membership1, err := cluster.NewGossipMembership(config1)
		if err != nil {
			t.Fatalf("Failed to create node1: %v", err)
		}
		
		ctx := context.Background()
		err = membership1.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start node1: %v", err)
		}
		defer membership1.Stop(ctx)
		
		// Create second node and join
		config2 := cluster.DefaultClusterConfig()
		config2.NodeID = "node2"
		config2.BindPort = 9002
		
		membership2, err := cluster.NewGossipMembership(config2)
		if err != nil {
			t.Fatalf("Failed to create node2: %v", err)
		}
		
		err = membership2.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start node2: %v", err)
		}
		defer membership2.Stop(ctx)
		
		// Join node2 to node1
		err = membership2.Join(ctx, []string{"127.0.0.1:9001"})
		if err != nil {
			t.Fatalf("Failed to join cluster: %v", err)
		}
		
		// Wait for gossip to propagate
		time.Sleep(2 * time.Second)
		
		// Both nodes should see each other
		members1 := membership1.GetMembers()
		members2 := membership2.GetMembers()
		
		if len(members1) != 2 {
			t.Errorf("Node1 should see 2 members, got %d", len(members1))
		}
		
		if len(members2) != 2 {
			t.Errorf("Node2 should see 2 members, got %d", len(members2))
		}
	})

	t.Run("Node_Failure_Detection", func(t *testing.T) {
		// Create and start nodes
		config1 := cluster.DefaultClusterConfig()
		config1.NodeID = "node1"
		config1.BindPort = 9003
		
		config2 := cluster.DefaultClusterConfig()
		config2.NodeID = "node2"
		config2.BindPort = 9004
		
		membership1, err := cluster.NewGossipMembership(config1)
		if err != nil {
			t.Fatalf("Failed to create node1: %v", err)
		}
		
		membership2, err := cluster.NewGossipMembership(config2)
		if err != nil {
			t.Fatalf("Failed to create node2: %v", err)
		}
		
		ctx := context.Background()
		membership1.Start(ctx)
		membership2.Start(ctx)
		
		// Join cluster
		membership2.Join(ctx, []string{"127.0.0.1:9003"})
		time.Sleep(1 * time.Second)
		
		// Stop node2 abruptly
		membership2.Stop(ctx)
		
		// Wait for failure detection
		time.Sleep(5 * time.Second)
		
		// Node1 should detect node2 failure
		members := membership1.GetMembers()
		aliveCount := 0
		for _, member := range members {
			if member.Status == cluster.NodeAlive {
				aliveCount++
			}
		}
		
		if aliveCount != 1 {
			t.Errorf("Expected 1 alive member after failure, got %d", aliveCount)
		}
		
		membership1.Stop(ctx)
	})
}

func TestSimpleCoordinator(t *testing.T) {
	t.Run("Basic_Coordination", func(t *testing.T) {
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		
		coordinator, err := cluster.NewSimpleCoordinator(config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		
		ctx := context.Background()
		
		// Test coordinator lifecycle
		err = coordinator.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start coordinator: %v", err)
		}
		defer coordinator.Stop(ctx)
		
		// Test basic coordinator operations
		nodeID := coordinator.GetLocalNodeID()
		if nodeID != "node1" {
			t.Errorf("Expected local node ID 'node1', got %s", nodeID)
		}
		
		// Test health
		health := coordinator.GetHealth()
		if !health.Healthy {
			t.Errorf("Coordinator should be healthy after start")
		}
		
		if health.LocalNodeID != "node1" {
			t.Errorf("Expected health local node ID 'node1', got %s", health.LocalNodeID)
		}
		
		// Test membership
		membership := coordinator.GetMembership()
		if membership == nil {
			t.Fatalf("Membership provider should not be nil")
		}
		
		members := membership.GetMembers()
		if len(members) != 1 {
			t.Errorf("Expected 1 member (self), got %d", len(members))
		}
		
		// Test routing
		routing := coordinator.GetRouting()
		if routing == nil {
			t.Fatalf("Routing provider should not be nil")
		}
		
		// Test key routing (should route to self since we're the only node)
		targetNode := routing.RouteKey("test-key")
		if targetNode != "node1" {
			t.Errorf("Expected key to route to 'node1', got %s", targetNode)
		}
		
		// Test event bus
		eventBus := coordinator.GetEventBus()
		if eventBus == nil {
			t.Fatalf("Event bus should not be nil")
		}
	})

	t.Run("Event_Bus_Operations", func(t *testing.T) {
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		
		coordinator, err := cluster.NewSimpleCoordinator(config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		
		ctx := context.Background()
		coordinator.Start(ctx)
		defer coordinator.Stop(ctx)
		
		eventBus := coordinator.GetEventBus()
		
		// Subscribe to events
		eventCh := eventBus.Subscribe(cluster.EventRebalanceStarted)
		defer eventBus.Unsubscribe(eventCh)
		
		// Trigger rebalance to generate an event
		err = coordinator.TriggerRebalance(ctx)
		if err != nil {
			t.Fatalf("Failed to trigger rebalance: %v", err)
		}
		
		// Check that we receive the event
		select {
		case event := <-eventCh:
			if event.Type != cluster.EventRebalanceStarted {
				t.Errorf("Expected EventRebalanceStarted, got %v", event.Type)
			}
			if event.NodeID != "node1" {
				t.Errorf("Expected event from node1, got %s", event.NodeID)
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for rebalance event")
		}
	})

	t.Run("Coordinator_Metrics", func(t *testing.T) {
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		
		coordinator, err := cluster.NewSimpleCoordinator(config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		
		ctx := context.Background()
		coordinator.Start(ctx)
		defer coordinator.Stop(ctx)
		
		// Get metrics
		metrics := coordinator.GetMetrics()
		
		if metrics.Membership.TotalMembers != 1 {
			t.Errorf("Expected 1 total member, got %d", metrics.Membership.TotalMembers)
		}
		
		if metrics.Membership.HealthyMembers != 1 {
			t.Errorf("Expected 1 healthy member, got %d", metrics.Membership.HealthyMembers)
		}
		
		if metrics.Uptime <= 0 {
			t.Errorf("Expected positive uptime, got %v", metrics.Uptime)
		}
	})
}

func TestDistributedEventBus(t *testing.T) {
	t.Run("Local_Event_Publishing", func(t *testing.T) {
		// Create membership for event bus
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		config.BindPort = 9005
		
		membership, err := cluster.NewGossipMembership(config)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}
		
		ctx := context.Background()
		err = membership.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start membership: %v", err)
		}
		defer membership.Stop(ctx)
		
		eventBus := cluster.NewDistributedEventBus("node1", membership)
		
		// Start the event bus
		err = eventBus.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start event bus: %v", err)
		}
		defer eventBus.Stop(ctx)
		
		// Subscribe to events
		eventCh := eventBus.Subscribe(cluster.EventDataOperation)
		defer eventBus.Unsubscribe(eventCh)
		
		// Publish event
		testEvent := cluster.ClusterEvent{
			Type:      cluster.EventDataOperation,
			NodeID:    "node1",
			Data:      map[string]interface{}{"message": "hello"},
			Timestamp: time.Now(),
		}
		
		err = eventBus.Publish(ctx, testEvent)
		if err != nil {
			t.Fatalf("Failed to publish event: %v", err)
		}
		
		// Should receive the event
		select {
		case receivedEvent := <-eventCh:
			if receivedEvent.Type != cluster.EventDataOperation {
				t.Errorf("Expected EventDataOperation, got %s", receivedEvent.Type)
			}
			if data, ok := receivedEvent.Data.(map[string]interface{}); ok {
				if data["message"] != "hello" {
					t.Errorf("Expected message 'hello', got %v", data["message"])
				}
			} else {
				t.Errorf("Expected data to be map[string]interface{}, got %T", receivedEvent.Data)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Did not receive event within timeout")
		}
	})

	t.Run("Event_Filtering", func(t *testing.T) {
		// Create membership for event bus
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		config.BindPort = 9006
		
		membership, err := cluster.NewGossipMembership(config)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}
		
		ctx := context.Background()
		err = membership.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start membership: %v", err)
		}
		defer membership.Stop(ctx)
		
		eventBus := cluster.NewDistributedEventBus("node1", membership)
		
		// Start the event bus
		err = eventBus.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start event bus: %v", err)
		}
		defer eventBus.Stop(ctx)
		
		// Subscribe to specific event types
		rebalanceEvents := eventBus.Subscribe(cluster.EventRebalanceStarted)
		defer eventBus.Unsubscribe(rebalanceEvents)
		
		topologyEvents := eventBus.Subscribe(cluster.EventTopologyChanged)
		defer eventBus.Unsubscribe(topologyEvents)
		
		// Publish different types of events
		eventBus.Publish(ctx, cluster.ClusterEvent{Type: cluster.EventRebalanceStarted, NodeID: "node1"})
		eventBus.Publish(ctx, cluster.ClusterEvent{Type: cluster.EventTopologyChanged, NodeID: "node1"})
		eventBus.Publish(ctx, cluster.ClusterEvent{Type: cluster.EventRebalanceStarted, NodeID: "node1"})
		
		// Check that events are properly filtered
		time.Sleep(100 * time.Millisecond)
		
		rebalanceCount := 0
		topologyCount := 0
		
		// Count rebalance events
	checkRebalance:
		for {
			select {
			case <-rebalanceEvents:
				rebalanceCount++
			default:
				break checkRebalance
			}
		}
		
		// Count topology events
	checkTopology:
		for {
			select {
			case <-topologyEvents:
				topologyCount++
			default:
				break checkTopology
			}
		}
		
		if rebalanceCount != 2 {
			t.Errorf("Expected 2 rebalance events, got %d", rebalanceCount)
		}
		
		if topologyCount != 1 {
			t.Errorf("Expected 1 topology event, got %d", topologyCount)
		}
	})

	t.Run("Event_Metrics", func(t *testing.T) {
		// Create membership for event bus
		config := cluster.DefaultClusterConfig()
		config.NodeID = "node1"
		config.BindPort = 9007
		
		membership, err := cluster.NewGossipMembership(config)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}
		
		ctx := context.Background()
		err = membership.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start membership: %v", err)
		}
		defer membership.Stop(ctx)
		
		eventBus := cluster.NewDistributedEventBus("node1", membership)
		
		// Start the event bus
		err = eventBus.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start event bus: %v", err)
		}
		defer eventBus.Stop(ctx)
		
		// Subscribe and publish events
		eventCh := eventBus.Subscribe(cluster.EventDataOperation)
		defer eventBus.Unsubscribe(eventCh)
		
		for i := 0; i < 5; i++ {
			eventBus.Publish(ctx, cluster.ClusterEvent{Type: cluster.EventDataOperation, NodeID: "node1"})
		}
		
		// Get metrics
		metrics := eventBus.GetMetrics()
		
		if metrics.EventsPublished < 5 {
			t.Errorf("Expected at least 5 published events, got %d", metrics.EventsPublished)
		}
		
		if metrics.ActiveSubscribers < 1 {
			t.Errorf("Expected at least 1 active subscriber, got %d", metrics.ActiveSubscribers)
		}
	})
}
