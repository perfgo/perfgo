package perf

// record.go contains utilities for building perf record commands and
// processing perf data to pprof profiles.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/perfscript"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// RecordOptions contains options for perf record command.
type RecordOptions struct {
	Event      string   // Event to record
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
func BuildRecordCommand(opts RecordOptions) string {
	cmd := "perf record -g --call-graph fp"

	// Add event
	if opts.Event != "" {
		cmd += fmt.Sprintf(" -e %s", opts.Event)
	}

	// Add output path
	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = "perf.data"
	}
	cmd += fmt.Sprintf(" -o %s", outputPath)

	// Add PIDs or binary execution
	if len(opts.PIDs) > 0 {
		pidList := strings.Join(opts.PIDs, ",")
		cmd += fmt.Sprintf(" -p %s", pidList)

		// When attaching to PIDs, use sleep for duration
		cmd += fmt.Sprintf(" sleep %d", opts.Duration)
	} else if opts.Binary != "" {
		cmd += " --"
		cmd += fmt.Sprintf(" %s", opts.Binary)

		// Append arguments with proper shell escaping
		for _, arg := range opts.Args {
			escapedArg := strings.ReplaceAll(arg, "'", "'\\''")
			cmd += fmt.Sprintf(" '%s'", escapedArg)
		}
	}

	return cmd
}

// ProfileEventFlag returns the event flag for perf record (single event).
func ProfileEventFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "event",
		Aliases: []string{"e"},
		Usage:   "Event to record (default: cycles:u)",
		Value:   "cycles:u",
	}
}

// ConvertPerfToPprof converts a local perf.data file to pprof format.
func ConvertPerfToPprof(logger zerolog.Logger, perfDataPath string) error {
	logger.Info().Str("input", perfDataPath).Msg("Processing performance data locally")

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

	logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return ParsePerfScript(logger, bytes.NewReader(output))
}

// ParsePerfScript parses perf script output and creates a pprof profile.
func ParsePerfScript(logger zerolog.Logger, scriptOutput io.Reader) error {
	logger.Info().Msg("Parsing perf script output")

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

	logger.Info().
		Str("profile", profileFile).
		Str("script", "perf.script").
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	logger.Info().Msgf("View profile with: go tool pprof %s", profileFile)

	return nil
}

// ProcessPerfData processes perf data from a remote host and creates a pprof profile.
func ProcessPerfData(logger zerolog.Logger, sshClient *ssh.Client, remoteBaseDir string) error {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	logger.Info().
		Str("remote", remotePerfData).
		Msg("Processing performance data on remote host")

	// Run perf script remotely and capture output
	perfScriptCmd := fmt.Sprintf("perf script -i %s", remotePerfData)
	scriptOutput, _, err := sshClient.RunCommand(perfScriptCmd)
	if err != nil {
		return fmt.Errorf("failed to run perf script remotely: %w", err)
	}

	// Save the perf script output locally
	scriptFile := "perf.script"
	if err := os.WriteFile(scriptFile, []byte(scriptOutput), 0644); err != nil {
		return fmt.Errorf("failed to write perf script output: %w", err)
	}

	logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return ParsePerfScript(logger, strings.NewReader(scriptOutput))
}
