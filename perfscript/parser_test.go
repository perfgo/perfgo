package perfscript

import (
	"strings"
	"testing"

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
	require.Len(t, prof.Sample, 2)
	require.Equal(t, []int64{14, 0}, prof.Sample[0].Value)
	require.Equal(t, []int64{0, 34}, prof.Sample[1].Value)

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

	// Check that mappings were created
	require.Len(t, prof.Mapping, 2)
	require.Equal(t, "[kernel.kallsyms]", prof.Mapping[0].File)
	require.Equal(t, "perfgo.test.linux.amd64", prof.Mapping[1].File)

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
