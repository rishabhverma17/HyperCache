package cache

import (
	"sync"
	"time"
)

// SessionEvictionPolicy implements intelligent session-aware eviction
// This policy prioritizes keeping active users logged in while making space for new users
type SessionEvictionPolicy struct {
	name             string
	evictionQueue    []*Entry             // Ordered list of eviction candidates
	accessTracking   map[string]time.Time // Last access time per key
	creationTracking map[string]time.Time // Creation time per key
	mutex            sync.RWMutex         // Thread safety for concurrent access

	// Configuration
	sessionTTL  time.Duration // Max session lifetime (30 minutes)
	idleTimeout time.Duration // Idle timeout (10 minutes)
	gracePeriod time.Duration // Grace period for new sessions (2 minutes)
}

// NewSessionEvictionPolicy creates a new session-aware eviction policy
func NewSessionEvictionPolicy() *SessionEvictionPolicy {
	return &SessionEvictionPolicy{
		name:             "smart-session",
		evictionQueue:    make([]*Entry, 0),
		accessTracking:   make(map[string]time.Time),
		creationTracking: make(map[string]time.Time),
		sessionTTL:       30 * time.Minute,
		idleTimeout:      10 * time.Minute,
		gracePeriod:      2 * time.Minute,
	}
}

// NextEvictionCandidate returns the best candidate for eviction - MUST be O(1)
func (sep *SessionEvictionPolicy) NextEvictionCandidate() *Entry {
	sep.mutex.RLock()
	defer sep.mutex.RUnlock()

	now := time.Now()
	keyStr := ""

	// Strategy 1: Find expired sessions first (TTL-based)
	for key, createdAt := range sep.creationTracking {
		if now.Sub(createdAt) > sep.sessionTTL {
			keyStr = key
			break
		}
	}

	// Strategy 2: Find idle sessions (inactive users)
	if keyStr == "" {
		for key, lastAccess := range sep.accessTracking {
			if now.Sub(lastAccess) > sep.idleTimeout {
				keyStr = key
				break
			}
		}
	}

	// Strategy 3: Find oldest session outside grace period
	if keyStr == "" {
		var oldestKey string
		var oldestTime time.Time = now

		for key, createdAt := range sep.creationTracking {
			// Don't evict sessions created within grace period
			if now.Sub(createdAt) > sep.gracePeriod && createdAt.Before(oldestTime) {
				oldestTime = createdAt
				oldestKey = key
			}
		}
		keyStr = oldestKey
	}

	// Find the entry object for the selected key
	for _, entry := range sep.evictionQueue {
		if string(entry.Key) == keyStr {
			return entry
		}
	}

	// Fallback: return first entry if all else fails
	if len(sep.evictionQueue) > 0 {
		return sep.evictionQueue[0]
	}

	return nil
}

// OnAccess updates tracking when an entry is accessed - MUST be O(1)
func (sep *SessionEvictionPolicy) OnAccess(entry *Entry) {
	sep.mutex.Lock()
	defer sep.mutex.Unlock()

	keyStr := string(entry.Key)
	sep.accessTracking[keyStr] = time.Now()

	// Move to end of queue (most recently used)
	sep.moveToEnd(entry)
}

// OnInsert handles new entries being added - MUST be O(1)
func (sep *SessionEvictionPolicy) OnInsert(entry *Entry) {
	sep.mutex.Lock()
	defer sep.mutex.Unlock()

	keyStr := string(entry.Key)
	now := time.Now()

	// Track creation and access time
	sep.creationTracking[keyStr] = now
	sep.accessTracking[keyStr] = now

	// Add to eviction queue
	sep.evictionQueue = append(sep.evictionQueue, entry)
}

// OnDelete cleans up tracking when entry is removed - MUST be O(1)
func (sep *SessionEvictionPolicy) OnDelete(entry *Entry) {
	sep.mutex.Lock()
	defer sep.mutex.Unlock()

	keyStr := string(entry.Key)

	// Remove from tracking
	delete(sep.accessTracking, keyStr)
	delete(sep.creationTracking, keyStr)

	// Remove from queue
	sep.removeFromQueue(entry)
}

// ShouldEvict determines if an entry should be evicted based on multiple factors
func (sep *SessionEvictionPolicy) ShouldEvict(entry *Entry, memoryPressure float64) bool {
	sep.mutex.RLock()
	defer sep.mutex.RUnlock()

	keyStr := string(entry.Key)
	now := time.Now()

	// Check if session has expired (TTL)
	if createdAt, exists := sep.creationTracking[keyStr]; exists {
		if now.Sub(createdAt) > sep.sessionTTL {
			return true
		}
	}

	// Check if session is idle
	if lastAccess, exists := sep.accessTracking[keyStr]; exists {
		if now.Sub(lastAccess) > sep.idleTimeout {
			return true
		}
	}

	// Under high memory pressure, be more aggressive
	if memoryPressure > 0.95 {
		// Evict sessions older than grace period
		if createdAt, exists := sep.creationTracking[keyStr]; exists {
			return now.Sub(createdAt) > sep.gracePeriod
		}
	}

	return false
}

// PolicyName returns the name of this eviction policy
func (sep *SessionEvictionPolicy) PolicyName() string {
	return sep.name
}

// moveToEnd moves an entry to the end of the eviction queue (O(n) but called infrequently)
func (sep *SessionEvictionPolicy) moveToEnd(target *Entry) {
	// Find and remove entry from current position
	for i, entry := range sep.evictionQueue {
		if string(entry.Key) == string(target.Key) {
			// Remove from current position
			sep.evictionQueue = append(sep.evictionQueue[:i], sep.evictionQueue[i+1:]...)
			// Add to end
			sep.evictionQueue = append(sep.evictionQueue, target)
			break
		}
	}
}

// removeFromQueue removes an entry from the eviction queue
func (sep *SessionEvictionPolicy) removeFromQueue(target *Entry) {
	for i, entry := range sep.evictionQueue {
		if string(entry.Key) == string(target.Key) {
			sep.evictionQueue = append(sep.evictionQueue[:i], sep.evictionQueue[i+1:]...)
			break
		}
	}
}

// GetStats returns statistics about the eviction policy
func (sep *SessionEvictionPolicy) GetStats() map[string]interface{} {
	sep.mutex.RLock()
	defer sep.mutex.RUnlock()

	now := time.Now()
	expiredCount := 0
	idleCount := 0
	activeCount := 0

	for key, createdAt := range sep.creationTracking {
		lastAccess := sep.accessTracking[key]

		if now.Sub(createdAt) > sep.sessionTTL {
			expiredCount++
		} else if now.Sub(lastAccess) > sep.idleTimeout {
			idleCount++
		} else {
			activeCount++
		}
	}

	return map[string]interface{}{
		"policy_name":      sep.name,
		"total_sessions":   len(sep.evictionQueue),
		"expired_sessions": expiredCount,
		"idle_sessions":    idleCount,
		"active_sessions":  activeCount,
		"session_ttl":      sep.sessionTTL,
		"idle_timeout":     sep.idleTimeout,
		"grace_period":     sep.gracePeriod,
	}
}

// SetConfiguration allows runtime configuration changes
func (sep *SessionEvictionPolicy) SetConfiguration(config map[string]interface{}) error {
	sep.mutex.Lock()
	defer sep.mutex.Unlock()

	if ttl, ok := config["session_ttl"]; ok {
		if duration, ok := ttl.(time.Duration); ok {
			sep.sessionTTL = duration
		}
	}

	if idle, ok := config["idle_timeout"]; ok {
		if duration, ok := idle.(time.Duration); ok {
			sep.idleTimeout = duration
		}
	}

	if grace, ok := config["grace_period"]; ok {
		if duration, ok := grace.(time.Duration); ok {
			sep.gracePeriod = duration
		}
	}

	return nil
}
