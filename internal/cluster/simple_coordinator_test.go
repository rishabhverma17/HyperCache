package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewSimpleCoordinator(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	if coordinator == nil {
		t.Fatal("Coordinator is nil")
	}

	if coordinator.GetLocalNodeID() != "test-node-1" {
		t.Errorf("Expected node ID 'test-node-1', got %s", coordinator.GetLocalNodeID())
	}
}

func TestSimpleCoordinatorLifecycle(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()

	// Test start
	err = coordinator.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Verify it's running
	health := coordinator.GetHealth()
	if !health.Healthy {
		t.Error("Coordinator should be healthy after start")
	}

	if health.LocalNodeID != "test-node-1" {
		t.Errorf("Expected local node ID 'test-node-1', got %s", health.LocalNodeID)
	}

	if health.ClusterSize != 1 {
		t.Errorf("Expected cluster size 1, got %d", health.ClusterSize)
	}

	// Test double start (should fail)
	err = coordinator.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running coordinator")
	}

	// Test stop
	err = coordinator.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}

	// Test double stop (should not fail)
	err = coordinator.Stop(ctx)
	if err != nil {
		t.Errorf("Unexpected error on double stop: %v", err)
	}
}

func TestSimpleCoordinatorMembership(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	membership := coordinator.GetMembership()

	// Test getting members
	members := membership.GetMembers()
	if len(members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(members))
	}

	if members[0].NodeID != "test-node-1" {
		t.Errorf("Expected member node ID 'test-node-1', got %s", members[0].NodeID)
	}

	// Test getting specific member
	member, found := membership.GetMember("test-node-1")
	if !found {
		t.Error("Expected to find test-node-1")
	}

	if member.NodeID != "test-node-1" {
		t.Errorf("Expected node ID 'test-node-1', got %s", member.NodeID)
	}

	// Test getting non-existent member
	_, found = membership.GetMember("non-existent")
	if found {
		t.Error("Expected not to find non-existent member")
	}

	// Test health
	if !membership.IsHealthy() {
		t.Error("Membership should be healthy")
	}

	// Test metrics
	metrics := membership.GetMetrics()
	if metrics.TotalMembers != 1 {
		t.Errorf("Expected 1 total member, got %d", metrics.TotalMembers)
	}

	if metrics.HealthyMembers != 1 {
		t.Errorf("Expected 1 healthy member, got %d", metrics.HealthyMembers)
	}
}

func TestSimpleCoordinatorRouting(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	routing := coordinator.GetRouting()

	// Test key routing
	testKey := "user:123"
	node := routing.RouteKey(testKey)
	if node != "test-node-1" {
		t.Errorf("Expected key to route to 'test-node-1', got %s", node)
	}

	// Test replicas
	replicas := routing.GetReplicas(testKey, 1)
	if len(replicas) != 1 {
		t.Errorf("Expected 1 replica, got %d", len(replicas))
	}

	if replicas[0] != "test-node-1" {
		t.Errorf("Expected replica 'test-node-1', got %s", replicas[0])
	}

	// Test locality checks
	if !routing.IsLocal(testKey) {
		t.Error("Key should be local for single-node cluster")
	}

	if !routing.IsReplica(testKey) {
		t.Error("Key should be replica for single-node cluster")
	}

	// Test distribution analysis
	testKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	stats := routing.AnalyzeDistribution(testKeys)

	if stats.TotalKeys != 5 {
		t.Errorf("Expected 5 total keys, got %d", stats.TotalKeys)
	}

	if len(stats.NodeLoads) != 1 {
		t.Errorf("Expected 1 node in distribution, got %d", len(stats.NodeLoads))
	}

	if stats.NodeLoads["test-node-1"] != 5 {
		t.Errorf("Expected 5 keys on test-node-1, got %d", stats.NodeLoads["test-node-1"])
	}

	// Test keys for node
	nodeKeys := routing.GetKeysForNode("test-node-1", testKeys)
	if len(nodeKeys) != 5 {
		t.Errorf("Expected 5 keys for test-node-1, got %d", len(nodeKeys))
	}

	// Test keys for non-existent node
	nonExistentKeys := routing.GetKeysForNode("non-existent", testKeys)
	if len(nonExistentKeys) != 0 {
		t.Errorf("Expected 0 keys for non-existent node, got %d", len(nonExistentKeys))
	}
}

func TestSimpleCoordinatorEventBus(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	eventBus := coordinator.GetEventBus()

	// Subscribe to events
	eventCh := eventBus.Subscribe(EventRebalanceStarted, EventNodePromotion)

	// Publish an event
	event := ClusterEvent{
		Type:      EventRebalanceStarted,
		NodeID:    "test-node-1",
		Timestamp: time.Now(),
		Data:      "test data",
	}

	err = eventBus.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Receive the event
	select {
	case receivedEvent := <-eventCh:
		if receivedEvent.Type != EventRebalanceStarted {
			t.Errorf("Expected EventRebalanceStarted, got %v", receivedEvent.Type)
		}

		if receivedEvent.NodeID != "test-node-1" {
			t.Errorf("Expected node ID 'test-node-1', got %s", receivedEvent.NodeID)
		}

		if receivedEvent.Data != "test data" {
			t.Errorf("Expected data 'test data', got %v", receivedEvent.Data)
		}

	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}

	// Test unsubscribe
	eventBus.Unsubscribe(eventCh)

	// Publish another event (should not be received because channel is closed)
	event.Type = EventNodePromotion
	err = eventBus.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Failed to publish second event: %v", err)
	}

	// Should not receive this event (channel should be closed)
	select {
	case _, ok := <-eventCh:
		if ok {
			t.Error("Should not receive event after unsubscribe")
		}
		// Channel is closed, which is expected
	case <-time.After(100 * time.Millisecond):
		// Also acceptable - no event received
	}

	// Test metrics
	metrics := eventBus.GetMetrics()
	if metrics.EventsPublished != 2 {
		t.Errorf("Expected 2 published events, got %d", metrics.EventsPublished)
	}
}

func TestSimpleCoordinatorRebalance(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	// Subscribe to rebalance events
	eventBus := coordinator.GetEventBus()
	eventCh := eventBus.Subscribe(EventRebalanceStarted)

	// Trigger rebalance
	err = coordinator.TriggerRebalance(ctx)
	if err != nil {
		t.Fatalf("Failed to trigger rebalance: %v", err)
	}

	// Should receive rebalance event
	select {
	case event := <-eventCh:
		if event.Type != EventRebalanceStarted {
			t.Errorf("Expected EventRebalanceStarted, got %v", event.Type)
		}

	case <-time.After(time.Second):
		t.Error("Timeout waiting for rebalance event")
	}
}

func TestSimpleCoordinatorMetrics(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	// Give it a moment to establish heartbeat
	time.Sleep(100 * time.Millisecond)

	metrics := coordinator.GetMetrics()

	if metrics.Membership.TotalMembers != 1 {
		t.Errorf("Expected 1 total member, got %d", metrics.Membership.TotalMembers)
	}

	if metrics.Membership.HealthyMembers != 1 {
		t.Errorf("Expected 1 healthy member, got %d", metrics.Membership.HealthyMembers)
	}

	if metrics.Uptime <= 0 {
		t.Error("Expected positive uptime")
	}

	if metrics.StartTime.IsZero() {
		t.Error("Expected non-zero start time")
	}

	// Generate some routing activity
	routing := coordinator.GetRouting()
	for i := 0; i < 10; i++ {
		routing.RouteKey(fmt.Sprintf("key:%d", i))
	}

	// Check updated metrics
	updatedMetrics := coordinator.GetMetrics()
	if updatedMetrics.Routing.LookupCount <= metrics.Routing.LookupCount {
		t.Error("Expected routing lookup count to increase")
	}
}

func TestSimpleCoordinatorConcurrency(t *testing.T) {
	config := DefaultClusterConfig()
	config.NodeID = "test-node-1"

	coordinator, err := NewSimpleCoordinator(config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	ctx := context.Background()
	coordinator.Start(ctx)
	defer coordinator.Stop(ctx)

	// Run concurrent operations
	done := make(chan bool)

	// Routing operations
	go func() {
		routing := coordinator.GetRouting()
		for i := 0; i < 1000; i++ {
			routing.RouteKey(fmt.Sprintf("key:%d", i))
			routing.GetReplicas(fmt.Sprintf("key:%d", i), 1)
			routing.IsLocal(fmt.Sprintf("key:%d", i))
		}
		done <- true
	}()

	// Event operations
	go func() {
		eventBus := coordinator.GetEventBus()
		ch := eventBus.Subscribe(EventRebalanceStarted)

		for i := 0; i < 100; i++ {
			event := ClusterEvent{
				Type:      EventRebalanceStarted,
				NodeID:    "test-node-1",
				Timestamp: time.Now(),
			}
			eventBus.Publish(ctx, event)
		}

		// Consume some events
		for i := 0; i < 10; i++ {
			select {
			case <-ch:
			case <-time.After(10 * time.Millisecond):
				break
			}
		}

		eventBus.Unsubscribe(ch)
		done <- true
	}()

	// Metrics operations
	go func() {
		for i := 0; i < 100; i++ {
			coordinator.GetMetrics()
			coordinator.GetHealth()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify coordinator is still healthy
	health := coordinator.GetHealth()
	if !health.Healthy {
		t.Error("Coordinator should still be healthy after concurrent operations")
	}
}

func TestValidateConfig(t *testing.T) {
	// Valid config
	config := DefaultClusterConfig()
	config.NodeID = "test-node"

	err := ValidateConfig(config)
	if err != nil {
		t.Errorf("Expected valid config to pass validation: %v", err)
	}

	// Invalid configs
	invalidConfigs := []struct {
		name   string
		modify func(*ClusterConfig)
	}{
		{
			name: "missing node ID",
			modify: func(c *ClusterConfig) {
				c.NodeID = ""
			},
		},
		{
			name: "missing cluster name",
			modify: func(c *ClusterConfig) {
				c.ClusterName = ""
			},
		},
		{
			name: "invalid port - zero",
			modify: func(c *ClusterConfig) {
				c.BindPort = 0
			},
		},
		{
			name: "invalid port - too high",
			modify: func(c *ClusterConfig) {
				c.BindPort = 70000
			},
		},
		{
			name: "invalid heartbeat interval",
			modify: func(c *ClusterConfig) {
				c.HeartbeatInterval = 0
			},
		},
		{
			name: "invalid failure detection timeout",
			modify: func(c *ClusterConfig) {
				c.FailureDetectionTimeout = c.HeartbeatInterval - 1
			},
		},
	}

	for _, tc := range invalidConfigs {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultClusterConfig()
			config.NodeID = "test-node"
			tc.modify(&config)

			err := ValidateConfig(config)
			if err == nil {
				t.Errorf("Expected config validation to fail for %s", tc.name)
			}
		})
	}
}

func TestGenerateNodeID(t *testing.T) {
	id1 := GenerateNodeID()
	id2 := GenerateNodeID()

	if id1 == "" {
		t.Error("Expected non-empty node ID")
	}

	if id2 == "" {
		t.Error("Expected non-empty node ID")
	}

	if id1 == id2 {
		t.Error("Expected unique node IDs")
	}

	// Check format
	if len(id1) < 5 || id1[:5] != "node-" {
		t.Errorf("Expected node ID to start with 'node-', got %s", id1)
	}
}
