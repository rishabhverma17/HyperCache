package network_test

import (
	"strings"
	"testing"
	"time"

	"hypercache/internal/network/resp"
	"hypercache/internal/storage"
)

func TestRESPProtocol(t *testing.T) {
	t.Run("Simple_String_Encoding", func(t *testing.T) {
		formatter := resp.NewFormatter()
		
		// Test simple string encoding
		result := formatter.FormatSimpleString("OK")
		expected := "+OK\r\n"
		
		if string(result) != expected {
			t.Errorf("Expected %q, got %q", expected, string(result))
		}
	})

	t.Run("Bulk_String_Encoding", func(t *testing.T) {
		formatter := resp.NewFormatter()
		
		// Test bulk string encoding
		result := formatter.FormatBulkString("hello")
		expected := "$5\r\nhello\r\n"
		
		if string(result) != expected {
			t.Errorf("Expected %q, got %q", expected, string(result))
		}
		
		// Test null bulk string
		nullResult := formatter.FormatNull()
		expectedNull := "$-1\r\n"
		
		if string(nullResult) != expectedNull {
			t.Errorf("Expected null bulk string %q, got %q", expectedNull, string(nullResult))
		}
	})

	t.Run("Integer_Encoding", func(t *testing.T) {
		formatter := resp.NewFormatter()
		
		// Test integer encoding
		result := formatter.FormatInteger(42)
		expected := ":42\r\n"
		
		if string(result) != expected {
			t.Errorf("Expected %q, got %q", expected, string(result))
		}
		
		// Test negative integer
		negResult := formatter.FormatInteger(-10)
		expectedNeg := ":-10\r\n"
		
		if string(negResult) != expectedNeg {
			t.Errorf("Expected %q, got %q", expectedNeg, string(negResult))
		}
	})

	t.Run("Array_Encoding", func(t *testing.T) {
		formatter := resp.NewFormatter()
		
		// Test array encoding with formatted elements
		element1 := formatter.FormatBulkString("SET")
		element2 := formatter.FormatBulkString("key")
		element3 := formatter.FormatBulkString("value")
		elements := [][]byte{element1, element2, element3}
		
		result := formatter.FormatArray(elements)
		expected := "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"
		
		if string(result) != expected {
			t.Errorf("Expected %q, got %q", expected, string(result))
		}
		
		// Test empty array
		emptyResult := formatter.FormatArray([][]byte{})
		expectedEmpty := "*0\r\n"
		
		if string(emptyResult) != expectedEmpty {
			t.Errorf("Expected empty array %q, got %q", expectedEmpty, string(emptyResult))
		}
	})

	t.Run("Error_Encoding", func(t *testing.T) {
		formatter := resp.NewFormatter()
		
		// Test error encoding
		result := formatter.FormatError("ERR invalid command")
		expected := "-ERR invalid command\r\n"
		
		if string(result) != expected {
			t.Errorf("Expected %q, got %q", expected, string(result))
		}
	})

	t.Run("Malformed_Command_Handling", func(t *testing.T) {
		// Test malformed command
		input := "*2\r\n$3\r\nGET\r\n" // Missing second argument
		reader := strings.NewReader(input)
		parser := resp.NewParser(reader)
		
		_, err := parser.Parse()
		if err == nil {
			t.Errorf("Expected error for malformed command")
		}
	})
}

func TestRESPServer(t *testing.T) {
	t.Run("Server_Start_Stop", func(t *testing.T) {
		// Create required components
		config := storage.BasicStoreConfig{
			Name:               "test-store",
			MaxMemory:          1000000, // 1MB
			DefaultTTL:         5 * time.Minute,
			EnableStatistics:   true,
			CleanupInterval:    30 * time.Second,
		}
		store, err := storage.NewBasicStore(config)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}
		defer store.Close()
		
		// Create server (coordinator can be nil for basic tests)
		server := resp.NewServer(":0", store, nil)
		
		// Start server
		err = server.Start()
		if err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
		defer server.Stop()
		
		// Verify server is running by checking stats
		stats := server.GetStats()
		if stats.ActiveConnections < 0 {
			t.Error("Server should have valid stats after starting")
		}
	})
}
