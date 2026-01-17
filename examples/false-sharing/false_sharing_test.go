package falsesharing

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheLineSize(t *testing.T) {
	// Verify that PaddedCounter is at least cache line sized
	assert.GreaterOrEqual(t, int(unsafe.Sizeof(PaddedCounter{})), CacheLineSize,
		"PaddedCounter should be at least cache line sized")
}

func TestSharedCountersBasic(t *testing.T) {
	shared := &SharedCounters{}

	// Test basic increment
	shared.IncrementShared(0, 100)
	assert.Equal(t, int64(100), shared.GetShared(0))

	// Test multiple counters
	shared.IncrementShared(1, 200)
	assert.Equal(t, int64(200), shared.GetShared(1))

	// First counter should be unchanged
	assert.Equal(t, int64(100), shared.GetShared(0))
}

func TestPaddedCountersBasic(t *testing.T) {
	padded := &PaddedCounters{}

	// Test basic increment
	padded.IncrementPadded(0, 100)
	assert.Equal(t, int64(100), padded.GetPadded(0))

	// Test multiple counters
	padded.IncrementPadded(1, 200)
	assert.Equal(t, int64(200), padded.GetPadded(1))

	// First counter should be unchanged
	assert.Equal(t, int64(100), padded.GetPadded(0))
}

func TestRunWithFalseSharing(t *testing.T) {
	numGoroutines := 4
	iterations := 10000

	shared := RunWithFalseSharing(numGoroutines, iterations)

	// Calculate expected sum: each goroutine increments iterations times
	var sum int64
	for i := 0; i < 8; i++ {
		sum += shared.GetShared(i)
	}

	expectedSum := int64(numGoroutines * iterations)
	assert.Equal(t, expectedSum, sum, "Total sum should equal numGoroutines * iterations")
}

func TestRunWithoutFalseSharing(t *testing.T) {
	numGoroutines := 4
	iterations := 10000

	padded := RunWithoutFalseSharing(numGoroutines, iterations)

	// Calculate expected sum: each goroutine increments iterations times
	var sum int64
	for i := 0; i < 8; i++ {
		sum += padded.GetPadded(i)
	}

	expectedSum := int64(numGoroutines * iterations)
	assert.Equal(t, expectedSum, sum, "Total sum should equal numGoroutines * iterations")
}

func TestDemoFalseSharing(t *testing.T) {
	sharedSum, paddedSum := DemoFalseSharing()

	numCPUs := runtime.NumCPU()
	iterations := 1_000_000
	expectedSum := int64(numCPUs * iterations)

	assert.Equal(t, expectedSum, sharedSum, "Shared sum should match expected")
	assert.Equal(t, expectedSum, paddedSum, "Padded sum should match expected")
}

func TestPaddedCounterAlignment(t *testing.T) {
	padded := &PaddedCounters{}

	// Verify that consecutive counters are at least CacheLineSize apart
	addr0 := uintptr(unsafe.Pointer(&padded.counters[0]))
	addr1 := uintptr(unsafe.Pointer(&padded.counters[1]))

	diff := addr1 - addr0
	require.GreaterOrEqual(t, int(diff), CacheLineSize,
		"Padded counters should be at least CacheLineSize apart to avoid false sharing")
}

func TestSharedCounterProximity(t *testing.T) {
	shared := &SharedCounters{}

	// Verify that counters in SharedCounters are adjacent (8 bytes apart for int64)
	addr0 := uintptr(unsafe.Pointer(&shared.counters[0]))
	addr1 := uintptr(unsafe.Pointer(&shared.counters[1]))

	diff := addr1 - addr0
	assert.Equal(t, int(diff), 8,
		"Shared counters should be adjacent (8 bytes apart)")
}

func TestConcurrentCorrectness(t *testing.T) {
	// Test that both implementations produce correct results under heavy concurrency
	numGoroutines := runtime.NumCPU() * 2
	iterations := 100000

	// Run multiple times to increase chance of catching race conditions
	for run := 0; run < 5; run++ {
		shared := RunWithFalseSharing(numGoroutines, iterations)
		padded := RunWithoutFalseSharing(numGoroutines, iterations)

		var sharedSum, paddedSum int64
		for i := 0; i < 8; i++ {
			sharedSum += shared.GetShared(i)
			paddedSum += padded.GetPadded(i)
		}

		expectedSum := int64(numGoroutines * iterations)
		assert.Equal(t, expectedSum, sharedSum, "Run %d: Shared sum mismatch", run)
		assert.Equal(t, expectedSum, paddedSum, "Run %d: Padded sum mismatch", run)
	}
}
