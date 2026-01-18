package model

import "time"

// TestRun represents a single test execution
type TestRun struct {
	// Unique ID for this test run (16 random bytes, hex encoded)
	ID string `json:"id"`
	// Timestamp when the test was executed
	Timestamp time.Time `json:"timestamp"`
	// Command-line arguments (including command name)
	Args []string `json:"args"`
	// Working directory where test was run (relative to repo root)
	WorkDir string `json:"workdir"`
	// Exit code of the test execution
	ExitCode int `json:"exit_code"`
	// Duration of test execution
	Duration time.Duration `json:"duration"`
	// Git commit hash at time of execution
	Commit string `json:"commit,omitempty"`
	// Git branch at time of execution
	Branch string `json:"branch,omitempty"`
	// Repository name
	Repo string `json:"repo,omitempty"`
	// Remote host where test was executed (e.g., "user@host")
	RemoteHost string `json:"remote_host,omitempty"`
	// Operating system of the test execution environment
	OS string `json:"os,omitempty"`
	// CPU architecture of the test execution environment
	Arch string `json:"arch,omitempty"`
	// Standard output file name (relative to run dir)
	StdoutFile string `json:"stdout_file,omitempty"`
	// Standard error file name (relative to run dir)
	StderrFile string `json:"stderr_file,omitempty"`
	// Artifacts generated during this run
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

type ArtifactType uint8

const (
	ArtifactTypePprofProfile ArtifactType = iota
	ArtifactTypeTestBinary
)

type Artifact struct {
	Type ArtifactType `json:"type"`
	Size uint64       `json:"size"`
	File string       `json:"file"` // relative to run dir
}
