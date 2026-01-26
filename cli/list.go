package cli

// This file contains the list command for displaying previous test runs.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/perfgo/perfgo/history"
	"github.com/perfgo/perfgo/model"
	"github.com/urfave/cli/v2"
)

func (a *App) list(ctx *cli.Context) error {
	filterPath := ctx.String("path")
	limit := ctx.Int("limit")

	// Get perfgo root directory
	perfgoRoot, err := history.GetPerfgoRoot()
	if err != nil {
		return err
	}

	// Load all history entries
	historyEntries, err := history.LoadEntries(a.logger, perfgoRoot)
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	// Apply path filter if specified
	var filteredEntries []history.Entry
	for _, entry := range historyEntries {
		if filterPath == "" || strings.Contains(entry.History.WorkDir, filterPath) {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	if len(filteredEntries) == 0 {
		if filterPath != "" {
			fmt.Printf("No history entries found matching path: %s\n", filterPath)
		} else {
			fmt.Println("No history entries found")
		}
		return nil
	}

	// Sort by timestamp (newest first)
	sort.Slice(filteredEntries, func(i, j int) bool {
		return filteredEntries[i].History.Timestamp.After(filteredEntries[j].History.Timestamp)
	})

	// Apply limit
	displayRuns := filteredEntries
	if limit > 0 && limit < len(displayRuns) {
		displayRuns = displayRuns[:limit]
	}

	fmt.Printf("\n=== History (%d total) ===\n\n", len(filteredEntries))

	for _, entry := range displayRuns {
		tr := entry.History
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
		fmt.Printf("   %s\n", entry.FullPath)
		fmt.Println()
	}

	fmt.Println("\nView test output: cat <path>/stdout.txt")
	fmt.Println("View profile: perfgo view <ID>")

	return nil
}
