package cli

import "testing"

func TestBuildAttachShellCommand(t *testing.T) {
	tests := []struct {
		name string
		pids []string
		want string
	}{
		{
			name: "no PIDs",
			want: "PERF_PID_ARGS='' /bin/bash",
		},
		{
			name: "one PID",
			pids: []string{"1"},
			want: "PERF_PID_ARGS='-p 1' /bin/bash",
		},
		{
			name: "multiple PIDs",
			pids: []string{"1", "2", "3"},
			want: "PERF_PID_ARGS='-p 1 -p 2 -p 3' /bin/bash",
		},
		{
			name: "preserves PID order",
			pids: []string{"10", "2"},
			want: "PERF_PID_ARGS='-p 10 -p 2' /bin/bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAttachShellCommand(tt.pids); got != tt.want {
				t.Fatalf("buildAttachShellCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
