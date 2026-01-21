package cli

// This file contains remote test execution functionality for
// running tests on remote hosts via SSH with optional perf integration.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/perfgo/perfgo/cli/perf"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/model"
)

func (a *App) executeRemoteTest(host, controlPath, remotePath string, args []string) error {
	a.logger.Debug().
		Str("host", host).
		Str("binary", remotePath).
		Strs("args", args).
		Msg("Starting remote test execution")

	// Build the remote command with arguments
	// Need to properly quote the remote path and arguments
	remoteCmd := remotePath
	if len(args) > 0 {
		// Append arguments to the remote command
		for _, arg := range args {
			// Simple shell escaping - wrap in single quotes and escape any single quotes
			escapedArg := strings.ReplaceAll(arg, "'", "'\\''")
			remoteCmd += fmt.Sprintf(" '%s'", escapedArg)
		}
	}

	// Execute the test binary remotely
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		host,
		remoteCmd,
	)

	// Connect stdout and stderr to display test output in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Test failures are expected to return non-zero exit codes
		// Check if it's an ExitError (test failed) vs other errors
		if exitErr, ok := err.(*exec.ExitError); ok {
			a.logger.Info().
				Int("exit_code", exitErr.ExitCode()).
				Msg("Tests completed with failures")
			return fmt.Errorf("tests failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute remote test: %w", err)
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeRemoteTestInDir(sshClient *ssh.Client, remotePath, remoteDir, remoteBaseDir, packagePath, perfMode, perfEvent string, args []string, testRun *model.TestRun) error {
	// Construct the full working directory path
	workDir := remoteDir
	if packagePath != "." && packagePath != "" {
		workDir = fmt.Sprintf("%s/%s", remoteDir, packagePath)
	}

	a.logger.Debug().
		Str("host", sshClient.Host()).
		Str("binary", remotePath).
		Str("sync_dir", remoteDir).
		Str("work_dir", workDir).
		Str("package", packagePath).
		Str("perfMode", perfMode).
		Str("perfEvent", perfEvent).
		Strs("args", args).
		Msg("Starting remote test execution")

	var remoteCmd string

	if perfMode == "profile" {
		// Build perf record command
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		recordOpts := perf.RecordOptions{
			Event:      perfEvent,
			OutputPath: perfDataPath,
			Binary:     remotePath,
			Args:       args,
		}
		perfCmd := perf.BuildRecordCommand(recordOpts)
		remoteCmd = fmt.Sprintf("cd %s && %s", workDir, perfCmd)

		a.logger.Info().
			Str("event", perfEvent).
			Str("output", perfDataPath).
			Msg("Wrapping remote test execution with perf record")
	} else if perfMode == "stat" {
		// Build perf stat command
		var events []string
		if perfEvent != "" {
			events = strings.Split(perfEvent, ",")
		}
		statOpts := perf.StatOptions{
			Events: events,
			Binary: remotePath,
			Args:   args,
		}
		perfCmd := perf.BuildStatCommand(statOpts)
		remoteCmd = fmt.Sprintf("cd %s && %s", workDir, perfCmd)

		a.logger.Info().
			Str("events", perfEvent).
			Msg("Wrapping remote test execution with perf stat")
	} else {
		// Direct execution without perf
		remoteCmd = fmt.Sprintf("cd %s && %s", workDir, remotePath)

		// Append arguments for direct execution
		if len(args) > 0 {
			for _, arg := range args {
				escapedArg := strings.ReplaceAll(arg, "'", "'\\''")
				remoteCmd += fmt.Sprintf(" '%s'", escapedArg)
			}
		}
	}

	// Execute the test binary remotely
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", sshClient.ControlPath()),
		"-o", "ControlMaster=no",
		sshClient.Host(),
		remoteCmd,
	)

	// Capture stdout and stderr for history
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create multi-writers to both capture and display output
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Save captured output to testRun
		testRun.StdoutFile = stdoutBuf.String()
		testRun.StderrFile = stderrBuf.String()

		// Test failures are expected to return non-zero exit codes
		// Check if it's an ExitError (test failed) vs other errors
		if exitErr, ok := err.(*exec.ExitError); ok {
			a.logger.Info().
				Int("exit_code", exitErr.ExitCode()).
				Msg("Tests completed with failures")
			return fmt.Errorf("tests failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute remote test: %w", err)
	}

	// Save captured output to testRun
	testRun.StdoutFile = stdoutBuf.String()
	testRun.StderrFile = stderrBuf.String()

	if perfMode == "profile" {
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		a.logger.Info().
			Str("output", perfDataPath).
			Msg("Performance data collected on remote host")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}
