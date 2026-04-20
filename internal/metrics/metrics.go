// Package metrics provides Prometheus-compatible metrics collection for HyperCache.
// Uses atomic counters and fixed-bucket histograms — no external dependencies.
package metrics

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Collector holds all HyperCache metrics.
var global = NewCollector()

// Global returns the singleton metrics collector.
func Global() *Collector { return global }

// Collector aggregates counters, gauges, and histograms.
type Collector struct {
	counters   map[string]*atomic.Int64
	gauges     map[string]*atomic.Int64
	histograms map[string]*Histogram
	mu         sync.RWMutex
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	c := &Collector{
		counters:   make(map[string]*atomic.Int64),
		gauges:     make(map[string]*atomic.Int64),
		histograms: make(map[string]*Histogram),
	}
	// Pre-register latency histograms for hot-path operations
	// Buckets in seconds: 10µs, 50µs, 100µs, 250µs, 500µs, 1ms, 2.5ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s
	latencyBuckets := []float64{
		0.00001, 0.00005, 0.0001, 0.00025, 0.0005,
		0.001, 0.0025, 0.005, 0.01, 0.025,
		0.05, 0.1, 0.25, 0.5, 1.0,
	}
	c.histograms["hypercache_operation_duration_seconds_set"] = NewHistogram(latencyBuckets)
	c.histograms["hypercache_operation_duration_seconds_get"] = NewHistogram(latencyBuckets)
	c.histograms["hypercache_operation_duration_seconds_del"] = NewHistogram(latencyBuckets)
	c.histograms["hypercache_snapshot_duration_seconds"] = NewHistogram([]float64{0.001, 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 5.0, 10.0, 30.0})
	return c
}

// IncCounter increments a counter by 1.
func (c *Collector) IncCounter(name string) {
	c.mu.RLock()
	ctr, ok := c.counters[name]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		ctr, ok = c.counters[name]
		if !ok {
			ctr = &atomic.Int64{}
			c.counters[name] = ctr
		}
		c.mu.Unlock()
	}
	ctr.Add(1)
}

// SetGauge sets a gauge value.
func (c *Collector) SetGauge(name string, val int64) {
	c.mu.RLock()
	g, ok := c.gauges[name]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		g, ok = c.gauges[name]
		if !ok {
			g = &atomic.Int64{}
			c.gauges[name] = g
		}
		c.mu.Unlock()
	}
	g.Store(val)
}

// ObserveLatency records a duration observation in a histogram.
func (c *Collector) ObserveLatency(name string, d time.Duration) {
	c.mu.RLock()
	h, ok := c.histograms[name]
	c.mu.RUnlock()
	if ok {
		h.Observe(d.Seconds())
	}
}

// RecordOp is a convenience wrapper: records a counter increment and latency observation.
func (c *Collector) RecordOp(op string, start time.Time) {
	d := time.Since(start)
	c.IncCounter("hypercache_operations_total_" + op)
	c.ObserveLatency("hypercache_operation_duration_seconds_"+op, d)
}

// WritePrometheus writes all metrics in Prometheus text exposition format.
func (c *Collector) WritePrometheus(b *strings.Builder, nodeID string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Counters
	for name, ctr := range c.counters {
		fmt.Fprintf(b, "# TYPE %s counter\n", name)
		fmt.Fprintf(b, "%s{node=\"%s\"} %d\n", name, nodeID, ctr.Load())
	}

	// Gauges
	for name, g := range c.gauges {
		fmt.Fprintf(b, "# TYPE %s gauge\n", name)
		fmt.Fprintf(b, "%s{node=\"%s\"} %d\n", name, nodeID, g.Load())
	}

	// Histograms
	for name, h := range c.histograms {
		h.WritePrometheus(b, name, nodeID)
	}
}

// Histogram is a fixed-bucket histogram using atomic counters.
type Histogram struct {
	buckets    []float64      // upper bounds (sorted ascending)
	counts     []atomic.Int64 // per-bucket counts
	totalCount atomic.Int64
	totalSum   atomic.Int64 // sum stored as fixed-point (nanoseconds)
}

// NewHistogram creates a histogram with the given bucket boundaries (upper bounds in seconds).
func NewHistogram(buckets []float64) *Histogram {
	return &Histogram{
		buckets: buckets,
		counts:  make([]atomic.Int64, len(buckets)+1), // +1 for +Inf
	}
}

// Observe records a value (in seconds).
func (h *Histogram) Observe(v float64) {
	for i, bound := range h.buckets {
		if v <= bound {
			h.counts[i].Add(1)
		}
	}
	// +Inf bucket always incremented
	h.counts[len(h.buckets)].Add(1)
	h.totalCount.Add(1)
	// Store sum as nanoseconds for precision
	h.totalSum.Add(int64(v * 1e9))
}

// WritePrometheus writes the histogram in Prometheus text format.
func (h *Histogram) WritePrometheus(b *strings.Builder, name, nodeID string) {
	fmt.Fprintf(b, "# TYPE %s histogram\n", name)
	for i, bound := range h.buckets {
		le := formatFloat(bound)
		fmt.Fprintf(b, "%s_bucket{node=\"%s\",le=\"%s\"} %d\n", name, nodeID, le, h.counts[i].Load())
	}
	fmt.Fprintf(b, "%s_bucket{node=\"%s\",le=\"+Inf\"} %d\n", name, nodeID, h.counts[len(h.buckets)].Load())
	sumSec := float64(h.totalSum.Load()) / 1e9
	fmt.Fprintf(b, "%s_sum{node=\"%s\"} %s\n", name, nodeID, formatFloat(sumSec))
	fmt.Fprintf(b, "%s_count{node=\"%s\"} %d\n", name, nodeID, h.totalCount.Load())
}

func formatFloat(f float64) string {
	if f == math.Inf(1) {
		return "+Inf"
	}
	return fmt.Sprintf("%g", f)
}
