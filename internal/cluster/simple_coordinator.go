package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SimpleCoordinator provides a basic implementation of CoordinatorService
// This is a minimal implementation focused on single-node operation with
// the hash ring as the routing foundation. It can be extended with actual
// distributed membership and consensus later.
type SimpleCoordinator struct {
	config      ClusterConfig
	localNodeID string
	hashRing    *HashRing

	// Event handling
	eventSubs  map[chan ClusterEvent][]ClusterEventType
	memberSubs []chan<- MembershipEvent
	eventMu    sync.RWMutex

	// Lifecycle
	startTime time.Time
	running   bool
	runMu     sync.RWMutex

	// Health monitoring
	lastHeartbeat time.Time
	healthMu      sync.RWMutex

	// Metrics
	eventsPublished int64
	eventsReceived  int64
	metricsMu       sync.RWMutex
}

// NewSimpleCoordinator creates a new simple coordinator
func NewSimpleCoordinator(config ClusterConfig) (*SimpleCoordinator, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}

	hashRing := NewHashRing(config.HashRing)

	coordinator := &SimpleCoordinator{
		config:        config,
		localNodeID:   config.NodeID,
		hashRing:      hashRing,
		eventSubs:     make(map[chan ClusterEvent][]ClusterEventType),
		memberSubs:    make([]chan<- MembershipEvent, 0),
		lastHeartbeat: time.Now(),
	}

	return coordinator, nil
}

// Start implements CoordinatorService.Start
func (c *SimpleCoordinator) Start(ctx context.Context) error {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	if c.running {
		return fmt.Errorf("coordinator already running")
	}

	c.startTime = time.Now()
	c.running = true

	// Add local node to hash ring
	err := c.hashRing.AddNode(
		c.localNodeID,
		c.config.AdvertiseAddress,
		c.config.BindPort,
	)
	if err != nil {
		c.running = false
		return fmt.Errorf("failed to add local node to ring: %w", err)
	}

	// Start background heartbeat
	go c.heartbeatLoop(ctx)

	return nil
}

// Stop implements CoordinatorService.Stop
func (c *SimpleCoordinator) Stop(ctx context.Context) error {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	if !c.running {
		return nil
	}

	c.running = false

	// Remove local node from hash ring
	c.hashRing.RemoveNode(c.localNodeID)

	// Close event channels
	c.eventMu.Lock()
	for ch := range c.eventSubs {
		close(ch)
	}
	c.eventSubs = make(map[chan ClusterEvent][]ClusterEventType)

	for _, ch := range c.memberSubs {
		close(ch)
	}
	c.memberSubs = nil
	c.eventMu.Unlock()

	return nil
}

// GetLocalNodeID implements CoordinatorService.GetLocalNodeID
func (c *SimpleCoordinator) GetLocalNodeID() string {
	return c.localNodeID
}

// GetMembership implements CoordinatorService.GetMembership
func (c *SimpleCoordinator) GetMembership() MembershipProvider {
	return &simpleMembership{coordinator: c}
}

// GetRouting implements CoordinatorService.GetRouting
func (c *SimpleCoordinator) GetRouting() RoutingProvider {
	return &simpleRouting{coordinator: c}
}

// GetEventBus implements CoordinatorService.GetEventBus
func (c *SimpleCoordinator) GetEventBus() EventBus {
	return &simpleEventBus{coordinator: c}
}

// TriggerRebalance implements CoordinatorService.TriggerRebalance
func (c *SimpleCoordinator) TriggerRebalance(ctx context.Context) error {
	// For simple coordinator, rebalancing is automatic via hash ring
	// Just publish an event to notify subscribers
	event := ClusterEvent{
		Type:      EventRebalanceStarted,
		NodeID:    c.localNodeID,
		Timestamp: time.Now(),
	}

	return c.publishEvent(ctx, event)
}

// GetHealth implements CoordinatorService.GetHealth
func (c *SimpleCoordinator) GetHealth() CoordinatorHealth {
	c.runMu.RLock()
	c.healthMu.RLock()
	defer c.runMu.RUnlock()
	defer c.healthMu.RUnlock()

	health := CoordinatorHealth{
		Healthy:          c.running && time.Since(c.lastHeartbeat) < time.Duration(c.config.FailureDetectionTimeout)*time.Second,
		LocalNodeID:      c.localNodeID,
		ClusterSize:      c.hashRing.NodeCount(),
		ConnectedToSeeds: true, // Simple coordinator is always "connected"
		LastHeartbeat:    c.lastHeartbeat,
		Uptime:           time.Since(c.startTime),
		Issues:           []string{},
	}

	if !health.Healthy {
		health.Issues = append(health.Issues, "heartbeat timeout")
	}

	if c.hashRing.NodeCount() == 0 {
		health.Issues = append(health.Issues, "no nodes in cluster")
		health.Healthy = false
	}

	return health
}

// GetMetrics implements CoordinatorService.GetMetrics
func (c *SimpleCoordinator) GetMetrics() CoordinatorMetrics {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()

	return CoordinatorMetrics{
		Membership: MembershipMetrics{
			TotalMembers:   c.hashRing.NodeCount(),
			HealthyMembers: c.hashRing.NodeCount(),
			ClusterAge:     time.Since(c.startTime),
		},
		Routing:   c.hashRing.GetMetrics(),
		EventBus:  EventBusMetrics{EventsPublished: c.eventsPublished, EventsReceived: c.eventsReceived},
		Uptime:    time.Since(c.startTime),
		StartTime: c.startTime,
	}
}

// heartbeatLoop runs the background heartbeat
func (c *SimpleCoordinator) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(c.config.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.healthMu.Lock()
			c.lastHeartbeat = time.Now()
			c.healthMu.Unlock()

			// Check if we should continue
			c.runMu.RLock()
			if !c.running {
				c.runMu.RUnlock()
				return
			}
			c.runMu.RUnlock()
		}
	}
}

// publishEvent publishes an event to all subscribers
func (c *SimpleCoordinator) publishEvent(ctx context.Context, event ClusterEvent) error {
	c.eventMu.RLock()
	defer c.eventMu.RUnlock()

	c.metricsMu.Lock()
	c.eventsPublished++
	c.metricsMu.Unlock()

	for ch, eventTypes := range c.eventSubs {
		// Check if subscriber is interested in this event type
		interested := false
		for _, eventType := range eventTypes {
			if eventType == event.Type {
				interested = true
				break
			}
		}

		if interested {
			select {
			case ch <- event:
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Non-blocking send, skip if channel is full
			}
		}
	}

	return nil
}

// simpleMembership implements MembershipProvider for the simple coordinator
type simpleMembership struct {
	coordinator *SimpleCoordinator
}

func (m *simpleMembership) Join(ctx context.Context, seedNodes []string) error {
	// Simple coordinator doesn't join external clusters
	return nil
}

func (m *simpleMembership) Leave(ctx context.Context) error {
	return m.coordinator.Stop(ctx)
}

func (m *simpleMembership) GetMembers() []ClusterMember {
	nodes := m.coordinator.hashRing.GetNodes()
	members := make([]ClusterMember, 0, len(nodes))

	for _, node := range nodes {
		member := ClusterMember{
			NodeID:   node.ID,
			Address:  node.Address,
			Port:     node.Port,
			Status:   node.Status,
			Metadata: map[string]string{},
			LastSeen: node.LastSeen,
			JoinedAt: m.coordinator.startTime,
		}
		members = append(members, member)
	}

	return members
}

func (m *simpleMembership) GetMember(nodeID string) (*ClusterMember, bool) {
	members := m.GetMembers()
	for _, member := range members {
		if member.NodeID == nodeID {
			return &member, true
		}
	}
	return nil, false
}

func (m *simpleMembership) UpdateMetadata(metadata map[string]string) error {
	// Simple coordinator doesn't persist metadata
	return nil
}

func (m *simpleMembership) Subscribe() <-chan MembershipEvent {
	ch := make(chan MembershipEvent, 100)

	m.coordinator.eventMu.Lock()
	m.coordinator.memberSubs = append(m.coordinator.memberSubs, ch)
	m.coordinator.eventMu.Unlock()

	return ch
}

func (m *simpleMembership) GetMetrics() MembershipMetrics {
	return MembershipMetrics{
		TotalMembers:   m.coordinator.hashRing.NodeCount(),
		HealthyMembers: m.coordinator.hashRing.NodeCount(),
		ClusterAge:     time.Since(m.coordinator.startTime),
	}
}

func (m *simpleMembership) IsHealthy() bool {
	return m.coordinator.GetHealth().Healthy
}

func (m *simpleMembership) GetAliveNodes() []ClusterMember {
	// For simple coordinator, all members are considered alive
	return m.GetMembers()
}

// simpleRouting implements RoutingProvider for the simple coordinator
type simpleRouting struct {
	coordinator *SimpleCoordinator
}

func (r *simpleRouting) RouteKey(key string) string {
	return r.coordinator.hashRing.GetNode(key)
}

func (r *simpleRouting) GetReplicas(key string, count int) []string {
	return r.coordinator.hashRing.GetReplicas(key, count)
}

func (r *simpleRouting) IsLocal(key string) bool {
	primaryNode := r.coordinator.hashRing.GetNode(key)
	return primaryNode == r.coordinator.localNodeID
}

func (r *simpleRouting) IsReplica(key string) bool {
	replicas := r.coordinator.hashRing.GetReplicas(key, r.coordinator.config.HashRing.ReplicationFactor)
	for _, replica := range replicas {
		if replica == r.coordinator.localNodeID {
			return true
		}
	}
	return false
}

func (r *simpleRouting) GetKeysForNode(nodeID string, allKeys []string) []string {
	var nodeKeys []string
	for _, key := range allKeys {
		if r.coordinator.hashRing.GetNode(key) == nodeID {
			nodeKeys = append(nodeKeys, key)
		}
	}
	return nodeKeys
}

func (r *simpleRouting) AnalyzeDistribution(keys []string) DistributionStats {
	return r.coordinator.hashRing.AnalyzeDistribution(keys)
}

func (r *simpleRouting) GetMetrics() HashRingMetrics {
	return r.coordinator.hashRing.GetMetrics()
}

// Redis-compatible slot-based routing methods
func (r *simpleRouting) GetHashSlot(key string) uint16 {
	return GetHashSlot(key)
}

func (r *simpleRouting) GetNodeBySlot(slot uint16) (nodeID string, address string, port int) {
	nodeID = r.coordinator.hashRing.GetNodeBySlot(slot)
	if nodeID == "" {
		return "", "", 0
	}
	
	// Get node details from hash ring
	nodes := r.coordinator.hashRing.GetNodes()
	if node, exists := nodes[nodeID]; exists {
		return nodeID, node.Address, node.Port
	}
	
	return "", "", 0
}

func (r *simpleRouting) GetNodeByKey(key string) (nodeID string, address string, port int) {
	nodeID = r.coordinator.hashRing.GetNodeByKey(key)
	if nodeID == "" {
		return "", "", 0
	}
	
	// Get node details from hash ring
	nodes := r.coordinator.hashRing.GetNodes()
	if node, exists := nodes[nodeID]; exists {
		return nodeID, node.Address, node.Port
	}
	
	return "", "", 0
}

func (r *simpleRouting) GetSlotsByNode(nodeID string) []uint16 {
	return r.coordinator.hashRing.GetSlotsByNode(nodeID)
}

// simpleEventBus implements EventBus for the simple coordinator
type simpleEventBus struct {
	coordinator *SimpleCoordinator
}

func (e *simpleEventBus) Publish(ctx context.Context, event ClusterEvent) error {
	return e.coordinator.publishEvent(ctx, event)
}

func (e *simpleEventBus) Subscribe(eventTypes ...ClusterEventType) <-chan ClusterEvent {
	ch := make(chan ClusterEvent, 100)

	e.coordinator.eventMu.Lock()
	e.coordinator.eventSubs[ch] = eventTypes
	e.coordinator.eventMu.Unlock()

	return ch
}

func (e *simpleEventBus) Unsubscribe(ch <-chan ClusterEvent) {
	e.coordinator.eventMu.Lock()
	defer e.coordinator.eventMu.Unlock()

	// Find and remove the channel
	for eventCh := range e.coordinator.eventSubs {
		if eventCh == ch {
			delete(e.coordinator.eventSubs, eventCh)
			close(eventCh)
			break
		}
	}
}

func (e *simpleEventBus) GetMetrics() EventBusMetrics {
	e.coordinator.metricsMu.RLock()
	defer e.coordinator.metricsMu.RUnlock()

	return EventBusMetrics{
		EventsPublished:   e.coordinator.eventsPublished,
		EventsReceived:    e.coordinator.eventsReceived,
		ActiveSubscribers: len(e.coordinator.eventSubs),
		LastEventTime:     time.Now(), // Simple approximation
	}
}

// Factory function
func NewSimpleCoordinatorFactory() CoordinatorFactory {
	return func(config ClusterConfig) (CoordinatorService, error) {
		return NewSimpleCoordinator(config)
	}
}
