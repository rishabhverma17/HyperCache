package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

		// Use node-specific data directory, but avoid double-nesting
		// (e.g., if config already has /tmp/hypercache/node-1 and nodeID is "node-1")
		if !strings.HasSuffix(cfg.Node.DataDir, "/"+*nodeID) {
			cfg.Node.DataDir = fmt.Sprintf("%s/%s", cfg.Node.DataDir, *nodeID)
		}
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
		cfg.Network.HTTPPort = *port + 1000 // HTTP on RESP port + 1000
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

	logging.Info(ctx, logging.ComponentMain, logging.ActionStart, "Starting HyperCache Node", map[string]interface{}{
		"node_id":     cfg.Node.ID,
		"protocol":    *protocol,
		"resp_addr":   fmt.Sprintf("%s:%d", cfg.Network.RESPBindAddr, cfg.Network.RESPPort),
		"http_addr":   fmt.Sprintf("%s:%d", cfg.Network.HTTPBindAddr, cfg.Network.HTTPPort),
		"gossip_addr": fmt.Sprintf("%s:%d", cfg.Network.AdvertiseAddr, cfg.Network.GossipPort),
		"data_dir":    cfg.Node.DataDir,
	})

	// Log configuration details
	logging.Info(ctx, logging.ComponentMain, logging.ActionStart, "Node configuration loaded", map[string]interface{}{
		"resp_port":     cfg.Network.RESPPort,
		"http_port":     cfg.Network.HTTPPort,
		"gossip_port":   cfg.Network.GossipPort,
		"data_dir":      cfg.Node.DataDir,
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
				Enabled:          cfg.Persistence.Enabled,
				Strategy:         cfg.Persistence.Strategy,
				DataDirectory:    cfg.Node.DataDir,
				EnableAOF:        cfg.Persistence.EnableAOF,
				SyncPolicy:       cfg.Persistence.SyncPolicy,
				SyncInterval:     cfg.Persistence.SyncInterval,
				SnapshotInterval: cfg.Persistence.SnapshotInterval,
				MaxLogSize:       parseSize(cfg.Persistence.MaxLogSize),
				CompressionLevel: cfg.Persistence.CompressionLevel,
				RetainLogs:       cfg.Persistence.RetainLogs,
			},
			// Enable cuckoo filter
			FilterConfig: &filter.FilterConfig{
				Name:              "default",
				FilterType:        "cuckoo",
				ExpectedItems:     1000000, // 1M items
				FalsePositiveRate: cfg.Cache.CuckooFilterFPP,
				FingerprintSize:   12,
				BucketSize:        4,
				EnableAutoResize:  true,
				EnableStatistics:  true,
				HashFunction:      "xxhash",
			},
		}

		store, err := storage.NewBasicStore(storeConfig)
		if err != nil {
			logging.Fatal(ctx, logging.ComponentMain, logging.ActionStart, "Failed to create storage", err)
			os.Exit(1)
		}
		defer store.Close()

		// Start persistence
		if err := store.StartPersistence(shutdownCtx); err != nil {
			logging.Warn(ctx, logging.ComponentPersistence, logging.ActionStart, "Failed to start persistence", map[string]interface{}{"error": err.Error()})
		} else {
			logging.Info(ctx, logging.ComponentPersistence, logging.ActionStart, "Persistence enabled and started", map[string]interface{}{"data_dir": cfg.Node.DataDir})
		}

		// Create distributed coordinator with configuration-driven clustering
		clusterConfig := cluster.ClusterConfig{
			NodeID:                  cfg.Node.ID,
			ClusterName:             "hypercache",
			BindAddress:             cfg.Network.RESPBindAddr, // Bind to all interfaces for multi-VM
			BindPort:                cfg.Network.GossipPort,
			AdvertiseAddress:        cfg.Network.AdvertiseAddr, // VM-specific IP for multi-VM
			SeedNodes:               cfg.Cluster.Seeds,
			JoinTimeout:             30, // 30 seconds
			HeartbeatInterval:       5,  // 5 seconds
			FailureDetectionTimeout: 15, // 15 seconds (must be > heartbeat)
		}

		coord, err := cluster.NewDistributedCoordinator(clusterConfig)
		if err != nil {
			logging.Fatal(ctx, logging.ComponentMain, logging.ActionStart, "Failed to create distributed coordinator", err)
			os.Exit(1)
		}
		defer func() {
			// DistributedCoordinator doesn't have Close(), stop via context
		}()

		// Start coordinator (this handles clustering, replication, and gossip)
		if err := coord.Start(shutdownCtx); err != nil {
			logging.Fatal(ctx, logging.ComponentMain, logging.ActionStart, "Failed to start coordinator", err)
			os.Exit(1)
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
						handleReplicationEvent(shutdownCtx, event, store, cfg.Node.ID, coord)
					case <-shutdownCtx.Done():
						logging.Info(shutdownCtx, logging.ComponentCluster, logging.ActionStop, "Event subscription stopping", map[string]interface{}{
							"node_id": cfg.Node.ID,
						})
						return
					}
				}
			}()
		} else {
			logging.Warn(ctx, logging.ComponentEventBus, logging.ActionStart, "Event bus not available - replication disabled")
		}

		// Create distributed-aware RESP server using configured address
		respBindAddr := fmt.Sprintf("%s:%d", cfg.Network.RESPBindAddr, cfg.Network.RESPPort)
		respServer := resp.NewServer(respBindAddr, store, coord)

		// Start RESP server
		go func() {
			logging.Info(ctx, logging.ComponentRESP, logging.ActionStart, "RESP server listening", map[string]interface{}{"bind_addr": respBindAddr})

			if err := respServer.Start(); err != nil {
				logging.Error(ctx, logging.ComponentRESP, logging.ActionStart, "RESP server error", err, nil)
			}
		}()

		// Start HTTP API server alongside RESP using configured port
		go func() {
			if err := startHTTPServer(shutdownCtx, coord, store, cfg.Network.HTTPPort, cfg.Node.ID); err != nil {
				logging.Error(ctx, logging.ComponentHTTP, logging.ActionStart, "HTTP API server error", err, nil)
			}
		}()
	} else {
		// Internal protocol mode (future implementation)
		logging.Info(ctx, logging.ComponentMain, logging.ActionStart, "HyperCache running in standalone mode")
	}

	// Wait for interrupt signal for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	logging.Info(ctx, logging.ComponentMain, logging.ActionStop, "Shutting down HyperCache node", map[string]interface{}{"node_id": cfg.Node.ID})

	// Cancel context to stop server
	cancel()

	logging.Info(ctx, logging.ComponentMain, logging.ActionStop, "HyperCache shutdown complete")
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
		logging.Warn(nil, logging.ComponentConfig, logging.ActionValidation, "Invalid size format, defaulting to 0", map[string]interface{}{"input": sizeStr})
		return 0
	}

	multiplier, exists := multipliers[unit]
	if !exists {
		logging.Warn(nil, logging.ComponentConfig, logging.ActionValidation, "Unknown unit in size, defaulting to bytes", map[string]interface{}{"unit": unit, "input": sizeStr})
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

		logging.Debug(r.Context(), logging.ComponentHTTP, "health_check", "Health check requested")

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

	// Cuckoo filter endpoints
	mux.HandleFunc("/api/filter/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := store.FilterStats()
		if stats == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "filter not enabled", "node": nodeID})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node":  nodeID,
			"stats": stats,
		})
	})

	mux.HandleFunc("/api/filter/check/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/api/filter/check/")
		if key == "" {
			http.Error(w, "Key is required", http.StatusBadRequest)
			return
		}
		stats := store.FilterStats()
		if stats == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "filter not enabled", "node": nodeID})
			return
		}
		// Check the filter — this is a probabilistic check (might return false positive)
		mightExist := store.FilterContains(key)
		// Also check if key actually exists in the store
		_, actualErr := store.Get(key)
		actuallyExists := actualErr == nil

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":             key,
			"filter_says":     mightExist,
			"actually_exists": actuallyExists,
			"false_positive":  mightExist && !actuallyExists,
			"node":            nodeID,
		})
	})

	// Prometheus-compatible metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := store.Stats()
		health := coordinator.GetHealth()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Cache metrics
		fmt.Fprintf(w, "# HELP hypercache_items_total Total number of items in the cache\n")
		fmt.Fprintf(w, "# TYPE hypercache_items_total gauge\n")
		fmt.Fprintf(w, "hypercache_items_total{node=\"%s\"} %d\n", nodeID, stats.TotalItems)

		fmt.Fprintf(w, "# HELP hypercache_memory_bytes Current memory usage in bytes\n")
		fmt.Fprintf(w, "# TYPE hypercache_memory_bytes gauge\n")
		fmt.Fprintf(w, "hypercache_memory_bytes{node=\"%s\"} %d\n", nodeID, stats.TotalMemory)

		fmt.Fprintf(w, "# HELP hypercache_hits_total Total cache hits\n")
		fmt.Fprintf(w, "# TYPE hypercache_hits_total counter\n")
		fmt.Fprintf(w, "hypercache_hits_total{node=\"%s\"} %d\n", nodeID, stats.HitCount)

		fmt.Fprintf(w, "# HELP hypercache_misses_total Total cache misses\n")
		fmt.Fprintf(w, "# TYPE hypercache_misses_total counter\n")
		fmt.Fprintf(w, "hypercache_misses_total{node=\"%s\"} %d\n", nodeID, stats.MissCount)

		fmt.Fprintf(w, "# HELP hypercache_evictions_total Total evictions\n")
		fmt.Fprintf(w, "# TYPE hypercache_evictions_total counter\n")
		fmt.Fprintf(w, "hypercache_evictions_total{node=\"%s\"} %d\n", nodeID, stats.EvictionCount)

		fmt.Fprintf(w, "# HELP hypercache_errors_total Total errors\n")
		fmt.Fprintf(w, "# TYPE hypercache_errors_total counter\n")
		fmt.Fprintf(w, "hypercache_errors_total{node=\"%s\"} %d\n", nodeID, stats.ErrorCount)

		fmt.Fprintf(w, "# HELP hypercache_hit_rate Cache hit rate percentage\n")
		fmt.Fprintf(w, "# TYPE hypercache_hit_rate gauge\n")
		fmt.Fprintf(w, "hypercache_hit_rate{node=\"%s\"} %.2f\n", nodeID, stats.HitRate())

		// Cluster metrics
		fmt.Fprintf(w, "# HELP hypercache_cluster_healthy Whether the cluster is healthy (1=yes, 0=no)\n")
		fmt.Fprintf(w, "# TYPE hypercache_cluster_healthy gauge\n")
		healthVal := 0
		if health.Healthy {
			healthVal = 1
		}
		fmt.Fprintf(w, "hypercache_cluster_healthy{node=\"%s\"} %d\n", nodeID, healthVal)

		fmt.Fprintf(w, "# HELP hypercache_cluster_size Number of nodes in the cluster\n")
		fmt.Fprintf(w, "# TYPE hypercache_cluster_size gauge\n")
		fmt.Fprintf(w, "hypercache_cluster_size{node=\"%s\"} %d\n", nodeID, health.ClusterSize)

		fmt.Fprintf(w, "# HELP hypercache_up Whether the node is up (always 1 if reachable)\n")
		fmt.Fprintf(w, "# TYPE hypercache_up gauge\n")
		fmt.Fprintf(w, "hypercache_up{node=\"%s\"} 1\n", nodeID)
	})

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
		logging.Info(ctx, logging.ComponentHTTP, logging.ActionStart, "HTTP API server started", map[string]interface{}{"port": port, "node_id": nodeID})
	case <-time.After(100 * time.Millisecond):
		logging.Info(ctx, logging.ComponentHTTP, logging.ActionStart, "HTTP API server started", map[string]interface{}{"port": port, "node_id": nodeID})
	}

	// Wait for context cancellation
	select {
	case <-ctx.Done():
		logging.Info(ctx, logging.ComponentHTTP, logging.ActionStop, "HTTP API server shutting down", map[string]interface{}{"node_id": nodeID})
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
				logging.Info(r.Context(), logging.ComponentCache, "get_request", "Cache miss", map[string]interface{}{
					"key": key,
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

			// Ensure []byte values are converted to string to avoid JSON base64 encoding
			if b, ok := value.([]byte); ok {
				value = string(b)
			}

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
				// Tick the Lamport clock for this write
				lamportTS := uint64(0)
				if coordinator.GetClock() != nil {
					lamportTS = coordinator.GetClock().Tick()
				}

				setEvent := cluster.ClusterEvent{
					Type:          cluster.EventDataOperation,
					NodeID:        nodeID,
					CorrelationID: logging.GetCorrelationID(r.Context()),
					Timestamp:     time.Now(),
					Data: map[string]interface{}{
						"operation":  "SET",
						"key":        key,
						"value":      requestBody.Value,
						"ttl":        ttl.Seconds(),
						"lamport_ts": lamportTS,
					},
				}

				// Publish event for other nodes to process
				logging.Debug(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Publishing SET event for replication", map[string]interface{}{
					"key":       key,
					"node_id":   nodeID,
					"operation": "SET",
				})

				if pubErr := eventBus.Publish(r.Context(), setEvent); pubErr != nil {
					logging.Error(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Failed to publish SET event", pubErr, map[string]interface{}{
						"key":       key,
						"operation": "SET",
					})
				} else {
					logging.Info(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "SET event published for replication", map[string]interface{}{
						"key":       key,
						"operation": "SET",
					})
				}
			} else {
				logging.Warn(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Event bus not available - no replication", map[string]interface{}{
					"key":       key,
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

			existed := err == nil
			if err != nil {
				logging.Info(r.Context(), logging.ComponentCache, "delete_request", "Key not found for deletion", map[string]interface{}{
					"key": key,
				})
			} else {
				logging.Info(r.Context(), logging.ComponentCache, "delete_request", "Cache DELETE operation successful", map[string]interface{}{
					"key": key,
				})
			}

			// Publish DELETE event to event bus for replication to other nodes
			if existed && coordinator != nil && coordinator.GetEventBus() != nil {
				eventBus := coordinator.GetEventBus()

				// Tick the Lamport clock for this delete
				lamportTS := uint64(0)
				if coordinator.GetClock() != nil {
					lamportTS = coordinator.GetClock().Tick()
				}

				// Create DELETE event with correlation ID
				deleteEvent := cluster.ClusterEvent{
					Type:          cluster.EventDataOperation,
					NodeID:        nodeID,
					CorrelationID: logging.GetCorrelationID(r.Context()),
					Timestamp:     time.Now(),
					Data: map[string]interface{}{
						"operation":  "DELETE",
						"key":        key,
						"lamport_ts": lamportTS,
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
			} else if existed {
				logging.Warn(r.Context(), logging.ComponentEventBus, logging.ActionReplication, "Event bus not available - no replication", map[string]interface{}{
					"key":       key,
					"operation": "DELETE",
				})
			}

			response := map[string]interface{}{
				"success":        true,
				"message":        "Key deleted",
				"existed":        existed,
				"key":            key,
				"node":           nodeID,
				"replicated":     existed,
				"correlation_id": logging.GetCorrelationID(r.Context()),
			}

			if !existed {
				response["message"] = "Key not found"
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
func handleReplicationEvent(ctx context.Context, event cluster.ClusterEvent, store *storage.BasicStore, nodeID string, coordinator cluster.CoordinatorService) {
	// Skip events from ourselves — no need to log, the originating request already has full tracing
	if event.NodeID == nodeID {
		return
	}

	// Create context with correlation ID from the event
	correlationCtx := logging.WithCorrelationID(ctx, event.CorrelationID)

	logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Received replication event", map[string]interface{}{
		"event_type":     event.Type,
		"source_node":    event.NodeID,
		"target_node":    nodeID,
		"correlation_id": event.CorrelationID,
	})

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

		// Extract Lamport timestamp from the event
		var lamportTS uint64
		if tsInterface, exists := eventData["lamport_ts"]; exists {
			if tsFloat, ok := tsInterface.(float64); ok {
				lamportTS = uint64(tsFloat)
			}
		}

		// Witness the remote clock to advance our own
		if coordinator.GetClock() != nil && lamportTS > 0 {
			coordinator.GetClock().Witness(lamportTS)
		}

		switch operation {
		case "SET":
			value := eventData["value"] // Accept any type — store handles serialization

			var ttl time.Duration
			if ttlInterface, exists := eventData["ttl"]; exists {
				if ttlFloat, ok := ttlInterface.(float64); ok {
					ttl = time.Duration(ttlFloat) * time.Second
				}
			}

			// Use SetWithTimestamp to enforce causal ordering —
			// only overwrite if the incoming timestamp is newer
			applied, err := store.SetWithTimestamp(correlationCtx, key, value, "replication", ttl, lamportTS)
			if err != nil {
				logging.Error(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Failed to apply replicated SET", err, map[string]interface{}{
					"key":        key,
					"lamport_ts": lamportTS,
				})
			} else if applied {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Successfully applied replicated SET", map[string]interface{}{
					"key":        key,
					"lamport_ts": lamportTS,
				})
			} else {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Skipped stale replicated SET (local is newer)", map[string]interface{}{
					"key":            key,
					"remote_ts":      lamportTS,
					"local_ts":       store.GetTimestamp(key),
				})
			}

		case "DELETE":
			// Apply the replicated DELETE operation — idempotent, key may already be gone
			// For deletes, check timestamp: don't delete if a newer SET has occurred locally
			localTS := store.GetTimestamp(key)
			if lamportTS > 0 && localTS > lamportTS {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Skipped stale replicated DELETE (local SET is newer)", map[string]interface{}{
					"key":       key,
					"remote_ts": lamportTS,
					"local_ts":  localTS,
				})
			} else if err := store.Delete(key); err != nil {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Replicated DELETE (key already absent)", map[string]interface{}{
					"key": key,
				})
			} else {
				logging.Info(correlationCtx, logging.ComponentCluster, logging.ActionReplication, "Successfully applied replicated DELETE", map[string]interface{}{
					"key":        key,
					"lamport_ts": lamportTS,
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
