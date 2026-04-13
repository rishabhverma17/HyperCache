package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"hypercache/internal/logging"
)

// ReadRepairResult holds the result of a read-repair attempt.
type ReadRepairResult struct {
	Found bool
	Value interface{}
	Node  string // Which node had the key
}

// ReadRepairer handles fetching keys from peer nodes when a local cache miss
// occurs during the gossip replication propagation window. In HyperCache's
// full-replication model, every node should eventually have every key.
// Read-repair bridges the gap between "gossip hasn't arrived yet" and "key
// truly doesn't exist".
type ReadRepairer struct {
	coordinator CoordinatorService
	client      *http.Client
}

// NewReadRepairer creates a new read repairer.
func NewReadRepairer(coordinator CoordinatorService) *ReadRepairer {
	return &ReadRepairer{
		coordinator: coordinator,
		client: &http.Client{
			Timeout: 2 * time.Second, // Tight timeout — this is on the GET hot path
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

// TryPeers attempts to fetch a key from replica nodes identified by the hash ring.
// Returns on the first hit. This is called only when local GET misses,
// covering the replication propagation window.
func (rr *ReadRepairer) TryPeers(ctx context.Context, key string) *ReadRepairResult {
	localNodeID := rr.coordinator.GetLocalNodeID()
	routing := rr.coordinator.GetRouting()

	// Use hash-ring to find the owner + replicas for this key
	var candidates []string
	if routing != nil {
		candidates = routing.GetReplicas(key, 3) // check owner + replicas
	}

	// Fallback: if no routing or no candidates, try all alive nodes (backward compat)
	if len(candidates) == 0 {
		membership := rr.coordinator.GetMembership()
		if membership == nil {
			return nil
		}
		members := membership.GetAliveNodes()
		if len(members) <= 1 {
			return nil
		}
		for _, m := range members {
			if m.NodeID != localNodeID {
				candidates = append(candidates, m.NodeID)
			}
		}
	}

	for _, nodeID := range candidates {
		if nodeID == localNodeID {
			continue
		}

		addr := rr.coordinator.GetNodeHTTPAddress(nodeID)
		if addr == "" {
			continue
		}

		result, err := rr.fetchFromPeer(ctx, addr, key)
		if err != nil {
			logging.Debug(ctx, logging.ComponentCluster, "read_repair", "Peer fetch failed", map[string]interface{}{
				"peer": nodeID, "key": key, "error": err.Error(),
			})
			continue
		}

		if result.Found {
			result.Node = nodeID
			logging.Info(ctx, logging.ComponentCluster, "read_repair", "Read-repair success: key found on peer", map[string]interface{}{
				"peer": nodeID, "key": key,
			})
			return result
		}
	}

	return nil // No peer had it — genuine miss
}

func (rr *ReadRepairer) fetchFromPeer(ctx context.Context, addr string, key string) (*ReadRepairResult, error) {
	url := fmt.Sprintf("http://%s/internal/get/%s", addr, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if cid := logging.GetCorrelationID(ctx); cid != "" {
		req.Header.Set("X-Correlation-ID", cid)
	}

	resp, err := rr.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ReadRepairResult{Found: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer returned %d", resp.StatusCode)
	}

	var body struct {
		Value interface{} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode peer response: %w", err)
	}

	return &ReadRepairResult{Found: true, Value: body.Value}, nil
}
