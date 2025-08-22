// Real-World Scenario: Session Cache Overflow
// What happens when active sessions compete for space?

package main

import (
	"fmt"
	"strings"
	"time"
)

// Current State: Sessions Store
type SessionsStore struct {
	name        string
	maxMemory   int64  // 10MB = 10,485,760 bytes
	currentSize int64  // 9MB = 9,437,184 bytes (90% full)
	sessions    map[string]*SessionEntry
}

type SessionEntry struct {
	UserID      string
	SessionData []byte    // 1MB each
	LastAccess  time.Time
	CreatedAt   time.Time
}

// Scenario Setup
func ScenarioSetup() *SessionsStore {
	store := &SessionsStore{
		name:        "sessions", 
		maxMemory:   10 * 1024 * 1024, // 10MB
		currentSize: 0,
		sessions:    make(map[string]*SessionEntry),
	}
	
	// Add 9 active user sessions (1MB each)
	activeUsers := []string{
		"user_001_alice",   // CEO using system
		"user_002_bob",     // Manager in meeting
		"user_003_carol",   // Developer coding
		"user_004_dave",    // Customer support agent
		"user_005_eve",     // Sales rep on call
		"user_006_frank",   // Accountant doing monthly reports
		"user_007_grace",   // HR reviewing applications  
		"user_008_henry",   // Marketing analyzing data
		"user_009_ivy",     // Operations monitoring
	}
	
	now := time.Now()
	
	for i, userID := range activeUsers {
		sessionData := make([]byte, 1024*1024) // 1MB of session data
		
		session := &SessionEntry{
			UserID:      userID,
			SessionData: sessionData,
			LastAccess:  now.Add(-time.Duration(i) * time.Minute), // Different access times
			CreatedAt:   now.Add(-time.Duration(i+10) * time.Minute),
		}
		
		store.sessions[userID] = session
		store.currentSize += int64(len(sessionData)) + 100 // +100 for metadata
		
		fmt.Printf("✅ Added session for %s (Last access: %v)\n", 
			userID, session.LastAccess.Format("15:04:05"))
	}
	
	fmt.Printf("\n📊 Current state: %d sessions, %.1f MB used (%.1f%% full)\n\n",
		len(store.sessions), 
		float64(store.currentSize)/(1024*1024),
		float64(store.currentSize)/float64(store.maxMemory)*100)
	
	return store
}

// The Critical Moment: 2 New Users Try to Login
func HandleNewLoginRequests(store *SessionsStore) {
	newUsers := []string{
		"user_010_jack",  // New customer trying to place order
		"user_011_kate",  // New employee starting work
	}
	
	fmt.Println("🚨 CRITICAL MOMENT: 2 new users trying to login...")
	fmt.Println("Each needs 1MB session space, but only 1MB available!\n")
	
	for _, userID := range newUsers {
		fmt.Printf("--- Handling login for %s ---\n", userID)
		
		sessionData := make([]byte, 1024*1024) // 1MB needed
		spaceNeeded := int64(len(sessionData)) + 100
		
		availableSpace := store.maxMemory - store.currentSize
		
		fmt.Printf("Space needed: %.1f MB, Available: %.1f MB\n",
			float64(spaceNeeded)/(1024*1024), 
			float64(availableSpace)/(1024*1024))
		
		if spaceNeeded <= availableSpace {
			// First user: Can fit in remaining space
			fmt.Println("✅ Sufficient space - adding session")
			
			session := &SessionEntry{
				UserID:      userID,
				SessionData: sessionData,
				LastAccess:  time.Now(),
				CreatedAt:   time.Now(),
			}
			
			store.sessions[userID] = session
			store.currentSize += spaceNeeded
			
		} else {
			// Second user: Need to evict someone!
			fmt.Println("🚨 INSUFFICIENT SPACE - Need to evict existing session!")
			
			// Find victim using LRU policy
			victim := findLRUVictim(store)
			if victim == nil {
				fmt.Println("❌ LOGIN FAILED - No sessions can be evicted")
				continue
			}
			
			fmt.Printf("🎯 EVICTING: %s (last access: %v)\n", 
				victim.UserID, victim.LastAccess.Format("15:04:05"))
			
			// Remove victim session
			victimSize := int64(len(victim.SessionData)) + 100
			delete(store.sessions, victim.UserID)
			store.currentSize -= victimSize
			
			// Add new session
			session := &SessionEntry{
				UserID:      userID,
				SessionData: sessionData,
				LastAccess:  time.Now(),
				CreatedAt:   time.Now(),
			}
			
			store.sessions[userID] = session
			store.currentSize += spaceNeeded
			
			fmt.Println("✅ New session added after eviction")
		}
		
		fmt.Printf("📊 Store now: %d sessions, %.1f MB used\n\n",
			len(store.sessions), float64(store.currentSize)/(1024*1024))
	}
}

// Find least recently used session for eviction
func findLRUVictim(store *SessionsStore) *SessionEntry {
	var oldestSession *SessionEntry
	var oldestTime time.Time = time.Now()
	
	for _, session := range store.sessions {
		if session.LastAccess.Before(oldestTime) {
			oldestTime = session.LastAccess
			oldestSession = session
		}
	}
	
	return oldestSession
}

// What happens to the evicted user?
func UserExperienceAnalysis() {
	fmt.Println(`
🎭 USER EXPERIENCE ANALYSIS:

👤 EVICTED USER (e.g., Alice the CEO):
1. 💥 Next request fails with "Session expired"
2. 🔄 Forced to login again  
3. 📝 Loses any unsaved work/state
4. 😠 Frustrated - was actively using the system
5. 📞 Calls IT support complaining

👤 NEW USER (Jack the customer):
1. ✅ Successfully logs in
2. 😊 Can place their order
3. 💰 Business transaction succeeds

📊 BUSINESS IMPACT:
✅ Pros:
  - New customers can access system
  - Cache stays within memory limits
  - System remains stable

❌ Cons:
  - Existing users get kicked out unexpectedly
  - Loss of user state/context
  - Support tickets increase
  - User satisfaction decreases
  - Potential revenue loss if CEO was in important meeting
`)
}

// Better Solutions for This Scenario
func BetterSolutionsAnalysis() {
	fmt.Println(`
🛠️  BETTER SOLUTIONS:

1. 📈 CAPACITY PLANNING
   - Monitor peak concurrent users
   - Size cache for 120% of peak usage
   - Current: 10MB → Recommended: 15MB

2. ⏰ TTL-BASED EVICTION (Better for Sessions)
   - Sessions expire automatically after 30 minutes
   - Recently created sessions are less likely to be evicted
   - More predictable for users

3. 🎯 HYBRID EVICTION POLICY
   - Check TTL first (expired sessions)
   - Then check idle time (inactive > 10 minutes)  
   - LRU as last resort

4. 🚀 GRACEFUL DEGRADATION
   - Temporarily allow cache to exceed limit by small %
   - Background cleanup process
   - Alert ops team to add capacity

5. 💾 OVERFLOW TO SECONDARY STORAGE
   - Move idle sessions to Redis/Database
   - Keep hot sessions in memory
   - Transparent to users

6. 🔄 DYNAMIC ALLOCATION
   - Allow sessions store to borrow from other stores
   - analytics store might be less critical
`)
}

// Improved eviction strategy for sessions
func ImprovedSessionEviction(store *SessionsStore) *SessionEntry {
	now := time.Now()
	
	// Strategy 1: Find expired sessions first (TTL-based)
	for userID, session := range store.sessions {
		if now.Sub(session.CreatedAt) > 30*time.Minute {
			fmt.Printf("🕒 Found expired session: %s (age: %v)\n", 
				userID, now.Sub(session.CreatedAt))
			return session
		}
	}
	
	// Strategy 2: Find idle sessions (inactive > 10 minutes)
	for userID, session := range store.sessions {
		if now.Sub(session.LastAccess) > 10*time.Minute {
			fmt.Printf("😴 Found idle session: %s (idle: %v)\n", 
				userID, now.Sub(session.LastAccess))
			return session
		}
	}
	
	// Strategy 3: LRU as last resort (all sessions are active!)
	fmt.Println("😰 All sessions are active! Using LRU as last resort...")
	return findLRUVictim(store)
}

// Complete scenario execution
func RunCompleteScenario() {
	fmt.Println("🎬 REAL-WORLD SCENARIO: Session Cache Overflow")
	fmt.Println(strings.Repeat("=", 60) + "\n")
	
	// Setup initial state
	store := ScenarioSetup()
	
	// Show current active sessions
	fmt.Println("👥 All users are actively working:")
	for userID, session := range store.sessions {
		idleTime := time.Since(session.LastAccess)
		fmt.Printf("  %s - idle for %v\n", userID, idleTime.Truncate(time.Second))
	}
	
	// Handle new login requests
	HandleNewLoginRequests(store)
	
	// Analyze what happened
	UserExperienceAnalysis()
	BetterSolutionsAnalysis()
}

func main() {
	fmt.Println("=== Session Overflow Scenario Demo ===")
	
	// Run the complete scenario
	RunCompleteScenario()
	
	fmt.Println("=== Demo Complete ===")
}
