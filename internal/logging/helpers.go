package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LogLevelFromString converts string to LogLevel
func LogLevelFromString(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

// InitializeFromConfig initializes the global logger from configuration
func InitializeFromConfig(nodeID string, logConfig LogConfig) (*Logger, error) {
	// Ensure log directory exists
	if logConfig.LogDir != "" {
		if err := os.MkdirAll(logConfig.LogDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}
	}

	// Set log file path if not specified
	logFile := logConfig.LogFile
	if logFile == "" && logConfig.EnableFile {
		if logConfig.LogDir != "" {
			logFile = filepath.Join(logConfig.LogDir, fmt.Sprintf("%s.log", nodeID))
		} else {
			logFile = fmt.Sprintf("%s.log", nodeID)
		}
	}

	config := Config{
		Level:         LogLevelFromString(logConfig.Level),
		NodeID:        nodeID,
		LogFile:       logFile,
		EnableConsole: logConfig.EnableConsole,
		EnableFile:    logConfig.EnableFile,
		BufferSize:    logConfig.BufferSize,
	}

	logger := NewLogger(config)
	SetGlobalLogger(logger)

	return logger, nil
}

// LogConfig represents logging configuration (matching the YAML structure)
type LogConfig struct {
	Level         string `yaml:"level"`
	EnableConsole bool   `yaml:"enable_console"`
	EnableFile    bool   `yaml:"enable_file"`
	LogFile       string `yaml:"log_file"`
	BufferSize    int    `yaml:"buffer_size"`
	LogDir        string `yaml:"log_dir"`
	MaxFileSize   string `yaml:"max_file_size"`
	MaxFiles      int    `yaml:"max_files"`
}

// ComponentNames for structured logging
const (
	ComponentRESP        = "resp"
	ComponentHTTP        = "http"
	ComponentCluster     = "cluster"
	ComponentGossip      = "gossip"
	ComponentEventBus    = "event_bus"
	ComponentCoordinator = "coordinator"
	ComponentStorage     = "storage"
	ComponentCache       = "cache"
	ComponentPersistence = "persistence"
	ComponentFilter      = "filter"
	ComponentAuth        = "auth"
	ComponentHealth      = "health"
	ComponentConfig      = "config"
	ComponentMain        = "main"
)

// ActionNames for structured logging
const (
	ActionStart       = "start"
	ActionStop        = "stop"
	ActionRequest     = "request"
	ActionResponse    = "response"
	ActionConnect     = "connect"
	ActionDisconnect  = "disconnect"
	ActionJoin        = "join"
	ActionLeave       = "leave"
	ActionReplication = "replication"
	ActionPersist     = "persist"
	ActionRestore     = "restore"
	ActionSnapshot    = "snapshot"
	ActionCompaction  = "compaction"
	ActionElection    = "election"
	ActionConsensus   = "consensus"
	ActionSync        = "sync"
	ActionValidation  = "validation"
	ActionTimeout     = "timeout"
	ActionRetry       = "retry"
	ActionFailover    = "failover"
	ActionBackup      = "backup"
	ActionCleanup     = "cleanup"
)
