# PerfGo

![PerfGo Logo](https://raw.githubusercontent.com/perfgo/perfgo/refs/heads/assets/assets/logo.png)

> Bring hardware performance insights to the Go ecosystem

A tool to manage benchmarking and performance measurements of your Go projects using Linux's `perf` command. PerfGo leverages CPU Performance Monitoring Unit (PMU) counters to provide deep insights into cache behavior, branch prediction, and other microarchitectural events.

![Demo](https://raw.githubusercontent.com/perfgo/perfgo/refs/heads/assets/assets/perfgo-demo-video.gif)

## Execution Model

PerfGo supports two main execution modes:

### Test Mode

Builds and executes Go test/benchmark packages with optional performance monitoring. Supports two executors:

- **Local executor** - Run tests on the local Linux system
- **Remote executor** - Build locally and execute on a remote system via SSH

Enable performance collection by adding the `stat`, `profile`, or `cache-to-cache` subcommands to your test runs.

```bash
# Local execution - collect statistics
perfgo test stat -- ./examples/false-sharing -bench=. -benchmem -benchtime=10000000x -run=^$

# Local execution - generate profile
perfgo test profile -e cache-loads -- ./examples/false-sharing -bench=. -benchmem -benchtime=10000000x -run=^$

# Remote execution - run on Linux server over SSH
perfgo test stat --remote-host user@remote.example.com -- ./package -bench=.

# Cache-to-cache analysis for false sharing detection
perfgo test cache-to-cache -- ./examples/false-sharing -bench=NoPadding -benchtime=10s -run=^$
```

### Attach Mode

Collect performance data from a running Kubernetes pod by deploying a sidecar container that attaches to the target process. Supports all three analysis modes (`stat`, `profile`, `cache-to-cache`).

```bash
# Collect statistics from a pod for 10 seconds
perfgo attach stat --pod my-app-pod --namespace production --duration 10

# Profile cache misses on a specific node
perfgo attach profile --node worker-01 --event cache-misses --duration 30

# Detect false sharing in a running pod
perfgo attach cache-to-cache --pod my-app-pod -n production --duration 15

# Open an interactive shell for manual perf commands
perfgo attach shell --pod my-app-pod --namespace production
```

## Collection Modes

PerfGo supports three analysis modes:

- **stat** - Collect hardware counter statistics for your Go tests
- **profile** - Generate flame graphs showing where PMU events occur in your code
- **cache-to-cache** - Analyze cache line transfers between CPU cores

## Historical Data

PerfGo stores all benchmark results and performance data in the `.perfgo` directory at your project root. This allows you to revisit and compare previous benchmark runs:

- `perfgo list` - View all stored benchmark runs
- `perfgo view` - Open and analyze a specific benchmark result

## Typical Workflow

A recommended approach for performance investigation after you notice CPU contention in your service benchmark:

1. **Establish baseline** - Run `stat` to get a hardware performance overview
1. **Identify bottlenecks** - Look for high cache miss rates, branch mispredictions, or low IPC (instructions per cycle)
1. **Profile hot spots** - Use `perfgo test profile -e <event>` to see which source code lines contribute to the bottleneck
1. **Investigate cache issues** - If cache misses are high, run `perfgo test cache-to-cache` to detect false sharing or contention
1. **Compare optimizations** - Re-run the same tests after code changes to validate improvements

## About PMU Counters

Performance Monitoring Unit (PMU) counters are hardware registers in modern CPUs that track microarchitectural events like cache misses, branch mispredictions, and instruction execution. These counters provide direct insight into how your code interacts with the CPU's internal architecture.

**Accuracy**: PMU counters offer high-precision measurements with minimal overhead. However, some events may be subject to:
- **Sampling skid** - A small delay between when an event occurs and when it's recorded
- **Multiplexing** - On systems with more events than physical counters, events are time-shared
- **Approximation** - Some complex events are estimated rather than directly counted

Despite these limitations, PMU counters remain the most accurate way to understand hardware-level performance characteristics of your code.

### Event specification

PerfGo allows you to specify which PMU events to monitor. Common events include:

**CPU Performance:**
- `cycles` - CPU cycles elapsed
- `instructions` - Instructions executed
- `branches` - Branch instructions
- `branch-misses` - Mispredicted branches

**Cache Events:**
- `cache-references` - Total cache accesses
- `cache-misses` - Cache misses at all levels
- `L1-dcache-loads` - L1 data cache loads
- `L1-dcache-load-misses` - L1 data cache load misses
- `LLC-loads` - Last Level Cache loads
- `LLC-load-misses` - Last Level Cache load misses

**Memory Access:**
- `dTLB-loads` - Data TLB loads
- `dTLB-load-misses` - Data TLB load misses

**Event Modifiers:**

Events can be refined with suffix modifiers:
- `:u` - Count only user-space events
- `:k` - Count only kernel-space events
- `:p` - Set precise event sampling (PEBS on Intel, IBS on AMD)

Example: `cache-misses:u` counts only user-space cache misses.

**Raw Hardware Events:**

You can also specify raw PMU events using hexadecimal values from your CPU vendor's documentation:

```bash
# Intel: Monitor event 0x2e (LLC references), umask 0x41
perfgo test profile -e r412e -- ./package -bench=.

# ARM: Monitor event 0x40 L1_reads 0x41 L1_writes 
perfgo test profile -e r0040,r0041 -- ./package -bench=.

# View with perf list to find available events
perf list
```

**CPU Vendor Documentation:**

- [Intel 64 and IA-32 Architectures Software Developer Manuals](https://www.intel.com/content/www/us/en/developer/articles/technical/intel-sdm.html) - Volume 3B, Chapter 21 contains PMU event listings
- [AMD Processor Programming Reference](https://docs.amd.com/search/all?query=Processor+Programming+Reference+(PPR)+&content-lang=en-US) - Search for "PPR" for your specific CPU family
- [ARM PMU Event Specifications](https://developer.arm.com/documentation/#f-navigationhierarchiescontenttype=Technical%20Reference%20Manual&numberOfResults=48) - Look for "Technical Reference Manual" of your CPU, then "Performance Monitoring Unit" > "PMU Events"


## Concete examples

PerfGo includes three comprehensive examples demonstrating different performance analysis techniques:

### 1. [False Sharing](examples/false-sharing/)
Demonstrates how cache line contention between CPU cores causes dramatic performance degradation. Shows how cache-to-cache analysis can pinpoint false sharing issues, and how padding can provide a 3.4x speedup.

### 2. [Branch Prediction](examples/branch-prediction/)
Explores the impact of branch predictability on CPU performance. Compares predictable vs. random branching patterns and demonstrates branchless alternatives, showing how branch misses can reduce IPC from 5.28 to 0.70.

### 3. [Data Locality](examples/data-locality/)
Compares Array of Structs (AoS) vs. Struct of Arrays (SoA) memory layouts. Demonstrates how data layout affects cache efficiency, with SoA providing 1.77x better performance and 8.62x fewer cache misses.

Each example includes step-by-step workflows using `perfgo test stat`, `perfgo test profile`, and `perfgo test cache-to-cache` commands.

## Requirements

**General:**
- Go
- LLVM binutils (llvm-symbolizer and llvm-objdump)

**Test mode - Local executor:**
- Linux system with `perf` installed

**Test mode - Remote executor:**
- SSH client (local)
- Linux system with `perf` and SSH server (remote)

**Attach mode:**
- kubectl
- Access to a Kubernetes cluster with appropriate permissions

## License

[Apache 2.0](LICENSE)

## Acknowledgements

PerfGo builds upon the powerful Linux `perf` subsystem and the Go ecosystem's excellent tooling.

- The Linux perf team for building the foundation this tool relies on
- The Go team for excellent profiling and tooling infrastructure

This project was developed with significant assistance from AI agents. If you notice any issues, misattributions, or concerns about the workflow, please reach out via GitHub Issues/Pull Requests.

## Learn more about this

- [Linux perf Wiki](https://perf.wiki.kernel.org/) - Comprehensive documentation on perf
- [Intel 64 and IA-32 Architectures Developer Manuals](https://www.intel.com/content/www/us/en/developer/articles/technical/intel-sdm.html) - Deep dive into x86 PMU events
- [Brendan Gregg's perf Examples](https://www.brendangregg.com/perf.html) - Practical perf usage patterns
- [Andi Kleen's pmu-tools](https://github.com/andikleen/pmu-tools) - Advanced PMU analysis tools

