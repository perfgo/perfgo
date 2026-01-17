package falsesharing

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkFalseSharing benchmarks concurrent counter increments WITH false sharing.
// Multiple goroutines increment adjacent counters that share cache lines.
func BenchmarkFalseSharing(b *testing.B) {
	shared := &SharedCounters{}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		index := 0
		for pb.Next() {
			atomic.AddInt64(&shared.counters[index%8], 1)
		}
	})
}

// BenchmarkNoPadding benchmarks with explicit goroutine management to ensure
// each goroutine uses a different counter index (maximizing false sharing).
func BenchmarkNoPadding(b *testing.B) {
	numGoroutines := runtime.NumCPU()
	shared := &SharedCounters{}
	iterationsPerGoroutine := b.N / numGoroutines

	b.ResetTimer()

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				atomic.AddInt64(&shared.counters[index%8], 1)
			}
		}(g)
	}
	wg.Wait()
}

// BenchmarkWithPadding benchmarks concurrent counter increments WITHOUT false sharing.
// Each counter is padded to occupy its own cache line.
func BenchmarkWithPadding(b *testing.B) {
	numGoroutines := runtime.NumCPU()
	padded := &PaddedCounters{}
	iterationsPerGoroutine := b.N / numGoroutines

	b.ResetTimer()

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				atomic.AddInt64(&padded.counters[index%8].value, 1)
			}
		}(g)
	}
	wg.Wait()
}

// BenchmarkSingleGoroutineNoPadding benchmarks single-threaded access (baseline).
func BenchmarkSingleGoroutineNoPadding(b *testing.B) {
	shared := &SharedCounters{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.AddInt64(&shared.counters[0], 1)
	}
}

// BenchmarkSingleGoroutineWithPadding benchmarks single-threaded access with padding.
func BenchmarkSingleGoroutineWithPadding(b *testing.B) {
	padded := &PaddedCounters{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.AddInt64(&padded.counters[0].value, 1)
	}
}

// Benchmarks with varying number of goroutines to show scaling behavior

func BenchmarkFalseSharing_2Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 2, false)
}

func BenchmarkFalseSharing_4Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 4, false)
}

func BenchmarkFalseSharing_8Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 8, false)
}

func BenchmarkPadded_2Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 2, true)
}

func BenchmarkPadded_4Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 4, true)
}

func BenchmarkPadded_8Goroutines(b *testing.B) {
	benchmarkWithGoroutines(b, 8, true)
}

func benchmarkWithGoroutines(b *testing.B, numGoroutines int, usePadding bool) {
	iterationsPerGoroutine := b.N / numGoroutines
	if iterationsPerGoroutine == 0 {
		iterationsPerGoroutine = 1
	}

	b.ResetTimer()

	var wg sync.WaitGroup

	if usePadding {
		padded := &PaddedCounters{}
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				for i := 0; i < iterationsPerGoroutine; i++ {
					atomic.AddInt64(&padded.counters[index].value, 1)
				}
			}(g % 8)
		}
	} else {
		shared := &SharedCounters{}
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				for i := 0; i < iterationsPerGoroutine; i++ {
					atomic.AddInt64(&shared.counters[index], 1)
				}
			}(g % 8)
		}
	}

	wg.Wait()
}

// BenchmarkAdjacentCounters specifically tests two goroutines accessing
// adjacent memory locations (index 0 and 1) - maximum false sharing.
func BenchmarkAdjacentCounters_NoPadding(b *testing.B) {
	shared := &SharedCounters{}
	iterationsPerGoroutine := b.N / 2

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			atomic.AddInt64(&shared.counters[0], 1)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			atomic.AddInt64(&shared.counters[1], 1)
		}
	}()

	wg.Wait()
}

// BenchmarkAdjacentCounters_WithPadding tests two goroutines with padded counters.
func BenchmarkAdjacentCounters_WithPadding(b *testing.B) {
	padded := &PaddedCounters{}
	iterationsPerGoroutine := b.N / 2

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			atomic.AddInt64(&padded.counters[0].value, 1)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			atomic.AddInt64(&padded.counters[1].value, 1)
		}
	}()

	wg.Wait()
}
