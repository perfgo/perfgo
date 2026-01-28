package cli

// This file contains local test execution functionality for
// running tests on the local machine with optional perf integration.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/perfgo/perfgo/cli/perf"
)

func (a *App) executeLocalTest(binaryPath string, recordOpts *perf.RecordOptions, args []string, stdout, stderr *string) error {
	return a.executeLocalTestWithOptions(binaryPath, recordOpts, args, stdout, stderr)
}

func (a *App) executeLocalTestWithStatOptions(binaryPath string, statOpts perf.StatOptions, args []string, stdout, stderr *string) error {
	a.logger.Debug().
		Str("binary", binaryPath).
		Strs("args", args).
		Msg("Starting local test execution with perf stat")

	statOpts.Binary = binaryPath
	statOpts.Args = args
	perfArgs := perf.BuildStatArgs(statOpts)
	cmd := exec.Command("perf", perfArgs...)

	a.logger.Info().
		Strs("events", statOpts.Events).
		Bool("detail", statOpts.Detail).
		Msg("Wrapping test execution with perf stat")

	// Capture stdout and stderr for history
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create multi-writers to both capture and display output
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Save captured output
		*stdout = stdoutBuf.String()
		*stderr = stderrBuf.String()

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

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeLocalTestWithOptions(binaryPath string, recordOpts *perf.RecordOptions, args []string, stdout, stderr *string) error {
	logMsg := a.logger.Debug().
		Str("binary", binaryPath).
		Strs("args", args)

	if recordOpts != nil && recordOpts.Event != "" {
		logMsg.Str("perfEvent", recordOpts.Event)
		if recordOpts.Count > 0 {
			logMsg.Int("perfCount", recordOpts.Count)
		}
	}
	logMsg.Msg("Starting local test execution")

	var cmd *exec.Cmd

	if recordOpts != nil {
		// Build perf record command
		recordOpts.OutputPath = "perf.data"
		recordOpts.Binary = binaryPath
		recordOpts.Args = args

		perfArgs := perf.BuildRecordArgs(*recordOpts)
		cmd = exec.Command("perf", perfArgs...)

		logEvent := a.logger.Info()
		if recordOpts.Event != "" {
			logEvent.Str("event", recordOpts.Event)
			if recordOpts.Count > 0 {
				logEvent.Int("count", recordOpts.Count)
			}
		}
		logEvent.Msg("Wrapping test execution with perf record")
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
		// Save captured output
		*stdout = stdoutBuf.String()
		*stderr = stderrBuf.String()

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

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	if recordOpts != nil {
		a.logger.Info().Str("output", "perf.data").Msg("Performance data collected")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeLocalTestWithC2COptions(binaryPath string, c2cOpts perf.C2COptions, reportOpts perf.C2CReportOptions, args []string, stdout, stderr *string) error {
	a.logger.Debug().
		Str("binary", binaryPath).
		Strs("args", args).
		Msg("Starting local test execution with perf c2c")

	c2cOpts.Binary = binaryPath
	c2cOpts.Args = args
	perfArgs := perf.BuildC2CRecordArgs(c2cOpts)
	cmd := exec.Command("perf", perfArgs...)

	logMsg := a.logger.Info()
	if c2cOpts.Event != "" {
		logMsg.Str("event", c2cOpts.Event)
		if c2cOpts.Count > 0 {
			logMsg.Int("count", c2cOpts.Count)
		}
	}
	logMsg.Msg("Wrapping test execution with perf c2c record")

	// Capture stdout and stderr for history
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create multi-writers to both capture and display output
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Save captured output
		*stdout = stdoutBuf.String()
		*stderr = stderrBuf.String()

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

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	a.logger.Info().Str("output", "perf.data").Msg("C2C performance data collected")
	a.logger.Info().Msg("Tests completed successfully")
	return nil
}
