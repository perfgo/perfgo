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

## Benchmarks

The example includes four benchmarks testing both approaches with different data patterns:

- **BenchmarkBranchWithPredictable**: Sorted data (first half negative, second half positive) - branch predictor performs well
- **BenchmarkBranchWithRandom**: Random data - branch predictor struggles with unpredictable pattern
- **BenchmarkBranchlessWithPredictable**: Branchless on sorted data
- **BenchmarkBranchlessWithRandom**: Branchless on random data

## Expected Results

- **Predictable data**: Branching code should perform similarly or better than branchless
- **Random data**: Branchless code should significantly outperform branching due to eliminating mispredictions

## Running the Benchmarks

```bash
go test -bench=. -benchmem
```

## Key Takeaways

1. **Context matters**: Branchless code isn't always faster - it depends on data patterns
2. **Branch mispredictions are expensive**: Random branches can be 2-4x slower than branchless alternatives
3. **Predictable patterns are fast**: Well-predicted branches often outperform branchless code
4. **Measure don't guess**: Always benchmark with realistic data patterns for your use case
