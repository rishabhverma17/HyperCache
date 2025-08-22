package resp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"hypercache/internal/cluster"
	"hypercache/internal/storage"
)

// Mock coordinator for testing (minimal implementation)
type mockCoordinator struct{}

func (m *mockCoordinator) Start(ctx context.Context) error { return nil }
func (m *mockCoordinator) Stop(ctx context.Context) error { return nil }
func (m *mockCoordinator) GetLocalNodeID() string { return "test-node" }
func (m *mockCoordinator) GetMembership() cluster.MembershipProvider { return nil }
func (m *mockCoordinator) GetRouting() cluster.RoutingProvider { return nil }
func (m *mockCoordinator) GetEventBus() cluster.EventBus { return nil }
func (m *mockCoordinator) TriggerRebalance(ctx context.Context) error { return nil }
func (m *mockCoordinator) GetHealth() cluster.CoordinatorHealth { return cluster.CoordinatorHealth{} }
func (m *mockCoordinator) GetMetrics() cluster.CoordinatorMetrics { return cluster.CoordinatorMetrics{} }

func newTestServer(t *testing.T) (*Server, func()) {
	// Create BasicStore directly
	config := storage.BasicStoreConfig{
		Name:             "test-store",
		MaxMemory:        1024 * 1024, // 1MB
		DefaultTTL:       0,           // No default TTL
		EnableStatistics: true,
		CleanupInterval:  time.Minute,
	}
	
	store, err := storage.NewBasicStore(config)
	if err != nil {
		t.Fatalf("Failed to create basic store: %v", err)
	}
	
	// Create mock coordinator
	coord := &mockCoordinator{}
	
	// Create server with test config
	serverConfig := ServerConfig{
		MaxConnections:   10,
		IdleTimeout:      time.Minute,
		CommandTimeout:   5 * time.Second,
		BufferSize:       1024,
		KeepAlive:        true,
		KeepAlivePeriod:  30 * time.Second,
		EnablePipelining: false, // Disable for simpler testing
		MaxPipelineDepth: 10,
	}
	
	server := NewServerWithConfig(":0", store, coord, serverConfig)
	
	// Start server
	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	
	// Update address with actual port
	server.address = server.listener.Addr().String()
	
	cleanup := func() {
		server.Stop()
		store.Close()
	}
	
	return server, cleanup
}

func TestServer_StartStop(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	// Verify server is running
	if !server.running.Load() {
		t.Error("Server should be running")
	}
	
	// Test stats
	stats := server.GetStats()
	if stats.TotalConnections != 0 {
		t.Error("Total connections should be 0 initially")
	}
	
	// Stop server
	err := server.Stop()
	if err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
	
	if server.running.Load() {
		t.Error("Server should not be running after stop")
	}
}

func TestServer_BasicCommands(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Test PING
	sendCommand(t, conn, "*1\r\n$4\r\nPING\r\n")
	response := readResponse(t, conn)
	expectedPing := "+PONG\r\n"
	if response != expectedPing {
		t.Errorf("PING: expected %q, got %q", expectedPing, response)
	}
	
	// Test PING with message
	sendCommand(t, conn, "*2\r\n$4\r\nPING\r\n$5\r\nhello\r\n")
	response = readResponse(t, conn)
	expectedPingMsg := "$5\r\nhello\r\n"
	if response != expectedPingMsg {
		t.Errorf("PING with message: expected %q, got %q", expectedPingMsg, response)
	}
}

func TestServer_KeyValueCommands(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Test SET
	sendCommand(t, conn, "*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$6\r\nvalue1\r\n")
	response := readResponse(t, conn)
	expectedSet := "+OK\r\n"
	if response != expectedSet {
		t.Errorf("SET: expected %q, got %q", expectedSet, response)
	}
	
	// Test GET existing key
	sendCommand(t, conn, "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n")
	response = readResponse(t, conn)
	expectedGet := "$6\r\nvalue1\r\n"
	if response != expectedGet {
		t.Errorf("GET existing: expected %q, got %q", expectedGet, response)
	}
	
	// Test GET non-existing key
	sendCommand(t, conn, "*2\r\n$3\r\nGET\r\n$4\r\nkey2\r\n")
	response = readResponse(t, conn)
	expectedNull := "$-1\r\n"
	if response != expectedNull {
		t.Errorf("GET non-existing: expected %q, got %q", expectedNull, response)
	}
	
	// Test EXISTS
	sendCommand(t, conn, "*2\r\n$6\r\nEXISTS\r\n$4\r\nkey1\r\n")
	response = readResponse(t, conn)
	expectedExists := ":1\r\n"
	if response != expectedExists {
		t.Errorf("EXISTS: expected %q, got %q", expectedExists, response)
	}
	
	// Test DEL
	sendCommand(t, conn, "*2\r\n$3\r\nDEL\r\n$4\r\nkey1\r\n")
	response = readResponse(t, conn)
	expectedDel := ":1\r\n"
	if response != expectedDel {
		t.Errorf("DEL: expected %q, got %q", expectedDel, response)
	}
	
	// Verify key is deleted
	sendCommand(t, conn, "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n")
	response = readResponse(t, conn)
	if response != expectedNull {
		t.Errorf("GET after DEL: expected %q, got %q", expectedNull, response)
	}
}

func TestServer_SetWithTTL(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Test SET with EX (expire in seconds)
	sendCommand(t, conn, "*5\r\n$3\r\nSET\r\n$8\r\nttl_key1\r\n$5\r\nvalue\r\n$2\r\nEX\r\n$1\r\n1\r\n")
	response := readResponse(t, conn)
	expectedSet := "+OK\r\n"
	if response != expectedSet {
		t.Errorf("SET with EX: expected %q, got %q", expectedSet, response)
	}
	
	// Verify key exists
	sendCommand(t, conn, "*2\r\n$3\r\nGET\r\n$8\r\nttl_key1\r\n")
	response = readResponse(t, conn)
	expectedGet := "$5\r\nvalue\r\n"
	if response != expectedGet {
		t.Errorf("GET TTL key: expected %q, got %q", expectedGet, response)
	}
	
	// Test SET with PX (expire in milliseconds)
	sendCommand(t, conn, "*5\r\n$3\r\nSET\r\n$8\r\nttl_key2\r\n$5\r\nvalue\r\n$2\r\nPX\r\n$3\r\n500\r\n")
	response = readResponse(t, conn)
	if response != expectedSet {
		t.Errorf("SET with PX: expected %q, got %q", expectedSet, response)
	}
}

func TestServer_MultipleKeys(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Set multiple keys
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		cmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", 
			len(key), key, len(value), value)
		
		sendCommand(t, conn, cmd)
		response := readResponse(t, conn)
		if response != "+OK\r\n" {
			t.Errorf("SET key%d failed: %s", i, response)
		}
	}
	
	// Test EXISTS with multiple keys
	sendCommand(t, conn, "*4\r\n$6\r\nEXISTS\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n$4\r\nkey3\r\n")
	response := readResponse(t, conn)
	expectedExists := ":3\r\n"
	if response != expectedExists {
		t.Errorf("EXISTS multiple: expected %q, got %q", expectedExists, response)
	}
	
	// Test DEL with multiple keys
	sendCommand(t, conn, "*3\r\n$3\r\nDEL\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n")
	response = readResponse(t, conn)
	expectedDel := ":2\r\n"
	if response != expectedDel {
		t.Errorf("DEL multiple: expected %q, got %q", expectedDel, response)
	}
	
	// Test DBSIZE
	sendCommand(t, conn, "*1\r\n$6\r\nDBSIZE\r\n")
	response = readResponse(t, conn)
	expectedSize := ":1\r\n" // Only key3 should remain
	if response != expectedSize {
		t.Errorf("DBSIZE: expected %q, got %q", expectedSize, response)
	}
}

func TestServer_InfoCommand(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Test INFO command
	sendCommand(t, conn, "*1\r\n$4\r\nINFO\r\n")
	response := readResponse(t, conn)
	
	// Verify it's a bulk string response
	if !strings.HasPrefix(response, "$") {
		t.Errorf("INFO should return bulk string, got: %s", response)
	}
	
	// Verify it contains expected sections
	if !strings.Contains(response, "# Server") {
		t.Error("INFO should contain Server section")
	}
	if !strings.Contains(response, "# Clients") {
		t.Error("INFO should contain Clients section")
	}
	if !strings.Contains(response, "# Stats") {
		t.Error("INFO should contain Stats section")
	}
}

func TestServer_StatsCommand(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Execute a few commands to generate stats
	sendCommand(t, conn, "*1\r\n$4\r\nPING\r\n")
	readResponse(t, conn)
	
	sendCommand(t, conn, "*3\r\n$3\r\nSET\r\n$4\r\ntest\r\n$5\r\nvalue\r\n")
	readResponse(t, conn)
	
	// Test STATS command
	sendCommand(t, conn, "*1\r\n$5\r\nSTATS\r\n")
	response := readResponse(t, conn)
	
	// Verify it's an array response
	if !strings.HasPrefix(response, "*") {
		t.Errorf("STATS should return array, got: %s", response)
	}
	
	// Verify it contains expected stats
	if !strings.Contains(response, "commands_processed") {
		t.Error("STATS should contain commands_processed")
	}
}

func TestServer_FlushAllCommand(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Add some data
	sendCommand(t, conn, "*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$6\r\nvalue1\r\n")
	readResponse(t, conn)
	
	sendCommand(t, conn, "*3\r\n$3\r\nSET\r\n$4\r\nkey2\r\n$6\r\nvalue2\r\n")
	readResponse(t, conn)
	
	// Verify data exists
	sendCommand(t, conn, "*1\r\n$6\r\nDBSIZE\r\n")
	response := readResponse(t, conn)
	if response != ":2\r\n" {
		t.Errorf("DBSIZE before flush: expected :2\\r\\n, got %q", response)
	}
	
	// Test FLUSHALL
	sendCommand(t, conn, "*1\r\n$8\r\nFLUSHALL\r\n")
	response = readResponse(t, conn)
	expectedFlush := "+OK\r\n"
	if response != expectedFlush {
		t.Errorf("FLUSHALL: expected %q, got %q", expectedFlush, response)
	}
	
	// Verify data is cleared
	sendCommand(t, conn, "*1\r\n$6\r\nDBSIZE\r\n")
	response = readResponse(t, conn)
	expectedSize := ":0\r\n"
	if response != expectedSize {
		t.Errorf("DBSIZE after flush: expected %q, got %q", expectedSize, response)
	}
}

func TestServer_ConcurrentConnections(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	numClients := 5
	var wg sync.WaitGroup
	
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			
			conn, err := net.Dial("tcp", server.address)
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", clientID, err)
				return
			}
			defer conn.Close()
			
			// Each client sets and gets a unique key
			key := fmt.Sprintf("client_%d_key", clientID)
			value := fmt.Sprintf("client_%d_value", clientID)
			
			// SET
			setCmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
				len(key), key, len(value), value)
			sendCommand(t, conn, setCmd)
			response := readResponse(t, conn)
			if response != "+OK\r\n" {
				t.Errorf("Client %d SET failed: %s", clientID, response)
				return
			}
			
			// GET
			getCmd := fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key)
			sendCommand(t, conn, getCmd)
			response = readResponse(t, conn)
			expectedGet := fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
			if response != expectedGet {
				t.Errorf("Client %d GET: expected %q, got %q", clientID, expectedGet, response)
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify final state
	stats := server.GetStats()
	if stats.TotalConnections != uint64(numClients) {
		t.Errorf("Expected %d total connections, got %d", numClients, stats.TotalConnections)
	}
}

func TestServer_InvalidCommands(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	
	// Test unknown command
	sendCommand(t, conn, "*1\r\n$7\r\nUNKNOWN\r\n")
	response := readResponse(t, conn)
	if !strings.HasPrefix(response, "-ERR") {
		t.Errorf("Unknown command should return error, got: %s", response)
	}
	
	// Test wrong number of arguments
	sendCommand(t, conn, "*1\r\n$3\r\nSET\r\n") // SET needs at least 2 args
	response = readResponse(t, conn)
	if !strings.HasPrefix(response, "-ERR") {
		t.Errorf("SET with wrong args should return error, got: %s", response)
	}
	
	// Test GET with wrong number of arguments
	sendCommand(t, conn, "*3\r\n$3\r\nGET\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n") // GET takes only 1 arg
	response = readResponse(t, conn)
	if !strings.HasPrefix(response, "-ERR") {
		t.Errorf("GET with wrong args should return error, got: %s", response)
	}
}

// Helper functions

func sendCommand(t *testing.T, conn net.Conn, cmd string) {
	_, err := conn.Write([]byte(cmd))
	if err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}
}

func readResponse(t *testing.T, conn net.Conn) string {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	
	parser := NewParser(conn)
	value, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	
	return string(value.Raw)
}

// Benchmark tests

func BenchmarkServer_PingCommand(b *testing.B) {
	config := storage.BasicStoreConfig{
		Name:      "bench-store",
		MaxMemory: 1024 * 1024,
	}
	
	store, err := storage.NewBasicStore(config)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	
	coord := &mockCoordinator{}
	
	server := NewServer(":0", store, coord)
	server.Start()
	defer server.Stop()
	
	server.address = server.listener.Addr().String()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	
	cmd := "*1\r\n$4\r\nPING\r\n"
	parser := NewParser(conn)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Write([]byte(cmd))
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServer_SetGetCommand(b *testing.B) {
	config := storage.BasicStoreConfig{
		Name:      "bench-store",
		MaxMemory: 1024 * 1024,
	}
	
	store, err := storage.NewBasicStore(config)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	
	coord := &mockCoordinator{}
	
	server := NewServer(":0", store, coord)
	server.Start()
	defer server.Stop()
	
	server.address = server.listener.Addr().String()
	
	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	
	setCmd := "*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$6\r\nvalue1\r\n"
	getCmd := "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n"
	parser := NewParser(conn)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// SET
		conn.Write([]byte(setCmd))
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
		
		// GET
		conn.Write([]byte(getCmd))
		_, err = parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}
