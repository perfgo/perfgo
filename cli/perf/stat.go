package perf

// stat.go contains utilities for building perf stat commands.

import (
	"fmt"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/urfave/cli/v2"
)

// StatOptions contains options for perf stat command.
type StatOptions struct {
	Events   []string // Events to measure
	PIDs     []string // Process IDs to attach to
	Duration int      // Duration in seconds (used with sleep)
	Binary   string   // Binary to execute (mutually exclusive with PIDs)
	Args     []string // Arguments for the binary
	Detail   bool     // Add detailed statistics (-d flag)
}

// BuildStatArgs builds perf stat command arguments for local execution.
func BuildStatArgs(opts StatOptions) []string {
	args := []string{"stat"}

	// Add detailed statistics flag
	if opts.Detail {
		args = append(args, "-d")
	}

	// Add events
	if len(opts.Events) > 0 {
		for _, event := range opts.Events {
			args = append(args, "-e", strings.TrimSpace(event))
		}
	}

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

// BuildStatCommand builds perf stat command string for remote execution.
// It reuses BuildStatArgs and joins the arguments with proper shell escaping.
func BuildStatCommand(opts StatOptions) string {
	args := BuildStatArgs(opts)

	// Build command with proper shell escaping
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "perf")

	for _, arg := range args {
		parts = append(parts, shellescape.Quote(arg))
	}

	return strings.Join(parts, " ")
}

// StatEventFlag returns the event flag for perf stat (multiple events).
func StatEventFlag() cli.Flag {
	return &cli.StringSliceFlag{
		Name:    "event",
		Aliases: []string{"e"},
		Usage:   "Event to measure (can be specified multiple times)",
	}
}

// StatDetailFlag returns the detail flag for perf stat.
func StatDetailFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "detail",
		Usage: "Add detailed statistics (-d flag to perf)",
		Value: true,
	}
}

// DurationFlag returns the duration flag for performance data collection.
func DurationFlag() cli.Flag {
	return &cli.IntFlag{
		Name:    "duration",
		Aliases: []string{"d"},
		Usage:   "Duration in seconds to gather performance data",
		Value:   10,
	}
}
