package cli

// This file contains Git integration utilities for retrieving
// repository information.

import (
	"fmt"
	"os/exec"
	"strings"
)

func (a *App) getGitInfo() (commit, branch string, err error) {
	// Get current commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get git commit: %w", err)
	}
	commit = strings.TrimSpace(string(output))

	// Get current branch
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err = cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get git branch: %w", err)
	}
	branch = strings.TrimSpace(string(output))

	return commit, branch, nil
}
