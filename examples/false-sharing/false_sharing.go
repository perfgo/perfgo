// Package falsesharing demonstrates the performance impact of false sharing
// in concurrent Go programs. False sharing occurs when multiple goroutines
// access different variables that happen to reside on the same CPU cache line,
// causing unnecessary cache invalidations and performance degradation.
package falsesharing

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// CacheLineSize is the typical cache line size on modern x86 processors (64 bytes).
const CacheLineSize = 64

// SharedCounters demonstrates false sharing - counters are adjacent in memory
// and likely share the same cache line.
type SharedCounters struct {
	counters [8]int64
}

// PaddedCounter is a counter with padding to prevent false sharing.
// The padding ensures each counter occupies its own cache line.
type PaddedCounter struct {
	value int64
	_     [CacheLineSize - 8]byte // Padding to fill cache line
}

// PaddedCounters uses padding to avoid false sharing between counters.
type PaddedCounters struct {
	counters [8]PaddedCounter
}

// IncrementShared increments a specific counter in SharedCounters atomically.
// When multiple goroutines call this with different indices, false sharing occurs
// because adjacent counters share the same cache line.
func (s *SharedCounters) IncrementShared(index int, iterations int) {
	for i := 0; i < iterations; i++ {
		atomic.AddInt64(&s.counters[index], 1)
	}
}

// IncrementPadded increments a specific counter in PaddedCounters atomically.
// Padding prevents false sharing even when different goroutines access different counters.
func (p *PaddedCounters) IncrementPadded(index int, iterations int) {
	for i := 0; i < iterations; i++ {
		atomic.AddInt64(&p.counters[index].value, 1)
	}
}

// GetShared returns the value of a counter in SharedCounters.
func (s *SharedCounters) GetShared(index int) int64 {
	return atomic.LoadInt64(&s.counters[index])
}

// GetPadded returns the value of a counter in PaddedCounters.
func (p *PaddedCounters) GetPadded(index int) int64 {
	return atomic.LoadInt64(&p.counters[index].value)
}

// RunWithFalseSharing runs concurrent increments with false sharing.
// Each goroutine increments a different counter, but they share cache lines.
func RunWithFalseSharing(numGoroutines, iterations int) *SharedCounters {
	shared := &SharedCounters{}
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			shared.IncrementShared(index%8, iterations)
		}(i)
	}

	wg.Wait()
	return shared
}

// RunWithoutFalseSharing runs concurrent increments without false sharing.
// Each goroutine increments a different counter with padding to prevent cache line contention.
func RunWithoutFalseSharing(numGoroutines, iterations int) *PaddedCounters {
	padded := &PaddedCounters{}
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			padded.IncrementPadded(index%8, iterations)
		}(i)
	}

	wg.Wait()
	return padded
}

// DemoFalseSharing provides a simple demonstration comparing both approaches.
// Returns the sum of all counters for verification.
func DemoFalseSharing() (sharedSum, paddedSum int64) {
	numCPUs := runtime.NumCPU()
	iterations := 1_000_000

	shared := RunWithFalseSharing(numCPUs, iterations)
	padded := RunWithoutFalseSharing(numCPUs, iterations)

	for i := 0; i < 8; i++ {
		sharedSum += shared.GetShared(i)
		paddedSum += padded.GetPadded(i)
	}

	return sharedSum, paddedSum
}
