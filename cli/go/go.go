package gocmd

// go.go provides utilities for executing Go commands.

import (
	"fmt"
	"os/exec"
	"strings"
)

// List runs 'go list' on a package path and returns the list of packages.
// Returns the packages found (one per line from stdout) and any error.
// If an error occurs, it includes a user-friendly error message.
func List(path string) ([]string, error) {
	cmd := exec.Command("go", "list", path)

	// Capture stdout and stderr separately
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		// Extract the error message from stderr
		errMsg := strings.TrimSpace(stderr.String())

		// Simplify common error messages
		if strings.Contains(errMsg, "no Go files in") {
			return nil, fmt.Errorf("invalid package path %q: directory contains no Go files", path)
		}
		if strings.Contains(errMsg, "is not in std") || strings.Contains(errMsg, "is not in GOROOT") {
			return nil, fmt.Errorf("invalid package path %q: package not found", path)
		}
		if strings.Contains(errMsg, "cannot find package") {
			return nil, fmt.Errorf("invalid package path %q: package not found", path)
		}

		// For other errors, show the first line of the error
		lines := strings.Split(errMsg, "\n")
		if len(lines) > 0 && lines[0] != "" {
			return nil, fmt.Errorf("invalid package path %q: %s", path, lines[0])
		}

		return nil, fmt.Errorf("invalid package path %q: %s", path, err.Error())
	}

	// Parse packages from stdout (one per line)
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return []string{}, nil
	}

	packages := strings.Split(output, "\n")
	return packages, nil
}

// Command creates an exec.Cmd for running a Go command.
// The first argument is the Go subcommand (e.g., "build", "test"), followed by its arguments.
func Command(args ...string) *exec.Cmd {
	return exec.Command("go", args...)
}
