package perf

// record.go contains utilities for building perf record commands and
// processing perf data to pprof profiles.

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/perfscript"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// RecordOptions contains options for perf record command.
type RecordOptions struct {
	Event      string   // Event to record
	Count      int      // Event period to sample (e.g., -c 1000000)
	PIDs       []string // Process IDs to attach to
	Duration   int      // Duration in seconds (used with sleep)
	OutputPath string   // Output file path (default: perf.data)
	Binary     string   // Binary to execute (mutually exclusive with PIDs)
	Args       []string // Arguments for the binary
}

// BuildRecordArgs builds perf record command arguments for local execution.
func BuildRecordArgs(opts RecordOptions) []string {
	args := []string{"record", "-g", "--call-graph", "fp"}

	// Add event
	if opts.Event != "" {
		args = append(args, "-e", opts.Event)

		// Add count (event period) - only if event is specified
		if opts.Count > 0 {
			args = append(args, "-c", fmt.Sprintf("%d", opts.Count))
		}
	}

	// Add output path
	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = "perf.data"
	}
	args = append(args, "-o", outputPath)

	// Add PIDs or binary execution
	if len(opts.PIDs) > 0 {
		pidList := strings.Join(opts.PIDs, ",")
		args = append(args, "-p", pidList)

		// When attaching to PIDs, use sleep for duration
		args = append(args, "sleep", fmt.Sprintf("%d", opts.Duration))
	} else if opts.Binary != "" {
		args = append(args, "--", opts.Binary)
		args = append(args, opts.Args...)
	}

	return args
}

// BuildRecordCommand builds perf record command string for remote execution.
// It reuses BuildRecordArgs and joins the arguments with proper shell escaping.
func BuildRecordCommand(opts RecordOptions) string {
	args := BuildRecordArgs(opts)
	
	// Build command with proper shell escaping
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "perf")
	
	for _, arg := range args {
		parts = append(parts, shellescape.Quote(arg))
	}
	
	return strings.Join(parts, " ")
}

// ProfileEventFlag returns the event flag for perf record (single event).
func ProfileEventFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "event",
		Aliases: []string{"e"},
		Usage:   "Event to record",
	}
}

// ProfileCountFlag returns the count flag for perf record (event period).
func ProfileCountFlag() cli.Flag {
	return &cli.IntFlag{
		Name:    "count",
		Aliases: []string{"c"},
		Usage:   "Event period to sample (e.g., sample every N events)",
	}
}

// ConvertPerfToPprof converts a local perf.data file to pprof format.
// The perf script output is written to a temporary file that is deleted after processing.
func ConvertPerfToPprof(logger zerolog.Logger, perfDataPath string, outputPath string) error {
	logger.Info().Str("input", perfDataPath).Str("output", outputPath).Msg("Processing performance data locally")

	// Create temporary file for perf script output
	tempFile, err := os.CreateTemp("", "perf-script-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempPath)
		logger.Debug().Str("temp_file", tempPath).Msg("Cleaned up temporary perf script file")
	}()

	// Run perf script locally and write to temp file
	cmd := exec.Command("perf", "script", "-i", perfDataPath)
	cmd.Stdout = tempFile
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run perf script: %w", err)
	}

	// Get file size for logging
	fileInfo, _ := tempFile.Stat()
	logger.Info().
		Int64("size_bytes", fileInfo.Size()).
		Str("temp_file", tempPath).
		Msg("Performance script output written to temporary file")

	// Seek back to beginning for reading
	if _, err := tempFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek temporary file: %w", err)
	}

	// Parse the perf script output from temp file
	return ParsePerfScript(logger, tempFile, outputPath)
}

// ParsePerfScript parses perf script output and creates a pprof profile.
func ParsePerfScript(logger zerolog.Logger, scriptOutput io.Reader, outputPath string) error {
	logger.Info().Msg("Parsing perf script output")

	// Create parser and parse the output
	parser := perfscript.New()
	prof, err := parser.Parse(scriptOutput)
	if err != nil {
		return fmt.Errorf("failed to parse perf script: %w", err)
	}

	// Write profile to file
	profileFile := outputPath
	f, err := os.Create(profileFile)
	if err != nil {
		return fmt.Errorf("failed to create profile file: %w", err)
	}
	defer f.Close()

	if err := prof.Write(f); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	logger.Info().
		Str("profile", profileFile).
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	logger.Info().Msgf("View profile with: go tool pprof %s", profileFile)

	return nil
}

// BinaryArtifact represents a binary that was copied for the profile.
type BinaryArtifact struct {
	RemotePath string // Original path on remote
	LocalPath  string // Relative path in history directory
	Size       uint64 // File size in bytes
}

// ProcessPerfData processes perf data from a remote host and creates a pprof profile.
// It resolves binary paths through /proc/<pid>/root for containerized processes.
// The perf script output is written to a temporary file that is deleted after processing.
// Returns a list of binaries that were copied for artifact registration.
func ProcessPerfData(logger zerolog.Logger, sshClient *ssh.Client, remoteBaseDir string, outputPath string, binDir string, pids []string) ([]BinaryArtifact, error) {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	logger.Info().
		Str("remote", remotePerfData).
		Msg("Processing performance data on remote host")

	// Create temporary file for perf script output
	tempFile, err := os.CreateTemp("", "perf-script-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempPath)
		logger.Debug().Str("temp_file", tempPath).Msg("Cleaned up temporary perf script file")
	}()

	// Run perf script remotely and stream output to temp file
	perfScriptCmd := fmt.Sprintf("perf script -i %s", remotePerfData)
	if err := sshClient.Run(perfScriptCmd, ssh.WithStdOut(tempFile)); err != nil {
		return nil, fmt.Errorf("failed to run perf script remotely: %w", err)
	}

	// Get file size for logging
	fileInfo, _ := tempFile.Stat()
	logger.Info().
		Int64("size_bytes", fileInfo.Size()).
		Str("temp_file", tempPath).
		Msg("Performance script output written to temporary file")

	// Seek back to beginning for reading
	if _, err := tempFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to seek temporary file: %w", err)
	}

	// Read the entire file into memory for extractBinaryPaths
	scriptBytes, err := io.ReadAll(tempFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read temporary file: %w", err)
	}
	scriptOutput := string(scriptBytes)

	// Extract unique binary paths from perf script output
	binaryPaths := extractBinaryPaths(scriptOutput)
	logger.Info().
		Int("count", len(binaryPaths)).
		Msg("Found binaries referenced in profile")

	// Copy binaries from remote host
	localBinaries := make(map[string]string) // remote path -> local path
	var binaryArtifacts []BinaryArtifact
	if len(binaryPaths) > 0 && len(pids) > 0 {
		logger.Info().Msg("Copying binaries from remote host")

		// Create binaries directory
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create binaries directory: %w", err)
		}

		for _, remotePath := range binaryPaths {
			// Skip special paths like [kernel.kallsyms], [vdso], etc.
			if strings.HasPrefix(remotePath, "[") {
				continue
			}

			// Try to find binary through /proc/<pid>/root for each PID
			var foundProcPath string
			var foundPID string
			for _, pid := range pids {
				procPath := fmt.Sprintf("/proc/%s/root%s", pid, remotePath)
				
				// Check if binary exists via /proc/<pid>/root
				checkCmd := fmt.Sprintf("test -f %s && echo exists", procPath)
				output, _, err := sshClient.RunCommand(checkCmd)
				if err == nil && strings.TrimSpace(output) == "exists" {
					foundProcPath = procPath
					foundPID = pid
					break
				}
			}

			if foundProcPath == "" {
				logger.Debug().
					Str("path", remotePath).
					Msg("Binary not found via /proc/pid/root for any PID, skipping")
				continue
			}

			// Copy binary to local
			localPath := filepath.Join(binDir, filepath.Base(remotePath))
			if err := copyBinaryFromRemote(logger, sshClient, foundProcPath, localPath); err != nil {
				logger.Warn().
					Err(err).
					Str("remote", remotePath).
					Str("proc_path", foundProcPath).
					Msg("Failed to copy binary")
				continue
			}

			localBinaries[remotePath] = localPath

			// Collect artifact information
			if info, err := os.Stat(localPath); err == nil {
				binaryArtifacts = append(binaryArtifacts, BinaryArtifact{
					RemotePath: remotePath,
					LocalPath:  filepath.Join("profile-binaries", filepath.Base(remotePath)),
					Size:       uint64(info.Size()),
				})
			}

			logger.Debug().
				Str("remote", remotePath).
				Str("pid", foundPID).
				Str("proc_path", foundProcPath).
				Str("local", localPath).
				Msg("Copied binary")
		}

		logger.Info().
			Int("count", len(localBinaries)).
			Msg("Binaries copied successfully")
	}

	// Parse and create the profile
	parser := perfscript.New()
	prof, err := parser.Parse(strings.NewReader(scriptOutput))
	if err != nil {
		return nil, fmt.Errorf("failed to parse perf script: %w", err)
	}

	// Update binary paths in the profile to point to local copies
	for _, mapping := range prof.Mapping {
		if localPath, ok := localBinaries[mapping.File]; ok {
			mapping.File = localPath
		}
	}

	// Write profile to file
	profileFile := outputPath
	f, err := os.Create(profileFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile file: %w", err)
	}
	defer f.Close()

	if err := prof.Write(f); err != nil {
		return nil, fmt.Errorf("failed to write profile: %w", err)
	}

	logger.Info().
		Str("profile", profileFile).
		Str("binaries", binDir).
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	logger.Info().Msgf("View profile with: go tool pprof %s", profileFile)

	return binaryArtifacts, nil
}

// extractBinaryPaths extracts unique binary paths from perf script output.
func extractBinaryPaths(scriptOutput string) []string {
	binarySet := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(scriptOutput))

	for scanner.Scan() {
		line := scanner.Text()
		// Look for stack frame lines with binary paths: (/path/to/binary)
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
					binaryPath := strings.TrimSuffix(strings.TrimPrefix(part, "("), ")")
					if binaryPath != "" {
						binarySet[binaryPath] = true
					}
					break
				}
			}
		}
	}

	// Convert set to slice
	binaries := make([]string, 0, len(binarySet))
	for path := range binarySet {
		binaries = append(binaries, path)
	}
	return binaries
}

// copyBinaryFromRemote copies a binary from the remote host to local filesystem.
func copyBinaryFromRemote(logger zerolog.Logger, sshClient *ssh.Client, remotePath, localPath string) error {
	// Use base64 encoding to transfer the binary
	// This avoids issues with binary data in SSH output
	cmd := fmt.Sprintf("base64 %s", remotePath)
	base64Output, _, err := sshClient.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to read remote binary: %w", err)
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Output))
	if err != nil {
		return fmt.Errorf("failed to decode base64: %w", err)
	}

	// Write to local file
	if err := os.WriteFile(localPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write local binary: %w", err)
	}

	return nil
}
