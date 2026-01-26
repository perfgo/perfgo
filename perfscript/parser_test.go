package perfscript

import (
	"strings"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse(t *testing.T) {
	// Example perf script output
	output := `perfgo.test.lin  223225 7187035.622637:         14       L1-dcache-loads:
        ffffffffb27cac01 syscall_exit_to_user_mode+0x51 ([kernel.kallsyms])
        ffffffffb27c5074 do_syscall_64+0xc4 ([kernel.kallsyms])
        ffffffffb2800130 entry_SYSCALL_64_after_hwframe+0x77 ([kernel.kallsyms])
                  483ba3 runtime.futex.abi0+0x23 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  4172a7 runtime.notesleep+0x87 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44a133 runtime.stoplockedm+0x73 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44c4fa runtime.schedule+0x3a (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44c9e5 runtime.park_m+0x285 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  47feae runtime.mcall+0x4e (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  479b8e runtime.gopark+0xce (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  411de5 runtime.chanrecv+0x445 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  411992 runtime.chanrecv2+0x12 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  9ddba9 github.com/prometheus/client_golang/prometheus.(*Registry).Register+0x249 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.tes>
                  9de92e github.com/prometheus/client_golang/prometheus.(*Registry).MustRegister+0x4e (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.>
                  ced74b github.com/prometheus/client_golang/prometheus/promauto.Factory.NewCounter+0xeb (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perf>
                  f8a958 github.com/grafana/pyroscope/pkg/util.init+0xb8 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  453798 runtime.doInit1+0xd8 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  444bc5 runtime.main+0x345 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  481d61 runtime.goexit.abi0+0x1 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)

perfgo.test.lin  223225 7187035.622644:         34 L1-dcache-load-misses:
        ffffffffb27cac01 syscall_exit_to_user_mode+0x51 ([kernel.kallsyms])
        ffffffffb27c5074 do_syscall_64+0xc4 ([kernel.kallsyms])
        ffffffffb2800130 entry_SYSCALL_64_after_hwframe+0x77 ([kernel.kallsyms])
                  483ba3 runtime.futex.abi0+0x23 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  4172a7 runtime.notesleep+0x87 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44a133 runtime.stoplockedm+0x73 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44c4fa runtime.schedule+0x3a (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  44c9e5 runtime.park_m+0x285 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  47feae runtime.mcall+0x4e (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  479b8e runtime.gopark+0xce (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  411de5 runtime.chanrecv+0x445 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  411992 runtime.chanrecv2+0x12 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  9ddba9 github.com/prometheus/client_golang/prometheus.(*Registry).Register+0x249 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.tes>
                  9de92e github.com/prometheus/client_golang/prometheus.(*Registry).MustRegister+0x4e (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.>
                  ced74b github.com/prometheus/client_golang/prometheus/promauto.Factory.NewCounter+0xeb (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perf>
                  f8a958 github.com/grafana/pyroscope/pkg/util.init+0xb8 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  453798 runtime.doInit1+0xd8 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  444bc5 runtime.main+0x345 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
                  481d61 runtime.goexit.abi0+0x1 (/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64)
`

	parser := New()
	prof, err := parser.Parse(strings.NewReader(output))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check sample types
	require.Len(t, prof.SampleType, 2)
	require.Equal(t, "L1-dcache-loads", prof.SampleType[0].Type)
	require.Equal(t, "count", prof.SampleType[0].Unit)
	require.Equal(t, "L1-dcache-load-misses", prof.SampleType[1].Type)
	require.Equal(t, "count", prof.SampleType[1].Unit)
	
	// With merging, samples with identical stacks should be combined
	require.Len(t, prof.Sample, 1, "Identical stacks should be merged into one sample")
	require.Equal(t, []int64{14, 34}, prof.Sample[0].Value, "Merged sample should have both event counts")

	// Check that all samples have locations
	for i, sample := range prof.Sample {
		if len(sample.Location) == 0 {
			t.Errorf("Sample %d has no locations", i)
		}
	}

	// Check functions were created
	if len(prof.Function) == 0 {
		t.Error("Expected functions, got none")
	}

	// Verify runtime.mallocgc function exists
	foundDoInit1 := false
	for _, fn := range prof.Function {
		if fn.Name == "runtime.doInit1" {
			require.Equal(t, "", fn.Filename)
			foundDoInit1 = true
			break
		}
	}
	require.True(t, foundDoInit1)

	// Check that mappings were created with full paths
	require.Len(t, prof.Mapping, 2)
	require.Equal(t, "[kernel.kallsyms]", prof.Mapping[0].File)
	require.Equal(t, "/root/.cache/perfgo/repositories/pyroscope-5005b7d6/worktree/perfgo.test.linux.amd64", prof.Mapping[1].File)

	require.NoError(t, prof.CheckValid())
}

func TestParser_ParseEmpty(t *testing.T) {
	parser := New()
	prof, err := parser.Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(prof.Sample) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(prof.Sample))
	}

	if len(prof.Function) != 0 {
		t.Errorf("Expected 0 functions, got %d", len(prof.Function))
	}
}

func TestParser_MergeDuplicateStacks(t *testing.T) {
	// Test that samples with identical stacktraces are merged
	output := `program 12345 [000] 123.456789:          1 cycles:u:
	ffffffffa1234567 function_a+0x10 (/path/to/binary)
	ffffffffa2345678 function_b+0x20 (/path/to/binary)

program 12345 [000] 123.456790:          1 cycles:u:
	ffffffffa1234567 function_a+0x10 (/path/to/binary)
	ffffffffa2345678 function_b+0x20 (/path/to/binary)

program 12345 [000] 123.456791:          1 cycles:u:
	ffffffffa9999999 function_c+0x30 (/path/to/binary)
`

	parser := New()
	prof, err := parser.Parse(strings.NewReader(output))
	require.NoError(t, err)

	// Should have only 2 unique samples (first two have same stack)
	require.Len(t, prof.Sample, 2, "Expected 2 unique samples after merging duplicates")

	// Find the merged sample
	var mergedSample *profile.Sample
	for _, sample := range prof.Sample {
		if len(sample.Location) == 2 {
			if sample.Location[0].Line[0].Function.Name == "function_a" {
				mergedSample = sample
				break
			}
		}
	}

	require.NotNil(t, mergedSample, "Should find the merged sample")
	// The merged sample should have count = 2 (1+1 from the two identical stacks)
	require.Equal(t, int64(2), mergedSample.Value[0], "Merged sample should have combined count")
}

func TestParser_MergeDifferentEvents(t *testing.T) {
	// Test that samples with same stack but different events are merged correctly
	output := `program 12345 [000] 123.456789: 100 cycles:u:
	ffffffffa1234567 function_a+0x10 (/path/to/binary)

program 12345 [000] 123.456790: 50 instructions:u:
	ffffffffa1234567 function_a+0x10 (/path/to/binary)

program 12345 [000] 123.456791: 75 cycles:u:
	ffffffffa1234567 function_a+0x10 (/path/to/binary)
`

	parser := New()
	prof, err := parser.Parse(strings.NewReader(output))
	require.NoError(t, err)

	// Should have 1 sample with both event types
	require.Len(t, prof.Sample, 1, "Expected 1 sample")
	require.Len(t, prof.SampleType, 2, "Expected 2 event types")

	sample := prof.Sample[0]
	require.Len(t, sample.Value, 2, "Sample should have 2 values")

	// Verify the values are summed correctly
	// cycles:u should be 100 + 75 = 175
	// instructions:u should be 50
	cyclesIdx := -1
	instructionsIdx := -1
	for i, st := range prof.SampleType {
		if st.Type == "cycles:u" {
			cyclesIdx = i
		} else if st.Type == "instructions:u" {
			instructionsIdx = i
		}
	}

	require.NotEqual(t, -1, cyclesIdx, "Should find cycles:u")
	require.NotEqual(t, -1, instructionsIdx, "Should find instructions:u")
	require.Equal(t, int64(175), sample.Value[cyclesIdx], "cycles:u should be summed")
	require.Equal(t, int64(50), sample.Value[instructionsIdx], "instructions:u should be correct")
}

func TestParser_PreserveFullPaths(t *testing.T) {
	// Test that full binary paths are preserved for pprof symbolization
	output := `program 12345 [000] 123.456789:          1 cycles:u:
	ffffffffa1234567 function_a+0x10 (/usr/local/bin/myapp)
	ffffffffa2345678 function_b+0x20 (/home/user/code/project/binary)
	ffffffffa3456789 kernel_func+0x30 ([kernel.kallsyms])
`

	parser := New()
	prof, err := parser.Parse(strings.NewReader(output))
	require.NoError(t, err)

	// Should have 3 mappings with full paths
	require.Len(t, prof.Mapping, 3, "Expected 3 mappings")

	// Verify full paths are preserved
	mappingFiles := make(map[string]bool)
	for _, m := range prof.Mapping {
		mappingFiles[m.File] = true
	}

	require.True(t, mappingFiles["/usr/local/bin/myapp"], "Should preserve full path to myapp")
	require.True(t, mappingFiles["/home/user/code/project/binary"], "Should preserve full path to binary")
	require.True(t, mappingFiles["[kernel.kallsyms]"], "Should preserve kernel mapping")

	// Ensure no paths were stripped to basename
	require.False(t, mappingFiles["myapp"], "Should not strip path to basename")
	require.False(t, mappingFiles["binary"], "Should not strip path to basename")
}

func TestParser_MappingRanges(t *testing.T) {
	// Test that mapping Start and Limit are set to allow all addresses
	// We use Start=0 and Limit=max_uint64 to avoid interfering with pprof's
	// symbol resolution while still passing address validation.
	output := `program 12345 [000] 123.456789:          1 cycles:u:
	               52ab5a function_a+0x10 (/path/to/binary)
	               600123 function_b+0x20 (/path/to/binary)
	               400456 function_c+0x30 (/another/binary)
`

	parser := New()
	prof, err := parser.Parse(strings.NewReader(output))
	require.NoError(t, err)

	// Should have 2 mappings
	require.Len(t, prof.Mapping, 2, "Expected 2 mappings")

	// Check that each mapping has a wide range [0, max_uint64]
	for _, m := range prof.Mapping {
		require.Equal(t, uint64(0), m.Start, "Mapping Start should be 0 for %s", m.File)
		require.Equal(t, ^uint64(0), m.Limit, "Mapping Limit should be max_uint64 for %s", m.File)
		require.Greater(t, m.Limit, m.Start, "Mapping Limit should be greater than Start for %s", m.File)

		// Verify all locations with this mapping have addresses within the range
		for _, loc := range prof.Location {
			if loc.Mapping != nil && loc.Mapping.ID == m.ID && loc.Address != 0 {
				require.GreaterOrEqual(t, loc.Address, m.Start,
					"Location address 0x%x should be >= mapping Start 0x%x for %s",
					loc.Address, m.Start, m.File)
				require.Less(t, loc.Address, m.Limit,
					"Location address 0x%x should be < mapping Limit 0x%x for %s",
					loc.Address, m.Limit, m.File)
			}
		}
	}

	// Profile should pass validation
	require.NoError(t, prof.CheckValid())
}
