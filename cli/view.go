package cli

// This file contains the view command for displaying test results from history.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/perfgo/perfgo/history"
	"github.com/perfgo/perfgo/model"
	"github.com/urfave/cli/v2"
)

func removeFirstDashDash(in []string) []string {
	if len(in) > 0 && in[0] == "--" {
		return in[1:]
	}
	return in
}

func parseViewArgs(in []string) (idArg string, pprofArgs []string) {
	if len(in) == 0 {
		return "0", nil
	}

	// If first arg is "--", use default "0" and rest are pprof args
	if in[0] == "--" {
		return "0", in[1:]
	}

	// Check if first arg looks like a pprof flag instead of an ID
	// A negative index is: "-" followed by only digits (e.g., "-1", "-2")
	// A pprof flag is: "-" followed by non-digit or equals (e.g., "-http=:8080", "-top")
	if len(in[0]) > 1 && in[0][0] == '-' {
		// Check if it's a valid negative integer
		if _, err := strconv.ParseInt(in[0], 10, 64); err != nil {
			// Not a valid negative integer, so it's a pprof flag
			return "0", in
		}
	}

	// First arg is the ID/index, rest are pprof args (with optional "--" removed)
	return in[0], removeFirstDashDash(in[1:])
}

func (a *App) view(ctx *cli.Context) error {
	// Parse arguments to extract ID/index and pprof args
	arg, pprofArgs := parseViewArgs(ctx.Args().Slice())

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

	if len(historyEntries) == 0 {
		return fmt.Errorf("no history entries found")
	}

	// Sort by timestamp (newest first)
	sort.Slice(historyEntries, func(i, j int) bool {
		return historyEntries[i].History.Timestamp.After(historyEntries[j].History.Timestamp)
	})

	// Parse argument to find the target entry
	var targetEntry *history.Entry
	if parsed, err := strconv.ParseInt(arg, 10, 64); err == nil {
		// It's a number
		if parsed > 0 {
			// Positive integers are not allowed
			return fmt.Errorf("invalid index: %s (use 0 for last, -1 for second-to-last, -2 for third-to-last, etc.)", arg)
		}
		// 0 or negative integer: count from the end (0=last, -1=second-to-last, -2=third-to-last, etc.)
		index := int(-parsed) // Convert to positive index (0 -> 0, -1 -> 1, -2 -> 2)
		if index >= len(historyEntries) {
			return fmt.Errorf("index %s out of range (only %d history entries)", arg, len(historyEntries))
		}
		targetEntry = &historyEntries[index]
	} else {
		// Treat as hex ID prefix
		hexID := strings.ToLower(arg)
		found := false
		for i := range historyEntries {
			if strings.HasPrefix(strings.ToLower(historyEntries[i].History.ID), hexID) {
				targetEntry = &historyEntries[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no history entry found matching ID: %s", arg)
		}
	}

	// Display the entry
	return a.displayHistoryEntry(targetEntry, pprofArgs)
}

func (a *App) displayHistoryEntry(entry *history.Entry, pprofArgs []string) error {
	h := entry.History

	// Print header
	fmt.Printf("=== Test Run: %s ===\n", h.ID[:8])
	fmt.Printf("Time: %s\n", h.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %s\n", h.Duration)
	fmt.Printf("Exit Code: %d\n", h.ExitCode)
	if h.WorkDir != "" {
		fmt.Printf("Working Dir: %s\n", h.WorkDir)
	}
	if h.Git != nil {
		if h.Git.Commit != "" {
			fmt.Printf("Git Commit: %s", h.Git.Commit[:8])
			if h.Git.Branch != "" {
				fmt.Printf(" (%s)", h.Git.Branch)
			}
			fmt.Println()
		}
	}
	if h.Perf != nil {
		if h.Perf.Record != nil {
			fmt.Printf("Perf Record: event=%s", h.Perf.Record.Event)
			if h.Perf.Record.Count > 0 {
				fmt.Printf(", count=%d", h.Perf.Record.Count)
			}
			fmt.Println()
		}
		if h.Perf.Stat != nil {
			fmt.Printf("Perf Stat: events=%v\n", h.Perf.Stat.Events)
		}
	}
	fmt.Println()

	// Prioritize artifacts for display
	// Highest priority: pprof profiles, perf stat outputs
	// Lowest priority: binaries
	var profileArtifact *model.Artifact
	var statArtifact *model.Artifact
	var stdoutArtifact *model.Artifact
	var stderrArtifact *model.Artifact

	for i := range h.Artifacts {
		artifact := &h.Artifacts[i]
		switch artifact.Type {
		case model.ArtifactTypePprofProfile:
			profileArtifact = artifact
		case model.ArtifactTypePerfStat, model.ArtifactTypePerfStatDetailed:
			statArtifact = artifact
		case model.ArtifactTypeStdout:
			stdoutArtifact = artifact
		case model.ArtifactTypeStderr:
			stderrArtifact = artifact
		}
	}

	// Display highest priority artifact first
	if profileArtifact != nil {
		return a.displayProfile(entry.FullPath, profileArtifact, pprofArgs)
	}

	if statArtifact != nil {
		return a.displayPerfStat(entry.FullPath, statArtifact)
	}

	if stdoutArtifact != nil {
		return a.displayStdout(entry.FullPath, stdoutArtifact)
	}

	if stderrArtifact != nil {
		return a.displayStderr(entry.FullPath, stderrArtifact)
	}

	// No displayable artifacts found
	fmt.Println("No displayable artifacts found (only binaries)")
	fmt.Printf("History directory: %s\n", entry.FullPath)
	return nil
}

func (a *App) displayProfile(runDir string, artifact *model.Artifact, pprofArgs []string) error {
	profilePath := filepath.Join(runDir, artifact.File)
	fmt.Printf("Profile: %s (%.1f KB)\n", profilePath, float64(artifact.Size)/1024)

	// Build pprof command with any additional args
	args := []string{"tool", "pprof"}
	args = append(args, pprofArgs...)
	args = append(args, profilePath)

	cmd := exec.Command("go", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = runDir

	return cmd.Run()
}

func (a *App) displayPerfStat(runDir string, artifact *model.Artifact) error {
	statPath := filepath.Join(runDir, artifact.File)
	fmt.Printf("Perf Stat Output: %s\n", statPath)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return fmt.Errorf("failed to read stat output: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func (a *App) displayStdout(runDir string, artifact *model.Artifact) error {
	stdoutPath := filepath.Join(runDir, artifact.File)
	fmt.Printf("Test Output (stdout): %s\n", stdoutPath)
	data, err := os.ReadFile(stdoutPath)
	if err != nil {
		return fmt.Errorf("failed to read stdout: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func (a *App) displayStderr(runDir string, artifact *model.Artifact) error {
	stderrPath := filepath.Join(runDir, artifact.File)
	fmt.Printf("Test Output (stderr): %s\n", stderrPath)
	data, err := os.ReadFile(stderrPath)
	if err != nil {
		return fmt.Errorf("failed to read stderr: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
