package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Node        NodeConfig        `yaml:"node"`
	Network     NetworkConfig     `yaml:"network"`
	Cluster     ClusterConfig     `yaml:"cluster"`
	Storage     StorageConfig     `yaml:"storage"`
	Cache       CacheConfig       `yaml:"cache"`
	Persistence PersistenceConfig `yaml:"persistence"`
	Logging     LoggingConfig     `yaml:"logging"`
	Stores      []StoreConfig     `yaml:"stores"`
}

// NodeConfig contains node-specific configuration
type NodeConfig struct {
	ID      string `yaml:"id"`
	DataDir string `yaml:"data_dir"`
}

// NetworkConfig contains network-specific configuration for multi-VM deployments
type NetworkConfig struct {
	// RESP API configuration
	RESPBindAddr string `yaml:"resp_bind_addr"`
	RESPPort     int    `yaml:"resp_port"`
	
	// HTTP API configuration
	HTTPBindAddr string `yaml:"http_bind_addr"`
	HTTPPort     int    `yaml:"http_port"`
	
	// Cluster gossip configuration
	AdvertiseAddr string `yaml:"advertise_addr"`  // IP that other nodes use to connect
	GossipPort    int    `yaml:"gossip_port"`     // Serf gossip port
}

// ClusterConfig contains clustering configuration
type ClusterConfig struct {
	Seeds             []string `yaml:"seeds"`              // Seed nodes for joining cluster
	ReplicationFactor int      `yaml:"replication_factor"`
	ConsistencyLevel  string   `yaml:"consistency_level"`
}

// StorageConfig contains storage engine configuration
type StorageConfig struct {
	WALSyncInterval   time.Duration `yaml:"wal_sync_interval"`
	MemTableSize      string        `yaml:"memtable_size"`
	CompactionThreads int           `yaml:"compaction_threads"`
}

// PersistenceConfig defines persistence behavior
type PersistenceConfig struct {
	Enabled          bool          `yaml:"enabled"`
	Strategy         string        `yaml:"strategy"`          // "aof", "snapshot", "hybrid"
	EnableAOF        bool          `yaml:"enable_aof"`
	SyncPolicy       string        `yaml:"sync_policy"`       // "always", "everysec", "no"
	SyncInterval     time.Duration `yaml:"sync_interval"`
	SnapshotInterval time.Duration `yaml:"snapshot_interval"`
	MaxLogSize       string        `yaml:"max_log_size"`
	CompressionLevel int           `yaml:"compression_level"` // 0-9
	RetainLogs       int           `yaml:"retain_logs"`
}

// CacheConfig contains global cache configuration
type CacheConfig struct {
	MaxMemory       string  `yaml:"max_memory"`
	DefaultTTL      string  `yaml:"default_ttl"`
	CuckooFilterFPP float64 `yaml:"cuckoo_filter_fpp"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level         string `yaml:"level"`           // debug, info, warn, error, fatal
	EnableConsole bool   `yaml:"enable_console"`  // Enable console output
	EnableFile    bool   `yaml:"enable_file"`     // Enable file output
	LogFile       string `yaml:"log_file"`        // Log file path
	BufferSize    int    `yaml:"buffer_size"`     // Async log buffer size
	LogDir        string `yaml:"log_dir"`         // Log directory
	MaxFileSize   string `yaml:"max_file_size"`   // Maximum log file size before rotation
	MaxFiles      int    `yaml:"max_files"`       // Maximum number of log files to keep
}

// StoreConfig represents configuration for individual stores
type StoreConfig struct {
	Name           string        `yaml:"name"`
	EvictionPolicy string        `yaml:"eviction_policy"`
	MaxMemory      string        `yaml:"max_memory"`
	DefaultTTL     time.Duration `yaml:"default_ttl"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	// Set defaults
	config := &Config{
		Node: NodeConfig{
			ID:      "hypercache-node-1",
			DataDir: "/tmp/hypercache",
		},
		Network: NetworkConfig{
			RESPBindAddr:  "0.0.0.0",
			RESPPort:      8080,
			HTTPBindAddr:  "0.0.0.0", 
			HTTPPort:      9080,
			AdvertiseAddr: "",        // Auto-detect if empty
			GossipPort:    7946,
		},
		Cluster: ClusterConfig{
			Seeds:             []string{},
			ReplicationFactor: 3,
			ConsistencyLevel:  "eventual",
		},
		Storage: StorageConfig{
			WALSyncInterval:   10 * time.Millisecond,
			MemTableSize:      "64MB",
			CompactionThreads: 4,
		},
		Persistence: PersistenceConfig{
			Enabled:          true,
			Strategy:         "hybrid",
			EnableAOF:        true,
			SyncPolicy:       "everysec",
			SyncInterval:     1 * time.Second,
			SnapshotInterval: 15 * time.Minute,
			MaxLogSize:       "100MB",
			CompressionLevel: 6,
			RetainLogs:       3,
		},
		Cache: CacheConfig{
			MaxMemory:       "8GB",
			DefaultTTL:      "1h",
			CuckooFilterFPP: 0.01, // 1% false positive rate
		},
		Logging: LoggingConfig{
			Level:         "info",
			EnableConsole: true,
			EnableFile:    true,
			LogFile:       "",        // Will be set based on node ID
			BufferSize:    1000,
			LogDir:        "logs",
			MaxFileSize:   "100MB",
			MaxFiles:      10,
		},
		Stores: []StoreConfig{
			{
				Name:           "default",
				EvictionPolicy: "lru",
				MaxMemory:      "4GB",
				DefaultTTL:     time.Hour,
			},
			{
				Name:           "sessions",
				EvictionPolicy: "ttl",
				MaxMemory:      "1GB",
				DefaultTTL:     30 * time.Minute,
			},
			{
				Name:           "analytics",
				EvictionPolicy: "lfu",
				MaxMemory:      "2GB",
				DefaultTTL:     24 * time.Hour,
			},
		},
	}

	// Try to read file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use defaults
			fmt.Printf("⚠️  Configuration file %s not found, using defaults\n", path)
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Node.ID == "" {
		return fmt.Errorf("node.id cannot be empty")
	}
	if c.Network.RESPPort <= 0 || c.Network.RESPPort > 65535 {
		return fmt.Errorf("network.resp_port must be between 1 and 65535")
	}
	if c.Network.HTTPPort <= 0 || c.Network.HTTPPort > 65535 {
		return fmt.Errorf("network.http_port must be between 1 and 65535")
	}
	if c.Network.GossipPort <= 0 || c.Network.GossipPort > 65535 {
		return fmt.Errorf("network.gossip_port must be between 1 and 65535")
	}
	if c.Cluster.ReplicationFactor < 1 {
		return fmt.Errorf("cluster.replication_factor must be >= 1")
	}
	if len(c.Stores) == 0 {
		return fmt.Errorf("at least one store must be configured")
	}
	
	// Validate store configurations
	storeNames := make(map[string]bool)
	for _, store := range c.Stores {
		if store.Name == "" {
			return fmt.Errorf("store name cannot be empty")
		}
		if storeNames[store.Name] {
			return fmt.Errorf("duplicate store name: %s", store.Name)
		}
		storeNames[store.Name] = true
		
		if !isValidEvictionPolicy(store.EvictionPolicy) {
			return fmt.Errorf("invalid eviction policy for store %s: %s", store.Name, store.EvictionPolicy)
		}
	}
	
	// Validate persistence configuration
	if c.Persistence.Enabled {
		if !isValidPersistenceStrategy(c.Persistence.Strategy) {
			return fmt.Errorf("invalid persistence strategy: %s", c.Persistence.Strategy)
		}
		
		if !isValidSyncPolicy(c.Persistence.SyncPolicy) {
			return fmt.Errorf("invalid persistence sync policy: %s", c.Persistence.SyncPolicy)
		}
		
		if c.Persistence.CompressionLevel < 0 || c.Persistence.CompressionLevel > 9 {
			return fmt.Errorf("compression level must be between 0 and 9")
		}
	}
	
	return nil
}

// isValidEvictionPolicy checks if the eviction policy is supported
func isValidEvictionPolicy(policy string) bool {
	validPolicies := map[string]bool{
		"lru":  true, // Least Recently Used
		"lfu":  true, // Least Frequently Used  
		"fifo": true, // First In First Out
		"ttl":  true, // Time To Live based
	}
	return validPolicies[policy]
}

// isValidPersistenceStrategy checks if the persistence strategy is supported
func isValidPersistenceStrategy(strategy string) bool {
	validStrategies := map[string]bool{
		"aof":     true, // Append-Only File
		"snapshot": true, // Point-in-time snapshots
		"hybrid":   true, // Combination of AOF and snapshots
	}
	return validStrategies[strategy]
}

// isValidSyncPolicy checks if the sync policy is supported
func isValidSyncPolicy(policy string) bool {
	validPolicies := map[string]bool{
		"always":    true, // Sync after every write
		"everysec":  true, // Sync once per second
		"no":        true, // Let the OS handle syncing
	}
	return validPolicies[policy]
}

// ToClusterConfig converts the application config to internal cluster config format
func (c *Config) ToClusterConfig() interface{} {
	// This needs to match the ClusterConfig from internal/cluster/interfaces.go
	return struct {
		NodeID           string   `yaml:"node_id" json:"node_id"`
		ClusterName      string   `yaml:"cluster_name" json:"cluster_name"`
		BindAddress      string   `yaml:"bind_address" json:"bind_address"`
		BindPort         int      `yaml:"bind_port" json:"bind_port"`
		AdvertiseAddress string   `yaml:"advertise_address" json:"advertise_address"`
		SeedNodes        []string `yaml:"seed_nodes" json:"seed_nodes"`
		JoinTimeout      int      `yaml:"join_timeout_seconds" json:"join_timeout_seconds"`
		HeartbeatInterval int     `yaml:"heartbeat_interval_seconds" json:"heartbeat_interval_seconds"`
	}{
		NodeID:           c.Node.ID,
		ClusterName:      "hypercache",
		BindAddress:      "0.0.0.0",      // Always bind to all interfaces
		BindPort:         c.Network.GossipPort,
		AdvertiseAddress: c.Network.AdvertiseAddr,
		SeedNodes:        c.Cluster.Seeds,
		JoinTimeout:      30,
		HeartbeatInterval: 5,
	}
}
