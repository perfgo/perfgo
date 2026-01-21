package perf

// stat.go contains utilities for building perf stat commands.

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// StatOptions contains options for perf stat command.
type StatOptions struct {
	Events   []string // Events to measure
	PIDs     []string // Process IDs to attach to
	Duration int      // Duration in seconds (used with sleep)
	Binary   string   // Binary to execute (mutually exclusive with PIDs)
	Args     []string // Arguments for the binary
}

// BuildStatArgs builds perf stat command arguments for local execution.
func BuildStatArgs(opts StatOptions) []string {
	args := []string{"stat"}

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
func BuildStatCommand(opts StatOptions) string {
	cmd := "perf stat"

	// Add events
	if len(opts.Events) > 0 {
		for _, event := range opts.Events {
			cmd += fmt.Sprintf(" -e %s", strings.TrimSpace(event))
		}
	}

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

// StatEventFlag returns the event flag for perf stat (multiple events).
func StatEventFlag() cli.Flag {
	return &cli.StringSliceFlag{
		Name:    "event",
		Aliases: []string{"e"},
		Usage:   "Event to measure (can be specified multiple times)",
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
