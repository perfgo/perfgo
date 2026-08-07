package perf

// record.go contains utilities for building perf record commands and
// processing perf data to pprof profiles.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/base32"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/google/pprof/profile"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/perfscript"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

const progressLogInterval = 10 * time.Second

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

// runPerfScriptToFile runs perf script into a temporary file and rewinds it for reading.
func runPerfScriptToFile(logger zerolog.Logger, run func(w *os.File) error) (*os.File, error) {
	tempFile, err := os.CreateTemp("", "perf-script-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	if err := run(tempFile); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return nil, err
	}

	fileInfo, err := tempFile.Stat()
	if err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return nil, fmt.Errorf("failed to stat perf script output: %w", err)
	}
	logger.Info().
		Int64("size_bytes", fileInfo.Size()).
		Str("temp_file", tempFile.Name()).
		Msg("Performance script output written to temporary file")

	if _, err := tempFile.Seek(0, 0); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return nil, fmt.Errorf("failed to seek temporary file: %w", err)
	}

	return tempFile, nil
}

func cleanupPerfScriptFile(logger zerolog.Logger, file *os.File) {
	path := file.Name()
	file.Close()
	os.Remove(path)
	logger.Debug().Str("temp_file", path).Msg("Cleaned up temporary perf script file")
}

// binaryArtifactPaths returns the artifact filename and full local path for a binary.
// Binaries are named <hash>.<original-basename>.binary inside runDir.
func binaryArtifactPaths(runDir, originalPath, hash string) (filename, fullPath string) {
	basename := filepath.Base(originalPath)
	filename = hash + "." + basename + ".binary"
	fullPath = filepath.Join(runDir, filename)
	return
}

// buildAndWriteProfile parses perf script output into a pprof profile,
// rewrites mapping paths using localBinaries, verifies and corrects symbolization,
// then writes the result to outputPath and logs a summary.
func buildAndWriteProfile(logger zerolog.Logger, scriptOutput io.Reader, localBinaries map[string]string, outputPath string, binaryCount int, historyID string) error {
	parser := perfscript.New()
	prof, err := parser.Parse(scriptOutput)
	if err != nil {
		return fmt.Errorf("failed to parse perf script: %w", err)
	}

	for _, mapping := range prof.Mapping {
		if newPath, ok := localBinaries[mapping.File]; ok {
			mapping.File = newPath
		}
	}

	// Check and correct symbolization before writing so any offset fixes are persisted.
	checkProfileSymbolization(logger, prof)

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create profile file: %w", err)
	}
	defer f.Close()

	if err := prof.Write(f); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	logger.Info().
		Str("profile", outputPath).
		Int("binaries", binaryCount).
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	shortID := historyID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	logger.Info().Msgf("View profile with: perfgo view %s", shortID)

	return nil
}

// ConvertPerfToPprof converts a local perf.data file to pprof format.
// The perf script output is written to a temporary file that is deleted after processing.
// Returns a list of binaries that were copied for artifact registration.
// Binaries are stored as <base32-sha256>.<basename>.binary in runDir.
func ConvertPerfToPprof(logger zerolog.Logger, perfDataPath string, outputPath string, runDir string, historyID string) ([]BinaryArtifact, error) {
	logger.Info().Str("input", perfDataPath).Str("output", outputPath).Msg("Processing performance data locally")

	scriptFile, err := runPerfScriptToFile(logger, func(w *os.File) error {
		cmd := exec.Command("perf", "script", "-i", perfDataPath)
		var stderr bytes.Buffer
		cmd.Stdout = w
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run perf script: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer cleanupPerfScriptFile(logger, scriptFile)

	binaryPaths, err := extractBinaryPaths(scriptFile)
	if err != nil {
		return nil, err
	}
	logger.Info().
		Int("count", len(binaryPaths)).
		Msg("Found binaries referenced in profile")

	localBinaries := make(map[string]string)
	var binaryArtifacts []BinaryArtifact
	if len(binaryPaths) > 0 {
		logger.Info().Msg("Processing local binaries")

		for _, binaryPath := range binaryPaths {
			// Skip special paths like [kernel.kallsyms], [vdso], etc.
			if strings.HasPrefix(binaryPath, "[") {
				continue
			}

			if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
				logger.Debug().
					Str("path", binaryPath).
					Msg("Binary not found, skipping")
				continue
			}

			hash, size, err := hashLocalBinary(binaryPath)
			if err != nil {
				logger.Warn().
					Err(err).
					Str("path", binaryPath).
					Msg("Failed to hash binary")
				continue
			}

			binaryFilename, destPath := binaryArtifactPaths(runDir, binaryPath, hash)
			if err := copyLocalBinary(binaryPath, destPath); err != nil {
				logger.Warn().
					Err(err).
					Str("src", binaryPath).
					Str("dest", destPath).
					Msg("Failed to copy binary")
				continue
			}

			localBinaries[binaryPath] = destPath
			binaryArtifacts = append(binaryArtifacts, BinaryArtifact{
				RemotePath: binaryPath,
				LocalPath:  binaryFilename,
				Size:       size,
				Hash:       hash,
			})

			logger.Debug().
				Str("original", binaryPath).
				Str("hash", hash).
				Str("dest", binaryFilename).
				Msg("Copied binary")
		}

		logger.Info().
			Int("count", len(localBinaries)).
			Msg("Binaries processed successfully")
	}

	if _, err := scriptFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to rewind perf script output: %w", err)
	}
	if err := buildAndWriteProfile(logger, scriptFile, localBinaries, outputPath, len(binaryArtifacts), historyID); err != nil {
		return nil, err
	}

	return binaryArtifacts, nil
}

// hashLocalBinary calculates the SHA256 hash of a local binary file.
// Returns the hash as base32-encoded lowercase string and the file size.
func hashLocalBinary(path string) (string, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return "", 0, fmt.Errorf("failed to hash file: %w", err)
	}
	hash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hasher.Sum(nil)))

	return hash, uint64(size), nil
}

// copyLocalBinary copies a local binary to a new location.
func copyLocalBinary(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	if _, err := io.Copy(destFile, srcFile); err != nil {
		destFile.Close()
		return fmt.Errorf("failed to copy file: %w", err)
	}
	if err := destFile.Close(); err != nil {
		return fmt.Errorf("failed to close destination file: %w", err)
	}

	return nil
}

// BinaryArtifact represents a binary that was copied for the profile.
type BinaryArtifact struct {
	RemotePath string // Original path on remote
	LocalPath  string // Filename in history directory (e.g., "ABC123...XYZ.binary")
	Size       uint64 // File size in bytes
	Hash       string // Base32-encoded SHA256 hash
}

// ProcessPerfData processes perf data from a remote host and creates a pprof profile.
// It resolves binary paths through /proc/<pid>/root for containerized processes.
// extraBinaries maps profile paths (as they appear in the perf output) to local file paths
// for binaries that are already available locally (e.g. a test binary built locally and
// deployed to the remote host). These are archived before the profile is written, so no
// later path-rewriting pass is needed.
// Returns a list of binaries that were copied for artifact registration.
// Binaries are stored as <base32-sha256>.<basename>.binary in runDir.
func ProcessPerfData(logger zerolog.Logger, sshClient *ssh.Client, remoteBaseDir string, outputPath string, runDir string, pids []string, extraBinaries map[string]string, historyID string) ([]BinaryArtifact, error) {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	logger.Info().
		Str("remote", remotePerfData).
		Msg("Processing performance data on remote host")

	scriptFile, err := runPerfScriptToFile(logger, func(w *os.File) error {
		perfScriptCmd := fmt.Sprintf("perf script -i %s", shellescape.Quote(remotePerfData))
		counter := &byteCounter{}
		var stderr bytes.Buffer
		started := time.Now()
		done := make(chan error, 1)
		go func() {
			done <- sshClient.Run(
				perfScriptCmd,
				ssh.WithStdOut(io.MultiWriter(w, counter)),
				ssh.WithStdErr(&stderr),
			)
		}()

		ticker := time.NewTicker(progressLogInterval)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				if err != nil {
					return fmt.Errorf("failed to run perf script remotely: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
				}
				logPerfScriptProgress(logger, counter.Load(), time.Since(started), "Remote perf script completed")
				return nil
			case <-ticker.C:
				logPerfScriptProgress(logger, counter.Load(), time.Since(started), "Processing remote perf script")
			}
		}
	})
	if err != nil {
		return nil, err
	}
	defer cleanupPerfScriptFile(logger, scriptFile)

	binaryPaths, err := extractBinaryPaths(scriptFile)
	if err != nil {
		return nil, err
	}
	logger.Info().
		Int("count", len(binaryPaths)).
		Msg("Found binaries referenced in profile")

	localBinaries := make(map[string]string)
	var binaryArtifacts []BinaryArtifact

	// Copy binaries from remote host via /proc/<pid>/root.
	if len(binaryPaths) > 0 && len(pids) > 0 {
		logger.Info().Msg("Copying binaries from remote host")

		for _, remotePath := range binaryPaths {
			// Skip special paths like [kernel.kallsyms], [vdso], etc.
			if strings.HasPrefix(remotePath, "[") {
				logger.Debug().Str("path", remotePath).Msg("Skipping special binary path")
				continue
			}

			// Try to find binary through /proc/<pid>/root for each PID.
			var foundProcPath string
			var foundPID string
			for _, pid := range pids {
				procPath := fmt.Sprintf("/proc/%s/root%s", pid, remotePath)
				logger.Debug().
					Str("remote", remotePath).
					Str("pid", pid).
					Str("proc_path", procPath).
					Msg("Checking binary through process root")
				checkCmd := fmt.Sprintf("test -f %s && echo exists", shellescape.Quote(procPath))
				output, _, err := sshClient.RunCommand(checkCmd)
				if err == nil && strings.TrimSpace(output) == "exists" {
					foundProcPath = procPath
					foundPID = pid
					logger.Debug().
						Str("remote", remotePath).
						Str("pid", pid).
						Str("proc_path", procPath).
						Msg("Found binary through process root")
					break
				}
			}

			if foundProcPath == "" {
				logger.Debug().
					Str("path", remotePath).
					Msg("Binary not found via /proc/pid/root for any PID, skipping")
				continue
			}

			logger.Debug().
				Str("remote", remotePath).
				Str("proc_path", foundProcPath).
				Msg("Hashing remote binary")
			hash, size, err := getRemoteBinaryHash(logger, sshClient, foundProcPath)
			if err != nil {
				logger.Warn().
					Err(err).
					Str("remote", remotePath).
					Str("proc_path", foundProcPath).
					Msg("Failed to get binary hash")
				continue
			}

			binaryFilename, localPath := binaryArtifactPaths(runDir, remotePath, hash)
			cached := reuseCachedBinary(logger, runDir, binaryFilename, localPath, hash, size)
			if !cached {
				logger.Debug().
					Str("remote", remotePath).
					Str("proc_path", foundProcPath).
					Str("local", localPath).
					Uint64("size_bytes", size).
					Str("hash", hash).
					Msg("Copying remote binary")
				if err := copyBinaryFromRemote(logger, sshClient, foundProcPath, localPath, hash, size); err != nil {
					logger.Warn().
						Err(err).
						Str("remote", remotePath).
						Str("proc_path", foundProcPath).
						Msg("Failed to copy binary")
					continue
				}
			}

			localBinaries[remotePath] = localPath
			binaryArtifacts = append(binaryArtifacts, BinaryArtifact{
				RemotePath: remotePath,
				LocalPath:  binaryFilename,
				Size:       size,
				Hash:       hash,
			})

			logger.Debug().
				Str("remote", remotePath).
				Str("pid", foundPID).
				Str("proc_path", foundProcPath).
				Str("hash", hash).
				Str("local", binaryFilename).
				Msg("Copied binary")
		}

		logger.Info().
			Int("count", len(localBinaries)).
			Msg("Binaries copied successfully")
	}

	// Archive locally-available binaries (e.g. test binary built locally and deployed remotely).
	for profilePath, localPath := range extraBinaries {
		if strings.HasPrefix(profilePath, "[") {
			continue
		}

		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			logger.Debug().
				Str("path", localPath).
				Msg("Extra binary not found locally, skipping")
			continue
		}

		hash, size, err := hashLocalBinary(localPath)
		if err != nil {
			logger.Warn().
				Err(err).
				Str("path", localPath).
				Msg("Failed to hash extra binary")
			continue
		}

		binaryFilename, destPath := binaryArtifactPaths(runDir, profilePath, hash)
		if err := copyLocalBinary(localPath, destPath); err != nil {
			logger.Warn().
				Err(err).
				Str("src", localPath).
				Str("dest", destPath).
				Msg("Failed to copy extra binary")
			continue
		}

		localBinaries[profilePath] = destPath
		binaryArtifacts = append(binaryArtifacts, BinaryArtifact{
			RemotePath: profilePath,
			LocalPath:  binaryFilename,
			Size:       size,
			Hash:       hash,
		})

		logger.Debug().
			Str("profile_path", profilePath).
			Str("local", localPath).
			Str("dest", binaryFilename).
			Msg("Archived extra binary")
	}

	if _, err := scriptFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to rewind perf script output: %w", err)
	}
	if err := buildAndWriteProfile(logger, scriptFile, localBinaries, outputPath, len(binaryArtifacts), historyID); err != nil {
		return nil, err
	}

	return binaryArtifacts, nil
}

// extractBinaryPaths extracts unique binary paths from perf script output.
func extractBinaryPaths(scriptOutput io.Reader) ([]string, error) {
	binarySet := make(map[string]bool)
	scanner := bufio.NewScanner(scriptOutput)

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
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read perf script output: %w", err)
	}
	return binaries, nil
}

// getRemoteBinaryHash calculates SHA256 hash of a remote binary.
// Returns the hash as base32-encoded lowercase string and the file size.
func getRemoteBinaryHash(logger zerolog.Logger, sshClient *ssh.Client, remotePath string) (string, uint64, error) {
	// Get SHA256 hash from remote system
	quotedPath := shellescape.Quote(remotePath)
	hashCmd := fmt.Sprintf("sha256sum %s | cut -d' ' -f1", quotedPath)
	hashOutput, _, err := sshClient.RunCommand(hashCmd)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get remote hash: %w", err)
	}

	// Parse hex hash
	hexHash := strings.TrimSpace(hashOutput)
	if len(hexHash) != 64 {
		return "", 0, fmt.Errorf("invalid hash length: expected 64, got %d", len(hexHash))
	}

	// Convert hex to bytes
	hashBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		if _, err := fmt.Sscanf(hexHash[i*2:i*2+2], "%02x", &hashBytes[i]); err != nil {
			return "", 0, fmt.Errorf("failed to parse hash byte %d: %w", i, err)
		}
	}

	// Encode as base32
	hashStr := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hashBytes))

	// Get file size
	sizeCmd := fmt.Sprintf("stat -c%%s %s 2>/dev/null || stat -f%%z %s", quotedPath, quotedPath)
	sizeOutput, _, err := sshClient.RunCommand(sizeCmd)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get file size: %w", err)
	}

	var size uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(sizeOutput), "%d", &size); err != nil {
		return "", 0, fmt.Errorf("failed to parse file size: %w", err)
	}

	logger.Debug().
		Str("remote", remotePath).
		Uint64("size_bytes", size).
		Str("hash", hashStr).
		Msg("Hashed remote binary")

	return hashStr, size, nil
}

type remoteBinaryCopier interface {
	CopyFromRemote(remotePath string, dst io.Writer) error
}

type byteCounter struct {
	bytes atomic.Uint64
}

func (c *byteCounter) Write(data []byte) (int, error) {
	c.bytes.Add(uint64(len(data)))
	return len(data), nil
}

func (c *byteCounter) Load() uint64 {
	return c.bytes.Load()
}

func formatBytes(size uint64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, suffix := range units {
		value /= unit
		if value < unit || suffix == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%d B", size)
}

func formatTransferRate(size uint64, elapsed time.Duration) string {
	if elapsed <= 0 {
		return "0 B/s"
	}
	return formatBytes(uint64(float64(size)/elapsed.Seconds())) + "/s"
}

func logPerfScriptProgress(logger zerolog.Logger, outputBytes uint64, elapsed time.Duration, message string) {
	logger.Info().
		Str("output", formatBytes(outputBytes)).
		Str("rate", formatTransferRate(outputBytes, elapsed)).
		Dur("elapsed", elapsed).
		Msg(message)
}

func logBinaryTransferProgress(logger zerolog.Logger, remotePath string, transferred, expected uint64, elapsed time.Duration, message string) {
	event := logger.Info().
		Str("remote", remotePath).
		Str("downloaded", formatBytes(transferred)).
		Str("total", formatBytes(expected)).
		Str("rate", formatTransferRate(transferred, elapsed)).
		Dur("elapsed", elapsed)
	if expected > 0 {
		event.Str("progress", fmt.Sprintf("%.1f%%", float64(transferred)*100/float64(expected)))
	}
	event.Msg(message)
}

// reuseCachedBinary links or copies a verified binary from an earlier history run.
func reuseCachedBinary(logger zerolog.Logger, runDir, binaryFilename, localPath, expectedHash string, expectedSize uint64) bool {
	historyDir := filepath.Dir(runDir)
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		logger.Debug().Err(err).Str("path", historyDir).Msg("Failed to search binary cache")
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == filepath.Base(runDir) {
			continue
		}
		candidate := filepath.Join(historyDir, entry.Name(), binaryFilename)
		info, err := os.Stat(candidate)
		if err != nil || uint64(info.Size()) != expectedSize {
			continue
		}

		hash, _, err := hashLocalBinary(candidate)
		if err != nil || hash != expectedHash {
			logger.Debug().
				Err(err).
				Str("candidate", candidate).
				Msg("Ignoring invalid cached binary")
			continue
		}

		if err := os.Link(candidate, localPath); err != nil {
			if err := copyLocalBinary(candidate, localPath); err != nil {
				logger.Debug().Err(err).Str("candidate", candidate).Msg("Failed to reuse cached binary")
				continue
			}
		}

		logger.Debug().
			Str("candidate", candidate).
			Str("local", localPath).
			Str("hash", expectedHash).
			Uint64("size_bytes", expectedSize).
			Msg("Reused cached binary")
		return true
	}

	return false
}

// copyBinaryFromRemote copies a binary from the remote host to local filesystem and verifies the hash.
func copyBinaryFromRemote(logger zerolog.Logger, sshClient remoteBinaryCopier, remotePath, localPath, expectedHash string, expectedSize uint64) error {
	tempFile, err := os.CreateTemp(filepath.Dir(localPath), "."+filepath.Base(localPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary binary file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	logger.Debug().
		Str("remote", remotePath).
		Str("local", localPath).
		Uint64("size_bytes", expectedSize).
		Msg("Streaming remote binary")

	hasher := sha256.New()
	counter := &byteCounter{}
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- sshClient.CopyFromRemote(remotePath, io.MultiWriter(tempFile, hasher, counter))
	}()

	ticker := time.NewTicker(progressLogInterval)
	defer ticker.Stop()

download:
	for {
		select {
		case err := <-done:
			if err != nil {
				tempFile.Close()
				return fmt.Errorf("failed to read remote binary: %w", err)
			}
			logBinaryTransferProgress(logger, remotePath, counter.Load(), expectedSize, time.Since(started), "Downloaded remote binary")
			break download
		case <-ticker.C:
			logBinaryTransferProgress(logger, remotePath, counter.Load(), expectedSize, time.Since(started), "Downloading remote binary")
		}
	}

	transferred := counter.Load()
	if transferred != expectedSize {
		tempFile.Close()
		return fmt.Errorf("size mismatch: expected %d bytes, got %d", expectedSize, transferred)
	}

	localHash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hasher.Sum(nil)))

	if localHash != expectedHash {
		tempFile.Close()
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, localHash)
	}

	if err := tempFile.Chmod(0755); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to make local binary executable: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close local binary: %w", err)
	}
	if err := os.Rename(tempPath, localPath); err != nil {
		return fmt.Errorf("failed to install local binary: %w", err)
	}

	logger.Debug().
		Str("path", localPath).
		Str("hash", localHash).
		Msg("Binary hash verified")

	return nil
}

// checkProfileSymbolization verifies that each non-kernel mapping can be
// symbolized correctly. For each mapping it picks one location whose function
// name is already known in the profile and checks whether the ELF symbol at
// the computed object address matches that name.
//
// If the names match, symbolization is already correct and nothing is changed.
//
// If they do not match, the ELF load base is added to the probe address and
// checked again. If that resolves the correct symbol, mapping.Offset is updated
// to the load base so the fix is persisted into the written profile, and an
// info message is logged.
//
// All other outcomes (binary inaccessible, stripped, address not found) are
// logged at debug level only.
func checkProfileSymbolization(logger zerolog.Logger, prof *profile.Profile) {
	for _, m := range prof.Mapping {
		if strings.HasPrefix(m.File, "[") {
			continue
		}

		// Find a representative address with a known function name in the profile.
		var testAddr uint64
		var expectedName string
		for _, loc := range prof.Location {
			if loc.Mapping != nil && loc.Mapping.ID == m.ID && loc.Address != 0 &&
				len(loc.Line) > 0 && loc.Line[0].Function != nil && loc.Line[0].Function.Name != "" {
				testAddr = loc.Address
				expectedName = loc.Line[0].Function.Name
				break
			}
		}
		if testAddr == 0 {
			continue
		}

		// pprof symbolizer computes: objAddr = addr - Start + Offset
		objAddr := testAddr - m.Start + m.Offset

		f, err := elf.Open(m.File)
		if err != nil {
			logger.Debug().
				Str("binary", filepath.Base(m.File)).
				Msg("mapping: binary not accessible, symbolization cannot be verified")
			continue
		}

		syms, symErr := f.Symbols()
		if symErr != nil {
			syms, symErr = f.DynamicSymbols()
		}

		// Extract load base while the file is open, to avoid reopening it.
		var loadBase uint64
		if f.Type == elf.ET_EXEC {
			for _, prog := range f.Progs {
				if prog.Type == elf.PT_LOAD {
					loadBase = prog.Vaddr
					break
				}
			}
		}
		f.Close()

		if symErr != nil {
			logger.Debug().
				Str("binary", filepath.Base(m.File)).
				Msg("mapping: no symbol table, symbolization cannot be verified")
			continue
		}

		// If the symbol at objAddr matches the expected name, symbolization is correct.
		if sym := elfFindSymbol(syms, objAddr); sym == expectedName {
			logger.Debug().
				Str("binary", filepath.Base(m.File)).
				Str("symbol", sym).
				Str("addr", fmt.Sprintf("0x%x", objAddr)).
				Msg("mapping: symbolization OK")
			continue
		}

		// Names did not match — try adding the ELF load base (fixes ET_EXEC binaries
		// whose mapping.Offset was not set during profile creation).
		if loadBase != 0 && loadBase != m.Offset {
			probeAddr := objAddr + loadBase
			if sym := elfFindSymbol(syms, probeAddr); sym == expectedName {
				m.Offset = loadBase
				logger.Info().
					Str("binary", filepath.Base(m.File)).
					Str("symbol", sym).
					Str("load_base", fmt.Sprintf("0x%x", loadBase)).
					Msg("mapping: corrected offset to ELF load base for symbolization")
				continue
			}
		}

		logger.Debug().
			Str("binary", filepath.Base(m.File)).
			Str("expected", expectedName).
			Str("addr", fmt.Sprintf("0x%x", objAddr)).
			Msg("mapping: cannot verify symbolization, address not found in symbol table")
	}
}

// elfFindSymbol returns the name of the function symbol whose range contains
// addr, or "" if none is found.
func elfFindSymbol(syms []elf.Symbol, addr uint64) string {
	for _, sym := range syms {
		if sym.Value != 0 && sym.Size != 0 && sym.Value <= addr && addr < sym.Value+sym.Size {
			return sym.Name
		}
	}
	return ""
}
