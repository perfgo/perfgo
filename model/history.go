package model

import "time"

// HistoryType represents the type of history entry
type HistoryType string

const (
	HistoryTypeTest   HistoryType = "test"
	HistoryTypeAttach HistoryType = "attach"
)

// History represents a single perfgo execution (test or attach)
// It contains common fields shared by all execution types.
type History struct {
	// Unique ID for this execution (16 random bytes, hex encoded)
	ID string `json:"id"`
	// Type of execution (test or attach)
	Type HistoryType `json:"type"`
	// Timestamp when the execution started
	Timestamp time.Time `json:"timestamp"`
	// Command-line arguments (including command name)
	Args []string `json:"args"`
	// Working directory where command was run (relative to repo root)
	WorkDir string `json:"workdir"`
	// Exit code of the execution
	ExitCode int `json:"exit_code"`
	// Duration of execution
	Duration time.Duration `json:"duration"`
	// Git information
	Git *Git `json:"git,omitempty"`
	// Target execution environment
	Target *Target `json:"target,omitempty"`
	// Artifacts generated during this run
	Artifacts []Artifact `json:"artifacts,omitempty"`
	// Perf options used (if any)
	Perf *Perf `json:"perf,omitempty"`

	// Type-specific data (only one should be populated based on Type)
	Test   *TestRun   `json:"test,omitempty"`
	Attach *AttachRun `json:"attach,omitempty"`
}

// Git contains git repository information
type Git struct {
	// Git commit hash at time of execution
	Commit string `json:"commit,omitempty"`
	// Git branch at time of execution
	Branch string `json:"branch,omitempty"`
	// Repository name
	Repo string `json:"repo,omitempty"`
}

// Target contains information about the execution environment
type Target struct {
	// Remote host where execution happened (e.g., "user@host" for SSH, node name for k8s)
	RemoteHost string `json:"remote_host,omitempty"`
	// Operating system of the execution environment
	OS string `json:"os,omitempty"`
	// CPU architecture of the execution environment
	Arch string `json:"arch,omitempty"`
}

// Perf contains performance profiling options that were used
type Perf struct {
	// Record options (for profile mode) - only one of Record, Stat, or C2C should be set
	Record *PerfRecord `json:"record,omitempty"`
	// Stat options (for stat mode) - only one of Record, Stat, or C2C should be set
	Stat *PerfStat `json:"stat,omitempty"`
	// C2C options (for cache-to-cache mode) - only one of Record, Stat, or C2C should be set
	C2C *PerfC2C `json:"c2c,omitempty"`
}

// PerfRecord contains perf record options that were used
type PerfRecord struct {
	// Event to record (e.g., "cycles", "instructions")
	Event string `json:"event,omitempty"`
	// Event period to sample
	Count int `json:"count,omitempty"`
	// Process IDs that were profiled
	PIDs []string `json:"pids,omitempty"`
	// Duration in seconds (for attach mode)
	Duration int `json:"duration,omitempty"`
}

// PerfStat contains perf stat options that were used
type PerfStat struct {
	// Events to measure
	Events []string `json:"events,omitempty"`
	// Process IDs that were measured
	PIDs []string `json:"pids,omitempty"`
	// Duration in seconds (for attach mode)
	Duration int `json:"duration,omitempty"`
	// Whether detailed statistics were enabled
	Detail bool `json:"detail,omitempty"`
}

// PerfC2C contains perf c2c options that were used
type PerfC2C struct {
	// Event to record (e.g., "mem-loads", "mem-stores")
	Event string `json:"event,omitempty"`
	// Event period to sample
	Count int `json:"count,omitempty"`
	// Process IDs that were profiled
	PIDs []string `json:"pids,omitempty"`
	// Duration in seconds (for attach mode)
	Duration int `json:"duration,omitempty"`
	// Report mode used (stdio or tui)
	ReportMode string `json:"report_mode,omitempty"`
	// Whether show-all flag was used
	ShowAll bool `json:"show_all,omitempty"`
}

// TestRun contains test-specific fields
type TestRun struct {
	// Package path that was tested (e.g., ".", "./pkg/foo")
	PackagePath string `json:"package_path,omitempty"`
}

// AttachRun contains attach-specific fields
type AttachRun struct {
	// Kubernetes context used
	KubeContext string `json:"kube_context,omitempty"`
	// Kubernetes namespace
	Namespace string `json:"namespace,omitempty"`
	// Pod name that was attached to
	PodName string `json:"pod_name,omitempty"`
	// Node name that was attached to
	NodeName string `json:"node_name,omitempty"`
}

// ArtifactType identifies the type of artifact
type ArtifactType uint8

const (
	ArtifactTypePprofProfile ArtifactType = iota
	ArtifactTypeTestBinary
	ArtifactTypeAttachBinary
	ArtifactTypeTestOutput
	ArtifactTypePerfStat
	ArtifactTypePerfStatDetailed
	ArtifactTypePerfC2CReport
	ArtifactTypeStdout
	ArtifactTypeStderr
)

// Artifact represents a file generated during execution
type Artifact struct {
	Type ArtifactType `json:"type"`
	Size uint64       `json:"size"`
	File string       `json:"file"` // relative to run dir
}
