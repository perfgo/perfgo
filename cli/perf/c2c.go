package perf

// c2c.go contains utilities for building perf c2c commands and
// processing c2c reports to detect cache contention and false sharing.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// C2COptions contains options for perf c2c record command.
type C2COptions struct {
	Event      string   // Event to record (platform-specific default if empty)
	Count      int      // Event period to sample (e.g., -c 1000000)
	PIDs       []string // Process IDs to attach to
	Duration   int      // Duration in seconds (used with sleep)
	OutputPath string   // Output file path (default: perf.data)
	Binary     string   // Binary to execute (mutually exclusive with PIDs)
	Args       []string // Arguments for the binary
}

// C2CReportOptions contains options for perf c2c report command.
type C2CReportOptions struct {
	InputPath string // Input perf.data file path
	Mode      string // Report mode: "stdio" (text) or "tui" (interactive)
	ShowAll   bool   // Show all captured entries (--show-all)
	NoSource  bool   // Don't show source line information (--no-source)
	ShowCPU   bool   // Show CPU information (--show-cpu-cnt)
}

// BuildC2CRecordArgs builds perf c2c record command arguments for local execution.
func BuildC2CRecordArgs(opts C2COptions) []string {
	args := []string{"c2c", "record"}

	// Add event if specified
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

// BuildC2CRecordCommand builds perf c2c record command string for remote execution.
// It reuses BuildC2CRecordArgs and joins the arguments with proper shell escaping.
func BuildC2CRecordCommand(opts C2COptions) string {
	args := BuildC2CRecordArgs(opts)

	// Build command with proper shell escaping
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "perf")

	for _, arg := range args {
		parts = append(parts, shellescape.Quote(arg))
	}

	return strings.Join(parts, " ")
}

// BuildC2CReportArgs builds perf c2c report command arguments for local execution.
func BuildC2CReportArgs(opts C2CReportOptions) []string {
	args := []string{"c2c", "report"}

	// Add input path
	if opts.InputPath != "" {
		args = append(args, "-i", opts.InputPath)
	}

	// Add report mode - default to stdio for text output
	if opts.Mode == "" || opts.Mode == "stdio" {
		args = append(args, "--stdio")
	}
	// Note: tui mode doesn't need a flag, it's the default interactive mode

	// Add show-all flag
	if opts.ShowAll {
		args = append(args, "--show-all")
	}

	// Add no-source flag
	if opts.NoSource {
		args = append(args, "--no-source")
	}

	// Add show-cpu flag
	if opts.ShowCPU {
		args = append(args, "--show-cpu-cnt")
	}

	return args
}

// BuildC2CReportCommand builds perf c2c report command string for remote execution.
func BuildC2CReportCommand(opts C2CReportOptions) string {
	args := BuildC2CReportArgs(opts)

	// Build command with proper shell escaping
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "perf")

	for _, arg := range args {
		parts = append(parts, shellescape.Quote(arg))
	}

	return strings.Join(parts, " ")
}

// C2CEventFlag returns the event flag for perf c2c record.
func C2CEventFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "c2c-event",
		Aliases: []string{"ce"},
		Usage:   "Event to record for c2c analysis (platform-specific default if not specified)",
	}
}

// C2CCountFlag returns the count flag for perf c2c record (event period).
func C2CCountFlag() cli.Flag {
	return &cli.IntFlag{
		Name:    "c2c-count",
		Aliases: []string{"cc"},
		Usage:   "Event period to sample for c2c (e.g., sample every N events)",
	}
}

// C2CReportModeFlag returns the report mode flag for perf c2c report.
func C2CReportModeFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "c2c-mode",
		Usage: "Report mode: stdio (text output) or tui (interactive)",
		Value: "stdio",
	}
}

// C2CShowAllFlag returns the show-all flag for perf c2c report.
func C2CShowAllFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "c2c-show-all",
		Usage: "Show all captured entries in the report",
		Value: false,
	}
}

// ConvertPerfC2CToReport converts a local perf.data file to a c2c report.
// The report is saved to c2c-report.txt in the runDir.
// Returns the artifact filename (relative to runDir).
func ConvertPerfC2CToReport(logger zerolog.Logger, perfDataPath string, runDir string, reportOpts C2CReportOptions, historyID string) (string, error) {
	logger.Info().Str("input", perfDataPath).Msg("Generating c2c report locally")

	// Set the input path
	reportOpts.InputPath = perfDataPath

	// Create output file for the report
	reportFilename := "c2c-report.txt"
	reportPath := fmt.Sprintf("%s/%s", runDir, reportFilename)
	reportFile, err := os.Create(reportPath)
	if err != nil {
		return "", fmt.Errorf("failed to create report file: %w", err)
	}
	defer reportFile.Close()

	// Build the perf c2c report command
	args := BuildC2CReportArgs(reportOpts)

	// Run perf c2c report locally and write to file
	cmd := exec.Command("perf", args...)
	cmd.Stdout = reportFile
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run perf c2c report: %w", err)
	}

	// Get file size for logging
	fileInfo, _ := reportFile.Stat()
	logger.Info().
		Int64("size_bytes", fileInfo.Size()).
		Str("report", reportPath).
		Msg("C2C report generated successfully")

	shortID := historyID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	logger.Info().Msgf("View report with: perfgo view %s", shortID)

	return reportFilename, nil
}

// ProcessC2CData processes perf c2c data from a remote host and creates a report.
// The report is saved to c2c-report.txt in the runDir.
// Returns the artifact filename (relative to runDir).
func ProcessC2CData(logger zerolog.Logger, sshClient *ssh.Client, remoteBaseDir string, runDir string, reportOpts C2CReportOptions, historyID string) (string, error) {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	logger.Info().
		Str("remote", remotePerfData).
		Msg("Generating c2c report on remote host")

	// Create output file for the report
	reportFilename := "c2c-report.txt"
	reportPath := fmt.Sprintf("%s/%s", runDir, reportFilename)
	reportFile, err := os.Create(reportPath)
	if err != nil {
		return "", fmt.Errorf("failed to create report file: %w", err)
	}
	defer reportFile.Close()

	// Set the remote input path
	reportOpts.InputPath = remotePerfData

	// Build the perf c2c report command for remote execution
	perfReportCmd := BuildC2CReportCommand(reportOpts)

	// Run perf c2c report remotely and stream output to file
	if err := sshClient.Run(perfReportCmd, ssh.WithStdOut(reportFile)); err != nil {
		return "", fmt.Errorf("failed to run perf c2c report remotely: %w", err)
	}

	// Get file size for logging
	fileInfo, _ := reportFile.Stat()
	logger.Info().
		Int64("size_bytes", fileInfo.Size()).
		Str("report", reportPath).
		Msg("C2C report generated successfully")

	shortID := historyID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	logger.Info().Msgf("View report with: perfgo view %s", shortID)

	return reportFilename, nil
}
