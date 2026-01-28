# Branch Prediction Example

This example demonstrates the impact of CPU branch prediction on performance by comparing branching vs. branchless code patterns.

## What is Branch Prediction?

Modern CPUs use branch prediction to speculatively execute instructions before knowing which path a conditional branch will take. When predictions are correct, performance is excellent. When predictions fail (branch mispredictions), the CPU must discard speculative work and restart, causing significant performance penalties.

## The Experiment

This example compares two approaches to counting positive and negative numbers:

### 1. Branching Approach (`evalBranch`)

```go
func (r *result) evalBranch(v int64) {
    if v < 0 {
        r.neg += 1
    } else {
        r.pos += 1
    }
}
```

Uses a traditional if-else branch. Performance depends heavily on branch predictability.

### 2. Branchless Approach (`evalBranchless`)

```go
func (r *result) evalBranchless(v int64) {
    // Use arithmetic to avoid branch: (v >> 63) gives -1 for negative, 0 for non-negative
    isNeg := int((v >> 63) & 1)
    r.neg += isNeg
    r.pos += 1 - isNeg
}
```

Uses bit manipulation to eliminate the branch entirely. The sign bit (bit 63) determines which counter to increment without conditional jumps.

## Running Benchmarks

### Step 1: Run Basic Benchmarks

```bash
go test ./ -bench=. -benchmem -benchtime=10000x -run=^$
```

<details>
<summary>View results</summary>

```
goos: darwin
goarch: arm64
pkg: github.com/perfgo/perfgo/examples/branch-prediction
cpu: Apple M2 Max
BenchmarkBranchWithPredictable-12        	   10000	    201602 ns/op	       0 B/op	       0 allocs/op
BenchmarkBranchWithRandom-12             	   10000	    326069 ns/op	       0 B/op	       0 allocs/op
BenchmarkBranchlessWithPredictable-12    	   10000	    236376 ns/op	       0 B/op	       0 allocs/op
BenchmarkBranchlessWithRandom-12         	   10000	    237159 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/perfgo/perfgo/examples/branch-prediction	10.343s
```

</details>

### Step 2: Collect Performance Stats (Branching with Predictable Data)

```bash
perfgo test stat -- ./ -bench=BranchWithPredictable -benchmem -benchtime=10000x -run=^$
```

<details>
<summary>View results</summary>

```
       464,157,217      task-clock                       #    1.006 CPUs utilized
               249      context-switches                 #  536.456 /sec
                14      cpu-migrations                   #   30.162 /sec
             1,507      page-faults                      #    3.247 K/sec
    10,081,754,122      instructions                     #    5.28  insn per cycle
                                                  #    0.02  stalled cycles per insn     (70.68%)
     1,908,999,659      cycles                           #    4.113 GHz                         (71.20%)
       210,848,110      stalled-cycles-frontend          #   11.04% frontend cycles idle        (71.90%)
     2,992,876,287      branches                         #    6.448 G/sec                       (72.00%)
           142,629      branch-misses                    #    0.00% of all branches             (72.00%)
     2,998,601,725      L1-dcache-loads                  #    6.460 G/sec                       (71.89%)
       125,814,748      L1-dcache-load-misses            #    4.20% of all L1-dcache accesses   (71.12%)

       0.461358658 seconds time elapsed

       0.457453000 seconds user
       0.004931000 seconds sys

```

</details>

**Analysis:** With predictable data (sorted sequence), the branching approach performs excellently. The branch predictor achieves near-perfect accuracy (0.00% miss rate with only 142,629 misses), resulting in high instruction throughput (5.28 IPC) and minimal pipeline stalls (11.04% frontend idle). The CPU efficiently speculates ahead, keeping the execution pipeline full and completing in just 0.46 seconds.

### Step 3: Collect Performance Stats (Branching with Random Data)

```bash
perfgo test stat -- ./ -bench=BranchWithRandom -benchmem -benchtime=10000x -run=^$
```

<details>
<summary>View results</summary>

```
    4,082,777,111      task-clock                       #    1.001 CPUs utilized
               848      context-switches                 #  207.702 /sec
                35      cpu-migrations                   #    8.573 /sec
             1,485      page-faults                      #  363.723 /sec
    10,077,671,973      instructions                     #    0.70  insn per cycle
                                                  #    0.62  stalled cycles per insn     (71.38%)
    14,415,563,728      cycles                           #    3.531 GHz                         (71.40%)
     6,266,243,121      stalled-cycles-frontend          #   43.47% frontend cycles idle        (71.45%)
     3,014,975,373      branches                         #  738.462 M/sec                       (71.47%)
       445,147,614      branch-misses                    #   14.76% of all branches             (71.50%)
     7,073,911,101      L1-dcache-loads                  #    1.733 G/sec                       (71.51%)
       127,617,506      L1-dcache-load-misses            #    1.80% of all L1-dcache accesses   (71.40%)

       4.076739041 seconds time elapsed

       4.050451000 seconds user
       0.005972000 seconds sys

```

</details>

**Analysis:** Random data causes catastrophic branch mispredictions, with a 14.76% miss rate (445M misses). This devastates performance: IPC plummets from 5.28 to just 0.70 (7.5x reduction), frontend stalls spike to 43.47%, and execution time balloons to 4.08 seconds—**8.8x slower** than with predictable data. Each misprediction forces a pipeline flush, discarding ~15-20 speculatively executed instructions. The CPU spends more time recovering from mispredictions than doing useful work, demonstrating why unpredictable branches are so expensive.

### Step 4: Collect Performance Stats (Branchless with Random Data)

```bash
perfgo test stat -- ./ -bench=BranchlessWithRandom -benchmem -benchtime=10000x -run=^$
```

<details>
<summary>View results</summary>

```

       472,309,054      task-clock                       #    1.005 CPUs utilized
               232      context-switches                 #  491.204 /sec
                 5      cpu-migrations                   #   10.586 /sec
             1,500      page-faults                      #    3.176 K/sec
    13,136,696,533      instructions                     #    6.25  insn per cycle
                                                  #    0.00  stalled cycles per insn     (70.74%)
     2,102,133,330      cycles                           #    4.451 GHz                         (71.28%)
        11,993,942      stalled-cycles-frontend          #    0.57% frontend cycles idle        (71.63%)
     1,000,581,626      branches                         #    2.118 G/sec                       (71.81%)
           119,664      branch-misses                    #    0.01% of all branches             (72.02%)
     4,997,131,197      L1-dcache-loads                  #   10.580 G/sec                       (71.95%)
       126,003,932      L1-dcache-load-misses            #    2.52% of all L1-dcache accesses   (71.31%)

       0.469964508 seconds time elapsed

       0.467652000 seconds user
       0.003011000 seconds sys

```

</details>

**Analysis:** The branchless approach eliminates the branch misprediction problem entirely. With random data, it maintains a near-zero miss rate (0.01%), achieves the highest IPC of 6.25, and keeps frontend stalls minimal (0.57%). Execution completes in 0.47 seconds—similar to branching with predictable data, but **8.7x faster** than branching with random data. The branchless code trades conditional jumps for arithmetic operations (bit shifting), which execute predictably regardless of data patterns. This demonstrates that when branches are unpredictable, the overhead of extra arithmetic is far cheaper than pipeline flushes from mispredictions.

### Step 5: Profile Branch Misses

```bash
perfgo test profile -e branch-misses -- ./ -bench=. -benchmem -benchtime=10000x -run=^$
```

<details>
<summary>View flamegraph</summary>

The [flamegraph] shows significantly higher branch miss rates in the `evalBranch` function when processing random data compared to predictable data, while `evalBranchless` maintains consistent low branch miss rates across all data patterns.

</details>

[flamegraph]:https://flamegraph.com/share/41773200-fc6e-11f0-bff4-ee750a85d76a


## Benchmark Results

| Benchmark | ns/op | Branch Miss Rate | Speedup vs Predictable Branch |
|-----------|-------|------------------|-------------------------------|
| `BenchmarkBranchWithPredictable` | 201,602 | 0.00% | 1.00x (baseline) |
| `BenchmarkBranchWithRandom` | 326,069 | 14.76% | **0.62x (1.62x slower)** |
| `BenchmarkBranchlessWithPredictable` | 236,376 | N/A* | 0.85x |
| `BenchmarkBranchlessWithRandom` | 237,159 | 0.01% | **0.85x (consistent)** |

*Branch miss rate not measured for this benchmark, but expected to be <0.01% based on branchless pattern.

## Why Does Branch Prediction Matter?

Branch mispredictions are expensive because:

1. **Pipeline flush**: Modern CPUs speculatively execute ~10-20 instructions ahead; mispredictions discard all speculative work
2. **Opportunity cost**: Stalled cycles could have been spent executing useful instructions
3. **Memory latency**: Pipeline stalls expose memory access latencies that would otherwise be hidden
4. **Compounding effects**: Multiple unpredictable branches multiply the performance impact

## Real-World Applications

Branch prediction optimization is important in:

- **Parsers and compilers**: Processing variable-length tokens or opcodes with unpredictable patterns
- **Network packet processing**: Handling diverse packet types with conditional logic
- **Database query execution**: Processing predicates on unordered data
- **Image/video processing**: Per-pixel operations with conditional logic
- **Cryptography**: Timing-safe implementations that must avoid data-dependent branches

## Best Practices

1. **Profile first**: Use `perf stat` or similar tools to measure actual branch miss rates before optimizing
2. **Consider data patterns**: Branchless code isn't always faster - sorted/predictable data often benefits from branches
3. **Beware of costs**: Branchless techniques may add arithmetic operations; ensure the trade-off is worthwhile
4. **Use compiler hints**: Modern compilers can optimize predictable branches; `[[likely]]`/`[[unlikely]]` attributes can help
5. **Benchmark with realistic data**: Test with data patterns representative of production workloads

## Key Takeaways

1. **Data patterns drive performance**: Predictable branches can be faster than branchless code when the branch predictor succeeds
2. **Branch mispredictions are expensive**: Random branches can be 2-3x slower than branchless alternatives due to pipeline stalls
3. **Branchless code provides consistency**: Performance remains stable regardless of data patterns
4. **Context matters**: Choose the technique based on your data distribution and performance requirements
5. **Measure don't guess**: Always profile with realistic data patterns for your use case
