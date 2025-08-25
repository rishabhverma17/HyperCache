package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DistributedCoordinator provides a full implementation of CoordinatorService
// with gossip-based membership, distributed hash ring, and inter-node communication
type DistributedCoordinator struct {
	config      ClusterConfig
	localNodeID string

	// Core components
	membership *GossipMembership
	hashRing   *HashRing
	eventBus   *DistributedEventBus

	// State management
	startTime time.Time
	running   bool
	runMu     sync.RWMutex

	// Lifecycle monitoring
	lastHeartbeat time.Time
	healthMu      sync.RWMutex
}

// NewDistributedCoordinator creates a new distributed coordinator
func NewDistributedCoordinator(config ClusterConfig) (*DistributedCoordinator, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create membership provider
	membership, err := NewGossipMembership(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create membership provider: %w", err)
	}

	// Create hash ring
	hashRing := NewHashRing(config.HashRing)

	// Create event bus
	eventBus := NewDistributedEventBus(config.NodeID, membership)

	coordinator := &DistributedCoordinator{
		config:        config,
		localNodeID:   config.NodeID,
		membership:    membership,
		hashRing:      hashRing,
		eventBus:      eventBus,
		lastHeartbeat: time.Now(),
	}

	return coordinator, nil
}

// Start implements CoordinatorService.Start
func (dc *DistributedCoordinator) Start(ctx context.Context) error {
	dc.runMu.Lock()
	defer dc.runMu.Unlock()

	if dc.running {
		return fmt.Errorf("coordinator already running")
	}

	dc.startTime = time.Now()

	// Start membership provider
	if err := dc.membership.Start(ctx); err != nil {
		return fmt.Errorf("failed to start membership provider: %w", err)
	}

	// Start event bus
	if err := dc.eventBus.Start(ctx); err != nil {
		dc.membership.Stop(ctx) // Cleanup
		return fmt.Errorf("failed to start event bus: %w", err)
	}

	// Add local node to hash ring
	err := dc.hashRing.AddNode(
		dc.localNodeID,
		dc.config.AdvertiseAddress,
		dc.config.BindPort,
	)
	if err != nil {
		dc.membership.Stop(ctx)
		dc.eventBus.Stop(ctx)
		return fmt.Errorf("failed to add local node to hash ring: %w", err)
	}

	// Join cluster if seed nodes are provided
	if len(dc.config.SeedNodes) > 0 {
		if err := dc.membership.Join(ctx, dc.config.SeedNodes); err != nil {
			fmt.Printf("Warning: failed to join cluster: %v\n", err)
			// Don't fail startup - we can operate as a single node
		}
	}

	// Start background processes
	go dc.membershipSync(ctx)
	go dc.heartbeatLoop(ctx)

	dc.running = true

	// Publish startup event
	startupEvent := ClusterEvent{
		Type:      EventTopologyChanged,
		NodeID:    dc.localNodeID,
		Data:      "node_started",
		Timestamp: time.Now(),
	}
	dc.eventBus.Publish(ctx, startupEvent)

	fmt.Printf("Distributed coordinator started: %s\n", dc.localNodeID)

	return nil
}

// Stop implements CoordinatorService.Stop
func (dc *DistributedCoordinator) Stop(ctx context.Context) error {
	dc.runMu.Lock()
	defer dc.runMu.Unlock()

	if !dc.running {
		return nil
	}

	dc.running = false

	// Publish shutdown event
	shutdownEvent := ClusterEvent{
		Type:      EventTopologyChanged,
		NodeID:    dc.localNodeID,
		Data:      "node_stopping",
		Timestamp: time.Now(),
	}
	dc.eventBus.Publish(ctx, shutdownEvent)

	// Stop components in reverse order
	dc.eventBus.Stop(ctx)
	dc.membership.Stop(ctx)

	// Remove local node from hash ring
	dc.hashRing.RemoveNode(dc.localNodeID)

	fmt.Printf("Distributed coordinator stopped: %s\n", dc.localNodeID)

	return nil
}

// GetLocalNodeID implements CoordinatorService.GetLocalNodeID
func (dc *DistributedCoordinator) GetLocalNodeID() string {
	return dc.localNodeID
}

// GetMembership implements CoordinatorService.GetMembership
func (dc *DistributedCoordinator) GetMembership() MembershipProvider {
	return dc.membership
}

// GetRouting implements CoordinatorService.GetRouting
func (dc *DistributedCoordinator) GetRouting() RoutingProvider {
	return &distributedRouting{coordinator: dc}
}

// GetEventBus implements CoordinatorService.GetEventBus
func (dc *DistributedCoordinator) GetEventBus() EventBus {
	return dc.eventBus
}

// TriggerRebalance implements CoordinatorService.TriggerRebalance
func (dc *DistributedCoordinator) TriggerRebalance(ctx context.Context) error {
	// Publish rebalance event to the cluster
	rebalanceEvent := ClusterEvent{
		Type:      EventRebalanceStarted,
		NodeID:    dc.localNodeID,
		Data:      "manual_trigger",
		Timestamp: time.Now(),
	}

	return dc.eventBus.Publish(ctx, rebalanceEvent)
}

// GetHealth implements CoordinatorService.GetHealth
func (dc *DistributedCoordinator) GetHealth() CoordinatorHealth {
	dc.runMu.RLock()
	dc.healthMu.RLock()
	defer dc.runMu.RUnlock()
	defer dc.healthMu.RUnlock()

	health := CoordinatorHealth{
		Healthy:          dc.running && dc.membership.IsHealthy(),
		LocalNodeID:      dc.localNodeID,
		ClusterSize:      len(dc.membership.GetMembers()),
		ConnectedToSeeds: dc.isConnectedToSeeds(),
		LastHeartbeat:    dc.lastHeartbeat,
		Uptime:           time.Since(dc.startTime),
		Issues:           []string{},
	}

	// Check for issues
	if !dc.membership.IsHealthy() {
		health.Issues = append(health.Issues, "membership provider unhealthy")
		health.Healthy = false
	}

	if time.Since(dc.lastHeartbeat) > time.Duration(dc.config.FailureDetectionTimeout)*time.Second {
		health.Issues = append(health.Issues, "heartbeat timeout")
		health.Healthy = false
	}

	aliveMembers := dc.membership.GetAliveNodes()
	if len(aliveMembers) == 0 {
		health.Issues = append(health.Issues, "no alive cluster members")
		health.Healthy = false
	}

	return health
}

// GetMetrics implements CoordinatorService.GetMetrics
func (dc *DistributedCoordinator) GetMetrics() CoordinatorMetrics {
	return CoordinatorMetrics{
		Membership: dc.membership.GetMetrics(),
		Routing:    dc.hashRing.GetMetrics(),
		EventBus:   dc.eventBus.GetMetrics(),
		Uptime:     time.Since(dc.startTime),
		StartTime:  dc.startTime,
	}
}

// membershipSync synchronizes membership changes with the hash ring
func (dc *DistributedCoordinator) membershipSync(ctx context.Context) {
	membershipEvents := dc.membership.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-membershipEvents:
			if !ok {
				return // Channel closed
			}

			dc.handleMembershipEvent(ctx, event)
		}
	}
}

// handleMembershipEvent processes membership changes and updates hash ring
func (dc *DistributedCoordinator) handleMembershipEvent(ctx context.Context, event MembershipEvent) {
	member := event.Member

	switch event.Type {
	case MemberJoined:
		// Add node to hash ring
		err := dc.hashRing.AddNode(member.NodeID, member.Address, member.Port)
		if err != nil {
			fmt.Printf("Failed to add node to hash ring: %s - %v\n", member.NodeID, err)
			return
		}

		fmt.Printf("Added node to hash ring: %s (%s:%d)\n", member.NodeID, member.Address, member.Port)

		// Publish topology change event
		topologyEvent := ClusterEvent{
			Type:      EventTopologyChanged,
			NodeID:    dc.localNodeID,
			Data:      fmt.Sprintf("node_added:%s", member.NodeID),
			Timestamp: time.Now(),
		}
		dc.eventBus.Publish(ctx, topologyEvent)

	case MemberLeft, MemberFailed:
		// Remove node from hash ring
		err := dc.hashRing.RemoveNode(member.NodeID)
		if err != nil {
			fmt.Printf("Failed to remove node from hash ring: %s - %v\n", member.NodeID, err)
			return
		}

		fmt.Printf("Removed node from hash ring: %s\n", member.NodeID)

		// Publish topology change event
		topologyEvent := ClusterEvent{
			Type:      EventTopologyChanged,
			NodeID:    dc.localNodeID,
			Data:      fmt.Sprintf("node_removed:%s", member.NodeID),
			Timestamp: time.Now(),
		}
		dc.eventBus.Publish(ctx, topologyEvent)

	case MemberRecovered:
		// Update node status in hash ring
		err := dc.hashRing.SetNodeStatus(member.NodeID, NodeAlive)
		if err != nil {
			fmt.Printf("Failed to update node status: %s - %v\n", member.NodeID, err)
			return
		}

		fmt.Printf("Node recovered: %s\n", member.NodeID)

	case MemberUpdated:
		// Node metadata updated - no hash ring changes needed
		fmt.Printf("Node metadata updated: %s\n", member.NodeID)
	}
}

// heartbeatLoop runs the background heartbeat
func (dc *DistributedCoordinator) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(dc.config.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dc.healthMu.Lock()
			dc.lastHeartbeat = time.Now()
			dc.healthMu.Unlock()

			// Update node load in hash ring (simplified metric)
			nodeCount := len(dc.membership.GetAliveNodes())
			load := 1.0 / float64(max(nodeCount, 1)) // Simple load distribution

			dc.hashRing.UpdateNodeLoad(dc.localNodeID, load)

			// Check if we should continue
			dc.runMu.RLock()
			if !dc.running {
				dc.runMu.RUnlock()
				return
			}
			dc.runMu.RUnlock()
		}
	}
}

// isConnectedToSeeds checks if we're connected to any seed nodes
func (dc *DistributedCoordinator) isConnectedToSeeds() bool {
	if len(dc.config.SeedNodes) == 0 {
		return true // No seeds required
	}

	members := dc.membership.GetAliveNodes()
	seedAddresses := make(map[string]bool)

	// Create a set of seed addresses
	for _, seed := range dc.config.SeedNodes {
		seedAddresses[seed] = true
	}

	// Check if any alive member is a seed
	for _, member := range members {
		memberAddr := fmt.Sprintf("%s:%d", member.Address, member.Port)
		if seedAddresses[memberAddr] {
			return true
		}
	}

	return false
}

// distributedRouting implements RoutingProvider for the distributed coordinator
type distributedRouting struct {
	coordinator *DistributedCoordinator
}

func (dr *distributedRouting) RouteKey(key string) string {
	return dr.coordinator.hashRing.GetNode(key)
}

func (dr *distributedRouting) GetReplicas(key string, count int) []string {
	return dr.coordinator.hashRing.GetReplicas(key, count)
}

func (dr *distributedRouting) IsLocal(key string) bool {
	primaryNode := dr.coordinator.hashRing.GetNode(key)
	return primaryNode == dr.coordinator.localNodeID
}

func (dr *distributedRouting) IsReplica(key string) bool {
	replicas := dr.coordinator.hashRing.GetReplicas(key, dr.coordinator.config.HashRing.ReplicationFactor)
	for _, replica := range replicas {
		if replica == dr.coordinator.localNodeID {
			return true
		}
	}
	return false
}

func (dr *distributedRouting) GetKeysForNode(nodeID string, allKeys []string) []string {
	var nodeKeys []string
	for _, key := range allKeys {
		if dr.coordinator.hashRing.GetNode(key) == nodeID {
			nodeKeys = append(nodeKeys, key)
		}
	}
	return nodeKeys
}

func (dr *distributedRouting) AnalyzeDistribution(keys []string) DistributionStats {
	return dr.coordinator.hashRing.AnalyzeDistribution(keys)
}

func (dr *distributedRouting) GetMetrics() HashRingMetrics {
	return dr.coordinator.hashRing.GetMetrics()
}

// Redis-compatible slot-based routing methods
func (dr *distributedRouting) GetHashSlot(key string) uint16 {
	return GetHashSlot(key)
}

func (dr *distributedRouting) GetNodeBySlot(slot uint16) (nodeID string, address string, port int) {
	nodeID = dr.coordinator.hashRing.GetNodeBySlot(slot)
	if nodeID == "" {
		return "", "", 0
	}
	
	// Get node details from hash ring
	nodes := dr.coordinator.hashRing.GetNodes()
	if node, exists := nodes[nodeID]; exists {
		return nodeID, node.Address, node.Port
	}
	
	return "", "", 0
}

func (dr *distributedRouting) GetNodeByKey(key string) (nodeID string, address string, port int) {
	nodeID = dr.coordinator.hashRing.GetNodeByKey(key)
	if nodeID == "" {
		return "", "", 0
	}
	
	// Get node details from hash ring
	nodes := dr.coordinator.hashRing.GetNodes()
	if node, exists := nodes[nodeID]; exists {
		return nodeID, node.Address, node.Port
	}
	
	return "", "", 0
}

func (dr *distributedRouting) GetSlotsByNode(nodeID string) []uint16 {
	return dr.coordinator.hashRing.GetSlotsByNode(nodeID)
}

// Factory function for distributed coordinator
func NewDistributedCoordinatorFactory() CoordinatorFactory {
	return func(config ClusterConfig) (CoordinatorService, error) {
		return NewDistributedCoordinator(config)
	}
}

// Helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
