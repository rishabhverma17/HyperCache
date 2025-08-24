//go:build ignore

package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ================================================================================
// CONCURRENCY PATTERNS DEMO - Understanding Locks, Race Conditions, and Safety
// ================================================================================

// UnsafeCounter demonstrates race conditions without proper locking
type UnsafeCounter struct {
	value int
}

func (uc *UnsafeCounter) Increment() {
	// DANGEROUS: Race condition here!
	// Multiple goroutines can read the same value, increment it, and write it back
	uc.value++
}

func (uc *UnsafeCounter) Get() int {
	return uc.value
}

// SafeCounter demonstrates proper mutex usage
type SafeCounter struct {
	value int
	mutex sync.Mutex
}

func (sc *SafeCounter) Increment() {
	sc.mutex.Lock()         // 🔒 Acquire exclusive lock
	defer sc.mutex.Unlock() // 🔓 Always release lock (even if panic occurs)
	sc.value++
}

func (sc *SafeCounter) Get() int {
	sc.mutex.Lock()         // 🔒 Even reads need locks for consistency
	defer sc.mutex.Unlock() // 🔓 Always release
	return sc.value
}

// RWCounter demonstrates read-write locks for better performance
type RWCounter struct {
	value int
	mutex sync.RWMutex // Read-Write mutex allows multiple readers
}

func (rwc *RWCounter) Increment() {
	rwc.mutex.Lock()         // 🔒 Exclusive write lock
	defer rwc.mutex.Unlock() // 🔓 Release write lock
	rwc.value++
}

func (rwc *RWCounter) Get() int {
	rwc.mutex.RLock()         // 📖 Shared read lock (multiple readers allowed)
	defer rwc.mutex.RUnlock() // 📖 Release read lock
	return rwc.value
}

// ConfigManager demonstrates safe configuration updates (like our session policy)
type ConfigManager struct {
	config map[string]interface{}
	mutex  sync.RWMutex
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		config: make(map[string]interface{}),
	}
}

// SafeUpdate demonstrates the double-ok pattern from our session policy
func (cm *ConfigManager) SafeUpdate(updates map[string]interface{}) []string {
	cm.mutex.Lock()         // 🔒 Exclusive lock for writes
	defer cm.mutex.Unlock() // 🔓 Always release

	var applied []string

	// Double-ok pattern for safe type assertions
	if timeout, ok := updates["timeout"]; ok {
		if duration, ok := timeout.(time.Duration); ok {
			cm.config["timeout"] = duration
			applied = append(applied, "timeout")
		}
	}

	if maxSize, ok := updates["max_size"]; ok {
		if size, ok := maxSize.(int); ok {
			cm.config["max_size"] = size
			applied = append(applied, "max_size")
		}
	}

	if name, ok := updates["name"]; ok {
		if str, ok := name.(string); ok {
			cm.config["name"] = str
			applied = append(applied, "name")
		}
	}

	return applied
}

// GetConfig demonstrates safe reading
func (cm *ConfigManager) GetConfig() map[string]interface{} {
	cm.mutex.RLock()         // 📖 Shared lock for reads
	defer cm.mutex.RUnlock() // 📖 Release read lock

	// Create a copy to avoid external modifications
	result := make(map[string]interface{})
	for k, v := range cm.config {
		result[k] = v
	}
	return result
}

// DangerousUpdate shows what NOT to do
func (cm *ConfigManager) DangerousUpdate(key string, value interface{}) {
	// NO LOCKS - DANGEROUS!
	cm.config[key] = value // 💥 Race condition possible
}

// ================================================================================
// DEMONSTRATION FUNCTIONS
// ================================================================================

func demonstrateRaceConditions() {
	fmt.Println("🚨 RACE CONDITION DEMONSTRATION")
	fmt.Println("================================")

	unsafe := &UnsafeCounter{}
	safe := &SafeCounter{}

	numGoroutines := 1000
	var wg sync.WaitGroup

	// Test unsafe counter
	fmt.Printf("Testing unsafe counter with %d goroutines...\n", numGoroutines)
	start := time.Now()
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsafe.Increment()
		}()
	}
	wg.Wait()
	
	fmt.Printf("Unsafe result: %d (should be %d) - took %v\n", 
		unsafe.Get(), numGoroutines, time.Since(start))

	// Test safe counter
	fmt.Printf("Testing safe counter with %d goroutines...\n", numGoroutines)
	start = time.Now()
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safe.Increment()
		}()
	}
	wg.Wait()
	
	fmt.Printf("Safe result: %d (should be %d) - took %v\n", 
		safe.Get(), numGoroutines, time.Since(start))
	fmt.Println()
}

func demonstrateRWMutexPerformance() {
	fmt.Println("📈 READ-WRITE MUTEX PERFORMANCE")
	fmt.Println("===============================")

	rwCounter := &RWCounter{}
	regularCounter := &SafeCounter{}

	// Add some initial value
	for i := 0; i < 100; i++ {
		rwCounter.Increment()
		regularCounter.Increment()
	}

	numReaders := 1000
	numReads := 100

	// Test RWMutex performance
	fmt.Printf("Testing RWMutex with %d readers doing %d reads each...\n", numReaders, numReads)
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numReads; j++ {
				rwCounter.Get() // Multiple readers can access simultaneously
			}
		}()
	}
	wg.Wait()

	rwTime := time.Since(start)
	fmt.Printf("RWMutex time: %v\n", rwTime)

	// Test regular mutex performance
	fmt.Printf("Testing regular mutex with %d readers doing %d reads each...\n", numReaders, numReads)
	start = time.Now()

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numReads; j++ {
				regularCounter.Get() // Readers block each other
			}
		}()
	}
	wg.Wait()

	regularTime := time.Since(start)
	fmt.Printf("Regular mutex time: %v\n", regularTime)
	fmt.Printf("RWMutex is %.2fx faster for read-heavy workloads\n", 
		float64(regularTime)/float64(rwTime))
	fmt.Println()
}

func demonstrateTypeAssertionPatterns() {
	fmt.Println("🔍 TYPE ASSERTION PATTERNS")
	fmt.Println("==========================")

	config := NewConfigManager()

	// Test successful updates
	fmt.Println("✅ Testing valid configuration updates:")
	goodUpdates := map[string]interface{}{
		"timeout":  30 * time.Second,
		"max_size": 1024,
		"name":     "my-cache",
	}

	applied := config.SafeUpdate(goodUpdates)
	fmt.Printf("Applied settings: %v\n", applied)
	fmt.Printf("Current config: %+v\n", config.GetConfig())

	// Test invalid updates (should be ignored gracefully)
	fmt.Println("\n❌ Testing invalid configuration updates:")
	badUpdates := map[string]interface{}{
		"timeout":     "30 seconds",  // Wrong type (string instead of duration)
		"max_size":    "1024",        // Wrong type (string instead of int)
		"nonexistent": 42,            // Unknown key
		"name":        123,           // Wrong type (int instead of string)
	}

	applied = config.SafeUpdate(badUpdates)
	fmt.Printf("Applied settings: %v (should be empty)\n", applied)
	fmt.Printf("Config unchanged: %+v\n", config.GetConfig())

	// Demonstrate what happens without type checking
	fmt.Println("\n💥 What happens without proper type checking:")
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("PANIC RECOVERED: %v\n", r)
		}
	}()

	// This would panic without the double-ok pattern
	var rawConfig map[string]interface{}
	rawConfig = map[string]interface{}{
		"timeout": "not a duration",
	}

	if timeout, exists := rawConfig["timeout"]; exists {
		// This would panic!
		// duration := timeout.(time.Duration)
		
		// Safe way:
		if duration, ok := timeout.(time.Duration); ok {
			fmt.Printf("Duration: %v\n", duration)
		} else {
			fmt.Printf("Type assertion failed - value is not a time.Duration\n")
		}
	}
	fmt.Println()
}

func demonstrateConcurrentConfigUpdates() {
	fmt.Println("🔄 CONCURRENT CONFIGURATION UPDATES")
	fmt.Println("===================================")

	config := NewConfigManager()
	numUpdaters := 10
	numReaders := 50

	var wg sync.WaitGroup

	// Start multiple updaters
	fmt.Printf("Starting %d concurrent updaters and %d readers...\n", numUpdaters, numReaders)

	// Updaters
	for i := 0; i < numUpdaters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				updates := map[string]interface{}{
					"timeout":  time.Duration(rand.Intn(60)) * time.Second,
					"max_size": rand.Intn(2048),
					"name":     fmt.Sprintf("updater-%d-%d", id, j),
				}
				config.SafeUpdate(updates)
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
			}
		}(i)
	}

	// Readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				cfg := config.GetConfig()
				_ = cfg // Just read the config
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("Final configuration: %+v\n", config.GetConfig())
	fmt.Println("✅ No race conditions or panics occurred!")
	fmt.Println()
}

func demonstrateDeferPattern() {
	fmt.Println("🎯 DEFER PATTERN DEMONSTRATION")
	fmt.Println("==============================")

	counter := &SafeCounter{}

	// Function that might panic
	riskyIncrement := func(shouldPanic bool) (recovered bool) {
		// This shows why defer is crucial
		counter.mutex.Lock()
		defer func() {
			counter.mutex.Unlock()
			if r := recover(); r != nil {
				fmt.Printf("PANIC in riskyIncrement: %v\n", r)
				recovered = true
			}
		}()

		counter.value++
		
		if shouldPanic {
			panic("Something went wrong!")
		}
		
		return false
	}

	fmt.Println("Normal increment:")
	riskyIncrement(false)
	fmt.Printf("Counter: %d\n", counter.Get())

	fmt.Println("\nIncrement that panics:")
	recovered := riskyIncrement(true)
	fmt.Printf("Panic was recovered: %v\n", recovered)
	fmt.Printf("Counter: %d (lock was still released!)\n", counter.Get())

	fmt.Println("\nCounter is still usable after panic:")
	riskyIncrement(false)
	fmt.Printf("Counter: %d\n", counter.Get())
	fmt.Println()
}

// ================================================================================
// MAIN DEMONSTRATION
// ================================================================================

func main() {
	fmt.Println("🎪 GO CONCURRENCY PATTERNS DEMONSTRATION")
	fmt.Println("==========================================")
	fmt.Println("This demo shows the patterns used in our session eviction policy\n")

	demonstrateRaceConditions()
	demonstrateRWMutexPerformance()
	demonstrateTypeAssertionPatterns()
	demonstrateDeferPattern()
	demonstrateConcurrentConfigUpdates()

	fmt.Println("🎓 KEY TAKEAWAYS:")
	fmt.Println("=================")
	fmt.Println("1. Always use mutexes to protect shared data")
	fmt.Println("2. Use RWMutex for read-heavy workloads")
	fmt.Println("3. Use defer for guaranteed cleanup")
	fmt.Println("4. Use double-ok pattern for safe type assertions")
	fmt.Println("5. Design for concurrency from the beginning")
	fmt.Println("\n🚀 These patterns make our cache system production-ready!")
}
