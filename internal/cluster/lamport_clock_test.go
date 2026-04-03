package cluster

import (
	"sync"
	"testing"
)

func TestLamportClockTick(t *testing.T) {
	clock := NewLamportClock()

	if clock.Current() != 0 {
		t.Fatalf("expected initial value 0, got %d", clock.Current())
	}

	v1 := clock.Tick()
	if v1 != 1 {
		t.Fatalf("expected first tick = 1, got %d", v1)
	}

	v2 := clock.Tick()
	if v2 != 2 {
		t.Fatalf("expected second tick = 2, got %d", v2)
	}
}

func TestLamportClockWitness(t *testing.T) {
	clock := NewLamportClock()

	// Advance local to 5
	for i := 0; i < 5; i++ {
		clock.Tick()
	}

	// Witness a lower remote value — should still advance past local
	v := clock.Witness(3)
	if v != 6 { // max(5, 3) + 1 = 6
		t.Fatalf("expected witness(3) with local=5 to give 6, got %d", v)
	}

	// Witness a higher remote value — should jump past it
	v = clock.Witness(100)
	if v != 101 { // max(6, 100) + 1 = 101
		t.Fatalf("expected witness(100) with local=6 to give 101, got %d", v)
	}
}

func TestLamportClockConcurrency(t *testing.T) {
	clock := NewLamportClock()
	var wg sync.WaitGroup

	// 100 goroutines each tick 100 times
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				clock.Tick()
			}
		}()
	}

	wg.Wait()

	expected := uint64(10000)
	if clock.Current() != expected {
		t.Fatalf("expected %d after 100*100 concurrent ticks, got %d", expected, clock.Current())
	}
}

func TestLamportClockWitnessConcurrency(t *testing.T) {
	clock := NewLamportClock()
	var wg sync.WaitGroup

	// 50 goroutines tick, 50 goroutines witness
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				clock.Tick()
			}
		}()
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				clock.Witness(uint64(id*100 + j))
			}
		}(i)
	}

	wg.Wait()

	// The clock should have advanced — exact value is nondeterministic
	// but must be at least 5000 (from the 50*100 ticks)
	if clock.Current() < 5000 {
		t.Fatalf("expected clock >= 5000 after concurrent ops, got %d", clock.Current())
	}
}
