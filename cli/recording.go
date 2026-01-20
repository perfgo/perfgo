package cli

// This file contains test run recording functionality for saving
// test run metadata and artifacts to the history directory.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/perfgo/perfgo/model"
)

func (a *App) recordTestRun(testRun *model.TestRun, testBinaryPath string) error {
	// Get repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoName := filepath.Base(repoRoot)

	// Store repo name in testRun
	testRun.Repo = repoName

	// Get relative path from repo root
	relPath := "."
	if testRun.WorkDir != "" {
		if rel, err := filepath.Rel(repoRoot, testRun.WorkDir); err == nil {
			relPath = rel
		}
	}

	// Update WorkDir to be relative to repo root
	testRun.WorkDir = relPath

	// Create directory in .perfgo/history/<timestamp>-<commit>-<id>
	timestamp := testRun.Timestamp.Format("20060102-150405")
	shortCommit := testRun.Commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	shortID := testRun.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	runName := fmt.Sprintf("%s-%s-%s", timestamp, shortCommit, shortID)
	runDir := filepath.Join(repoRoot, ".perfgo", "history", runName)

	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Write stdout to file if present
	if testRun.StdoutFile != "" {
		stdoutPath := filepath.Join(runDir, "stdout.txt")
		if err := os.WriteFile(stdoutPath, []byte(testRun.StdoutFile), 0644); err != nil {
			return fmt.Errorf("failed to write stdout: %w", err)
		}
		testRun.StdoutFile = "stdout.txt" // Store relative filename
	}

	// Write stderr to file if present
	if testRun.StderrFile != "" {
		stderrPath := filepath.Join(runDir, "stderr.txt")
		if err := os.WriteFile(stderrPath, []byte(testRun.StderrFile), 0644); err != nil {
			return fmt.Errorf("failed to write stderr: %w", err)
		}
		testRun.StderrFile = "stderr.txt" // Store relative filename
	}

	// Archive artifacts if they exist
	if err := a.saveArtifacts(runDir, testRun, testBinaryPath); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to save some artifacts")
		// Don't fail the test run on artifact errors
	}

	// Write test run metadata
	metadataPath := filepath.Join(runDir, "testrun.json")
	metadataJSON, err := json.MarshalIndent(testRun, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal test run: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write test run metadata: %w", err)
	}

	a.logger.Debug().Str("dir", runDir).Str("id", testRun.ID).Msg("Recorded test run")
	return nil
}
