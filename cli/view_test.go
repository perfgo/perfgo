package cli

import (
	"reflect"
	"testing"
)

func TestRemoveFirstDashDash(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty slice",
			in:   []string{},
			want: []string{},
		},
		{
			name: "starts with --",
			in:   []string{"--", "-http=:8080", "-top"},
			want: []string{"-http=:8080", "-top"},
		},
		{
			name: "no --",
			in:   []string{"-http=:8080", "-top"},
			want: []string{"-http=:8080", "-top"},
		},
		{
			name: "only --",
			in:   []string{"--"},
			want: []string{},
		},
		{
			name: "-- in middle",
			in:   []string{"-top", "--", "-http=:8080"},
			want: []string{"-top", "--", "-http=:8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeFirstDashDash(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeFirstDashDash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseViewArgs(t *testing.T) {
	tests := []struct {
		name          string
		in            []string
		wantID        string
		wantPprofArgs []string
	}{
		{
			name:          "empty args - default to 0",
			in:            []string{},
			wantID:        "0",
			wantPprofArgs: nil,
		},
		{
			name:          "only ID - index 0",
			in:            []string{"0"},
			wantID:        "0",
			wantPprofArgs: []string{},
		},
		{
			name:          "only ID - negative index",
			in:            []string{"-1"},
			wantID:        "-1",
			wantPprofArgs: []string{},
		},
		{
			name:          "only ID - hex string",
			in:            []string{"abc123"},
			wantID:        "abc123",
			wantPprofArgs: []string{},
		},
		{
			name:          "only pprof args",
			in:            []string{"-http=:8080"},
			wantID:        "0",
			wantPprofArgs: []string{"-http=:8080"},
		},
		{
			name:          "ID with pprof args",
			in:            []string{"0", "-http=:8080"},
			wantID:        "0",
			wantPprofArgs: []string{"-http=:8080"},
		},
		{
			name:          "ID with -- separator and pprof args",
			in:            []string{"0", "--", "-http=:8080", "-top"},
			wantID:        "0",
			wantPprofArgs: []string{"-http=:8080", "-top"},
		},
		{
			name:          "negative index with -- and pprof args",
			in:            []string{"-1", "--", "-top"},
			wantID:        "-1",
			wantPprofArgs: []string{"-top"},
		},
		{
			name:          "hex ID with pprof args no separator",
			in:            []string{"abc123", "-list=main"},
			wantID:        "abc123",
			wantPprofArgs: []string{"-list=main"},
		},
		{
			name:          "only -- uses default 0",
			in:            []string{"--", "-http=:8080"},
			wantID:        "0",
			wantPprofArgs: []string{"-http=:8080"},
		},
		{
			name:          "negative index with multiple pprof args",
			in:            []string{"-2", "-http=:8080", "-nodefraction=0.1"},
			wantID:        "-2",
			wantPprofArgs: []string{"-http=:8080", "-nodefraction=0.1"},
		},
		{
			name:          "ID 0 with -- and multiple pprof args",
			in:            []string{"0", "--", "-http=:8080", "-top", "-cum"},
			wantID:        "0",
			wantPprofArgs: []string{"-http=:8080", "-top", "-cum"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotPprofArgs := parseViewArgs(tt.in)
			if gotID != tt.wantID {
				t.Errorf("parseViewArgs() gotID = %v, want %v", gotID, tt.wantID)
			}
			if !reflect.DeepEqual(gotPprofArgs, tt.wantPprofArgs) {
				t.Errorf("parseViewArgs() gotPprofArgs = %v, want %v", gotPprofArgs, tt.wantPprofArgs)
			}
		})
	}
}
