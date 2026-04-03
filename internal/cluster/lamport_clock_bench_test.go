package cluster

import (
	"testing"
)

// BenchmarkLamportClockTick measures raw tick throughput (sequential).
func BenchmarkLamportClockTick(b *testing.B) {
	clock := NewLamportClock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clock.Tick()
	}
}

// BenchmarkLamportClockTick_Parallel measures tick throughput under contention.
func BenchmarkLamportClockTick_Parallel(b *testing.B) {
	clock := NewLamportClock()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			clock.Tick()
		}
	})
}

// BenchmarkLamportClockWitness measures witness (CAS loop) throughput.
func BenchmarkLamportClockWitness(b *testing.B) {
	clock := NewLamportClock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clock.Witness(uint64(i))
	}
}

// BenchmarkLamportClockWitness_Parallel measures witness under contention.
func BenchmarkLamportClockWitness_Parallel(b *testing.B) {
	clock := NewLamportClock()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := uint64(0)
		for pb.Next() {
			clock.Witness(i)
			i++
		}
	})
}
