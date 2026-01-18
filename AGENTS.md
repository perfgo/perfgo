# AGENTS.md

This document provides guidance for AI agents working in the perfgo repository.

## Project Overview

**perfgo** is a Go CLI tool for running and profiling Go tests, particularly with support for remote execution and Linux `perf` integration. It converts `perf script` output to pprof format for analysis with standard Go profiling tools.

### Key Capabilities

- Run Go tests locally or on remote hosts via SSH
- Wrap test execution with `perf record` to capture hardware performance counters
- Parse `perf script` output and convert to pprof profiles (`*.pb.gz`)
- Cross-compile test binaries for remote systems (auto-detects OS/architecture)

## Project Structure

```
perfgo/
├── main.go              # Entry point - creates and runs CLI
├── cli/
│   └── cli.go           # CLI implementation (urfave/cli/v2)
│                        # - test command: build, sync, execute tests
│                        # - SSH multiplexing for remote execution
│                        # - perf integration and profile conversion
├── perfscript/
│   ├── parser.go        # Parses perf script output → pprof profile
│   ├── parser_test.go   # Parser tests
│   └── README.md        # Package documentation
└── examples/
    └── false-sharing/   # Example demonstrating false sharing detection
        ├── false_sharing.go      # SharedCounters vs PaddedCounters
        ├── false_sharing_test.go # Unit tests
        ├── benchmark_test.go     # Performance benchmarks
        └── README.md             # Detailed explanation
```

## Commands

### Build

```bash
go build ./...
```

### Run Tests

```bash
# All tests
go test ./...

# Specific package
go test ./perfscript
go test ./examples/false-sharing

# With race detection
go test -race ./...

# Verbose
go test -v ./...
```

### Run Benchmarks

```bash
# All benchmarks in false-sharing example
go test -bench=. ./examples/false-sharing

# With memory stats
go test -bench=. -benchmem ./examples/false-sharing

# Specific benchmark
go test -bench=BenchmarkFalseSharing_8Goroutines ./examples/false-sharing
```

### Run the CLI

```bash
# Build
go build -o perfgo .

# Run tests locally
./perfgo test ./examples/false-sharing

# Run tests on remote host with perf profiling
./perfgo test --remote-host user@host --event cycles:u ./examples/false-sharing

# Keep remote artifacts for debugging
./perfgo test --remote-host user@host --keep ./examples/false-sharing
```

### Test Run History

Every test run is automatically recorded to `.perfgo/history/<id>/` with:
- Unique 16-byte random ID (hex encoded)
- Command-line arguments
- Exit code and duration
- Full stdout/stderr output
- Git commit and branch
- Links to generated artifacts (profiles, etc.)

**List test runs:**
```bash
# Show all test runs
./perfgo list

# Show only test runs (no profiles)
./perfgo list --type runs

# Show only archived profiles
./perfgo list --type profiles

# Filter by path
./perfgo list --path examples/false-sharing

# Limit results
./perfgo list --limit 10
```

**View test output:**
```bash
# Find the test run path from list output
./perfgo list

# View stdout
cat <path>/stdout.txt

# View stderr
cat <path>/stderr.txt

# View full metadata (includes ID, timestamp, args, etc.)
cat <path>/testrun.json
```

**Storage structure:**
```
.perfgo/
├── history/
│   └── <32-char-hex-id>/
│       ├── testrun.json    # Metadata with ID, timestamp, args, etc.
│       ├── stdout.txt      # Standard output
│       └── stderr.txt      # Standard error (if any)
└── <path>/
    └── <timestamp>-<commit>/  # Archived profiles
```

### Test Artifact Archiving

When running tests with `--event` (perf profiling), perfgo automatically archives artifacts to `~/perfgo-profiles/` for later analysis:

**Archive structure:**
```
~/perfgo-profiles/
  └── <repo-name>/
      └── <relative-path>/
          └── <timestamp>-<commit>/
              ├── perf.pb.gz          # pprof profile
              ├── perfgo.test.*       # test binary with symbols
              ├── perf.script         # raw perf script output
              └── git-info.txt        # git metadata (commit, branch, timestamp)
```

**Example:**
```bash
./perfgo test --event cycles:u ./examples/false-sharing

# Creates:
# ~/perfgo-profiles/perfgo/examples/false-sharing/20240117-154523-a1b2c3d4/
#   ├── perf.pb.gz
#   ├── perfgo.test.linux.amd64
#   ├── perf.script
#   └── git-info.txt
```

**Later analysis:**
```bash
cd ~/perfgo-profiles/perfgo/examples/false-sharing/20240117-154523-a1b2c3d4
go tool pprof perf.pb.gz  # Automatically finds test binary for symbolization
```

## Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/google/pprof` | Profile data structures and writing |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/stretchr/testify` | Test assertions (`require`, `assert`) |
| `github.com/urfave/cli/v2` | CLI framework |

## Code Patterns

### Testing

- Uses `github.com/stretchr/testify` for assertions
- Table-driven tests where appropriate
- Benchmarks use `testing.B` with `b.RunParallel` or explicit goroutine management

Example test pattern:
```go
func TestFoo(t *testing.T) {
    result := DoSomething()
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Error Handling

- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return errors to caller rather than logging and continuing
- Use `zerolog` for structured logging with context fields

### Logging

```go
a.logger.Info().
    Str("host", host).
    Str("path", path).
    Msg("Description of action")

a.logger.Debug().Strs("args", args).Msg("Debug message")
```

### SSH Remote Execution

- Uses SSH multiplexing (`ControlMaster`, `ControlPath`) for efficient connection reuse
- Control sockets stored in `$XDG_RUNTIME_DIR/perfgo` or `~/.config/perfgo`
- Hashes host names to keep socket paths short (Unix socket path length limits)

### Profile Parsing

The `perfscript.Parser` converts `perf script` output to pprof:
1. Create parser: `parser := perfscript.New()`
2. Parse: `prof, err := parser.Parse(reader)`
3. Write: `prof.Write(file)`

**Sample Merging**: The parser automatically merges samples with identical stacktraces, accumulating counts for each event type. This significantly reduces profile size when the same stack appears multiple times.

**Full Paths**: Binary paths from perf script output are preserved in full (e.g., `/path/to/binary`) so pprof can automatically find and symbolize the binaries.

### Test Artifact Archiving

After successful perf profiling, `archiveTestArtifacts()` saves artifacts to `~/perfgo-profiles/`:
- Gets git commit, branch, and repository info
- Creates timestamped directory: `<repo>/<rel-path>/<timestamp>-<commit>`
- Copies test binary to archive
- Reads profile, rewrites binary paths to archived location via `rewriteProfilePaths()`
- Writes updated profile, `perf.script`, and `git-info.txt`
- Non-fatal: logs warning if archiving fails, doesn't fail the test run
- Only triggered when `--event` flag is used

**Path Rewriting**: The profile's `Mapping.File` fields are updated to point to the archived binary path instead of cache/remote paths. This ensures pprof can find the binary for symbolization without manual intervention.

## File Naming Conventions

- Go files: `snake_case.go`
- Test files: `*_test.go`
- Benchmark files: `benchmark_test.go` or included in `*_test.go`

## Important Notes

### perf Script Format

The parser expects standard `perf script` output:
```
program 12345 [000] 123.456789: event:count
	address function+0xoffset (/path/to/binary)
	address function+0xoffset (/path/to/binary)
```

### Cross-Compilation

Test binaries are built with `CGO_ENABLED=0` for easier cross-compilation to remote hosts.

### Cache Line Padding

When working with concurrent data structures, be aware of false sharing. The `examples/false-sharing` package demonstrates proper padding:
```go
type PaddedCounter struct {
    value int64
    _     [56]byte  // Padding to 64-byte cache line
}
```

## Go Version

Requires Go 1.24+ (see `go.mod`).
