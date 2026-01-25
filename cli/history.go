package cli

// This file contains test run history functionality for listing
// and displaying previous test runs.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/perfgo/perfgo/model"
	"github.com/urfave/cli/v2"
)

type historyEntry struct {
	history  model.History
	fullPath string
}

func (a *App) list(ctx *cli.Context) error {
	filterPath := ctx.String("path")
	limit := ctx.Int("limit")

	// Get git repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))

	perfgoRoot := filepath.Join(repoRoot, ".perfgo")

	// Check if .perfgo directory exists
	if _, err := os.Stat(perfgoRoot); os.IsNotExist(err) {
		fmt.Println("No test runs found")
		fmt.Printf("Test runs are saved to %s/history/<timestamp>-<commit>-<id>/\n", perfgoRoot)
		return nil
	}

	// Collect history entries
	var historyEntries []historyEntry
	err = filepath.WalkDir(perfgoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			historyPath := filepath.Join(path, "history.json")
			if _, err := os.Stat(historyPath); err == nil {
				history, err := a.parseHistoryJSON(historyPath)
				if err != nil {
					a.logger.Warn().Err(err).Str("path", historyPath).Msg("Failed to parse history.json")
					return nil
				}

				entry := historyEntry{
					history:  history,
					fullPath: path,
				}

				// Apply path filter if specified
				if filterPath == "" || strings.Contains(history.WorkDir, filterPath) {
					historyEntries = append(historyEntries, entry)
				}
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk .perfgo directory: %w", err)
	}

	if len(historyEntries) == 0 {
		if filterPath != "" {
			fmt.Printf("No history entries found matching path: %s\n", filterPath)
		} else {
			fmt.Println("No history entries found")
		}
		return nil
	}

	// Sort by timestamp (newest first)
	sort.Slice(historyEntries, func(i, j int) bool {
		return historyEntries[i].history.Timestamp.After(historyEntries[j].history.Timestamp)
	})

	// Apply limit
	displayRuns := historyEntries
	if limit > 0 && limit < len(displayRuns) {
		displayRuns = displayRuns[:limit]
	}

	fmt.Printf("\n=== History (%d total) ===\n\n", len(historyEntries))

	for _, entry := range displayRuns {
		tr := entry.history
		timestamp := tr.Timestamp.Format("2006-01-02 15:04:05")

		// Format duration
		duration := tr.Duration.Round(time.Millisecond)

		// Determine status indicator
		status := "✓"
		if tr.ExitCode != 0 {
			status = "✗"
		}

		// Format args (skip the program name)
		args := ""
		if len(tr.Args) > 1 {
			args = strings.Join(tr.Args[1:], " ")
		}

		// Show short ID (first 8 chars)
		shortID := tr.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		fmt.Printf("%s  %s  [%s]  exit=%d  id=%s\n", status, timestamp, duration, tr.ExitCode, shortID)
		if args != "" {
			fmt.Printf("   Args: %s\n", args)
		}
		if tr.WorkDir != "" {
			fmt.Printf("   Path: %s\n", tr.WorkDir)
		}
		if tr.Target != nil {
			if tr.Target.RemoteHost != "" {
				fmt.Printf("   Remote: %s", tr.Target.RemoteHost)
				if tr.Target.OS != "" && tr.Target.Arch != "" {
					fmt.Printf(" (%s/%s)", tr.Target.OS, tr.Target.Arch)
				}
				fmt.Println()
			} else if tr.Target.OS != "" && tr.Target.Arch != "" {
				fmt.Printf("   Local: %s/%s\n", tr.Target.OS, tr.Target.Arch)
			}
		}
		if tr.Git != nil && tr.Git.Commit != "" {
			shortCommit := tr.Git.Commit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			fmt.Printf("   Commit: %s", shortCommit)
			if tr.Git.Branch != "" {
				fmt.Printf(" (%s)", tr.Git.Branch)
			}
			fmt.Println()
		}
		if len(tr.Artifacts) > 0 {
			for _, artifact := range tr.Artifacts {
				var typeName string
				switch artifact.Type {
				case model.ArtifactTypePprofProfile:
					typeName = "profile"
				case model.ArtifactTypeTestBinary:
					typeName = "binary"
				case model.ArtifactTypeAttachBinary:
					typeName = "binary"
				case model.ArtifactTypeStdout:
					typeName = "stdout"
				case model.ArtifactTypeStderr:
					typeName = "stderr"
				}
				if typeName != "" {
					fmt.Printf("   %s: %s (%.1f KB)\n", typeName, artifact.File, float64(artifact.Size)/1024)
				}
			}
		}
		fmt.Printf("   %s\n", entry.fullPath)
		fmt.Println()
	}

	fmt.Println("\nView test output: cat <path>/stdout.txt")
	fmt.Println("View profile: go tool pprof <path>/perf.pb.gz")

	return nil
}

func (a *App) parseHistoryJSON(historyPath string) (model.History, error) {
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
