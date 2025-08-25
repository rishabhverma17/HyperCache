package cluster

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/serf/serf"
)

// GossipMembership implements MembershipProvider using Serf gossip protocol
type GossipMembership struct {
	config     ClusterConfig
	serf       *serf.Serf
	eventCh    chan serf.Event
	memberSubs []chan<- MembershipEvent

	// State management
	members     map[string]*ClusterMember
	localMember *ClusterMember

	// User event handler
	userEventHandler func(eventName string, payload []byte)

	// Synchronization
	mu     sync.RWMutex
	subsMu sync.RWMutex

	// Metrics
	metrics    MembershipMetrics
	startTime  time.Time
	eventCount int64
}

// NewGossipMembership creates a new gossip-based membership provider
func NewGossipMembership(config ClusterConfig) (*GossipMembership, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	gm := &GossipMembership{
		config:    config,
		eventCh:   make(chan serf.Event, 256),
		members:   make(map[string]*ClusterMember),
		startTime: time.Now(),
	}

	// Create local member representation
	gm.localMember = &ClusterMember{
		NodeID:  config.NodeID,
		Address: config.AdvertiseAddress,
		Port:    config.BindPort,
		Status:  NodeAlive,
		Metadata: map[string]string{
			"cluster":      config.ClusterName,
			"version":      "1.0.0",
			"capabilities": "filters,persistence,resp",
			"resp_port":    fmt.Sprintf("%d", config.RESPPort), // Add RESP port for client routing
		},
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}

	return gm, nil
}

// Start initializes the gossip membership provider
func (gm *GossipMembership) Start(ctx context.Context) error {
	// Create Serf configuration
	conf := serf.DefaultConfig()
	conf.Init()

	// Set basic configuration
	conf.NodeName = gm.config.NodeID
	conf.MemberlistConfig.BindAddr = gm.config.BindAddress
	conf.MemberlistConfig.BindPort = gm.config.BindPort

	// Set advertise address if specified
	if gm.config.AdvertiseAddress != "" {
		conf.MemberlistConfig.AdvertiseAddr = gm.config.AdvertiseAddress
		conf.MemberlistConfig.AdvertisePort = gm.config.BindPort
	}

	// Configure event handling
	conf.EventCh = gm.eventCh

	// Configure gossip intervals
	conf.MemberlistConfig.GossipInterval = time.Duration(gm.config.HeartbeatInterval) * time.Second

	// Set tags (metadata)
	conf.Tags = gm.localMember.Metadata

	// Create Serf instance
	serfInstance, err := serf.Create(conf)
	if err != nil {
		return fmt.Errorf("failed to create serf instance: %w", err)
	}

	gm.serf = serfInstance

	// Start event processing
	go gm.processEvents(ctx)

	// Add self to members
	gm.mu.Lock()
	gm.members[gm.config.NodeID] = gm.localMember
	gm.mu.Unlock()

	return nil
}

// Stop shuts down the gossip membership provider
func (gm *GossipMembership) Stop(ctx context.Context) error {
	if gm.serf != nil {
		// Leave the cluster gracefully
		err := gm.serf.Leave()
		if err != nil {
			// Log error but continue shutdown
			fmt.Printf("Error leaving serf cluster: %v\n", err)
		}

		// Shutdown serf
		err = gm.serf.Shutdown()
		if err != nil {
			return fmt.Errorf("failed to shutdown serf: %w", err)
		}
	}

	// Close event subscriptions
	gm.subsMu.Lock()
	for _, ch := range gm.memberSubs {
		close(ch)
	}
	gm.memberSubs = nil
	gm.subsMu.Unlock()

	return nil
}

// Join implements MembershipProvider.Join
func (gm *GossipMembership) Join(ctx context.Context, seedNodes []string) error {
	if gm.serf == nil {
		return fmt.Errorf("membership provider not started")
	}

	if len(seedNodes) == 0 {
		// No seed nodes - this is the first node or we're bootstrapping
		return nil
	}

	// Join the cluster using seed nodes
	joinCtx, cancel := context.WithTimeout(ctx, time.Duration(gm.config.JoinTimeout)*time.Second)
	defer cancel()

	// Try to join each seed node
	var lastErr error
	for _, seedAddr := range seedNodes {
		select {
		case <-joinCtx.Done():
			return fmt.Errorf("join timeout: %w", joinCtx.Err())
		default:
		}

		// Try to join this seed node
		fmt.Printf("Attempting to join cluster via %s...\n", seedAddr)
		num, err := gm.serf.Join([]string{seedAddr}, false)
		if err != nil {
			lastErr = err
			continue
		}

		if num > 0 {
			fmt.Printf("Successfully joined cluster via %s (%d members)\n", seedAddr, num)
			return nil
		}
	}

	if lastErr != nil {
		return fmt.Errorf("failed to join any seed nodes: %w", lastErr)
	}

	return fmt.Errorf("no seed nodes responded")
}

// Leave implements MembershipProvider.Leave
func (gm *GossipMembership) Leave(ctx context.Context) error {
	if gm.serf == nil {
		return fmt.Errorf("membership provider not started")
	}

	// Leave gracefully
	err := gm.serf.Leave()
	if err != nil {
		return fmt.Errorf("failed to leave cluster: %w", err)
	}

	return gm.Stop(ctx)
}

// GetMembers implements MembershipProvider.GetMembers
func (gm *GossipMembership) GetMembers() []ClusterMember {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	members := make([]ClusterMember, 0, len(gm.members))
	for _, member := range gm.members {
		// Create a copy to avoid external modifications
		memberCopy := *member
		members = append(members, memberCopy)
	}

	return members
}

// GetMember implements MembershipProvider.GetMember
func (gm *GossipMembership) GetMember(nodeID string) (*ClusterMember, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	member, exists := gm.members[nodeID]
	if !exists {
		return nil, false
	}

	// Return a copy
	memberCopy := *member
	return &memberCopy, true
}

// UpdateMetadata implements MembershipProvider.UpdateMetadata
func (gm *GossipMembership) UpdateMetadata(metadata map[string]string) error {
	if gm.serf == nil {
		return fmt.Errorf("membership provider not started")
	}

	// Update local metadata
	gm.mu.Lock()
	for key, value := range metadata {
		gm.localMember.Metadata[key] = value
	}
	gm.mu.Unlock()

	// Update serf tags (this will gossip the changes)
	return gm.serf.SetTags(gm.localMember.Metadata)
}

// Subscribe implements MembershipProvider.Subscribe
func (gm *GossipMembership) Subscribe() <-chan MembershipEvent {
	ch := make(chan MembershipEvent, 100)

	gm.subsMu.Lock()
	gm.memberSubs = append(gm.memberSubs, ch)
	gm.subsMu.Unlock()

	return ch
}

// GetMetrics implements MembershipProvider.GetMetrics
func (gm *GossipMembership) GetMetrics() MembershipMetrics {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	metrics := gm.metrics
	metrics.TotalMembers = len(gm.members)
	metrics.ClusterAge = time.Since(gm.startTime)
	metrics.EventCount = gm.eventCount

	// Count members by status
	for _, member := range gm.members {
		switch member.Status {
		case NodeAlive:
			metrics.HealthyMembers++
		case NodeSuspected:
			metrics.SuspectedMembers++
		case NodeDead:
			metrics.FailedMembers++
		}
	}

	return metrics
}

// IsHealthy implements MembershipProvider.IsHealthy
func (gm *GossipMembership) IsHealthy() bool {
	if gm.serf == nil {
		return false
	}

	// Check if we have a reasonable number of members
	gm.mu.RLock()
	memberCount := len(gm.members)
	gm.mu.RUnlock()

	// Consider healthy if we have at least one member (ourselves) and serf is running
	return memberCount > 0 && gm.serf.State() == serf.SerfAlive
}

// processEvents handles membership events from Serf
func (gm *GossipMembership) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-gm.eventCh:
			gm.handleSerfEvent(event)
		}
	}
}

// handleSerfEvent processes individual Serf events
func (gm *GossipMembership) handleSerfEvent(event serf.Event) {
	gm.mu.Lock()
	gm.eventCount++
	gm.mu.Unlock()

	switch e := event.(type) {
	case serf.MemberEvent:
		gm.handleMemberEvent(e)
	case serf.UserEvent:
		gm.handleUserEvent(e)
	case *serf.Query:
		gm.handleQuery(e)
	default:
		fmt.Printf("Unknown serf event type: %T\n", event)
	}
}

// handleMemberEvent processes member join/leave/update events
func (gm *GossipMembership) handleMemberEvent(event serf.MemberEvent) {
	for _, member := range event.Members {
		gm.processMemberChange(member, event.EventType())
	}
}

// processMemberChange updates member state and notifies subscribers
func (gm *GossipMembership) processMemberChange(serfMember serf.Member, eventType serf.EventType) {
	// Convert serf member to cluster member
	clusterMember := &ClusterMember{
		NodeID:   serfMember.Name,
		Address:  serfMember.Addr.String(),
		Port:     int(serfMember.Port),
		Metadata: serfMember.Tags,
		LastSeen: time.Now(),
	}

	// Determine status and event type
	var membershipEventType MembershipEventType

	switch eventType {
	case serf.EventMemberJoin:
		clusterMember.Status = NodeAlive
		clusterMember.JoinedAt = time.Now()
		membershipEventType = MemberJoined

		// Add to members map
		gm.mu.Lock()
		gm.members[clusterMember.NodeID] = clusterMember
		gm.mu.Unlock()

		fmt.Printf("Node joined: %s (%s:%d)\n", clusterMember.NodeID, clusterMember.Address, clusterMember.Port)

	case serf.EventMemberLeave:
		clusterMember.Status = NodeLeaving
		membershipEventType = MemberLeft

		// Remove from members map
		gm.mu.Lock()
		delete(gm.members, clusterMember.NodeID)
		gm.mu.Unlock()

		fmt.Printf("Node left: %s\n", clusterMember.NodeID)

	case serf.EventMemberFailed:
		clusterMember.Status = NodeDead
		membershipEventType = MemberFailed

		// Update status in members map
		gm.mu.Lock()
		if existing, exists := gm.members[clusterMember.NodeID]; exists {
			existing.Status = NodeDead
			existing.LastSeen = time.Now()
		}
		gm.mu.Unlock()

		fmt.Printf("Node failed: %s\n", clusterMember.NodeID)

	case serf.EventMemberUpdate:
		clusterMember.Status = NodeAlive
		membershipEventType = MemberUpdated

		// Update member in map
		gm.mu.Lock()
		if existing, exists := gm.members[clusterMember.NodeID]; exists {
			existing.Metadata = clusterMember.Metadata
			existing.LastSeen = time.Now()
		}
		gm.mu.Unlock()

		fmt.Printf("Node updated: %s\n", clusterMember.NodeID)

	case serf.EventMemberReap:
		// Member reaped (removed from failed state)
		gm.mu.Lock()
		delete(gm.members, clusterMember.NodeID)
		gm.mu.Unlock()

		fmt.Printf("Node reaped: %s\n", clusterMember.NodeID)
		return // Don't send event for reaping

	default:
		fmt.Printf("Unknown member event type: %s\n", eventType)
		return
	}

	// Create membership event
	membershipEvent := MembershipEvent{
		Type:      membershipEventType,
		Member:    *clusterMember,
		Timestamp: time.Now(),
	}

	// Notify subscribers
	gm.notifySubscribers(membershipEvent)
}

// notifySubscribers sends membership events to all subscribers
func (gm *GossipMembership) notifySubscribers(event MembershipEvent) {
	gm.subsMu.RLock()
	defer gm.subsMu.RUnlock()

	for _, ch := range gm.memberSubs {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber
			fmt.Printf("Warning: membership event channel full for subscriber\n")
		}
	}
}

// handleUserEvent processes custom user events
func (gm *GossipMembership) handleUserEvent(event serf.UserEvent) {
	fmt.Printf("[GOSSIP] User event received: %s (payload: %s)\n", event.Name, string(event.Payload))

	// Forward to registered handler if available
	if gm.userEventHandler != nil {
		gm.userEventHandler(event.Name, event.Payload)
	}
}

// handleQuery processes queries from other nodes
func (gm *GossipMembership) handleQuery(query *serf.Query) {
	fmt.Printf("Query received: %s (payload: %s)\n", query.Name, string(query.Payload))
	// Queries can be used for cluster-wide operations

	// Example: health check query
	if query.Name == "health-check" {
		response := map[string]interface{}{
			"healthy":   gm.IsHealthy(),
			"node_id":   gm.config.NodeID,
			"timestamp": time.Now(),
		}

		// Send response (simplified - would normally serialize properly)
		query.Respond([]byte(fmt.Sprintf("%+v", response)))
	}
}

// SendUserEvent sends a custom event to the cluster
func (gm *GossipMembership) SendUserEvent(name string, payload []byte) error {
	if gm.serf == nil {
		return fmt.Errorf("membership provider not started")
	}

	return gm.serf.UserEvent(name, payload, false)
}

// Query sends a query to the cluster and collects responses
func (gm *GossipMembership) Query(name string, payload []byte, timeout time.Duration) ([][]byte, error) {
	if gm.serf == nil {
		return nil, fmt.Errorf("membership provider not started")
	}

	params := &serf.QueryParam{
		FilterNodes: nil, // Send to all nodes
		FilterTags:  nil, // No tag filtering
		RequestAck:  true,
		Timeout:     timeout,
	}

	queryResult, err := gm.serf.Query(name, payload, params)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Collect responses
	var responses [][]byte
	for response := range queryResult.ResponseCh() {
		responses = append(responses, response.Payload)
	}

	return responses, nil
}

// GetNodeAddress returns the address of a specific node
func (gm *GossipMembership) GetNodeAddress(nodeID string) (string, bool) {
	member, exists := gm.GetMember(nodeID)
	if !exists {
		return "", false
	}

	return net.JoinHostPort(member.Address, strconv.Itoa(member.Port)), true
}

// GetAliveNodes returns only the nodes that are currently alive
func (gm *GossipMembership) GetAliveNodes() []ClusterMember {
	members := gm.GetMembers()
	var aliveMembers []ClusterMember

	for _, member := range members {
		if member.Status == NodeAlive {
			aliveMembers = append(aliveMembers, member)
		}
	}

	return aliveMembers
}

// SetUserEventHandler sets a handler function for user events
func (gm *GossipMembership) SetUserEventHandler(handler func(eventName string, payload []byte)) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.userEventHandler = handler
}
