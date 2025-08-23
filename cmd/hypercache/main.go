package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"hypercache/internal/cluster"
	"hypercache/internal/filter"
	"hypercache/internal/logging"
	"hypercache/internal/network/resp"
	"hypercache/internal/persistence"
	"hypercache/internal/storage"
	"hypercache/pkg/config"
)

var (
	configPath = flag.String("config", "configs/hypercache.yaml", "Path to configuration file")
	nodeID     = flag.String("node-id", "", "Unique node identifier")
	bindAddr   = flag.String("bind", "127.0.0.1:7000", "Address to bind the server")
	protocol   = flag.String("protocol", "internal", "Protocol to use (internal, resp)")
	port       = flag.Int("port", 7000, "Port to bind the server")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		// Early error before logging is initialized
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	
	// Override with command line flags
	if *nodeID != "" {
		cfg.Node.ID = *nodeID
		
		// Use node-specific data directory
		cfg.Node.DataDir = fmt.Sprintf("%s/%s", cfg.Node.DataDir, *nodeID)
	}
	
	// Initialize structured logging system
	logger, err := logging.InitializeFromConfig(cfg.Node.ID, logging.LogConfig{
		Level:         cfg.Logging.Level,
		EnableConsole: cfg.Logging.EnableConsole,
		EnableFile:    cfg.Logging.EnableFile,
		LogFile:       cfg.Logging.LogFile,
		BufferSize:    cfg.Logging.BufferSize,
		LogDir:        cfg.Logging.LogDir,
		MaxFileSize:   cfg.Logging.MaxFileSize,
		MaxFiles:      cfg.Logging.MaxFiles,
	})
	if err != nil {
		// Early error before logging is fully initialized
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Create context with correlation ID for startup
	startupCorrelationID := logging.NewCorrelationID()
	ctx := logging.WithCorrelationID(context.Background(), startupCorrelationID)
	
	// Log system startup
	logging.Info(ctx, logging.ComponentMain, logging.ActionStart, "HyperCache node starting", map[string]interface{}{
		"node_id":     cfg.Node.ID,
		"protocol":    *protocol,
		"config_file": *configPath,
		"version":     "2.0.0",
	})
	
	// Ensure data directory exists
	if _, err := os.Stat(cfg.Node.DataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.Node.DataDir, 0755); err != nil {
			logging.Fatal(ctx, logging.ComponentMain, logging.ActionStart, "Failed to create data directory", err)
			os.Exit(1)
		}
	}
	
	// Use port flag if explicitly specified (different from default)
	if *protocol == "resp" && *port != 7000 {
		cfg.Network.RESPPort = *port
		cfg.Network.HTTPPort = *port + 1000  // HTTP on RESP port + 1000
	}
	// For RESP protocol, ensure we have valid ports from config if not overridden
	if *protocol == "resp" && *port == 7000 {
		// Use config file ports (they should already be loaded correctly)
		// Just ensure they're reasonable defaults if not set
		if cfg.Network.RESPPort == 0 {
			cfg.Network.RESPPort = 7000
		}
		if cfg.Network.HTTPPort == 0 {
			cfg.Network.HTTPPort = 8000
		}
	}

	fmt.Printf("🚀 Starting HyperCache Node: %s\n", cfg.Node.ID)
	fmt.Printf("📡 Protocol: %s\n", *protocol)
	fmt.Printf("📡 RESP API: %s:%d\n", cfg.Network.RESPBindAddr, cfg.Network.RESPPort)
	fmt.Printf("📡 HTTP API: %s:%d\n", cfg.Network.HTTPBindAddr, cfg.Network.HTTPPort)
	fmt.Printf("📡 Gossip: %s:%d\n", cfg.Network.AdvertiseAddr, cfg.Network.GossipPort)
	fmt.Printf("💾 Data directory: %s\n", cfg.Node.DataDir)

	// Log configuration details
	logging.Info(ctx, logging.ComponentMain, logging.ActionStart, "Node configuration loaded", map[string]interface{}{
		"resp_port":    cfg.Network.RESPPort,
		"http_port":    cfg.Network.HTTPPort,
		"gossip_port":  cfg.Network.GossipPort,
		"data_dir":     cfg.Node.DataDir,
		"cluster_seeds": cfg.Cluster.Seeds,
	})

	// Create context for graceful shutdown (preserving correlation ID)
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start server based on protocol
	if *protocol == "resp" {
		// Parse cache memory limit
		maxMemory := uint64(8 * 1024 * 1024 * 1024) // Default 8GB
		
		// Parse default TTL
		defaultTTL := time.Hour
		if cfg.Cache.DefaultTTL != "" {
			if parsed, err := time.ParseDuration(cfg.Cache.DefaultTTL); err == nil {
				defaultTTL = parsed
			}
		}
		
		// Create storage for RESP server using configuration
		storeConfig := storage.BasicStoreConfig{
			Name:             "default",
			MaxMemory:        maxMemory,
			DefaultTTL:       defaultTTL,
			EnableStatistics: true,
			CleanupInterval:  time.Minute,
			// Enable persistence if configured
			PersistenceConfig: &persistence.PersistenceConfig{
				Enabled:           cfg.Persistence.Enabled,
				Strategy:          cfg.Persistence.Strategy,
				DataDirectory:     cfg.Node.DataDir,
				EnableAOF:         cfg.Persistence.EnableAOF,
				SyncPolicy:        cfg.Persistence.SyncPolicy,
				SyncInterval:      cfg.Persistence.SyncInterval,
				SnapshotInterval:  cfg.Persistence.SnapshotInterval,
				MaxLogSize:        parseSize(cfg.Persistence.MaxLogSize),
				CompressionLevel:  cfg.Persistence.CompressionLevel,
				RetainLogs:        cfg.Persistence.RetainLogs,
			},
			// Enable cuckoo filter
			FilterConfig: &filter.FilterConfig{
				Name:                 "default",
				FilterType:           "cuckoo",
				ExpectedItems:        1000000, // 1M items
				FalsePositiveRate:    cfg.Cache.CuckooFilterFPP,
				FingerprintSize:      12,
				BucketSize:          4,
				EnableAutoResize:    true,
				EnableStatistics:    true,
				HashFunction:        "xxhash",
			},
		}
		
		store, err := storage.NewBasicStore(storeConfig)
		if err != nil {
			log.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()
		
		// Start persistence
		if err := store.StartPersistence(shutdownCtx); err != nil {
			log.Printf("Warning: Failed to start persistence: %v", err)
		} else {
			log.Printf("✅ Persistence enabled and started: %s", cfg.Node.DataDir)
		}
		
		// Create distributed coordinator with configuration-driven clustering
		clusterConfig := cluster.ClusterConfig{
			NodeID:                  cfg.Node.ID,
			ClusterName:            "hypercache",
			BindAddress:            cfg.Network.RESPBindAddr,  // Bind to all interfaces for multi-VM
			BindPort:               cfg.Network.GossipPort,
			AdvertiseAddress:       cfg.Network.AdvertiseAddr, // VM-specific IP for multi-VM
			SeedNodes:              cfg.Cluster.Seeds,
			JoinTimeout:            30,  // 30 seconds
			HeartbeatInterval:      5,   // 5 seconds
			FailureDetectionTimeout: 15, // 15 seconds (must be > heartbeat)
		}
		
		coord, err := cluster.NewDistributedCoordinator(clusterConfig)
		if err != nil {
			log.Fatalf("Failed to create distributed coordinator: %v", err)
		}
		defer func() {
			// DistributedCoordinator doesn't have Close(), stop via context
		}()
		
		// Start coordinator (this handles clustering, replication, and gossip)
		if err := coord.Start(shutdownCtx); err != nil {
			log.Fatalf("Failed to start coordinator: %v", err)
		}
		
		// Subscribe to replication events
		if eventBus := coord.GetEventBus(); eventBus != nil {
			eventsChan := eventBus.Subscribe(cluster.EventDataOperation)
			
			// Start event handler in background
			go func() {
				logging.Info(shutdownCtx, logging.ComponentCluster, logging.ActionStart, "Event subscription started for replication", map[string]interface{}{
					"node_id": cfg.Node.ID,
				})
				
				for {
					select {
					case event := <-eventsChan:
						handleReplicationEvent(shutdownCtx, event, store, cfg.Node.ID)
					case <-shutdownCtx.Done():
						logging.Info(shutdownCtx, logging.ComponentCluster, logging.ActionStop, "Event subscription stopping", map[string]interface{}{
							"node_id": cfg.Node.ID,
						})
						return
					}
				}
			}()
		} else {
			log.Printf("Warning: Event bus not available - replication disabled")
		}
		
		// Create distributed-aware RESP server using configured address
		respBindAddr := fmt.Sprintf("%s:%d", cfg.Network.RESPBindAddr, cfg.Network.RESPPort)
		respServer := resp.NewServer(respBindAddr, store, coord)
		
		// Start RESP server
		go func() {
			fmt.Printf("🌐 RESP server listening on %s (Redis-compatible)\n", respBindAddr)
			
			if err := respServer.Start(); err != nil {
				log.Printf("RESP server error: %v", err)
			}
		}()
		
		// Start HTTP API server alongside RESP using configured port
		go func() {
			if err := startHTTPServer(shutdownCtx, coord, store, cfg.Network.HTTPPort, cfg.Node.ID); err != nil {
				log.Printf("HTTP API server error: %v", err)
			}
		}()
	} else {
		// Internal protocol mode (future implementation)
		fmt.Printf("🌐 HyperCache running in standalone mode (internal protocol)\n")
	}

	// Wait for interrupt signal for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	fmt.Printf("\n🛑 Shutting down HyperCache node: %s\n", cfg.Node.ID)

	// Cancel context to stop server
	cancel()

	fmt.Println("✅ HyperCache shutdown complete")
}

// Helper function to parse size strings (e.g., "100MB") into int64 bytes
func parseSize(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}
	
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}
	
	var size int64
	var unit string
	
	n, err := fmt.Sscanf(sizeStr, "%d%s", &size, &unit)
	if err != nil || n != 2 {
		fmt.Printf("Warning: invalid size format '%s', defaulting to 0\n", sizeStr)
		return 0
	}
	
	multiplier, exists := multipliers[unit]
	if !exists {
		fmt.Printf("Warning: unknown unit '%s' in size '%s', defaulting to bytes\n", unit, sizeStr)
		multiplier = 1
	}
	
	return size * multiplier
}

// HTTP API Server for REST endpoints
func startHTTPServer(ctx context.Context, coordinator cluster.CoordinatorService, store *storage.BasicStore, port int, nodeID string) error {
	mux := http.NewServeMux()
	
	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Extract correlation ID from context
		correlationID := logging.GetCorrelationID(r.Context())
		if correlationID == "" {
			correlationID = logging.NewCorrelationID()
			r = r.WithContext(logging.WithCorrelationID(r.Context(), correlationID))
		}
		
		logging.Info(r.Context(), logging.ComponentHTTP, "health_check", "Health check requested")
		
		health := coordinator.GetHealth()
		response := map[string]interface{}{
			"healthy":        health.Healthy,
			"node":           nodeID,
			"cluster_size":   health.ClusterSize,
			"correlation_id": correlationID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Correlation-ID", correlationID)
		json.NewEncoder(w).Encode(response)
	})
	
	// Cluster members endpoint
	mux.HandleFunc("/api/cluster/members", func(w http.ResponseWriter, r *http.Request) {
		// Extract correlation ID from context
		correlationID := logging.GetCorrelationID(r.Context())
		if correlationID == "" {
			correlationID = logging.NewCorrelationID()
			r = r.WithContext(logging.WithCorrelationID(r.Context(), correlationID))
		}
		
		logging.Info(r.Context(), logging.ComponentHTTP, "cluster_members", "Cluster members requested")
		
		membership := coordinator.GetMembership()
		if membership == nil {
			http.Error(w, "Cluster membership not available", http.StatusServiceUnavailable)
			return
		}
		
		members := membership.GetMembers()
		response := map[string]interface{}{
			"members":        members,
			"total_count":    len(members),
			"node":           nodeID,
			"correlation_id": correlationID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Correlation-ID", correlationID)
		json.NewEncoder(w).Encode(response)
	})
	
	// Cache operations with middleware
	mux.Handle("/api/cache/", logging.HTTPMiddleware(http.HandlerFunc(handleCacheRequest(coordinator, store, nodeID))))
	
	// Wrap the main handler with CORS and logging middleware
	handler := logging.CorrelationIDMiddleware(mux)
	
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	
	logging.Info(ctx, logging.ComponentHTTP, logging.ActionStart, "HTTP API server starting", map[string]interface{}{
		"port":    port,
		"node_id": nodeID,
	})
	fmt.Printf("🌐 HTTP API server starting on port %d for node %s\n", port, nodeID)
	
	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("HTTP server failed: %v", err)
		} else {
			serverErr <- nil
		}
	}()
	
	// Wait a moment to see if server startup fails immediately
	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
		fmt.Printf("✅ HTTP API server started successfully on port %d for node %s\n", port, nodeID)
	case <-time.After(100 * time.Millisecond):
		fmt.Printf("✅ HTTP API server started successfully on port %d for node %s\n", port, nodeID)
	}
	
	// Wait for context cancellation
	select {
	case <-ctx.Done():
		fmt.Printf("🛑 HTTP API server shutting down for node %s\n", nodeID)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}

func handleCacheRequest(coordinator cluster.CoordinatorService, store *storage.BasicStore, nodeID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract key from URL path
		path := strings.TrimPrefix(r.URL.Path, "/api/cache/")
		if path == "" {
			http.Error(w, "Key is required", http.StatusBadRequest)
			return
		}
		
		key := path
		
		switch r.Method {
		case http.MethodGet:
			// Log GET request start
			logging.Info(r.Context(), logging.ComponentCache, "get_request", "Cache GET operation started", map[string]interface{}{
				"key":     key,
				"node_id": nodeID,
			})
			
			// Get operation
			timer := logging.StartTimer(r.Context(), logging.ComponentCache, "get_operation", "Cache GET operation")
			value, err := store.Get(key)
			timer()
			
			if err != nil {
				logging.Warn(r.Context(), logging.ComponentCache, "get_request", "Cache GET operation failed - key not found", map[string]interface{}{
					"key":   key,
					"error": err.Error(),
				})
				
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success":        false,
					"error":          "Key not found",
					"key":            key,
					"node":           nodeID,
					"correlation_id": logging.GetCorrelationID(r.Context()),
				})
				return
			}
			
			logging.Info(r.Context(), logging.ComponentCache, "get_request", "Cache GET operation successful", map[string]interface{}{
				"key":   key,
				"found": true,
			})
			
			response := map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"key":   key,
					"value": value,
				},
				"node":           nodeID,
				"local":          true, // Since we're getting from local store
				"correlation_id": logging.GetCorrelationID(r.Context()),
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			
		case http.MethodPut:
			// Log PUT request start
			logging.Info(r.Context(), logging.ComponentCache, "put_request", "Cache PUT operation started", map[string]interface{}{
				"key":     key,
				"node_id": nodeID,
			})
			
			// Set operation
			var requestBody struct {
				Value interface{} `json:"value"`
			}
			
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				logging.Error(r.Context(), logging.ComponentCache, "put_request", "Failed to decode PUT request body", err, map[string]interface{}{
					"key": key,
				})
				http.Error(w, "Invalid JSON body", http.StatusBadRequest)
				return
			}
			
			// Store the value with default TTL (1 hour)
			ttl := time.Hour
			timer := logging.StartTimer(r.Context(), logging.ComponentCache, "set_operation", "Cache SET operation")
			err := store.Set(key, requestBody.Value, "http-api", ttl)
			timer()
			
			if err != nil {
				logging.Error(r.Context(), logging.ComponentCache, "put_request", "Failed to set key in cache", err, map[string]interface{}{
					"key":   key,
					"value": requestBody.Value,
				})
				http.Error(w, fmt.Sprintf("Failed to set key: %v", err), http.StatusInternalServerError)
				return
			}
			
			logging.Info(r.Context(), logging.ComponentCache, "put_request", "Cache PUT operation successful", map[string]interface{}{
				"key":       key,
				"ttl_hours": ttl.Hours(),
			})
			
			// Publish SET event to event bus for replication to other nodes
			if coordinator != nil && coordinator.GetEventBus() != nil {
				eventBus := coordinator.GetEventBus()
				
				// Create SET event with correlation ID
				setEvent := cluster.ClusterEvent{
					Type:          cluster.EventDataOperation,
					NodeID:        nodeID,
					CorrelationID: logging.GetCorrelationID(r.Context()),
					Timestamp:     time.Now(),
					Data: map[string]interface{}{
						"operation": "SET",
						"key":       key,
						"value":     requestBody.Value,
						"ttl":       ttl.Seconds(),
					},
				}
				
				// Publish event for other nodes to process
				logging.Debug(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Publishing SET event for replication", map[string]interface{}{
					"key":        key,
					"node_id":    nodeID,
					"operation":  "SET",
				})
				
				if pubErr := eventBus.Publish(r.Context(), setEvent); pubErr != nil {
					logging.Error(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Failed to publish SET event", pubErr, map[string]interface{}{
						"key": key,
						"operation": "SET",
					})
				} else {
					logging.Info(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "SET event published for replication", map[string]interface{}{
						"key": key,
						"operation": "SET",
					})
				}
			} else {
				logging.Warn(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Event bus not available - no replication", map[string]interface{}{
					"key": key,
					"operation": "SET",
				})
			}
			
			response := map[string]interface{}{
				"success": true,
				"message": "Key set successfully",
				"data": map[string]interface{}{
					"key":   key,
					"value": requestBody.Value,
				},
				"node":           nodeID,
				"replicated":     true,
				"correlation_id": logging.GetCorrelationID(r.Context()),
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			
		case http.MethodDelete:
			// Log DELETE request start
			logging.Info(r.Context(), logging.ComponentCache, "delete_request", "Cache DELETE operation started", map[string]interface{}{
				"key":     key,
				"node_id": nodeID,
			})
			
			// Delete operation
			timer := logging.StartTimer(r.Context(), logging.ComponentCache, "delete_operation", "Cache DELETE operation")
			err := store.Delete(key)
			timer()
			
			if err != nil {
				logging.Error(r.Context(), logging.ComponentCache, "delete_request", "Failed to delete key from cache", err, map[string]interface{}{
					"key": key,
				})
				http.Error(w, fmt.Sprintf("Failed to delete key: %v", err), http.StatusInternalServerError)
				return
			}
			
			logging.Info(r.Context(), logging.ComponentCache, "delete_request", "Cache DELETE operation successful", map[string]interface{}{
				"key": key,
			})
			
			// Publish DELETE event to event bus for replication to other nodes
			if coordinator != nil && coordinator.GetEventBus() != nil {
				eventBus := coordinator.GetEventBus()
				
				// Create DELETE event with correlation ID
				deleteEvent := cluster.ClusterEvent{
					Type:          cluster.EventDataOperation,
					NodeID:        nodeID,
					CorrelationID: logging.GetCorrelationID(r.Context()),
					Timestamp:     time.Now(),
					Data: map[string]interface{}{
						"operation": "DELETE",
						"key":       key,
					},
				}
				
				// Log replication attempt
				logging.Debug(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Publishing DELETE event for replication", map[string]interface{}{
					"key":       key,
					"node_id":   nodeID,
					"operation": "DELETE",
				})
				
				// Publish event for other nodes to process
				if pubErr := eventBus.Publish(r.Context(), deleteEvent); pubErr != nil {
					logging.Error(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Failed to publish DELETE event", pubErr, map[string]interface{}{
						"key":       key,
						"operation": "DELETE",
					})
				} else {
					logging.Info(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "DELETE event published for replication", map[string]interface{}{
						"key":       key,
						"operation": "DELETE",
					})
				}
			} else {
				logging.Warn(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Event bus not available - no replication", map[string]interface{}{
					"key":       key,
					"operation": "DELETE",
				})
			}
			
			response := map[string]interface{}{
				"success":        true,
				"message":        "Key deleted",
				"existed":        true,
				"key":            key,
				"node":           nodeID,
				"replicated":     true,
				"correlation_id": logging.GetCorrelationID(r.Context()),
			}
			
			if err != nil {
				response["message"] = "Key not found or delete failed"
				response["existed"] = false
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
				
		default:
			logging.Warn(r.Context(), logging.ComponentHTTP, "unsupported_method", "Unsupported HTTP method", map[string]interface{}{
				"method": r.Method,
				"path":   r.URL.Path,
			})
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handleReplicationEvent processes incoming replication events from other nodes
func handleReplicationEvent(ctx context.Context, event cluster.ClusterEvent, store *storage.BasicStore, nodeID string) {
	// Create context with correlation ID from the event
	correlationCtx := logging.WithCorrelationID(ctx, event.CorrelationID)
	
	logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Received replication event", map[string]interface{}{
		"event_type":      event.Type,
		"source_node":     event.NodeID,
		"target_node":     nodeID,
		"correlation_id":  event.CorrelationID,
	})
	
	// Skip events from ourselves
	if event.NodeID == nodeID {
		return
	}
	
	// Process data operation events
	if event.Type == cluster.EventDataOperation {
		eventData, ok := event.Data.(map[string]interface{})
		if !ok {
			logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Invalid event data format", fmt.Errorf("expected map[string]interface{}, got %T", event.Data), nil)
			return
		}
		
		operation, ok := eventData["operation"].(string)
		if !ok {
			logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Invalid operation format", fmt.Errorf("expected string, got %T", eventData["operation"]), nil)
			return
		}
		
		key, ok := eventData["key"].(string)
		if !ok {
			logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Invalid key format", fmt.Errorf("expected string, got %T", eventData["key"]), nil)
			return
		}
		
		switch operation {
		case "SET":
			value, ok := eventData["value"].(string)
			if !ok {
				logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Invalid value format for SET", fmt.Errorf("expected string, got %T", eventData["value"]), nil)
				return
			}
			
			var ttl time.Duration
			if ttlInterface, exists := eventData["ttl"]; exists {
				if ttlFloat, ok := ttlInterface.(float64); ok {
					ttl = time.Duration(ttlFloat) * time.Second
				}
			}
			
			// Apply the replicated SET operation
			logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Applying replicated SET operation", map[string]interface{}{
				"key":   key,
				"value": value,
				"ttl":   ttl.String(),
			})
			
			if err := store.SetWithContext(correlationCtx, key, value, "replication", ttl); err != nil {
				logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Failed to apply replicated SET", err, map[string]interface{}{
					"key": key,
				})
			} else {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Successfully applied replicated SET", map[string]interface{}{
					"key": key,
				})
			}
			
		case "DELETE":
			// Apply the replicated DELETE operation
			logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Applying replicated DELETE operation", map[string]interface{}{
				"key": key,
			})
			
			if err := store.Delete(key); err != nil {
				logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Failed to apply replicated DELETE", err, map[string]interface{}{
					"key": key,
				})
			} else {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Successfully applied replicated DELETE", map[string]interface{}{
					"key": key,
				})
			}
			
		default:
			logging.Warn(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Unknown replication operation", map[string]interface{}{
				"operation": operation,
				"key":       key,
			})
		}
	}
}
