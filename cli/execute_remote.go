package cli

// This file contains remote test execution functionality for
// running tests on remote hosts via SSH with optional perf integration.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"al.essio.dev/pkg/shellescape"
	"github.com/perfgo/perfgo/cli/perf"
	"github.com/perfgo/perfgo/cli/ssh"
)

func (a *App) executeRemoteTestInDir(sshClient *ssh.Client, remotePath, remoteDir, remoteBaseDir, packagePath string, recordOpts *perf.RecordOptions, args []string, stdout, stderr *string) error {
	return a.executeRemoteTestInDirWithOptions(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, recordOpts, args, stdout, stderr)
}

func (a *App) executeRemoteTestInDirWithStatOptions(sshClient *ssh.Client, remotePath, remoteDir, remoteBaseDir, packagePath string, statOpts perf.StatOptions, args []string, stdout, stderr *string) error {
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
		Strs("args", args).
		Msg("Starting remote test execution with perf stat")

	statOpts.Binary = remotePath
	statOpts.Args = args
	perfCmd := perf.BuildStatCommand(statOpts)
	remoteCmd := fmt.Sprintf("cd %s && %s", shellescape.Quote(workDir), perfCmd)

	a.logger.Info().
		Strs("events", statOpts.Events).
		Bool("detail", statOpts.Detail).
		Msg("Wrapping remote test execution with perf stat")

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
		return fmt.Errorf("failed to execute remote test: %w", err)
	}

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeRemoteTestInDirWithOptions(sshClient *ssh.Client, remotePath, remoteDir, remoteBaseDir, packagePath string, recordOpts *perf.RecordOptions, args []string, stdout, stderr *string) error {
	// Construct the full working directory path
	workDir := remoteDir
	if packagePath != "." && packagePath != "" {
		workDir = fmt.Sprintf("%s/%s", remoteDir, packagePath)
	}

	logMsg := a.logger.Debug().
		Str("host", sshClient.Host()).
		Str("binary", remotePath).
		Str("sync_dir", remoteDir).
		Str("work_dir", workDir).
		Str("package", packagePath).
		Strs("args", args)

	if recordOpts != nil && recordOpts.Event != "" {
		logMsg.Str("perfEvent", recordOpts.Event)
		if recordOpts.Count > 0 {
			logMsg.Int("perfCount", recordOpts.Count)
		}
	}
	logMsg.Msg("Starting remote test execution")

	var remoteCmd string

	if recordOpts != nil {
		// Build perf record command
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		recordOpts.OutputPath = perfDataPath
		recordOpts.Binary = remotePath
		recordOpts.Args = args

		perfCmd := perf.BuildRecordCommand(*recordOpts)
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(workDir), perfCmd)

		logEvent := a.logger.Info().
			Str("output", perfDataPath)
		if recordOpts.Event != "" {
			logEvent.Str("event", recordOpts.Event)
			if recordOpts.Count > 0 {
				logEvent.Int("count", recordOpts.Count)
			}
		}
		logEvent.Msg("Wrapping remote test execution with perf record")
	} else {
		// Direct execution without perf
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(workDir), shellescape.Quote(remotePath))

		// Append arguments for direct execution
		if len(args) > 0 {
			for _, arg := range args {
				remoteCmd += " " + shellescape.Quote(arg)
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
		return fmt.Errorf("failed to execute remote test: %w", err)
	}

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	if recordOpts != nil {
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		a.logger.Info().
			Str("output", perfDataPath).
			Msg("Performance data collected on remote host")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeRemoteTestInDirWithC2COptions(sshClient *ssh.Client, remotePath, remoteDir, remoteBaseDir, packagePath string, c2cOpts perf.C2COptions, reportOpts perf.C2CReportOptions, args []string, stdout, stderr *string) error {
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
		Strs("args", args).
		Msg("Starting remote test execution with perf c2c")

	// Build perf c2c record command
	perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
	c2cOpts.OutputPath = perfDataPath
	c2cOpts.Binary = remotePath
	c2cOpts.Args = args
	perfCmd := perf.BuildC2CRecordCommand(c2cOpts)
	remoteCmd := fmt.Sprintf("cd %s && %s", shellescape.Quote(workDir), perfCmd)

	logMsg := a.logger.Info().
		Str("output", perfDataPath)
	if c2cOpts.Event != "" {
		logMsg.Str("event", c2cOpts.Event)
		if c2cOpts.Count > 0 {
			logMsg.Int("count", c2cOpts.Count)
		}
	}
	logMsg.Msg("Wrapping remote test execution with perf c2c record")

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
		return fmt.Errorf("failed to execute remote test: %w", err)
	}

	// Save captured output
	*stdout = stdoutBuf.String()
	*stderr = stderrBuf.String()

	a.logger.Info().
		Str("output", perfDataPath).
		Msg("C2C performance data collected on remote host")

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}
