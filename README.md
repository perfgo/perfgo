# perfgo

A Go CLI tool for running and profiling Go tests with Linux `perf` integration.

## Features

- **Remote test execution**: Run tests on remote hosts via SSH with automatic cross-compilation
- **perf integration**: Wrap tests with `perf record` to capture hardware performance counters
- **Automatic symbolization**: Converts perf data to pprof format with full binary paths for automatic symbol resolution
- **Test artifact archiving**: Saves profiles, binaries, and git metadata for later analysis
- **Sample merging**: Reduces profile size by merging duplicate stack traces

## Installation

```bash
go install github.com/perfgo/perfgo@latest
```

Or build from source:

```bash
git clone https://github.com/perfgo/perfgo.git
cd perfgo
go build -o perfgo .
```

## Usage

### Local test execution

```bash
# Run tests locally with perf profiling
perfgo test --event cycles:u ./examples/false-sharing

# View the generated profile
go tool pprof perf.pb.gz
```

### Remote test execution

```bash
# Run tests on remote Linux host
perfgo test --remote-host user@server --event cycles:u ./examples/false-sharing

# Keep remote artifacts for debugging
perfgo test --remote-host user@server --keep ./examples/false-sharing
```

### Available perf events

Common events to profile:
- `cycles:u` - CPU cycles (user-space)
- `instructions:u` - Instructions executed
- `cache-misses` - Cache misses
- `L1-dcache-load-misses` - L1 data cache load misses
- `branches` - Branch instructions
- `branch-misses` - Branch mispredictions

See `perf list` for all available events on your system.

## Test Artifact Archiving

When running tests with `--event`, perfgo automatically archives results to `~/perfgo-profiles/`:

```
~/perfgo-profiles/
  └── <repo-name>/
      └── <relative-path>/
          └── <timestamp>-<commit>/
              ├── perf.pb.gz          # pprof profile (paths rewritten to archive)
              ├── <test-binary>       # test binary with symbols
              ├── perf.script         # raw perf script output
              └── git-info.txt        # git metadata
```

**Self-contained profiles**: Binary paths in the pprof profile are automatically rewritten to point to the archived test binary. This means `go tool pprof` works immediately without manual path specification.

**Example:**
```bash
./perfgo test --event cycles:u ./pkg/server

# Creates archive at:
# ~/perfgo-profiles/myproject/pkg/server/20240117-154523-a1b2c3d4/
```

**Later analysis:**
```bash
# Navigate to archived profile
cd ~/perfgo-profiles/myproject/pkg/server/20240117-154523-a1b2c3d4

# Analyze with pprof (automatically finds test binary for symbolization)
go tool pprof perf.pb.gz

# Or use web UI
go tool pprof -http=:8080 perf.pb.gz
```

See [ARCHIVE.md](ARCHIVE.md) for detailed documentation on archive format and management.

## How It Works

1. **Build**: Compiles test binary for target architecture (with `CGO_ENABLED=0` for portability)
2. **Sync**: Uses git to transfer working tree to remote host (if remote execution)
3. **Execute**: Runs tests wrapped with `perf record` (if `--event` specified)
4. **Convert**: Parses `perf script` output and converts to pprof format
5. **Archive**: Saves profile, binary, and git info to `~/perfgo-profiles/` for later analysis

## Examples

See [examples/false-sharing/](examples/false-sharing/) for a complete example demonstrating cache false sharing detection with hardware performance counters.

## Documentation

- [README.md](README.md) - Main documentation (this file)
- [ARCHIVE.md](ARCHIVE.md) - Archive format and management
- [AGENTS.md](AGENTS.md) - Guide for AI agents working with this codebase
- [perfscript/README.md](perfscript/README.md) - Parser documentation
- [examples/false-sharing/README.md](examples/false-sharing/README.md) - False sharing detection example

## Requirements

- Go 1.24+
- Linux `perf` (for profiling)
- SSH access to remote hosts (for remote execution)

## License

See LICENSE file for details.
