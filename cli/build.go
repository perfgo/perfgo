package cli

// This file contains test binary building functionality for
// compiling Go test binaries with optional cross-compilation.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func (a *App) buildTestBinary(goos, goarch string, extraArgs []string) (string, error) {
	// Determine output binary name
	binaryName := "./perfgo.test"
	if goos == "windows" {
		binaryName = "./perfgo.test.exe"
	}
	if goos != "" && goarch != "" {
		binaryName = fmt.Sprintf("./perfgo.test.%s.%s", goos, goarch)
		if goos == "windows" {
			binaryName += ".exe"
		}
	}

	a.logger.Info().
		Str("goos", goos).
		Str("goarch", goarch).
		Str("output", binaryName).
		Msg("Building test binary")

	// Prepare the command arguments
	args := []string{"test", "-c", "-o", binaryName}

	// Add extra arguments passed by the user
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
		a.logger.Debug().Strs("extra_args", extraArgs).Msg("Adding extra arguments to go test")
	}

	cmd := exec.Command("go", args...)

	// Set environment for cross-compilation if needed
	if goos != "" && goarch != "" {
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("GOOS=%s", goos),
			fmt.Sprintf("GOARCH=%s", goarch),
			"CGO_ENABLED=0", // Disable CGO for easier cross-compilation
		)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.logger.Debug().
		Str("command", cmd.String()).
		Msg("Executing go test -c")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build test binary: %w (stderr: %s)", err, stderr.String())
	}

	// Verify the binary was created
	if _, err := os.Stat(binaryName); err != nil {
		return "", fmt.Errorf("test binary not found after build: %w", err)
	}

	return binaryName, nil
}
