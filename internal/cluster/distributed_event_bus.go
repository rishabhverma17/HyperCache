package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"hypercache/internal/logging"
)

// DistributedEventBus implements EventBus using gossip for cluster-wide events
type DistributedEventBus struct {
	nodeID     string
	membership *GossipMembership

	// Event subscriptions
	subscribers map[chan ClusterEvent][]ClusterEventType
	subsMu      sync.RWMutex

	// Metrics
	eventsPublished int64
	eventsReceived  int64
	metricsMu       sync.RWMutex

	// Lifecycle
	running bool
	runMu   sync.RWMutex
}

// NewDistributedEventBus creates a new distributed event bus
func NewDistributedEventBus(nodeID string, membership *GossipMembership) *DistributedEventBus {
	return &DistributedEventBus{
		nodeID:      nodeID,
		membership:  membership,
		subscribers: make(map[chan ClusterEvent][]ClusterEventType),
	}
}

// Start initializes the distributed event bus
func (deb *DistributedEventBus) Start(ctx context.Context) error {
	deb.runMu.Lock()
	defer deb.runMu.Unlock()

	if deb.running {
		return fmt.Errorf("event bus already running")
	}

	deb.running = true

	// Register with gossip membership to receive user events
	deb.membership.SetUserEventHandler(deb.processIncomingGossipEvent)

	// Start listening for gossip events that represent cluster events
	go deb.listenForGossipEvents(ctx)

	return nil
}

// Stop shuts down the distributed event bus
func (deb *DistributedEventBus) Stop(ctx context.Context) error {
	deb.runMu.Lock()
	defer deb.runMu.Unlock()

	if !deb.running {
		return nil
	}

	deb.running = false

	// Close all subscriber channels
	deb.subsMu.Lock()
	for ch := range deb.subscribers {
		close(ch)
	}
	deb.subscribers = make(map[chan ClusterEvent][]ClusterEventType)
	deb.subsMu.Unlock()

	return nil
}

// Publish implements EventBus.Publish
func (deb *DistributedEventBus) Publish(ctx context.Context, event ClusterEvent) error {
	// Update metrics
	deb.metricsMu.Lock()
	deb.eventsPublished++
	deb.metricsMu.Unlock()

	// First, deliver to local subscribers
	deb.deliverLocalEvent(event)

	// Then, send to other nodes via gossip
	return deb.publishToCluster(event)
}

// Subscribe implements EventBus.Subscribe
func (deb *DistributedEventBus) Subscribe(eventTypes ...ClusterEventType) <-chan ClusterEvent {
	ch := make(chan ClusterEvent, 100)

	deb.subsMu.Lock()
	deb.subscribers[ch] = eventTypes
	deb.subsMu.Unlock()

	return ch
}

// Unsubscribe implements EventBus.Unsubscribe
func (deb *DistributedEventBus) Unsubscribe(ch <-chan ClusterEvent) {
	deb.subsMu.Lock()
	defer deb.subsMu.Unlock()

	// Find and remove the channel
	for subscriberCh := range deb.subscribers {
		if subscriberCh == ch {
			delete(deb.subscribers, subscriberCh)
			close(subscriberCh)
			break
		}
	}
}

// GetMetrics implements EventBus.GetMetrics
func (deb *DistributedEventBus) GetMetrics() EventBusMetrics {
	deb.subsMu.RLock()
	deb.metricsMu.RLock()
	defer deb.subsMu.RUnlock()
	defer deb.metricsMu.RUnlock()

	return EventBusMetrics{
		EventsPublished:   deb.eventsPublished,
		EventsReceived:    deb.eventsReceived,
		ActiveSubscribers: len(deb.subscribers),
		LastEventTime:     time.Now(),            // Approximation
		AverageLatency:    time.Millisecond * 50, // Approximation
	}
}

// publishToCluster sends the event to other nodes in the cluster
func (deb *DistributedEventBus) publishToCluster(event ClusterEvent) error {
	// Serialize the event
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create a gossip user event
	gossipEventName := fmt.Sprintf("cluster-event:%s", event.Type)

	// Send via gossip to all nodes
	err = deb.membership.SendUserEvent(gossipEventName, eventData)
	if err != nil {
		return fmt.Errorf("failed to send gossip event: %w", err)
	}

	return nil
}

// deliverLocalEvent delivers an event to local subscribers
func (deb *DistributedEventBus) deliverLocalEvent(event ClusterEvent) {
	deb.subsMu.RLock()
	defer deb.subsMu.RUnlock()

	for ch, eventTypes := range deb.subscribers {
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
			default:
				// Channel is full, skip this subscriber
				logging.Warn(nil, logging.ComponentEventBus, "channel_full", "Event channel full for subscriber")
			}
		}
	}
}

// listenForGossipEvents processes incoming gossip events and converts them to cluster events
func (deb *DistributedEventBus) listenForGossipEvents(ctx context.Context) {
	// This is a simplified approach - in a full implementation, we'd need to
	// integrate more deeply with the gossip membership to receive user events

	// For now, we'll focus on the framework and add full gossip event
	// processing in the next iteration

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Placeholder for periodic health checks or other maintenance
			deb.runMu.RLock()
			if !deb.running {
				deb.runMu.RUnlock()
				return
			}
			deb.runMu.RUnlock()
		}
	}
}

// processIncomingGossipEvent handles incoming gossip events from other nodes
func (deb *DistributedEventBus) processIncomingGossipEvent(eventName string, payload []byte) {
	// Parse the event type from the gossip event name
	if !strings.HasPrefix(eventName, "cluster-event:") {
		return // Not a cluster event
	}

	// Deserialize the event
	var event ClusterEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		logging.Error(nil, logging.ComponentEventBus, "deserialize", "Failed to deserialize cluster event", err, nil)
		return
	}

	// Skip events from ourselves to avoid loops
	if event.NodeID == deb.nodeID {
		return
	}

	// Update metrics
	deb.metricsMu.Lock()
	deb.eventsReceived++
	deb.metricsMu.Unlock()

	logging.Debug(nil, logging.ComponentEventBus, logging.ActionReplication, "Delivering remote event to local subscribers", map[string]interface{}{
		"event_type":     string(event.Type),
		"source_node":    event.NodeID,
		"correlation_id": event.CorrelationID,
	})

	// Deliver to local subscribers (correlation ID is already preserved in the event)
	deb.deliverLocalEvent(event)
}

// QueryCluster sends a query to all nodes and collects responses
func (deb *DistributedEventBus) QueryCluster(queryName string, data interface{}, timeout time.Duration) ([]interface{}, error) {
	// Serialize the query data
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize query data: %w", err)
	}

	// Send query via gossip
	responses, err := deb.membership.Query(queryName, payload, timeout)
	if err != nil {
		return nil, fmt.Errorf("cluster query failed: %w", err)
	}

	// Deserialize responses
	var results []interface{}
	for _, response := range responses {
		var result interface{}
		if err := json.Unmarshal(response, &result); err != nil {
			logging.Warn(nil, logging.ComponentEventBus, "deserialize", "Failed to deserialize query response", map[string]interface{}{"error": err.Error()})
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// PublishRebalanceEvent is a convenience method for rebalancing events
func (deb *DistributedEventBus) PublishRebalanceEvent(ctx context.Context, eventType ClusterEventType, details string) error {
	event := ClusterEvent{
		Type:      eventType,
		NodeID:    deb.nodeID,
		Data:      details,
		Timestamp: time.Now(),
	}

	return deb.Publish(ctx, event)
}

// PublishTopologyChangeEvent is a convenience method for topology change events
func (deb *DistributedEventBus) PublishTopologyChangeEvent(ctx context.Context, changeType string, affectedNode string) error {
	event := ClusterEvent{
		Type:   EventTopologyChanged,
		NodeID: deb.nodeID,
		Data: map[string]interface{}{
			"change_type":   changeType,
			"affected_node": affectedNode,
			"cluster_size":  len(deb.membership.GetAliveNodes()),
		},
		Timestamp: time.Now(),
	}

	return deb.Publish(ctx, event)
}

// GetClusterHealth queries the health of all nodes in the cluster
func (deb *DistributedEventBus) GetClusterHealth(ctx context.Context) (map[string]interface{}, error) {
	_, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	responses, err := deb.QueryCluster("health-check", map[string]interface{}{
		"requestor": deb.nodeID,
		"timestamp": time.Now(),
	}, time.Second*5)

	if err != nil {
		return nil, fmt.Errorf("cluster health query failed: %w", err)
	}

	// Aggregate health information
	clusterHealth := map[string]interface{}{
		"total_nodes":      len(deb.membership.GetMembers()),
		"responding_nodes": len(responses),
		"health_responses": responses,
		"query_timestamp":  time.Now(),
	}

	return clusterHealth, nil
}
