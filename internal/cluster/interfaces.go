// Package cluster provides distributed coordination components for hypercache
package cluster

import (
	"context"
	"fmt"
	"time"
)

// ClusterConfig defines configuration for the entire cluster coordination system
type ClusterConfig struct {
	// Node identification
	NodeID      string `yaml:"node_id" json:"node_id"`
	ClusterName string `yaml:"cluster_name" json:"cluster_name"`

	// Network configuration
	BindAddress      string `yaml:"bind_address" json:"bind_address"`
	BindPort         int    `yaml:"bind_port" json:"bind_port"`
	AdvertiseAddress string `yaml:"advertise_address" json:"advertise_address"`

	// Seed nodes for bootstrap
	SeedNodes []string `yaml:"seed_nodes" json:"seed_nodes"`

	// Hash ring configuration
	HashRing HashRingConfig `yaml:"hash_ring" json:"hash_ring"`

	// Timeouts and intervals
	JoinTimeout             int `yaml:"join_timeout_seconds" json:"join_timeout_seconds"`
	HeartbeatInterval       int `yaml:"heartbeat_interval_seconds" json:"heartbeat_interval_seconds"`
	FailureDetectionTimeout int `yaml:"failure_detection_timeout_seconds" json:"failure_detection_timeout_seconds"`

	// Consensus configuration (for when we add Raft)
	ConsensusEnabled  bool   `yaml:"consensus_enabled" json:"consensus_enabled"`
	DataDirectory     string `yaml:"data_directory" json:"data_directory"`
	SnapshotThreshold int    `yaml:"snapshot_threshold" json:"snapshot_threshold"`
}

// DefaultClusterConfig returns a production-ready default configuration
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		NodeID:      "", // Must be set by caller
		ClusterName: "hypercache",

		BindAddress:      "0.0.0.0",
		BindPort:         7946,
		AdvertiseAddress: "", // Auto-detect

		SeedNodes: []string{},

		HashRing: DefaultHashRingConfig(),

		JoinTimeout:             30,
		HeartbeatInterval:       5,
		FailureDetectionTimeout: 30,

		ConsensusEnabled:  false, // Start simple
		DataDirectory:     "./data",
		SnapshotThreshold: 1000,
	}
}

// ClusterMember represents a member of the cluster
type ClusterMember struct {
	NodeID   string            `json:"node_id"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Status   NodeStatus        `json:"status"`
	Metadata map[string]string `json:"metadata"`
	LastSeen time.Time         `json:"last_seen"`
	JoinedAt time.Time         `json:"joined_at"`
}

// MembershipEvent represents changes in cluster membership
type MembershipEvent struct {
	Type      MembershipEventType `json:"type"`
	Member    ClusterMember       `json:"member"`
	Timestamp time.Time           `json:"timestamp"`
}

// MembershipEventType defines the type of membership change
type MembershipEventType string

const (
	MemberJoined    MembershipEventType = "joined"
	MemberLeft      MembershipEventType = "left"
	MemberFailed    MembershipEventType = "failed"
	MemberUpdated   MembershipEventType = "updated"
	MemberRecovered MembershipEventType = "recovered"
)

// ClusterEvent represents cluster-wide events
type ClusterEvent struct {
	Type          ClusterEventType `json:"type"`
	NodeID        string           `json:"node_id"`
	CorrelationID string           `json:"correlation_id,omitempty"` // Flow correlation ID across nodes
	Data          interface{}      `json:"data,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
}

// ClusterEventType defines types of cluster events
type ClusterEventType string

const (
	EventRebalanceStarted   ClusterEventType = "rebalance_started"
	EventRebalanceCompleted ClusterEventType = "rebalance_completed"
	EventNodePromotion      ClusterEventType = "node_promotion"
	EventNodeDemotion       ClusterEventType = "node_demotion"
	EventTopologyChanged    ClusterEventType = "topology_changed"
	EventConsensusLost      ClusterEventType = "consensus_lost"
	EventConsensusRestored  ClusterEventType = "consensus_restored"
	EventDataOperation      ClusterEventType = "data_operation"
)

// MembershipProvider defines the interface for cluster membership management
type MembershipProvider interface {
	// Join the cluster
	Join(ctx context.Context, seedNodes []string) error

	// Leave the cluster gracefully
	Leave(ctx context.Context) error

	// Get current cluster members
	GetMembers() []ClusterMember

	// Get a specific member by node ID
	GetMember(nodeID string) (*ClusterMember, bool)

	// Update local node metadata
	UpdateMetadata(metadata map[string]string) error

	// Subscribe to membership changes
	Subscribe() <-chan MembershipEvent

	// Get current membership metrics
	GetMetrics() MembershipMetrics

	// Health check
	IsHealthy() bool

	// Get only alive nodes (convenience method)
	GetAliveNodes() []ClusterMember
}

// MembershipMetrics provides statistics about cluster membership
type MembershipMetrics struct {
	TotalMembers     int           `json:"total_members"`
	HealthyMembers   int           `json:"healthy_members"`
	SuspectedMembers int           `json:"suspected_members"`
	FailedMembers    int           `json:"failed_members"`
	ClusterAge       time.Duration `json:"cluster_age"`
	LastEvent        time.Time     `json:"last_event"`
	EventCount       int64         `json:"event_count"`
}

// RoutingProvider defines the interface for key routing and data placement
type RoutingProvider interface {
	// Route a key to its primary node
	RouteKey(key string) string

	// Get replica nodes for a key
	GetReplicas(key string, count int) []string

	// Check if the local node should handle this key
	IsLocal(key string) bool

	// Check if the local node is a replica for this key
	IsReplica(key string) bool

	// Get all keys that should be handled by a specific node
	GetKeysForNode(nodeID string, allKeys []string) []string

	// Analyze current distribution
	AnalyzeDistribution(keys []string) DistributionStats

	// Get routing metrics
	GetMetrics() HashRingMetrics
	
	// Redis-compatible slot-based routing
	GetHashSlot(key string) uint16
	GetNodeBySlot(slot uint16) (nodeID string, address string, port int)
	GetNodeByKey(key string) (nodeID string, address string, port int)
	GetSlotsByNode(nodeID string) []uint16
}

// EventBus defines the interface for cluster-wide event distribution
type EventBus interface {
	// Publish an event to the cluster
	Publish(ctx context.Context, event ClusterEvent) error

	// Subscribe to specific event types
	Subscribe(eventTypes ...ClusterEventType) <-chan ClusterEvent

	// Unsubscribe from events
	Unsubscribe(ch <-chan ClusterEvent)

	// Get event metrics
	GetMetrics() EventBusMetrics
}

// EventBusMetrics provides statistics about event distribution
type EventBusMetrics struct {
	EventsPublished   int64         `json:"events_published"`
	EventsReceived    int64         `json:"events_received"`
	ActiveSubscribers int           `json:"active_subscribers"`
	LastEventTime     time.Time     `json:"last_event_time"`
	AverageLatency    time.Duration `json:"average_latency"`
}

// CoordinatorService manages the overall cluster coordination
type CoordinatorService interface {
	// Start the coordinator
	Start(ctx context.Context) error

	// Stop the coordinator gracefully
	Stop(ctx context.Context) error

	// Get the local node ID
	GetLocalNodeID() string

	// Get membership provider
	GetMembership() MembershipProvider

	// Get routing provider
	GetRouting() RoutingProvider

	// Get event bus
	GetEventBus() EventBus

	// Trigger rebalancing
	TriggerRebalance(ctx context.Context) error

	// Get coordinator health status
	GetHealth() CoordinatorHealth

	// Get comprehensive metrics
	GetMetrics() CoordinatorMetrics
}

// CoordinatorHealth represents the health status of the coordinator
type CoordinatorHealth struct {
	Healthy          bool          `json:"healthy"`
	LocalNodeID      string        `json:"local_node_id"`
	ClusterSize      int           `json:"cluster_size"`
	ConnectedToSeeds bool          `json:"connected_to_seeds"`
	LastHeartbeat    time.Time     `json:"last_heartbeat"`
	Uptime           time.Duration `json:"uptime"`
	Issues           []string      `json:"issues,omitempty"`
}

// CoordinatorMetrics provides comprehensive coordinator statistics
type CoordinatorMetrics struct {
	Membership MembershipMetrics `json:"membership"`
	Routing    HashRingMetrics   `json:"routing"`
	EventBus   EventBusMetrics   `json:"event_bus"`
	Uptime     time.Duration     `json:"uptime"`
	StartTime  time.Time         `json:"start_time"`
}

// RebalanceRequest represents a request to rebalance data
type RebalanceRequest struct {
	RequestID    string            `json:"request_id"`
	RequestedBy  string            `json:"requested_by"`
	Timestamp    time.Time         `json:"timestamp"`
	Reason       string            `json:"reason"`
	AffectedKeys []string          `json:"affected_keys,omitempty"`
	SourceNode   string            `json:"source_node,omitempty"`
	TargetNode   string            `json:"target_node,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// RebalanceResponse represents the result of a rebalance operation
type RebalanceResponse struct {
	RequestID   string        `json:"request_id"`
	Success     bool          `json:"success"`
	KeysMoved   int           `json:"keys_moved"`
	BytesMoved  int64         `json:"bytes_moved"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
	CompletedAt time.Time     `json:"completed_at"`
}

// DataMigrator defines the interface for moving data between nodes
type DataMigrator interface {
	// Migrate keys from one node to another
	MigrateKeys(ctx context.Context, keys []string, fromNode, toNode string) (*RebalanceResponse, error)

	// Get migration progress
	GetProgress(requestID string) (*MigrationProgress, error)

	// Cancel an ongoing migration
	Cancel(requestID string) error

	// Get migration metrics
	GetMetrics() MigrationMetrics
}

// MigrationProgress tracks the progress of data migration
type MigrationProgress struct {
	RequestID        string        `json:"request_id"`
	TotalKeys        int           `json:"total_keys"`
	CompletedKeys    int           `json:"completed_keys"`
	FailedKeys       int           `json:"failed_keys"`
	BytesTransferred int64         `json:"bytes_transferred"`
	StartTime        time.Time     `json:"start_time"`
	EstimatedETA     time.Duration `json:"estimated_eta"`
	CurrentPhase     string        `json:"current_phase"`
}

// MigrationMetrics provides statistics about data migrations
type MigrationMetrics struct {
	ActiveMigrations    int           `json:"active_migrations"`
	CompletedMigrations int64         `json:"completed_migrations"`
	FailedMigrations    int64         `json:"failed_migrations"`
	TotalKeysMigrated   int64         `json:"total_keys_migrated"`
	TotalBytesMigrated  int64         `json:"total_bytes_migrated"`
	AverageDuration     time.Duration `json:"average_duration"`
	LastMigrationTime   time.Time     `json:"last_migration_time"`
}

// Factory function type for creating coordinators
type CoordinatorFactory func(config ClusterConfig) (CoordinatorService, error)

// Errors
var (
	ErrClusterNotInitialized = fmt.Errorf("cluster not initialized")
	ErrNodeAlreadyExists     = fmt.Errorf("node already exists in cluster")
	ErrNodeNotFound          = fmt.Errorf("node not found in cluster")
	ErrInsufficientReplicas  = fmt.Errorf("insufficient replica nodes available")
	ErrMigrationInProgress   = fmt.Errorf("migration already in progress")
	ErrMigrationNotFound     = fmt.Errorf("migration not found")
	ErrInvalidConfiguration  = fmt.Errorf("invalid cluster configuration")
	ErrJoinTimeout           = fmt.Errorf("timeout joining cluster")
	ErrConsensusLost         = fmt.Errorf("cluster consensus lost")
)

// Helper functions

// ValidateConfig validates a cluster configuration
func ValidateConfig(config ClusterConfig) error {
	if config.NodeID == "" {
		return fmt.Errorf("node_id is required: %w", ErrInvalidConfiguration)
	}

	if config.ClusterName == "" {
		return fmt.Errorf("cluster_name is required: %w", ErrInvalidConfiguration)
	}

	if config.BindPort <= 0 || config.BindPort > 65535 {
		return fmt.Errorf("bind_port must be between 1 and 65535: %w", ErrInvalidConfiguration)
	}

	if config.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat_interval must be positive: %w", ErrInvalidConfiguration)
	}

	if config.FailureDetectionTimeout <= config.HeartbeatInterval {
		return fmt.Errorf("failure_detection_timeout must be greater than heartbeat_interval: %w", ErrInvalidConfiguration)
	}

	return nil
}

// GenerateNodeID generates a unique node identifier
func GenerateNodeID() string {
	// Simple implementation - can be made more sophisticated
	return fmt.Sprintf("node-%d", time.Now().UnixNano())
}
