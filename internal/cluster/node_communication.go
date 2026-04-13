package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// NodeCommunicator handles direct communication between nodes
type NodeCommunicator struct {
	localNodeID string
	membership  MembershipProvider
	httpClient  *http.Client

	// Request/response tracking
	pendingRequests map[string]chan *NodeResponse
	requestsMu      sync.RWMutex

	// Metrics
	requestCount  int64
	responseCount int64
	errorCount    int64
	metricsMu     sync.RWMutex
}

// NodeRequest represents a request to another node
type NodeRequest struct {
	RequestID  string        `json:"request_id"`
	FromNodeID string        `json:"from_node_id"`
	ToNodeID   string        `json:"to_node_id"`
	Type       RequestType   `json:"type"`
	Payload    interface{}   `json:"payload"`
	Timestamp  time.Time     `json:"timestamp"`
	Timeout    time.Duration `json:"timeout"`
}

// NodeResponse represents a response from another node
type NodeResponse struct {
	RequestID  string      `json:"request_id"`
	FromNodeID string      `json:"from_node_id"`
	ToNodeID   string      `json:"to_node_id"`
	Success    bool        `json:"success"`
	Payload    interface{} `json:"payload,omitempty"`
	Error      string      `json:"error,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// RequestType defines the type of inter-node request
type RequestType string

const (
	ReqTypeReplicateData RequestType = "replicate_data"
	ReqTypeGetData       RequestType = "get_data"
	ReqTypeDeleteData    RequestType = "delete_data"
	ReqTypeHealthCheck   RequestType = "health_check"
	ReqTypeClusterStatus RequestType = "cluster_status"
	ReqTypeSyncKeys      RequestType = "sync_keys"
	ReqTypeMigrateKeys   RequestType = "migrate_keys"
)

// NewNodeCommunicator creates a new node communicator
func NewNodeCommunicator(localNodeID string, membership MembershipProvider) *NodeCommunicator {
	return &NodeCommunicator{
		localNodeID:     localNodeID,
		membership:      membership,
		httpClient:      &http.Client{Timeout: time.Second * 30},
		pendingRequests: make(map[string]chan *NodeResponse),
	}
}

// SendRequest sends a request to another node
func (nc *NodeCommunicator) SendRequest(ctx context.Context, toNodeID string, reqType RequestType, payload interface{}) (*NodeResponse, error) {
	// Generate request ID
	requestID := fmt.Sprintf("%s-%d", nc.localNodeID, time.Now().UnixNano())

	// Create request
	request := &NodeRequest{
		RequestID:  requestID,
		FromNodeID: nc.localNodeID,
		ToNodeID:   toNodeID,
		Type:       reqType,
		Payload:    payload,
		Timestamp:  time.Now(),
		Timeout:    time.Second * 10,
	}

	// Get target node address
	member, exists := nc.membership.GetMember(toNodeID)
	if !exists {
		return nil, fmt.Errorf("node %s not found in cluster", toNodeID)
	}

	// Send HTTP request
	response, err := nc.sendHTTPRequest(ctx, member, request)
	if err != nil {
		nc.metricsMu.Lock()
		nc.errorCount++
		nc.metricsMu.Unlock()
		return nil, err
	}

	// Update metrics
	nc.metricsMu.Lock()
	nc.requestCount++
	nc.responseCount++
	nc.metricsMu.Unlock()

	return response, nil
}

// sendHTTPRequest sends an HTTP request to a specific node
func (nc *NodeCommunicator) sendHTTPRequest(ctx context.Context, target *ClusterMember, request *NodeRequest) (*NodeResponse, error) {
	// Serialize request
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("http://%s:%d/cluster/request", target.Address, target.Port+1000) // API port offset
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-HyperCache-Node-ID", nc.localNodeID)

	// Send request
	httpResp, err := nc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %d", httpResp.StatusCode)
	}

	// Read response
	responseData, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Deserialize response
	var response NodeResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("failed to deserialize response: %w", err)
	}

	return &response, nil
}

// BroadcastRequest sends a request to all nodes in the cluster
func (nc *NodeCommunicator) BroadcastRequest(ctx context.Context, reqType RequestType, payload interface{}) ([]*NodeResponse, error) {
	members := nc.membership.GetAliveNodes()
	responses := make([]*NodeResponse, 0, len(members))

	// Create error channel for collecting errors
	errorCh := make(chan error, len(members))
	responseCh := make(chan *NodeResponse, len(members))

	// Send requests in parallel
	var wg sync.WaitGroup
	for _, member := range members {
		// Skip self
		if member.NodeID == nc.localNodeID {
			continue
		}

		wg.Add(1)
		go func(nodeID string) {
			defer wg.Done()

			response, err := nc.SendRequest(ctx, nodeID, reqType, payload)
			if err != nil {
				errorCh <- fmt.Errorf("request to %s failed: %w", nodeID, err)
				return
			}

			responseCh <- response
		}(member.NodeID)
	}

	// Wait for all requests to complete
	go func() {
		wg.Wait()
		close(responseCh)
		close(errorCh)
	}()

	// Collect responses and errors
	var errors []error
	for {
		select {
		case response, ok := <-responseCh:
			if !ok {
				responseCh = nil
			} else {
				responses = append(responses, response)
			}
		case err, ok := <-errorCh:
			if !ok {
				errorCh = nil
			} else {
				errors = append(errors, err)
			}
		}

		if responseCh == nil && errorCh == nil {
			break
		}
	}

	// Return errors if all requests failed
	if len(errors) > 0 && len(responses) == 0 {
		return nil, fmt.Errorf("all broadcast requests failed: %v", errors)
	}

	return responses, nil
}

// ReplicateData sends data to replica nodes
func (nc *NodeCommunicator) ReplicateData(ctx context.Context, key string, value interface{}, replicaNodes []string) error {
	payload := map[string]interface{}{
		"key":       key,
		"value":     value,
		"timestamp": time.Now(),
	}

	// Send replication requests to all replica nodes
	var errors []error
	for _, nodeID := range replicaNodes {
		if nodeID == nc.localNodeID {
			continue // Skip self
		}

		response, err := nc.SendRequest(ctx, nodeID, ReqTypeReplicateData, payload)
		if err != nil {
			errors = append(errors, fmt.Errorf("replication to %s failed: %w", nodeID, err))
			continue
		}

		if !response.Success {
			errors = append(errors, fmt.Errorf("replication to %s failed: %s", nodeID, response.Error))
		}
	}

	// Consider replication successful if at least one replica succeeded
	if len(errors) > 0 && len(errors) == len(replicaNodes) {
		return fmt.Errorf("replication failed to all nodes: %v", errors)
	}

	return nil
}

// GetClusterHealth checks the health of all nodes
func (nc *NodeCommunicator) GetClusterHealth(ctx context.Context) (map[string]interface{}, error) {
	responses, err := nc.BroadcastRequest(ctx, ReqTypeHealthCheck, map[string]interface{}{
		"requestor": nc.localNodeID,
		"timestamp": time.Now(),
	})

	if err != nil {
		return nil, fmt.Errorf("health check broadcast failed: %w", err)
	}

	// Aggregate health data
	healthData := map[string]interface{}{
		"total_nodes":      len(nc.membership.GetMembers()),
		"responding_nodes": len(responses),
		"health_details":   make(map[string]interface{}),
		"timestamp":        time.Now(),
	}

	healthDetails := healthData["health_details"].(map[string]interface{})
	for _, response := range responses {
		if response.Success {
			healthDetails[response.FromNodeID] = response.Payload
		} else {
			healthDetails[response.FromNodeID] = map[string]interface{}{
				"error":   response.Error,
				"healthy": false,
			}
		}
	}

	return healthData, nil
}

// SyncKeysList synchronizes the list of keys with another node
func (nc *NodeCommunicator) SyncKeysList(ctx context.Context, targetNodeID string, localKeys []string) ([]string, error) {
	payload := map[string]interface{}{
		"keys":      localKeys,
		"node_id":   nc.localNodeID,
		"timestamp": time.Now(),
	}

	response, err := nc.SendRequest(ctx, targetNodeID, ReqTypeSyncKeys, payload)
	if err != nil {
		return nil, fmt.Errorf("key sync request failed: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("key sync failed: %s", response.Error)
	}

	// Extract remote keys from response
	responseData, ok := response.Payload.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid sync response format")
	}

	remoteKeysInterface, ok := responseData["keys"]
	if !ok {
		return nil, fmt.Errorf("no keys in sync response")
	}

	// Convert interface slice to string slice
	remoteKeysSlice, ok := remoteKeysInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("keys in wrong format")
	}

	remoteKeys := make([]string, len(remoteKeysSlice))
	for i, key := range remoteKeysSlice {
		if keyStr, ok := key.(string); ok {
			remoteKeys[i] = keyStr
		}
	}

	return remoteKeys, nil
}

// MigrateKeys requests another node to migrate specific keys
func (nc *NodeCommunicator) MigrateKeys(ctx context.Context, targetNodeID string, keys []string, destinationNodeID string) error {
	payload := map[string]interface{}{
		"keys":             keys,
		"destination_node": destinationNodeID,
		"requestor":        nc.localNodeID,
		"timestamp":        time.Now(),
	}

	response, err := nc.SendRequest(ctx, targetNodeID, ReqTypeMigrateKeys, payload)
	if err != nil {
		return fmt.Errorf("key migration request failed: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("key migration failed: %s", response.Error)
	}

	return nil
}

// GetMetrics returns communication metrics
func (nc *NodeCommunicator) GetMetrics() map[string]int64 {
	nc.metricsMu.RLock()
	defer nc.metricsMu.RUnlock()

	return map[string]int64{
		"requests_sent":      nc.requestCount,
		"responses_received": nc.responseCount,
		"errors":             nc.errorCount,
	}
}

// Close cleans up the node communicator
func (nc *NodeCommunicator) Close() {
	nc.requestsMu.Lock()
	defer nc.requestsMu.Unlock()

	// Close all pending request channels
	for _, ch := range nc.pendingRequests {
		close(ch)
	}
	nc.pendingRequests = make(map[string]chan *NodeResponse)
}

// ReplicateEntry sends a key-value pair directly to a node via HTTP POST /internal/replicate.
// This is used for hash-ring targeted replication (not gossip broadcast).
func (nc *NodeCommunicator) ReplicateEntry(ctx context.Context, nodeID string, key string, value interface{}, ttlSeconds float64, lamportTS uint64) error {
	member, exists := nc.membership.GetMember(nodeID)
	if !exists {
		return fmt.Errorf("node %s not found in cluster", nodeID)
	}

	httpPort := ""
	if hp, ok := member.Metadata["http_port"]; ok {
		httpPort = hp
	}
	if httpPort == "" || httpPort == "0" {
		// Fallback: API port = gossip port + 1000
		httpPort = fmt.Sprintf("%d", member.Port+1000)
	}

	payload := map[string]interface{}{
		"key":        key,
		"value":      value,
		"ttl":        ttlSeconds,
		"lamport_ts": lamportTS,
		"from_node":  nc.localNodeID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal replication payload: %w", err)
	}

	url := fmt.Sprintf("http://%s:%s/internal/replicate", member.Address, httpPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HyperCache-Node-ID", nc.localNodeID)

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("replication HTTP request to %s failed: %w", nodeID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("replication to %s returned %d: %s", nodeID, resp.StatusCode, string(body))
	}
	return nil
}

// ProxyGet forwards a GET request to the owner node and returns the raw value.
func (nc *NodeCommunicator) ProxyGet(ctx context.Context, nodeID string, key string) (interface{}, bool, error) {
	member, exists := nc.membership.GetMember(nodeID)
	if !exists {
		return nil, false, fmt.Errorf("node %s not found in cluster", nodeID)
	}

	httpPort := ""
	if hp, ok := member.Metadata["http_port"]; ok {
		httpPort = hp
	}
	if httpPort == "" || httpPort == "0" {
		httpPort = fmt.Sprintf("%d", member.Port+1000)
	}

	url := fmt.Sprintf("http://%s:%s/internal/get/%s", member.Address, httpPort, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("X-HyperCache-Node-ID", nc.localNodeID)

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("proxy GET to %s failed: %w", nodeID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("proxy GET to %s returned %d", nodeID, resp.StatusCode)
	}

	var body struct {
		Value interface{} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("failed to decode proxy GET response: %w", err)
	}
	return body.Value, true, nil
}

// ProxySet forwards a SET request to the owner node.
func (nc *NodeCommunicator) ProxySet(ctx context.Context, nodeID string, key string, value interface{}, ttlSeconds float64) error {
	member, exists := nc.membership.GetMember(nodeID)
	if !exists {
		return fmt.Errorf("node %s not found in cluster", nodeID)
	}

	httpPort := ""
	if hp, ok := member.Metadata["http_port"]; ok {
		httpPort = hp
	}
	if httpPort == "" || httpPort == "0" {
		httpPort = fmt.Sprintf("%d", member.Port+1000)
	}

	payload := map[string]interface{}{
		"value": value,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%s/api/cache/%s", member.Address, httpPort, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HyperCache-Node-ID", nc.localNodeID)
	req.Header.Set("X-HyperCache-Proxied", "true") // Prevent infinite proxy loops

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy SET to %s failed: %w", nodeID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("proxy SET to %s returned %d: %s", nodeID, resp.StatusCode, string(body))
	}
	return nil
}

// ProxyDelete forwards a DELETE request to the owner node.
func (nc *NodeCommunicator) ProxyDelete(ctx context.Context, nodeID string, key string) (bool, error) {
	member, exists := nc.membership.GetMember(nodeID)
	if !exists {
		return false, fmt.Errorf("node %s not found in cluster", nodeID)
	}

	httpPort := ""
	if hp, ok := member.Metadata["http_port"]; ok {
		httpPort = hp
	}
	if httpPort == "" || httpPort == "0" {
		httpPort = fmt.Sprintf("%d", member.Port+1000)
	}

	url := fmt.Sprintf("http://%s:%s/api/cache/%s", member.Address, httpPort, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-HyperCache-Node-ID", nc.localNodeID)
	req.Header.Set("X-HyperCache-Proxied", "true")

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("proxy DELETE to %s failed: %w", nodeID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("proxy DELETE to %s returned %d", nodeID, resp.StatusCode)
	}

	var body struct {
		Existed bool `json:"existed"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return body.Existed, nil
}
