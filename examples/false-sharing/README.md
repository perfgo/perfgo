# False Sharing Example

This package demonstrates the performance impact of **false sharing** in concurrent Go programs.

## What is False Sharing?

False sharing occurs when multiple CPU cores modify variables that reside on the same **cache line** (typically 64 bytes on x86/ARM). Even though the variables are logically independent, the CPU cache coherency protocol (MESI/MOESI) forces cache line invalidations across cores, causing significant performance degradation.

```
Cache Line (64 bytes)
┌────────────────────────────────────────────────────────────────┐
│ counter[0] │ counter[1] │ counter[2] │ counter[3] │ ... │
│  (8 bytes) │  (8 bytes) │  (8 bytes) │  (8 bytes) │     │
└────────────────────────────────────────────────────────────────┘
     ▲              ▲
     │              │
   Core 0        Core 1
   writes        writes
     │              │
     └──────────────┘
        Cache line bounces
        between cores!
```

## Files

| File | Description |
|------|-------------|
| `false_sharing.go` | Core implementation with `SharedCounters` (bad) vs `PaddedCounters` (good) |
| `false_sharing_test.go` | Unit tests for correctness and memory layout verification |
| `benchmark_test.go` | Micro benchmarks demonstrating the performance impact |

## Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem

# Run with multiple iterations for statistical significance
go test -bench=. -benchmem -count=5

# Compare specific benchmarks
go test -bench='BenchmarkFalseSharing_8Goroutines|BenchmarkPadded_8Goroutines' -benchmem
```

## Example Results (Apple M2 Max, 12 cores)

| Benchmark | ns/op | Speedup |
|-----------|-------|---------|
| `FalseSharing_8Goroutines` | ~38-40 | baseline |
| `Padded_8Goroutines` | ~4-5 | **~9x faster** |
| `AdjacentCounters_NoPadding` | ~13.5 | baseline |
| `AdjacentCounters_WithPadding` | ~5.2 | **2.6x faster** |

## The Fix: Cache Line Padding

```go
// BAD: Adjacent counters share cache lines
type SharedCounters struct {
    counters [8]int64  // All 64 bytes fit in ONE cache line!
}

// GOOD: Each counter gets its own cache line
type PaddedCounter struct {
    value int64
    _     [56]byte  // Padding to fill 64-byte cache line
}

type PaddedCounters struct {
    counters [8]PaddedCounter  // Each counter on separate cache line
}
```

## Detecting False Sharing with Performance Counters

False sharing manifests as excessive cache coherency traffic. Use these hardware performance counters to detect it:

### Linux (perf)

```bash
# Primary counters for detecting false sharing
perf stat -e L1-dcache-load-misses,L1-dcache-loads,\
l2_rqsts.all_demand_data_rd,l2_rqsts.demand_data_rd_miss,\
offcore_response.demand_data_rd.l3_miss.snoop_hitm \
./your_program

# Intel-specific: HITM (Hit Modified) - THE key indicator
# High HITM count = cache lines bouncing between cores in Modified state
perf stat -e mem_load_l3_hit_retired.xsnp_hitm,\
mem_load_l3_hit_retired.xsnp_miss \
./your_program

# Use perf c2c for detailed false sharing analysis (Linux 4.10+)
perf c2c record ./your_program
perf c2c report
```

### Key Performance Counters

| Counter | What it measures | False sharing indicator |
|---------|------------------|------------------------|
| `mem_load_l3_hit_retired.xsnp_hitm` | Loads that hit in L3 but another core had modified data | **Primary indicator** - high values suggest false sharing |
| `l2_rqsts.all_rfo` | Read-for-ownership requests (write intent) | High RFO with low actual stores = contention |
| `offcore_response.*.snoop_hitm` | Off-core requests hitting modified lines | Cross-core cache line bouncing |
| `L1-dcache-load-misses` | L1 data cache misses | Elevated misses during concurrent access |
| `machine_clears.memory_ordering` | Pipeline clears due to memory ordering | Memory ordering conflicts |

### macOS (Instruments / DTrace)

```bash
# Use Instruments with "Counters" template
# Look for:
# - L1D_CACHE_MISS_LD
# - L1D_CACHE_MISS_ST
# - BUS_ACCESS (memory bus transactions)

# Or use powermetrics for high-level cache stats
sudo powermetrics --samplers cpu_power -i 1000
```

### Intel VTune

```bash
# Use the "Memory Access" analysis
vtune -collect memory-access ./your_program

# Look for:
# - Contested Accesses (direct indicator)
# - Remote Cache Accesses
# - LLC Miss Count
```

### What to Look For

1. **High HITM ratio**: `snoop_hitm` / total loads > 1% is suspicious
2. **Scaling problems**: Performance gets *worse* with more cores
3. **High L1 miss rate** during concurrent access to "independent" data
4. **Memory bandwidth saturation** despite low actual data transfer needs

### Example: Profiling This Code

```bash
# On Linux with perf
cd examples/false-sharing

# Profile the false sharing version
perf stat -e cycles,instructions,L1-dcache-load-misses,\
l2_rqsts.demand_data_rd_miss \
go test -bench=BenchmarkNoPadding -benchtime=5s

# Profile the padded version  
perf stat -e cycles,instructions,L1-dcache-load-misses,\
l2_rqsts.demand_data_rd_miss \
go test -bench=BenchmarkWithPadding -benchtime=5s

# Compare the cache miss rates!
```

## Common Scenarios Where False Sharing Occurs

1. **Counters/metrics in structs** accessed by different goroutines
2. **Worker pools** with per-worker state in adjacent array slots
3. **Circular buffers** with head/tail pointers
4. **Lock-free data structures** with adjacent atomic variables
5. **Thread-local storage** packed into arrays

## Best Practices

1. **Pad hot variables** to cache line boundaries when accessed by multiple cores
2. **Group read-only and read-write data** separately
3. **Use `sync.Pool`** instead of per-goroutine arrays when possible
4. **Profile first** - padding increases memory usage, only apply where needed
5. **Consider `runtime.GOMAXPROCS`** - false sharing impact scales with core count

## Further Reading

- [What Every Programmer Should Know About Memory](https://people.freebsd.org/~lstewart/articles/cpumemory.pdf) - Ulrich Drepper
- [Intel 64 and IA-32 Optimization Reference Manual](https://www.intel.com/content/www/us/en/developer/articles/technical/intel-sdm.html) - Chapter on cache optimization
- [perf c2c documentation](https://man7.org/linux/man-pages/man1/perf-c2c.1.html) - Linux cache-to-cache analysis tool
