# Data Layout Example: Array of Structs vs Struct of Arrays

This example demonstrates how data layout affects performance by comparing two approaches to storing person data when calculating average age.

## The Problem

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
[Name|Age|Placeholder|Name|Age|Placeholder|Name|Age|Placeholder|...]
 <---  Person 0  ---> <---  Person 1  ---> <---  Person 2  --->
```

When calculating average age, the CPU must:
1. Load entire Person struct (~80 bytes) into cache
2. Extract only the Age field (8 bytes)
3. Discard the unused Name and Placeholder fields
4. Repeat for next person

**Cache efficiency:** ~10% (8 bytes used / 80 bytes loaded)

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
Names:        [Alice|Bob|Charlie|...]
Ages:         [30|25|35|...]
Placeholders: [64B|64B|64B|...]
```

When calculating average age, the CPU:
1. Only loads the Ages array
2. Processes contiguous integers
3. Never touches Names or Placeholders

**Cache efficiency:** ~100% (all loaded data is used)

## Benchmarks

- **BenchmarkAverageAge_AoS**: Calculate average with Array of Structs
- **BenchmarkAverageAge_SoA**: Calculate average with Struct of Arrays

## Expected Results

The SoA approach should be significantly faster because:

1. **Better cache utilization**: Only ages are loaded, not the entire struct
2. **Improved memory bandwidth**: ~10x less data transferred from RAM
3. **More data per cache line**: A 64-byte cache line holds 8 ages (SoA) vs 0-1 person (AoS)
4. **Better prefetching**: Sequential access to contiguous ages is easier to predict

Typical performance improvement: 5-10x faster for SoA

## Running the Benchmarks

```bash
go test -bench=. -benchmem
```

## Real-World Applications

This pattern is common in:

- **Game engines**: Entity Component Systems separate data by component type
- **Databases**: Column-oriented databases use this layout
- **SIMD processing**: Vectorized operations work best on contiguous data
- **Data analytics**: Processing large datasets with selective field access

## Key Takeaways

1. **Data layout matters**: How you organize data can have dramatic performance impacts
2. **Access patterns drive design**: If you access all fields together, AoS is fine; if you access fields selectively, SoA wins
3. **Cache is king**: Modern CPU performance is dominated by cache efficiency
4. **Measure your workload**: The best layout depends on your specific access patterns
