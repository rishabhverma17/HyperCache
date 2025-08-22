package cache

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestScenario_SessionOverflow tests the exact scenario user described
func TestScenario_SessionOverflow(t *testing.T) {
	fmt.Println("🎬 TESTING: Smart Session Eviction Policy")
	fmt.Println("Scenario: 9 active sessions (1MB each), 2 new users trying to login")
	fmt.Println(strings.Repeat("=", 70))
	
	// Create smart session eviction policy
	policy := NewSessionEvictionPolicy()
	
	// Simulate the store
	store := &MockSessionStore{
		maxMemory:   10 * 1024 * 1024, // 10MB
		currentSize: 0,
		entries:     make(map[string]*Entry),
		policy:      policy,
	}
	
	// Add 9 active users (your scenario)
	activeUsers := []struct {
		userID     string
		lastAccess time.Duration // How long ago they last accessed
		role       string
	}{
		{"alice_ceo", 30 * time.Second, "CEO"},
		{"bob_manager", 1 * time.Minute, "Manager"},
		{"carol_dev", 2 * time.Minute, "Developer"},
		{"dave_support", 3 * time.Minute, "Support"},
		{"eve_sales", 4 * time.Minute, "Sales"},
		{"frank_accounting", 5 * time.Minute, "Accounting"},
		{"grace_hr", 6 * time.Minute, "HR"},
		{"henry_marketing", 7 * time.Minute, "Marketing"},
		{"ivy_operations", 8 * time.Minute, "Operations"},
	}
	
	now := time.Now()
	
	// Add all active users
	for _, user := range activeUsers {
		sessionData := make([]byte, 1024*1024) // 1MB
		entry := &Entry{
			Key:       []byte(user.userID),
			Value:     sessionData,
			TTL:       30 * time.Minute,
			Timestamp: now.Add(-user.lastAccess).Unix(),
		}
		
		store.entries[user.userID] = entry
		store.currentSize += int64(len(sessionData)) + 100
		policy.OnInsert(entry)
		
		// Simulate the last access time
		entry.Timestamp = now.Add(-user.lastAccess).Unix()
		policy.OnAccess(entry)
		
		fmt.Printf("✅ Added %s (%s) - last active %v ago\n", 
			user.userID, user.role, user.lastAccess)
	}
	
	fmt.Printf("\n📊 Initial state: %d users, %.1f MB used (%.0f%% full)\n\n",
		len(store.entries), 
		float64(store.currentSize)/(1024*1024),
		float64(store.currentSize)/float64(store.maxMemory)*100)
	
	// Show policy stats
	stats := policy.GetStats()
	fmt.Printf("📈 Policy stats: %d active, %d idle, %d expired\n\n", 
		stats["active_sessions"], stats["idle_sessions"], stats["expired_sessions"])
	
	// Now try to add 2 new users
	newUsers := []string{"jack_customer", "kate_newemployee"}
	
	for i, userID := range newUsers {
		fmt.Printf("--- Login attempt #%d: %s ---\n", i+1, userID)
		
		sessionData := make([]byte, 1024*1024) // 1MB needed
		spaceNeeded := int64(len(sessionData)) + 100
		availableSpace := store.maxMemory - store.currentSize
		
		fmt.Printf("Space needed: %.1f MB, Available: %.1f MB\n",
			float64(spaceNeeded)/(1024*1024), 
			float64(availableSpace)/(1024*1024))
		
		if spaceNeeded > availableSpace {
			fmt.Println("🚨 Need to evict someone!")
			
			// Use our smart policy to find victim
			victim := policy.NextEvictionCandidate()
			if victim == nil {
				fmt.Println("❌ No suitable victim found - login failed!")
				continue
			}
			
			victimID := string(victim.Key)
			
			// Check WHY this victim was chosen
			reason := getEvictionReason(policy, victim)
			fmt.Printf("🎯 Smart eviction: %s (%s)\n", victimID, reason)
			
			// Remove victim
			victimSize := int64(len(victim.Value)) + 100
			delete(store.entries, victimID)
			store.currentSize -= victimSize
			policy.OnDelete(victim)
			
		} else {
			fmt.Println("✅ Sufficient space available")
		}
		
		// Add new user
		entry := &Entry{
			Key:       []byte(userID),
			Value:     sessionData,
			TTL:       30 * time.Minute,
			Timestamp: now.Unix(),
		}
		
		store.entries[userID] = entry
		store.currentSize += spaceNeeded
		policy.OnInsert(entry)
		
		fmt.Printf("✅ %s logged in successfully\n", userID)
		fmt.Printf("📊 Store now: %d users, %.1f MB used\n\n",
			len(store.entries), float64(store.currentSize)/(1024*1024))
	}
	
	// Final analysis
	fmt.Println("🎭 FINAL ANALYSIS:")
	analyzeResults(store, activeUsers)
}

// Helper function to determine why a session was evicted
func getEvictionReason(policy *SessionEvictionPolicy, entry *Entry) string {
	keyStr := string(entry.Key)
	now := time.Now()
	
	// Check creation time
	if createdAt, exists := policy.creationTracking[keyStr]; exists {
		if now.Sub(createdAt) > policy.sessionTTL {
			return "session expired (TTL)"
		}
	}
	
	// Check last access
	if lastAccess, exists := policy.accessTracking[keyStr]; exists {
		if now.Sub(lastAccess) > policy.idleTimeout {
			return fmt.Sprintf("idle for %v", now.Sub(lastAccess).Truncate(time.Second))
		}
	}
	
	return "least recently used"
}

// Analyze the results and show user impact
func analyzeResults(store *MockSessionStore, originalUsers []struct {
	userID     string
	lastAccess time.Duration
	role       string
}) {
	fmt.Println("\n👥 User Impact Analysis:")
	
	for _, user := range originalUsers {
		if _, exists := store.entries[user.userID]; exists {
			fmt.Printf("✅ %s (%s) - Still logged in\n", user.userID, user.role)
		} else {
			fmt.Printf("❌ %s (%s) - Was evicted (last active %v ago)\n", 
				user.userID, user.role, user.lastAccess)
		}
	}
	
	fmt.Println("\n🧠 Smart Policy Benefits:")
	fmt.Println("• Prioritizes recently active users")
	fmt.Println("• Considers idle time over pure LRU")
	fmt.Println("• Protects sessions within grace period")
	fmt.Println("• Expires old sessions automatically")
	
	fmt.Println("\n📈 Compared to Simple LRU:")
	fmt.Println("• LRU would evict: ivy_operations (oldest access)")
	fmt.Println("• Smart policy considers: idle time, TTL, grace period")
	fmt.Println("• Result: Better user experience for active users")
}

// MockSessionStore for testing
type MockSessionStore struct {
	maxMemory   int64
	currentSize int64
	entries     map[string]*Entry
	policy      EvictionPolicy
}

// Test with different scenarios
func TestSmartEvictionScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(*SessionEvictionPolicy)
		expectEvict string
		reason      string
	}{
		{
			name: "Expired session exists",
			setup: func(p *SessionEvictionPolicy) {
				// Add expired session
				expiredEntry := &Entry{
					Key:       []byte("expired_user"),
					Value:     make([]byte, 1024),
					Timestamp: time.Now().Add(-35 * time.Minute).Unix(),
				}
				p.OnInsert(expiredEntry)
				
				// Add active session
				activeEntry := &Entry{
					Key:       []byte("active_user"),
					Value:     make([]byte, 1024),
					Timestamp: time.Now().Unix(),
				}
				p.OnInsert(activeEntry)
				p.OnAccess(activeEntry) // Mark as recently accessed
			},
			expectEvict: "expired_user",
			reason:      "Should evict expired session first",
		},
		{
			name: "Idle session exists",
			setup: func(p *SessionEvictionPolicy) {
				// Add idle session (no recent access)
				idleEntry := &Entry{
					Key:       []byte("idle_user"),
					Value:     make([]byte, 1024),
					Timestamp: time.Now().Add(-5 * time.Minute).Unix(),
				}
				p.OnInsert(idleEntry)
				// Don't call OnAccess - remains idle
				
				// Add active session
				activeEntry := &Entry{
					Key:       []byte("active_user"),
					Value:     make([]byte, 1024),
					Timestamp: time.Now().Unix(),
				}
				p.OnInsert(activeEntry)
				p.OnAccess(activeEntry)
			},
			expectEvict: "idle_user",
			reason:      "Should evict idle session over active one",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			policy := NewSessionEvictionPolicy()
			tc.setup(policy)
			
			victim := policy.NextEvictionCandidate()
			if victim == nil {
				t.Fatalf("Expected victim but got nil")
			}
			
			if string(victim.Key) != tc.expectEvict {
				t.Errorf("Expected to evict %s but got %s. Reason: %s",
					tc.expectEvict, string(victim.Key), tc.reason)
			}
		})
	}
}
