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

// prepareHistoryDir creates the history directory and returns its path.
// It also updates the history object with git repository information.
func (a *App) prepareHistoryDir(history *model.History) (string, error) {
	// Get repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoName := filepath.Base(repoRoot)

	// Store repo name in history
	if history.Git == nil {
		history.Git = &model.Git{}
	}
	history.Git.Repo = repoName

	// Get relative path from repo root
	relPath := "."
	if history.WorkDir != "" {
		if rel, err := filepath.Rel(repoRoot, history.WorkDir); err == nil {
			relPath = rel
		}
	}

	// Update WorkDir to be relative to repo root
	history.WorkDir = relPath

	// Create directory in .perfgo/history/<timestamp>-<commit>-<id>
	timestamp := history.Timestamp.Format("20060102-150405")
	shortCommit := ""
	if history.Git != nil && history.Git.Commit != "" {
		shortCommit = history.Git.Commit
		if len(shortCommit) > 8 {
			shortCommit = shortCommit[:8]
		}
	}
	shortID := history.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	runName := fmt.Sprintf("%s-%s-%s", timestamp, shortCommit, shortID)
	runDir := filepath.Join(repoRoot, ".perfgo", "history", runName)

	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create run directory: %w", err)
	}

	return runDir, nil
}

func (a *App) recordHistory(history *model.History, runDir string, testBinaryPath string, stdoutContent string, stderrContent string) error {

	// Save stdout as artifact if present
	if stdoutContent != "" {
		stdoutPath := filepath.Join(runDir, "stdout.txt")
		if err := os.WriteFile(stdoutPath, []byte(stdoutContent), 0644); err != nil {
			a.logger.Warn().Err(err).Msg("Failed to write stdout")
		} else {
			info, _ := os.Stat(stdoutPath)
			history.Artifacts = append(history.Artifacts, model.Artifact{
				Type: model.ArtifactTypeStdout,
				Size: uint64(info.Size()),
				File: "stdout.txt",
			})
			a.logger.Debug().Str("file", "stdout.txt").Msg("Saved stdout artifact")
		}
	}

	// Save stderr as artifact if present
	if stderrContent != "" {
		stderrPath := filepath.Join(runDir, "stderr.txt")
		if err := os.WriteFile(stderrPath, []byte(stderrContent), 0644); err != nil {
			a.logger.Warn().Err(err).Msg("Failed to write stderr")
		} else {
			info, _ := os.Stat(stderrPath)
			history.Artifacts = append(history.Artifacts, model.Artifact{
				Type: model.ArtifactTypeStderr,
				Size: uint64(info.Size()),
				File: "stderr.txt",
			})
			a.logger.Debug().Str("file", "stderr.txt").Msg("Saved stderr artifact")
		}
	}

	// Archive other artifacts if they exist
	if err := a.saveArtifacts(runDir, history, testBinaryPath); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to save some artifacts")
		// Don't fail the run on artifact errors
	}

	// Write history metadata
	metadataPath := filepath.Join(runDir, "history.json")
	metadataJSON, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write history metadata: %w", err)
	}

	a.logger.Debug().Str("dir", runDir).Str("id", history.ID).Msg("Recorded history")
	return nil
}
