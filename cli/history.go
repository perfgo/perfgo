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

type testRunEntry struct {
	testRun  model.TestRun
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

	// Collect test runs
	var testRunEntries []testRunEntry
	err = filepath.WalkDir(perfgoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			testRunPath := filepath.Join(path, "testrun.json")
			if _, err := os.Stat(testRunPath); err == nil {
				testRun, err := a.parseTestRunJSON(testRunPath)
				if err != nil {
					a.logger.Warn().Err(err).Str("path", testRunPath).Msg("Failed to parse testrun.json")
					return nil
				}

				entry := testRunEntry{
					testRun:  testRun,
					fullPath: path,
				}

				// Apply path filter if specified
				if filterPath == "" || strings.Contains(testRun.WorkDir, filterPath) {
					testRunEntries = append(testRunEntries, entry)
				}
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk .perfgo directory: %w", err)
	}

	if len(testRunEntries) == 0 {
		if filterPath != "" {
			fmt.Printf("No test runs found matching path: %s\n", filterPath)
		} else {
			fmt.Println("No test runs found")
		}
		return nil
	}

	// Sort by timestamp (newest first)
	sort.Slice(testRunEntries, func(i, j int) bool {
		return testRunEntries[i].testRun.Timestamp.After(testRunEntries[j].testRun.Timestamp)
	})

	// Apply limit
	displayRuns := testRunEntries
	if limit > 0 && limit < len(displayRuns) {
		displayRuns = displayRuns[:limit]
	}

	fmt.Printf("\n=== Test Runs (%d total) ===\n\n", len(testRunEntries))

	for _, entry := range displayRuns {
		tr := entry.testRun
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
		if tr.RemoteHost != "" {
			fmt.Printf("   Remote: %s", tr.RemoteHost)
			if tr.OS != "" && tr.Arch != "" {
				fmt.Printf(" (%s/%s)", tr.OS, tr.Arch)
			}
			fmt.Println()
		} else if tr.OS != "" && tr.Arch != "" {
			fmt.Printf("   Local: %s/%s\n", tr.OS, tr.Arch)
		}
		if tr.Commit != "" {
			shortCommit := tr.Commit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			fmt.Printf("   Commit: %s", shortCommit)
			if tr.Branch != "" {
				fmt.Printf(" (%s)", tr.Branch)
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
				}
				fmt.Printf("   %s: %s (%.1f KB)\n", typeName, artifact.File, float64(artifact.Size)/1024)
			}
		}
		fmt.Printf("   %s\n", entry.fullPath)
		fmt.Println()
	}

	fmt.Println("\nView test output: cat <path>/stdout.txt")
	fmt.Println("View profile: go tool pprof <path>/perf.pb.gz")

	return nil
}

func (a *App) parseTestRunJSON(testRunPath string) (model.TestRun, error) {
	data, err := os.ReadFile(testRunPath)
	if err != nil {
		return model.TestRun{}, err
	}

	var testRun model.TestRun
	if err := json.Unmarshal(data, &testRun); err != nil {
		return model.TestRun{}, err
	}

	return testRun, nil
}
