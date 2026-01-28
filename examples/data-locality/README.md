# Data Layout Example: Array of Structs vs Struct of Arrays

This package demonstrates how data locality affects performance by comparing two approaches to storing person data when calculating average age.

## What is the Problem?

When you only need to access one field from a large struct, the traditional Array of Structs (AoS) layout loads unnecessary data into the CPU cache, wasting memory bandwidth and cache space.

## Array of Structs (AoS) - Traditional Approach

```go
type Person struct {
    Name        string
    Age         int
    Placeholder [64]byte
}

persons := []Person{{Name: "Alice", Age: 30}, {Name: "Bob", Age: 25}}
```

**Memory layout:**
```
[Name(ptr+len)|Age|Placeholder|Name(ptr+len)|Age|Placeholder|...]
 <---     Person 0      ---> <---     Person 1      --->
```

**Note:** In Go, a `string` is a 16-byte header (pointer + length) stored in the struct. The actual string data ("Alice", "Bob", etc.) is stored separately on the heap. So `Name` here represents the string header, not the string content itself.

When calculating average age, the CPU must:
1. Load entire Person struct (~88 bytes: 16-byte string header + 8-byte int + 64-byte array) into cache
2. Extract only the Age field (8 bytes)
3. Discard the unused Name header and Placeholder fields
4. Repeat for next person

**Cache efficiency:** ~9% (8 bytes used / 88 bytes loaded)

## Struct of Arrays (SoA) - Cache-Friendly Approach

```go
type PersonDatabase struct {
    Names        []string
    Ages         []int
    Placeholders [][64]byte
}

db := PersonDatabase{
    Names: []string{"Alice", "Bob"},
    Ages:  []int{30, 25},
}
```

**Memory layout:**
```
Names:        [ptr+len|ptr+len|ptr+len|...]  (16 bytes each, pointing to string data)
Ages:         [30|25|35|...]                  (8 bytes each)
Placeholders: [64B|64B|64B|...]               (64 bytes each)
```

**Note:** The Names slice stores string headers (pointer + length), not the actual string content. The actual string data is stored separately on the heap.

When calculating average age, the CPU:
1. Only loads the Ages array
2. Processes contiguous integers (8 bytes each)
3. Never touches Names or Placeholders arrays

**Cache efficiency:** ~100% (all loaded data is used)

## Running Benchmarks

### Step 1: Run Basic Benchmarks

```bash
go test ./ -bench=. -benchmem -benchtime=50000x -run=^$
```

<details>
<summary>View results</summary>

```
go test ./ -bench=. -benchmem -benchtime=50000x -run=^$
goos: darwin
goarch: arm64
pkg: github.com/perfgo/perfgo/examples/data-locality
cpu: Apple M2 Max
BenchmarkAverageAge_AoS-12    	   50000	     70802 ns/op	       0 B/op	       0 allocs/op
BenchmarkAverageAge_SoA-12    	   50000	     40113 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/perfgo/perfgo/examples/data-locality	5.743s
```

</details>

### Step 2: Collect Performance Stats (Array of Structs)

```bash
perfgo test stat -- ./ -bench=AverageAge_AoS -benchmem -benchtime=50000x -run=^$
```

<details>
<summary>View results</summary>

```
     3,123,971,950      task-clock                       #    1.002 CPUs utilized
               704      context-switches                 #  225.354 /sec
                25      cpu-migrations                   #    8.003 /sec
             3,714      page-faults                      #    1.189 K/sec
    30,141,456,299      instructions                     #    2.06  insn per cycle
                                                  #    0.01  stalled cycles per insn     (71.42%)
    14,663,677,162      cycles                           #    4.694 GHz                         (71.42%)
       367,535,303      stalled-cycles-frontend          #    2.51% frontend cycles idle        (71.49%)
     5,022,150,324      branches                         #    1.608 G/sec                       (71.51%)
         1,514,054      branch-misses                    #    0.03% of all branches             (71.51%)
     5,138,032,020      L1-dcache-loads                  #    1.645 G/sec                       (71.50%)
     5,406,365,301      L1-dcache-load-misses            #  105.22% of all L1-dcache accesses   (71.35%)

       3.118778099 seconds time elapsed

       3.098218000 seconds user
       0.008920000 seconds sys
```

</details>

**Analysis:** The AoS implementation shows poor cache performance:
- **L1 cache miss rate: 105.22%** - This extremely high miss rate indicates that nearly every memory access misses the L1 cache. The counter shows more misses than loads because speculative execution and hardware prefetching can cause additional misses.
- **8.62x more cache misses** compared to SoA (5.4B vs 627M misses)
- **IPC of 2.06** - Moderate instructions per cycle, limited by memory stalls
- The CPU must load entire 80-byte Person structs but only uses 8 bytes (the Age field), wasting ~90% of loaded data

### Step 3: Collect Performance Stats (Struct of Arrays)

```bash
perfgo test stat -- ./ -bench=AverageAge_AoS -benchmem -benchtime=50000x -run=^$
```

<details>
<summary>View results</summary>

```
 Performance counter stats for '/root/.cache/perfgo/repositories/data-locality-5d227d1e/perfgo.test.linux.amd64 -test.bench=AverageAge_SoA -test.benchmem -test.benchtime=50000x -test.run=^$':

     1,110,662,979      task-clock                       #    1.004 CPUs utilized
               384      context-switches                 #  345.739 /sec
                13      cpu-migrations                   #   11.705 /sec
             2,112      page-faults                      #    1.902 K/sec
    20,177,309,014      instructions                     #    3.98  insn per cycle
                                                  #    0.00  stalled cycles per insn     (71.05%)
     5,073,121,188      cycles                           #    4.568 GHz                         (71.27%)
        41,121,874      stalled-cycles-frontend          #    0.81% frontend cycles idle        (71.62%)
     5,004,803,196      branches                         #    4.506 G/sec                       (71.72%)
           235,780      branch-misses                    #    0.00% of all branches             (71.73%)
     5,035,501,416      L1-dcache-loads                  #    4.534 G/sec                       (71.68%)
       627,183,913      L1-dcache-load-misses            #   12.46% of all L1-dcache accesses   (71.35%)

       1.106152733 seconds time elapsed

       1.101687000 seconds user
       0.003974000 seconds sys
```

</details>

**Analysis:** The SoA implementation demonstrates excellent cache performance:
- **L1 cache miss rate: 12.46%** - Significantly better than AoS's 105.22%
- **8.62x fewer cache misses** - Only 627M misses vs 5.4B in AoS
- **IPC of 3.98** - Nearly double the AoS IPC (2.06), showing the CPU can execute instructions much faster without memory stalls
- **2.89x fewer cycles** - 5.1B cycles vs 14.7B in AoS, despite only 1.49x fewer instructions
- Sequential access to the contiguous Ages array enables efficient hardware prefetching and optimal cache line utilization

### Step 4: Profile Cache Misses

```bash
perfgo test profile -e cache-misses -- ./ -bench=. -benchmem -benchtime=50000x -run=^$
```

<details>
<summary>View [flamegraph]</summary>

The [flamegraph] shows a significantly higher cache miss ratein the AoS implementation when accessing the Age field.

</details>

[flamegraph]:https://flamegraph.com/share/fa16ff62-fc61-11f0-be3c-0235fc700989

## Example Results

| Benchmark | ns/op | Speedup |
|-----------|-------|------------|
| `BenchmarkAverageAge_AoS` | 70,802 ns/op | 1x (baseline) |
| `BenchmarkAverageAge_SoA` | 40,113 ns/op | **1.77x faster** |

## Why is SoA Faster?

The SoA approach should be significantly faster because:

1. **Better cache utilization**: Only ages are loaded, not the entire struct
2. **Improved memory bandwidth**: ~10x less data transferred from RAM
3. **More data per cache line**: A 64-byte cache line holds 8 ages (SoA) vs 0-1 person (AoS)
4. **Better prefetching**: Sequential access to contiguous ages is easier to predict

## Real-World Applications

This pattern is common in:

- **Game engines**: Entity Component Systems separate data by component type
- **Databases**: Column-oriented databases use this layout
- **SIMD processing**: Vectorized operations work best on contiguous data
- **Data analytics**: Processing large datasets with selective field access

## Best Practices

1. **Choose layout based on access patterns**: Use AoS when accessing all fields together, SoA for selective field access
2. **Profile before optimizing**: Measure cache misses and memory bandwidth to identify bottlenecks
3. **Consider your data size**: SoA benefits increase with larger datasets
4. **Balance complexity**: SoA adds code complexity - only use it where performance matters
5. **Think about SIMD**: SoA enables better vectorization opportunities

## Key Takeaways

1. **Data layout matters**: How you organize data can have dramatic performance impacts
2. **Access patterns drive design**: If you access all fields together, AoS is fine; if you access fields selectively, SoA wins
3. **Cache is king**: Modern CPU performance is dominated by cache efficiency
4. **Measure your workload**: The best layout depends on your specific access patterns
