package integration

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestClusterHealth verifies that all 3 nodes are healthy and responding
func TestClusterHealth(t *testing.T) {
	// Ensure cluster is running
	if !isClusterRunning() {
		t.Skip("Cluster not running. Start with: ./scripts/start-cluster.sh")
	}

	nodes := []string{
		"http://localhost:9080",
		"http://localhost:9081",
		"http://localhost:9082",
	}

	for i, node := range nodes {
		t.Run(fmt.Sprintf("Node_%d_Health", i+1), func(t *testing.T) {
			resp, err := http.Get(node + "/health")
			if err != nil {
				t.Fatalf("Failed to connect to node %s: %v", node, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Node %s health check failed: status %d", node, resp.StatusCode)
			}

			var health map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
				t.Fatalf("Failed to decode health response from %s: %v", node, err)
			}

			if !health["healthy"].(bool) {
				t.Fatalf("Node %s reports unhealthy", node)
			}

			t.Logf("✅ %s is healthy with cluster size: %.0f", node, health["cluster_size"])
		})
	}
}

// TestDataConsistency tests data replication across the cluster
func TestDataConsistency(t *testing.T) {
	// Ensure cluster is running
	if !isClusterRunning() {
		t.Skip("Cluster not running. Start with: ./scripts/start-cluster.sh")
	}

	testKey := "distributed-cache-test-key"
	testValue := "distributed-cache-test-value"
	
	writeNodes := []string{
		"http://localhost:9080",
		"http://localhost:9081",
		"http://localhost:9082",
	}
	
	readNodes := []string{
		"http://localhost:9080",
		"http://localhost:9081",
		"http://localhost:9082",
	}

	// Test writing to each node and reading from all others
	for i, writeNode := range writeNodes {
		t.Run(fmt.Sprintf("WriteNode_%d_ReadAll", i+1), func(t *testing.T) {
			uniqueValue := fmt.Sprintf("%s-%d", testValue, i+1)
			uniqueKey := fmt.Sprintf("%s-%d", testKey, i+1)
			
			// Write to one node
			writeURL := fmt.Sprintf("%s/api/cache/%s", writeNode, uniqueKey)
			payload := fmt.Sprintf(`{"value":"%s"}`, uniqueValue)
			
			req, err := http.NewRequest("PUT", writeURL, strings.NewReader(payload))
			if err != nil {
				t.Fatalf("Failed to create PUT request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to PUT to %s: %v", writeNode, err)
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				t.Fatalf("PUT to %s failed: status %d", writeNode, resp.StatusCode)
			}
			
			t.Logf("✅ PUT %s=%s to %s", uniqueKey, uniqueValue, writeNode)
			
			// Wait for replication (eventual consistency)
			time.Sleep(2 * time.Second)
			
			// Try to read from all nodes with retry logic
			for j, readNode := range readNodes {
				t.Run(fmt.Sprintf("ReadFromNode_%d", j+1), func(t *testing.T) {
					readURL := fmt.Sprintf("%s/api/cache/%s", readNode, uniqueKey)
					
					var lastErr error
					var retrievedValue string
					success := false
					
					// Exponential backoff retry: 1s, 2s, 4s, 8s, 16s
					for attempt := 0; attempt < 5; attempt++ {
						if attempt > 0 {
							delay := time.Duration(1<<attempt) * time.Second
							t.Logf("Retry attempt %d after %v delay", attempt+1, delay)
							time.Sleep(delay)
						}
						
						resp, err := http.Get(readURL)
						if err != nil {
							lastErr = fmt.Errorf("failed to GET from %s: %v", readNode, err)
							continue
						}
						defer resp.Body.Close()
						
						if resp.StatusCode == http.StatusNotFound {
							lastErr = fmt.Errorf("key not found on %s (status 404)", readNode)
							continue
						}
						
						if resp.StatusCode != http.StatusOK {
							lastErr = fmt.Errorf("GET from %s failed: status %d", readNode, resp.StatusCode)
							continue
						}
						
						var result map[string]interface{}
						if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
							lastErr = fmt.Errorf("failed to decode response from %s: %v", readNode, err)
							continue
						}
						
						// The API response has the value nested in the "data" field
						dataField, exists := result["data"]
						if !exists {
							lastErr = fmt.Errorf("no data field in response from %s", readNode)
							continue
						}
						
						data, ok := dataField.(map[string]interface{})
						if !ok {
							lastErr = fmt.Errorf("data field is not an object from %s", readNode)
							continue
						}
						
						rawValue, exists := data["value"]
						if !exists {
							lastErr = fmt.Errorf("no value field in data from %s", readNode)
							continue
						}
						
						// Handle potential base64 encoding
						valueStr, ok := rawValue.(string)
						if !ok {
							lastErr = fmt.Errorf("value is not a string from %s", readNode)
							continue
						}
						
						// Try to decode as base64 first
						if decodedBytes, err := base64.StdEncoding.DecodeString(valueStr); err == nil {
							// Successfully decoded, use decoded value
							retrievedValue = string(decodedBytes)
						} else {
							// Not base64 or decode failed, use raw value
							retrievedValue = valueStr
						}
						
						if retrievedValue == uniqueValue {
							success = true
							break
						} else {
							lastErr = fmt.Errorf("value mismatch on %s: expected '%s', got '%s' (raw: '%s')", 
								readNode, uniqueValue, retrievedValue, valueStr)
							continue
						}
					}
					
					if !success {
						t.Errorf("Failed to read correct value from %s after 5 attempts. Last error: %v", readNode, lastErr)
					} else {
						t.Logf("✅ Successfully read %s=%s from %s", uniqueKey, retrievedValue, readNode)
					}
				})
			}
		})
	}
	
	// Summary
	t.Logf("\n=== DATA CONSISTENCY TEST SUMMARY ===")
	t.Logf("✅ Tested writing to each node and reading from all others")
	t.Logf("✅ Used exponential backoff retry logic (up to 5 attempts)")
	t.Logf("✅ Handled base64 encoding/decoding automatically")
	t.Logf("✅ Waited 2 seconds for gossip replication")
}

// Helper function to check if cluster is running
func isClusterRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	
	nodes := []string{
		"http://localhost:9080/health",
		"http://localhost:9081/health", 
		"http://localhost:9082/health",
	}
	
	for _, node := range nodes {
		resp, err := client.Get(node)
		if err != nil {
			return false
		}
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return false
		}
	}
	
	return true
}
