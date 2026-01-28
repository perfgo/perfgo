# False Sharing Example

This package demonstrates the performance impact of **false sharing** in concurrent Go programs.

## What is False Sharing?

False sharing occurs when multiple CPU cores modify variables that reside on the same **cache line** (typically 64 bytes on x86/ARM). Even though the variables are logically independent, the CPU cache coherency protocol (MESI/MOESI) forces cache line invalidations across cores, causing significant performance degradation.

```
Cache Line (64 bytes)
┌────────────────────────────────────────────────────────────────┐
│ RequestsTotal │  CronJobRuns  │      (unused space)           │
│   (8 bytes)   │   (8 bytes)   │        (48 bytes)             │
└────────────────────────────────────────────────────────────────┘
       ▲                ▲
       │                │
     Core 0          Core 1
     writes          writes
       │                │
       └────────────────┘
          Cache line bounces
          between cores!
```

## Files

| File | Description |
|------|-------------|
| `false_sharing.go` | Core implementation with `Metrics` (bad) vs `PaddedMetrics` (good) |
| `benchmark_test.go` | Benchmarks demonstrating the performance impact |

## Running Benchmarks

### Step 1: Run Basic Benchmarks

```bash
go test ./ -bench=. -benchmem -benchtime=10000000x -run=^$
```

<details>
<summary>View results</summary>

```
goos: darwin
goarch: arm64
pkg: github.com/perfgo/perfgo/examples/false-sharing
cpu: Apple M2 Max
BenchmarkNoPadding-12      	10000000	         0.6496 ns/op	       0 B/op	       0 allocs/op
BenchmarkWithPadding-12    	10000000	         0.1892 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/perfgo/perfgo/examples/false-sharing	0.193s
```

</details>

### Step 2: Collect Performance Stats (No Padding)

```bash
perfgo test stat -- ./ -bench=WithNoPadding -benchmem -benchtime=10000000x -run=^$
```

<details>
<summary>View results</summary>

```
        25,604,929      task-clock                       #    1.794 CPUs utilized
               231      context-switches                 #    9.022 K/sec
                17      cpu-migrations                   #  663.935 /sec
             1,372      page-faults                      #   53.583 K/sec
>>>     58,983,263      instructions                     #    0.72  insn per cycle
>>>                                                      #    0.15  stalled cycles per insn     (67.29%)
        82,303,201      cycles                           #    3.214 GHz                         (50.31%)
         8,950,006      stalled-cycles-frontend          #   10.87% frontend cycles idle        (65.70%)
        13,286,491      branches                         #  518.904 M/sec                       (83.74%)
           109,291      branch-misses                    #    0.82% of all branches             (84.38%)
        30,202,774      L1-dcache-loads                  #    1.180 G/sec                       (83.01%)
           530,830      L1-dcache-load-misses            #    1.76% of all L1-dcache accesses   (79.17%)

       0.014275218 seconds time elapsed

       0.021153000 seconds user
       0.005875000 seconds sys
```

</details>

### Step 3: Collect Performance Stats (With Padding)

```bash
perfgo test stat -- ./ -bench=WithPadding -benchmem -benchtime=10000000x -run=^$
```

<details>
<summary>View results</summary>

```
        11,131,960      task-clock                       #    1.769 CPUs utilized
               220      context-switches                 #   19.763 K/sec
                11      cpu-migrations                   #  988.146 /sec
             1,398      page-faults                      #  125.584 K/sec
>>>     83,744,316      instructions                     #    3.68  insn per cycle
>>>                                                      #    0.10  stalled cycles per insn     (23.52%)
        22,729,257      cycles                           #    2.042 GHz                         (55.66%)
         8,387,353      stalled-cycles-frontend          #   36.90% frontend cycles idle        (84.99%)
        14,075,774      branches                         #    1.264 G/sec                       (97.90%)
            95,035      branch-misses                    #    0.68% of all branches
        19,953,683      L1-dcache-loads                  #    1.792 G/sec                       (94.27%)
           253,134      L1-dcache-load-misses            #    1.27% of all L1-dcache accesses   (66.98%)

       0.006291924 seconds time elapsed

       0.009082000 seconds user
       0.003652000 seconds sys
```

</details>

**Analysis:** While nothing clearly points to false sharing in the raw counters, comparing the two benchmarks reveals the impact. Despite doing the exact same work, we see a significant decrease in time elapsed and L1-dcache-loads with padding.

### Step 4: Profile Cache Loads

```bash
perfgo test profile -e cache-loads -- ./ -bench=. -benchmem -benchtime=10000000x -run=^$
```

<details>
<summary>View flamegraph</summary>

The [flamegraph] shows a moderately higher cache miss rate in the not padded counter struct.

</details>

[flamegraph]:https://flamegraph.com/share/eacb682c-fc52-11f0-bff4-ee750a85d76a

### Step 5: Detecting False Sharing with c2c Analysis

To find false sharing without prior knowledge, a `perf c2c` provides useful insights:

```bash
perfgo test cache-to-cache -- ./ -bench=NoPadding -benchmem -benchtime=10s -run=^$
```

<details>
<summary>View report</summary>

**Note**: The address to source line resolution is not yet working as expected.

```
=================================================
            Trace Event Information              
=================================================
  Total records                     :       9007
  Locked Load/Store Operations      :          1
  Load Operations                   :       3266
  Loads - uncacheable               :          0
  Loads - IO                        :          0
  Loads - Miss                      :          0
  Loads - no mapping                :         17
  Load Fill Buffer Hit              :          0
  Load L1D hit                      :       1149
  Load L2D hit                      :          2
  Load LLC hit                      :       2093
  Load Local HITM                   :       2092
  Load Remote HITM                  :          4
  Load Remote HIT                   :          0
  Load Local DRAM                   :          0
  Load Remote DRAM                  :          0
  Load MESI State Exclusive         :          0
  Load MESI State Shared            :          0
  Load LLC Misses                   :          4
  Load access blocked by data       :          0
  Load access blocked by address    :          0
  Load HIT Local Peer               :          0
  Load HIT Remote Peer              :          0
  LLC Misses to Local DRAM          :        0.0%
  LLC Misses to Remote DRAM         :        0.0%
  LLC Misses to Remote cache (HIT)  :        0.0%
  LLC Misses to Remote cache (HITM) :      100.0%
  Store Operations                  :         41
  Store - uncacheable               :          0
  Store - no mapping                :          4
  Store L1D Hit                     :         36
  Store L1D Miss                    :          0
  Store No available memory level   :          1
  No Page Map Rejects               :         38
  Unable to parse data source       :       5700

=================================================
    Global Shared Cache Line Event Information   
=================================================
  Total Shared Cache Lines          :          1
  Load HITs on shared lines         :       3152
  Fill Buffer Hits on shared lines  :          0
  L1D hits on shared lines          :       1056
  L2D hits on shared lines          :          0
  LLC hits on shared lines          :       2092
  Load hits on peer cache or nodes  :          0
  Locked Access on shared lines     :          0
  Blocked Access on shared lines    :          0
  Store HITs on shared lines        :          0
  Store L1D hits on shared lines    :          0
  Store No available memory level   :          0
  Total Merged records              :       2096

=================================================
                 c2c details                     
=================================================
  Events                            : ibs_op/ldlat=0/
  Cachelines sort on                : Total HITMs
  Cacheline data grouping           : offset,iaddr

=================================================
           Shared Data Cache Line Table          
=================================================
#
#        ----------- Cacheline ----------      Tot  ------- Load Hitm -------    Total    Total    Total  --------- Stores --------  ----- Core Load Hit -----  - LLC Load Hit --  - RMT Load Hit --  --- Load Dram ----
# Index             Address  Node  PA cnt     Hitm    Total  LclHitm  RmtHitm  records    Loads   Stores    L1Hit   L1Miss      N/A       FB       L1       L2    LclHit  LclHitm    RmtHit  RmtHitm       Lcl       Rmt
# .....  ..................  ....  ......  .......  .......  .......  .......  .......  .......  .......  .......  .......  .......  .......  .......  .......  ........  .......  ........  .......  ........  ........
#
      0      0x20ea08034080     0    1734  100.00%     2096     2092        4     3152     3152        0        0        0        0        0     1056        0         0     2092         0        4         0         0

=================================================
      Shared Cache Line Distribution Pareto      
=================================================
#
#        ----- HITM -----  ------- Store Refs ------  --------- Data address ---------                      ---------- cycles ----------    Total       cpu                                                   Shared                   
#   Num  RmtHitm  LclHitm   L1 Hit  L1 Miss      N/A              Offset  Node  PA cnt        Code address  rmt hitm  lcl hitm      load  records       cnt                          Symbol                   Object  Source:Line  Node
# .....  .......  .......  .......  .......  .......  ..................  ....  ......  ..................  ........  ........  ........  .......  ........  ..............................  .......................  ...........  ....
#
  ----------------------------------------------------------------------
      0        4     2092        0        0        0      0x20ea08034080
  ----------------------------------------------------------------------
          75.00%   49.62%    0.00%    0.00%    0.00%                0x30     0       1            0x53dc88       410        75         0     1552        12  [.] github.com/perfgo/perfgo/e  perfgo.test.linux.amd64  go.go:0       0
          25.00%   50.38%    0.00%    0.00%    0.00%                0x38     0       1            0x53dbc8         0        75         0     1600        10  [.] github.com/perfgo/perfgo/e  perfgo.test.linux.amd64  go.go:0       0

```

**Analysis**: The c2c report provides clear evidence of false sharing:

1. **Single Hot Cache Line**: Address `0x20ea08034080` accounts for 2,096 HITM (Hit In The Modified state) events - the smoking gun for false sharing. HITMs occur when one core reads a cache line that was just modified by another core.

2. **Memory Layout Evidence**: The report shows two hot spots exactly 8 bytes apart:
   - Offset `0x30`: 49.62% of local HITMs (1,552 records)
   - Offset `0x38`: 50.38% of local HITMs (1,600 records)

   This confirms two separate `int64` fields (`RequestsTotal` and `CronJobRuns`) are being modified independently but share the same 64-byte cache line.

3. **Performance Impact**: With 2,096 HITMs out of 3,152 total load operations, **66% of loads** cause cache coherency traffic. The cache line is constantly invalidated and transferred between cores, with 2,093 LLC hits indicating the data was frequently evicted from L1 cache.

4. **Perfect Split**: The nearly equal distribution (49.62% vs 50.38%) indicates two separate cores are hammering different fields at similar rates, causing continuous cache line bouncing.

This is textbook false sharing that padding fixes by giving each counter its own cache line, eliminating the 2,000+ cache invalidations during the benchmark.

</details>

## Example Results

| Benchmark | ns/op | Speedup |
|-----------|-------|---------|
| `BenchmarkNoPadding` | baseline | 1x |
| `BenchmarkWithPadding` | significantly faster | 2-10x depending on cores |

## The Fix: Cache Line Padding

```go
// BAD: Adjacent fields share cache lines
type Metrics struct {
    RequestsTotal int64  // These two fields fit in ONE cache line!
    CronJobRuns   int64
}

// GOOD: Each field gets its own cache line via padding
type PaddedMetrics struct {
    RequestsTotal int64
    _             [56]byte  // Padding to fill 64-byte cache line
    CronJobRuns   int64
    _             [56]byte  // Padding to fill 64-byte cache line
}
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
4. **Profile first** - padding increases memory usage, only apply where needed
5. **Consider `runtime.GOMAXPROCS`** - false sharing impact scales with core count

## Further Reading

- [What Every Programmer Should Know About Memory](https://people.freebsd.org/~lstewart/articles/cpumemory.pdf) - Ulrich Drepper
- [Intel 64 and IA-32 Optimization Reference Manual](https://www.intel.com/content/www/us/en/developer/articles/technical/intel-sdm.html) - Chapter on cache optimization
- [perf c2c documentation](https://man7.org/linux/man-pages/man1/perf-c2c.1.html) - Linux cache-to-cache analysis tool
