// Package falsesharing demonstrates the performance impact of false sharing
// in concurrent Go programs. False sharing occurs when multiple goroutines
// access different variables that happen to reside on the same CPU cache line,
// causing unnecessary cache invalidations and performance degradation.
package falsesharing

// CacheLineSize is the typical cache line size on modern x86 processors (64 bytes).
const CacheLineSize = 64

// Metrics demonstrates false sharing - counters are adjacent in memory
// and likely share the same cache line.
type Metrics struct {
	RequestsTotal int64
	CronJobRuns   int64
}

// PaddedMetrics uses padding to avoid false sharing between counters.
// Each field is padded to occupy its own cache line.
type PaddedMetrics struct {
	RequestsTotal int64
	_             [CacheLineSize - 8]byte // Padding to fill cache line
	CronJobRuns   int64
	_             [CacheLineSize - 8]byte // Padding to fill cache line
	// More metrics here
}
