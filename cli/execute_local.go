package cli

// This file contains local test execution functionality for
// running tests on the local machine with optional perf integration.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/perfgo/perfgo/cli/perf"
	"github.com/perfgo/perfgo/model"
)

func (a *App) executeLocalTest(binaryPath, perfMode, perfEvent string, args []string, testRun *model.TestRun) error {
	a.logger.Debug().
		Str("binary", binaryPath).
		Str("perfMode", perfMode).
		Str("perfEvent", perfEvent).
		Strs("args", args).
		Msg("Starting local test execution")

	var cmd *exec.Cmd

	if perfMode == "profile" {
		// Build perf record command
		recordOpts := perf.RecordOptions{
			Event:      perfEvent,
			OutputPath: "perf.data",
			Binary:     binaryPath,
			Args:       args,
		}
		perfArgs := perf.BuildRecordArgs(recordOpts)
		cmd = exec.Command("perf", perfArgs...)

		a.logger.Info().
			Str("event", perfEvent).
			Msg("Wrapping test execution with perf record")
	} else if perfMode == "stat" {
		// Build perf stat command
		var events []string
		if perfEvent != "" {
			events = strings.Split(perfEvent, ",")
		}
		statOpts := perf.StatOptions{
			Events: events,
			Binary: binaryPath,
			Args:   args,
		}
		perfArgs := perf.BuildStatArgs(statOpts)
		cmd = exec.Command("perf", perfArgs...)

		a.logger.Info().
			Str("events", perfEvent).
			Msg("Wrapping test execution with perf stat")
	} else {
		// Execute the test binary directly with arguments
		cmd = exec.Command(binaryPath, args...)
	}

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
		return fmt.Errorf("failed to execute test: %w", err)
	}

	// Save captured output to testRun
	testRun.StdoutFile = stdoutBuf.String()
	testRun.StderrFile = stderrBuf.String()

	if perfMode == "profile" {
		a.logger.Info().Str("output", "perf.data").Msg("Performance data collected")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}
