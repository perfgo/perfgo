package history

// This file contains shared history utilities for loading and parsing
// test run history.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/perfgo/perfgo/model"
	"github.com/rs/zerolog"
)

type Entry struct {
	History  model.History
	FullPath string
}

// GetPerfgoRoot returns the .perfgo directory path from the git repository root.
func GetPerfgoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	perfgoRoot := filepath.Join(repoRoot, ".perfgo")

	// Check if .perfgo directory exists
	if _, err := os.Stat(perfgoRoot); os.IsNotExist(err) {
		return "", fmt.Errorf("no test runs found in %s", perfgoRoot)
	}

	return perfgoRoot, nil
}

// LoadEntries loads all history entries from the .perfgo directory.
func LoadEntries(logger zerolog.Logger, perfgoRoot string) ([]Entry, error) {
	var entries []Entry

	err := filepath.WalkDir(perfgoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			historyPath := filepath.Join(path, "history.json")
			if _, err := os.Stat(historyPath); err == nil {
				history, err := parseHistoryJSON(historyPath)
				if err != nil {
					logger.Warn().Err(err).Str("path", historyPath).Msg("Failed to parse history.json")
					return nil
				}

				entries = append(entries, Entry{
					History:  history,
					FullPath: path,
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk .perfgo directory: %w", err)
	}

	return entries, nil
}

// parseHistoryJSON parses a history.json file.
func parseHistoryJSON(historyPath string) (model.History, error) {
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return model.History{}, err
	}

	var history model.History
	if err := json.Unmarshal(data, &history); err != nil {
		return model.History{}, err
	}

	return history, nil
}
