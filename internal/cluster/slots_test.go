package cluster

import (
	"fmt"
	"testing"
)

func TestHashSlotCalculation(t *testing.T) {
	testCases := []struct {
		key      string
		expected uint16 // We'll calculate these once and verify they're consistent
	}{
		{"user:123", 0},     // Will be filled after first run
		{"user:456", 0},     
		{"session:abc", 0},  
		{"{user:123}:profile", 0},
		{"{user:123}:settings", 0},
	}

	// First, calculate slots for all keys
	for i, tc := range testCases {
		slot := GetHashSlot(tc.key)
		testCases[i].expected = slot
		t.Logf("Key: %-20s -> Slot: %d", tc.key, slot)
	}

	// Verify hash tag behavior - keys with same hash tag should have same slot
	slot1 := GetHashSlot("{user:123}:profile")
	slot2 := GetHashSlot("{user:123}:settings")
	if slot1 != slot2 {
		t.Errorf("Hash tag not working: '{user:123}:profile' -> %d, '{user:123}:settings' -> %d", slot1, slot2)
	}
	
	// Verify hash tag with different content
	slot3 := GetHashSlot("{user:456}:profile")
	if slot1 == slot3 {
		t.Logf("Note: Different hash tags produced same slot (collision): %d", slot1)
	}

	// Verify consistency - same key should always produce same slot
	for _, tc := range testCases {
		for i := 0; i < 10; i++ {
			slot := GetHashSlot(tc.key)
			if slot != tc.expected {
				t.Errorf("Inconsistent slot calculation for key %s: expected %d, got %d", tc.key, tc.expected, slot)
			}
		}
	}
}

func TestSlotBasedRouting(t *testing.T) {
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)

	// Add nodes
	err := ring.AddNode("node1", "192.168.1.1", 6379)
	if err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}
	
	err = ring.AddNode("node2", "192.168.1.2", 6379)
	if err != nil {
		t.Fatalf("Failed to add node2: %v", err)
	}
	
	err = ring.AddNode("node3", "192.168.1.3", 6379)
	if err != nil {
		t.Fatalf("Failed to add node3: %v", err)
	}

	// Test slot distribution
	totalSlots := 0
	nodes := ring.GetNodes()
	for nodeID := range nodes {
		slots := ring.GetSlotsByNode(nodeID)
		totalSlots += len(slots)
		t.Logf("Node %s: %d slots", nodeID, len(slots))
		
		// Each node should have roughly 16384/3 slots
		expectedSlots := RedisHashSlots / 3
		tolerance := expectedSlots / 4 // 25% tolerance
		if len(slots) < expectedSlots-tolerance || len(slots) > expectedSlots+tolerance {
			t.Errorf("Node %s has %d slots, expected around %d (±%d)", nodeID, len(slots), expectedSlots, tolerance)
		}
	}

	// Should distribute all slots
	if totalSlots != RedisHashSlots {
		t.Errorf("Total slots distributed: %d, expected: %d", totalSlots, RedisHashSlots)
	}

	// Test slot-to-node mapping consistency
	testKeys := []string{"user:123", "session:abc", "product:789"}
	for _, key := range testKeys {
		slot := GetHashSlot(key)
		nodeBySlot := ring.GetNodeBySlot(slot)
		nodeByKey := ring.GetNodeByKey(key)
		
		if nodeBySlot != nodeByKey {
			t.Errorf("Inconsistent routing for key %s: slot %d -> node %s, but key -> node %s", 
				key, slot, nodeBySlot, nodeByKey)
		}
		
		if nodeBySlot == "" {
			t.Errorf("No node assigned for slot %d (key: %s)", slot, key)
		}
		
		t.Logf("Key: %s -> Slot: %d -> Node: %s", key, slot, nodeBySlot)
	}
}

func ExampleGetHashSlot() {
	// Calculate hash slot for a key
	key := "user:123"
	slot := GetHashSlot(key)
	fmt.Printf("Key '%s' maps to slot %d\n", key, slot)

	// Keys with same hash tag will have same slot
	key1 := "{user:123}:profile"
	key2 := "{user:123}:settings"
	slot1 := GetHashSlot(key1)
	slot2 := GetHashSlot(key2)
	fmt.Printf("Keys with same tag: %s->%d, %s->%d\n", key1, slot1, key2, slot2)

	// Create hash ring
	config := DefaultHashRingConfig()
	ring := NewHashRing(config)
	ring.AddNode("node1", "127.0.0.1", 6379)
	ring.AddNode("node2", "127.0.0.1", 6380)

	// Route key to node
	nodeID := ring.GetNodeByKey(key)
	fmt.Printf("Key '%s' should be handled by node: %s\n", key, nodeID)
}