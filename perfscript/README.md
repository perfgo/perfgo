# perfscript

Package perfscript provides a parser for Linux perf script output that generates pprof profiles.

## Overview

The perfscript package parses the text output from `perf script` command and converts it into a standard pprof profile format. This allows you to use the full suite of Go profiling tools (`go tool pprof`, `pprof` web UI, etc.) to analyze Linux perf data.

## Usage

```go
import (
    "os"
    "strings"
    "github.com/perfgo/perfgo/perfscript"
)

// Create a new parser
parser := perfscript.New()

// Parse perf script output from a string
scriptOutput := "..." // your perf script output
profile, err := parser.Parse(strings.NewReader(scriptOutput))
if err != nil {
    log.Fatal(err)
}

// Or parse from a file
file, err := os.Open("perf.script")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

profile, err = parser.Parse(file)
if err != nil {
    log.Fatal(err)
}

// Write the profile to a file
outFile, err := os.Create("perf.pb.gz")
if err != nil {
    log.Fatal(err)
}
defer outFile.Close()

if err := profile.Write(outFile); err != nil {
    log.Fatal(err)
}

// Now you can analyze with go tool pprof:
// $ go tool pprof perf.pb.gz
// $ go tool pprof -http=:8080 perf.pb.gz
```

## Output Format

The parser generates a standard pprof profile (`*profile.Profile` from `github.com/google/pprof/profile`). This profile includes:

- **Samples**: Each stack trace captured by perf, with full call chain
- **Functions**: Unique function names extracted from stack frames
- **Locations**: Code locations with addresses and function references
- **Mappings**: Binary/library files where code is located
- **Labels**: Event types (e.g., `cycles:u`, `instructions`) as sample labels

The profile is compatible with all standard pprof tools and can be analyzed using:
- `go tool pprof` (command line)
- `go tool pprof -http=:8080` (web UI with flame graphs)
- Third-party tools like Grafana, pprof-rs, etc.

## Perf Script Format

The parser expects standard `perf script` output format:

```
program 12345 [000] 123.456789: cycles:u:
	ffffffffa1234567 runtime.mallocgc+0x42 (/path/to/binary)
	ffffffffa2345678 main.BenchmarkTest+0x89 (/path/to/binary)
```

Each sample consists of:
1. Header line with timestamp and event type
2. Stack trace with function names and addresses

## Features

- **Full stack trace preservation**: Captures complete call chains, not just individual functions
- **Sample merging**: Automatically merges samples with identical stacktraces to reduce profile size
- **Full binary paths**: Preserves absolute paths to binaries for automatic symbolization by pprof
- **Standard pprof format**: Output is compatible with all pprof tools
- **Event labels**: Preserves event types (cycles, instructions, etc.) as sample labels
- **Offset removal**: Function offsets (e.g., `+0x42`) are automatically stripped for cleaner output
- **Binary mappings**: Tracks which binary/library each function belongs to with full paths
- **Address information**: Preserves memory addresses for detailed analysis
- **Streaming parser**: Uses `io.Reader` for memory-efficient processing of large files

## Example Workflow

Here's a complete workflow from capturing perf data to analyzing with pprof:

```bash
# 1. Record performance data with Linux perf
perf record -e cycles:u -g ./my-go-program

# 2. Convert to perf script format
perf script > perf.script

# 3. Use perfscript to convert to pprof format
# (Or use the perfgo CLI tool which does this automatically)

# 4. Analyze with pprof
go tool pprof perf.pb.gz

# Or launch the web UI
go tool pprof -http=:8080 perf.pb.gz
```

The pprof web UI provides:
- Flame graphs for visualizing call stacks
- Top functions by sample count
- Call graph visualization
- Source code annotation (if source is available)
- Comparative analysis between profiles

## Testing

Run tests with:

```bash
go test ./perfscript
```

Run benchmarks with:

```bash
go test -bench=. ./perfscript
```

