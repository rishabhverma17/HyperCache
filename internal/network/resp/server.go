package resp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hypercache/internal/cluster"
	"hypercache/internal/storage"
)

// Server represents a RESP protocol server that handles Redis-compatible commands
type Server struct {
	address  string
	listener net.Listener
	store    *storage.BasicStore
	coord    cluster.CoordinatorService

	// Multi-store support
	storeManager *storage.StoreManager

	// Connection management
	connections map[net.Conn]*ClientConn
	connMutex   sync.RWMutex
	connIDSeq   uint64

	// Server state
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool

	// Configuration
	config ServerConfig

	// Statistics
	stats ServerStats
}

// ServerConfig holds server configuration
type ServerConfig struct {
	MaxConnections   int
	IdleTimeout      time.Duration
	CommandTimeout   time.Duration
	BufferSize       int
	KeepAlive        bool
	KeepAlivePeriod  time.Duration
	EnablePipelining bool
	MaxPipelineDepth int
}

// ServerStats holds server statistics
type ServerStats struct {
	TotalConnections  uint64
	ActiveConnections int32
	CommandsProcessed uint64
	ErrorsEncountered uint64
	BytesSent         uint64
	BytesReceived     uint64
}

// ClientConn represents a client connection
type ClientConn struct {
	id            uint64
	conn          net.Conn
	reader        *bufio.Reader
	parser        *Parser
	formatter     *Formatter
	lastUsed      time.Time
	selectedStore string // per-connection store selection; empty = "default"
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		MaxConnections:   1000,
		IdleTimeout:      5 * time.Minute,
		CommandTimeout:   30 * time.Second,
		BufferSize:       4096,
		KeepAlive:        true,
		KeepAlivePeriod:  time.Minute,
		EnablePipelining: true,
		MaxPipelineDepth: 100,
	}
}

// NewServer creates a new RESP server
func NewServer(address string, store *storage.BasicStore, coord cluster.CoordinatorService) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		address:     address,
		store:       store,
		coord:       coord,
		connections: make(map[net.Conn]*ClientConn),
		ctx:         ctx,
		cancel:      cancel,
		config:      DefaultServerConfig(),
	}
}

// SetStoreManager sets the multi-store manager for SELECT/CREATE/STORES commands.
func (s *Server) SetStoreManager(sm *storage.StoreManager) {
	s.storeManager = sm
}

// NewServerWithConfig creates a new RESP server with custom configuration
func NewServerWithConfig(address string, store *storage.BasicStore, coord cluster.CoordinatorService, config ServerConfig) *Server {
	server := NewServer(address, store, coord)
	server.config = config
	return server
}

// Start starts the RESP server
func (s *Server) Start() error {
	if s.running.Load() {
		return fmt.Errorf("server is already running")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.address, err)
	}

	s.listener = listener
	s.running.Store(true)

	// Start connection cleanup goroutine
	s.wg.Add(1)
	go s.connectionCleaner()

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptConnections()

	// Note: cluster event replication is handled by main.go's handleReplicationEvent
	// to avoid duplicate processing of gossip events

	return nil
}

// Stop stops the RESP server
func (s *Server) Stop() error {
	if !s.running.Load() {
		return fmt.Errorf("server is not running")
	}

	s.running.Store(false)
	s.cancel()

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all connections
	s.connMutex.Lock()
	for conn := range s.connections {
		conn.Close()
	}
	s.connMutex.Unlock()

	// Wait for goroutines to finish
	s.wg.Wait()

	return nil
}

// GetStats returns server statistics
func (s *Server) GetStats() ServerStats {
	s.connMutex.RLock()
	activeConns := int32(len(s.connections))
	s.connMutex.RUnlock()

	return ServerStats{
		TotalConnections:  atomic.LoadUint64(&s.stats.TotalConnections),
		ActiveConnections: activeConns,
		CommandsProcessed: atomic.LoadUint64(&s.stats.CommandsProcessed),
		ErrorsEncountered: atomic.LoadUint64(&s.stats.ErrorsEncountered),
		BytesSent:         atomic.LoadUint64(&s.stats.BytesSent),
		BytesReceived:     atomic.LoadUint64(&s.stats.BytesReceived),
	}
}

// acceptConnections accepts new client connections
func (s *Server) acceptConnections() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.running.Load() {
				// Only log if we're still supposed to be running
				continue
			}
			return
		}

		// Check connection limits
		s.connMutex.RLock()
		connCount := len(s.connections)
		s.connMutex.RUnlock()

		if connCount >= s.config.MaxConnections {
			conn.Close()
			atomic.AddUint64(&s.stats.ErrorsEncountered, 1)
			continue
		}

		// Configure connection
		if tcpConn, ok := conn.(*net.TCPConn); ok && s.config.KeepAlive {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(s.config.KeepAlivePeriod)
		}

		// Create client connection
		clientConn := &ClientConn{
			id:        atomic.AddUint64(&s.connIDSeq, 1),
			conn:      conn,
			reader:    bufio.NewReaderSize(conn, s.config.BufferSize),
			formatter: NewFormatter(),
			lastUsed:  time.Now(),
		}
		clientConn.parser = NewParser(clientConn.reader)

		// Track connection
		s.connMutex.Lock()
		s.connections[conn] = clientConn
		s.connMutex.Unlock()

		atomic.AddUint64(&s.stats.TotalConnections, 1)

		// Handle connection
		s.wg.Add(1)
		go s.handleConnection(clientConn)
	}
}

// handleConnection handles a client connection
func (s *Server) handleConnection(clientConn *ClientConn) {
	defer s.wg.Done()
	defer func() {
		clientConn.conn.Close()
		s.connMutex.Lock()
		delete(s.connections, clientConn.conn)
		s.connMutex.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Set read timeout
		if s.config.CommandTimeout > 0 {
			clientConn.conn.SetReadDeadline(time.Now().Add(s.config.CommandTimeout))
		}

		// Parse command
		value, err := clientConn.parser.Parse()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Send timeout error
				response := clientConn.formatter.FormatError("ERR timeout")
				clientConn.conn.Write(response)
			}
			return
		}

		clientConn.lastUsed = time.Now()

		// Process command
		err = s.processCommand(clientConn, *value)
		if err != nil {
			// Send error response
			response := clientConn.formatter.FormatError(fmt.Sprintf("ERR %s", err.Error()))
			clientConn.conn.Write(response)
			atomic.AddUint64(&s.stats.ErrorsEncountered, 1)
		}

		atomic.AddUint64(&s.stats.CommandsProcessed, 1)
	}
}

// processCommand processes a Redis command
func (s *Server) processCommand(clientConn *ClientConn, value Value) error {
	// Parse command from value
	cmd, err := ParseCommand(&value)
	if err != nil {
		return err
	}

	// Route command
	response, err := s.routeCommand(clientConn, *cmd)
	if err != nil {
		return err
	}

	// Send response
	_, err = clientConn.conn.Write(response)
	if err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	atomic.AddUint64(&s.stats.BytesSent, uint64(len(response)))
	return nil
}

// routeCommand routes a command to the appropriate handler
func (s *Server) routeCommand(clientConn *ClientConn, cmd Command) ([]byte, error) {
	switch strings.ToUpper(cmd.Name) {
	// Key-value commands
	case "GET":
		return s.handleGet(clientConn, cmd)
	case "SET":
		return s.handleSet(clientConn, cmd)
	case "DEL", "DELETE":
		return s.handleDel(clientConn, cmd)
	case "EXISTS":
		return s.handleExists(clientConn, cmd)
	case "TTL":
		return s.handleTTL(cmd)
	case "EXPIRE":
		return s.handleExpire(cmd)

	// Info commands
	case "PING":
		return s.handlePing(cmd)
	case "INFO":
		return s.handleInfo(cmd)
	case "STATS":
		return s.handleStats(cmd)

	// Administrative commands
	case "FLUSHALL":
		return s.handleFlushAll(clientConn, cmd)
	case "DBSIZE":
		return s.handleDBSize(clientConn, cmd)

	// Multi-store commands
	case "SELECT":
		return s.handleSelect(clientConn, cmd)
	case "STORES":
		return s.handleStores(cmd)

	default:
		return nil, fmt.Errorf("unknown command: %s", cmd.Name)
	}
}

// getActiveStore returns the store for the current connection.
// Uses the per-connection selection, falling back to the default store.
func (s *Server) getActiveStore(clientConn *ClientConn) *storage.BasicStore {
	name := clientConn.selectedStore
	if name == "" {
		name = "default"
	}
	if s.storeManager != nil {
		if st := s.storeManager.GetStore(name); st != nil {
			return st
		}
	}
	return s.store // fallback to default
}

// Command handlers

func (s *Server) handleGet(clientConn *ClientConn, cmd Command) ([]byte, error) {
	if len(cmd.Args) != 1 {
		return nil, fmt.Errorf("wrong number of arguments for GET")
	}

	key := cmd.Args[0]
	store := s.getActiveStore(clientConn)

	// DISTRIBUTED GET: Check local store first
	value, err := store.Get(key)
	if err == nil {
		// Key found locally, return it
		var bytes []byte
		switch v := value.(type) {
		case []byte:
			bytes = v
		case string:
			bytes = []byte(v)
		default:
			// For other types, we can't handle them in RESP
			formatter := NewFormatter()
			return formatter.FormatNull(), nil
		}

		formatter := NewFormatter()
		return formatter.FormatBulkBytes(bytes), nil
	}

	// Key not found locally - if we have a coordinator, check if we should have this key
	if s.coord != nil && s.coord.GetRouting() != nil {
		routing := s.coord.GetRouting()

		// Check if this node should be a replica for this key
		if routing.IsReplica(key) || routing.IsLocal(key) {
			// We should have this key but don't - it might not be replicated yet
			// Return null for now (in production, could wait or check other replicas)
			formatter := NewFormatter()
			return formatter.FormatNull(), nil
		}

		// This key belongs to other nodes - we should return null
		// In a full implementation, we could forward the request to the appropriate node
		formatter := NewFormatter()
		return formatter.FormatNull(), nil
	}

	// No coordinator or key not found
	formatter := NewFormatter()
	return formatter.FormatNull(), nil
}

func (s *Server) handleSet(clientConn *ClientConn, cmd Command) ([]byte, error) {
	if len(cmd.Args) < 2 {
		return nil, fmt.Errorf("wrong number of arguments for SET")
	}

	key := cmd.Args[0]
	value := []byte(cmd.Args[1]) // Store as []byte — Redis-native binary-safe storage
	store := s.getActiveStore(clientConn)

	// Parse optional arguments (EX, PX, NX, XX, etc.)
	var ttl time.Duration

	for i := 2; i < len(cmd.Args); i += 2 {
		if i+1 >= len(cmd.Args) {
			return nil, fmt.Errorf("syntax error")
		}

		option := strings.ToUpper(cmd.Args[i])
		arg := cmd.Args[i+1]

		switch option {
		case "EX":
			seconds, err := strconv.Atoi(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid expire time")
			}
			ttl = time.Duration(seconds) * time.Second
		case "PX":
			millis, err := strconv.Atoi(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid expire time")
			}
			ttl = time.Duration(millis) * time.Millisecond
		case "NX", "XX":
			// TODO: Implement conditional sets
			// NX: Only set if key doesn't exist
			// XX: Only set if key exists
		default:
			return nil, fmt.Errorf("syntax error")
		}
	}

	// DISTRIBUTED SET: Store on all replica nodes
	// Set value in local store first
	err := store.Set(key, value, "", ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to set key locally: %w", err)
	}

	// If we have a coordinator, broadcast SET event to replicas
	if s.coord != nil && s.coord.GetEventBus() != nil {
		eventBus := s.coord.GetEventBus()

		// Create a SET event to be handled by other nodes
		setEvent := cluster.ClusterEvent{
			Type:      cluster.EventDataOperation,
			NodeID:    s.coord.GetLocalNodeID(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"operation": "SET",
				"key":       key,
				"value":     string(value),
				"ttl":       ttl.Seconds(),
			},
		}

		// Publish the event (other nodes will receive and apply it)
		ctx := context.Background()
		_ = eventBus.Publish(ctx, setEvent)
	}

	formatter := NewFormatter()
	return formatter.FormatSimpleString("OK"), nil
}

func (s *Server) handleDel(clientConn *ClientConn, cmd Command) ([]byte, error) {
	if len(cmd.Args) == 0 {
		return nil, fmt.Errorf("wrong number of arguments for DEL")
	}

	deleted := int64(0)
	store := s.getActiveStore(clientConn)

	for _, key := range cmd.Args {
		err := store.Delete(key)
		if err == nil {
			deleted++

			// DISTRIBUTED DELETE: Broadcast DELETE event to replicas
			if s.coord != nil && s.coord.GetEventBus() != nil {
				eventBus := s.coord.GetEventBus()

				// Create a DELETE event to be handled by other nodes
				deleteEvent := cluster.ClusterEvent{
					Type:      cluster.EventDataOperation,
					NodeID:    s.coord.GetLocalNodeID(),
					Timestamp: time.Now(),
					Data: map[string]interface{}{
						"operation": "DELETE",
						"key":       key,
					},
				}

				// Publish the event (other nodes will receive and apply it)
				ctx := context.Background()
				_ = eventBus.Publish(ctx, deleteEvent)
			}
		}
		// Ignore errors for non-existent keys (Redis behavior)
	}

	formatter := NewFormatter()
	return formatter.FormatInteger(deleted), nil
}

func (s *Server) handleExists(clientConn *ClientConn, cmd Command) ([]byte, error) {
	if len(cmd.Args) == 0 {
		return nil, fmt.Errorf("wrong number of arguments for EXISTS")
	}

	count := int64(0)
	store := s.getActiveStore(clientConn)

	for _, key := range cmd.Args {
		_, err := store.Get(key)
		if err == nil {
			count++
		}
		// Ignore errors for non-existent keys
	}

	formatter := NewFormatter()
	return formatter.FormatInteger(count), nil
}

func (s *Server) handleTTL(cmd Command) ([]byte, error) {
	if len(cmd.Args) != 1 {
		return nil, fmt.Errorf("wrong number of arguments for TTL")
	}

	// Direct store access (simplified - TTL not implemented yet)
	formatter := NewFormatter()
	return formatter.FormatInteger(-2), nil // Not implemented
}

func (s *Server) handleExpire(cmd Command) ([]byte, error) {
	if len(cmd.Args) != 2 {
		return nil, fmt.Errorf("wrong number of arguments for EXPIRE")
	}

	// Direct store access (simplified)
	formatter := NewFormatter()
	return formatter.FormatInteger(0), nil // Not implemented
}

func (s *Server) handlePing(cmd Command) ([]byte, error) {
	formatter := NewFormatter()

	if len(cmd.Args) == 0 {
		return formatter.FormatSimpleString("PONG"), nil
	}

	// Echo back the argument
	return formatter.FormatBulkString(cmd.Args[0]), nil
}

func (s *Server) handleInfo(cmd Command) ([]byte, error) {
	stats := s.GetStats()

	info := fmt.Sprintf("# Server\n"+
		"redis_version:7.0.0\n"+
		"redis_mode:standalone\n"+
		"os:Darwin\n"+
		"arch_bits:64\n"+
		"process_id:%d\n"+
		"tcp_port:%s\n"+
		"\n"+
		"# Clients\n"+
		"connected_clients:%d\n"+
		"maxclients:%d\n"+
		"\n"+
		"# Stats\n"+
		"total_connections_received:%d\n"+
		"total_commands_processed:%d\n"+
		"instantaneous_ops_per_sec:0\n"+
		"total_net_input_bytes:%d\n"+
		"total_net_output_bytes:%d\n",
		0, // Process ID placeholder
		s.address,
		stats.ActiveConnections,
		s.config.MaxConnections,
		stats.TotalConnections,
		stats.CommandsProcessed,
		stats.BytesReceived,
		stats.BytesSent,
	)

	formatter := NewFormatter()
	return formatter.FormatBulkString(info), nil
}

func (s *Server) handleStats(cmd Command) ([]byte, error) {
	stats := s.GetStats()

	result := [][]byte{
		NewFormatter().FormatBulkString(fmt.Sprintf("total_connections:%d", stats.TotalConnections)),
		NewFormatter().FormatBulkString(fmt.Sprintf("active_connections:%d", stats.ActiveConnections)),
		NewFormatter().FormatBulkString(fmt.Sprintf("commands_processed:%d", stats.CommandsProcessed)),
		NewFormatter().FormatBulkString(fmt.Sprintf("errors_encountered:%d", stats.ErrorsEncountered)),
		NewFormatter().FormatBulkString(fmt.Sprintf("bytes_sent:%d", stats.BytesSent)),
		NewFormatter().FormatBulkString(fmt.Sprintf("bytes_received:%d", stats.BytesReceived)),
	}

	formatter := NewFormatter()
	return formatter.FormatArray(result), nil
}

func (s *Server) handleFlushAll(clientConn *ClientConn, cmd Command) ([]byte, error) {
	store := s.getActiveStore(clientConn)
	err := store.Clear()
	if err != nil {
		return nil, fmt.Errorf("failed to clear store: %w", err)
	}

	formatter := NewFormatter()
	return formatter.FormatSimpleString("OK"), nil
}

func (s *Server) handleDBSize(clientConn *ClientConn, cmd Command) ([]byte, error) {
	store := s.getActiveStore(clientConn)
	size := store.Size() // BasicStore.Size() returns uint64
	formatter := NewFormatter()
	return formatter.FormatInteger(int64(size)), nil
}

// handleSelect switches the connection to a different store (SELECT <store_name>)
func (s *Server) handleSelect(clientConn *ClientConn, cmd Command) ([]byte, error) {
	if len(cmd.Args) != 1 {
		return nil, fmt.Errorf("wrong number of arguments for SELECT")
	}

	storeName := cmd.Args[0]

	if s.storeManager == nil {
		return nil, fmt.Errorf("multi-store not enabled")
	}

	if s.storeManager.GetStore(storeName) == nil {
		return nil, fmt.Errorf("ERR store '%s' not found", storeName)
	}

	clientConn.selectedStore = storeName

	formatter := NewFormatter()
	return formatter.FormatSimpleString("OK"), nil
}

// handleStores lists all available stores (STORES)
func (s *Server) handleStores(cmd Command) ([]byte, error) {
	formatter := NewFormatter()

	if s.storeManager == nil {
		return formatter.FormatBulkString("default"), nil
	}

	stores := s.storeManager.ListStores()
	result := make([][]byte, len(stores))
	for i, name := range stores {
		result[i] = formatter.FormatBulkString(name)
	}
	return formatter.FormatArray(result), nil
}

// connectionCleaner periodically cleans up idle connections
func (s *Server) connectionCleaner() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupIdleConnections()
		}
	}
}

// clusterEventListener listens for cluster events and applies data operations
// cleanupIdleConnections removes idle connections
func (s *Server) cleanupIdleConnections() {
	if s.config.IdleTimeout <= 0 {
		return
	}

	now := time.Now()

	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	for conn, clientConn := range s.connections {
		if now.Sub(clientConn.lastUsed) > s.config.IdleTimeout {
			conn.Close()
			delete(s.connections, conn)
		}
	}
}
