package falsesharing

import (
	"sync"
	"testing"
)

// BenchmarkNoPadding demonstrates false sharing.
// Two goroutines increment adjacent counters that share the same cache line,
// causing constant cache invalidation and poor performance.
func BenchmarkNoPadding(b *testing.B) {
	shared := &Metrics{}
	iterationsPerGoroutine := b.N / 2

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			shared.RequestsTotal++
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			shared.CronJobRuns++
		}
	}()

	wg.Wait()
	if shared.CronJobRuns != int64(iterationsPerGoroutine) {
		b.Errorf("unexpected result for CronJobRuns exp=%d act=%d", iterationsPerGoroutine, shared.CronJobRuns)
	}
	if shared.RequestsTotal != int64(iterationsPerGoroutine) {
		b.Errorf("unexpected result for RequestsTotal exp=%d act=%d", iterationsPerGoroutine, shared.RequestsTotal)
	}
}

// BenchmarkWithPadding avoids false sharing.
// Each counter is padded to occupy its own cache line, eliminating
// cache invalidation and showing much better performance.
func BenchmarkWithPadding(b *testing.B) {
	padded := &PaddedMetrics{}
	iterationsPerGoroutine := b.N / 2

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			padded.RequestsTotal++
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterationsPerGoroutine; i++ {
			padded.CronJobRuns++
		}
	}()

	wg.Wait()
	if padded.CronJobRuns != int64(iterationsPerGoroutine) {
		b.Errorf("unexpected result for CronJobRuns exp=%d act=%d", iterationsPerGoroutine, padded.CronJobRuns)
	}
	if padded.RequestsTotal != int64(iterationsPerGoroutine) {
		b.Errorf("unexpected result for RequestsTotal exp=%d act=%d", iterationsPerGoroutine, padded.RequestsTotal)
	}
}
