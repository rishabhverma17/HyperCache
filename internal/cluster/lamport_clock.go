package cluster

import (
	"sync/atomic"
)

// LamportClock provides a logical clock for causal ordering of distributed events.
// Every local operation increments the clock. On receiving a remote event, the clock
// is updated to max(local, remote) + 1. This ensures that causally related events
// are always ordered correctly, even when wall clocks diverge.
type LamportClock struct {
	counter uint64
}

// NewLamportClock creates a new Lamport clock starting at 0.
func NewLamportClock() *LamportClock {
	return &LamportClock{}
}

// Tick increments the clock and returns the new value.
// Called before every local write operation.
func (lc *LamportClock) Tick() uint64 {
	return atomic.AddUint64(&lc.counter, 1)
}

// Witness updates the clock to max(local, observed) + 1.
// Called when receiving a remote event with its timestamp.
func (lc *LamportClock) Witness(observed uint64) uint64 {
	for {
		current := atomic.LoadUint64(&lc.counter)
		next := observed
		if current > next {
			next = current
		}
		next++
		if atomic.CompareAndSwapUint64(&lc.counter, current, next) {
			return next
		}
	}
}

// Current returns the current clock value without incrementing.
func (lc *LamportClock) Current() uint64 {
	return atomic.LoadUint64(&lc.counter)
}
