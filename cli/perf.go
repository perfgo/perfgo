package cli

// This file contains performance data processing functionality for
// converting perf data to pprof profiles.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/perfscript"
)

func (a *App) convertPerfToPprof(perfDataPath string) error {
	a.logger.Info().Str("input", perfDataPath).Msg("Processing performance data locally")

	// Run perf script locally
	cmd := exec.Command("perf", "script", "-i", perfDataPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run perf script: %w", err)
	}

	// Save the perf script output
	scriptFile := "perf.script"
	if err := os.WriteFile(scriptFile, output, 0644); err != nil {
		return fmt.Errorf("failed to write perf script output: %w", err)
	}

	a.logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return a.parsePerfScript(bytes.NewReader(output))
}

func (a *App) parsePerfScript(scriptOutput io.Reader) error {
	a.logger.Info().Msg("Parsing perf script output")

	// Create parser and parse the output
	parser := perfscript.New()
	prof, err := parser.Parse(scriptOutput)
	if err != nil {
		return fmt.Errorf("failed to parse perf script: %w", err)
	}

	// Write profile to file
	profileFile := "perf.pb.gz"
	f, err := os.Create(profileFile)
	if err != nil {
		return fmt.Errorf("failed to create profile file: %w", err)
	}
	defer f.Close()

	if err := prof.Write(f); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	a.logger.Info().
		Str("profile", profileFile).
		Str("script", "perf.script").
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	a.logger.Info().Msgf("View profile with: go tool pprof %s", profileFile)

	return nil
}

func (a *App) processPerfData(sshClient *ssh.Client, remoteBaseDir string) error {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	a.logger.Info().
		Str("remote", remotePerfData).
		Msg("Processing performance data on remote host")

	// Run perf script remotely and capture output
	perfScriptCmd := fmt.Sprintf("perf script -i %s", remotePerfData)
	scriptOutput, err := sshClient.RunCommand(perfScriptCmd)
	if err != nil {
		return fmt.Errorf("failed to run perf script remotely: %w", err)
	}

	// Save the perf script output locally
	scriptFile := "perf.script"
	if err := os.WriteFile(scriptFile, []byte(scriptOutput), 0644); err != nil {
		return fmt.Errorf("failed to write perf script output: %w", err)
	}

	a.logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return a.parsePerfScript(strings.NewReader(scriptOutput))
}
