package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/perfgo/perfgo/cli/perf"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

const AppName = "perfgo"
const defaultPerfImage = "docker.io/simonswine/perf-image@sha256:877dd618f656feb8ac79bca1bbbd2c1b4103adb643b819f9b2cd8559eb12bf01"

type App struct {
	logger zerolog.Logger
	cli    *cli.App
}

func New() *App {

	// Set default log level to info
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	logger :=
		log.Output(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339Nano,
		})

	app := &App{
		logger: logger,
		cli: &cli.App{
			Name: AppName,
			Authors: []*cli.Author{
				{Name: "Christian Simon", Email: fmt.Sprintf("simon+%s@swine.de", AppName)},
			},
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "verbose",
					Usage: "Enable verbose (debug) logging",
				},
			},
			Before: func(ctx *cli.Context) error {
				if ctx.Bool("verbose") {
					zerolog.SetGlobalLevel(zerolog.DebugLevel)
				}
				return nil
			},
		},
	}
	app.cli.Commands = append(app.cli.Commands, &cli.Command{
		Name:  "test",
		Usage: "Run Go tests with optional perf integration",
		Subcommands: []*cli.Command{
			{
				Name:   "default",
				Usage:  "Run tests without perf (default behavior)",
				Action: app.testDefault,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "remote-host",
						Usage: "SSH host to run tests on (will auto-detect OS and architecture)",
					},
					&cli.BoolFlag{
						Name:  "keep",
						Usage: "Keep remote artifacts (don't clean up after test execution)",
					},
				},
			},
			{
				Name:   "stat",
				Usage:  "Run tests with perf stat",
				Action: app.testStat,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "remote-host",
						Usage: "SSH host to run tests on (will auto-detect OS and architecture)",
					},
					&cli.BoolFlag{
						Name:  "keep",
						Usage: "Keep remote artifacts (don't clean up after test execution)",
					},
					perf.StatEventFlag(),
					perf.StatDetailFlag(),
				},
			},
			{
				Name:   "profile",
				Usage:  "Run tests with perf record and generate pprof profile",
				Action: app.testProfile,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "remote-host",
						Usage: "SSH host to run tests on (will auto-detect OS and architecture)",
					},
					&cli.BoolFlag{
						Name:  "keep",
						Usage: "Keep remote artifacts (don't clean up after test execution)",
					},
					perf.ProfileEventFlag(),
					perf.ProfileCountFlag(),
				},
			},
			{
				Name:    "c2c",
				Aliases: []string{"cache-to-cache"},
				Usage:   "Run tests with perf c2c to detect cache contention and false sharing",
				Action:  app.testC2C,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "remote-host",
						Usage: "SSH host to run tests on (will auto-detect OS and architecture)",
					},
					&cli.BoolFlag{
						Name:  "keep",
						Usage: "Keep remote artifacts (don't clean up after test execution)",
					},
				},
			},
		},
		// Default action when no subcommand is specified
		Action: app.testDefault,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "remote-host",
				Usage: "SSH host to run tests on (will auto-detect OS and architecture)",
			},
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "Keep remote artifacts (don't clean up after test execution)",
			},
		},
	})
	app.cli.Commands = append(app.cli.Commands, &cli.Command{
		Name:   "list",
		Usage:  "List previous test runs",
		Action: app.list,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "path",
				Aliases: []string{"p"},
				Usage:   "Filter by relative path (e.g., examples/false-sharing)",
			},
			&cli.IntFlag{
				Name:    "limit",
				Aliases: []string{"n"},
				Usage:   "Limit number of results (default: 20)",
				Value:   20,
			},
		},
	})
	app.cli.Commands = append(app.cli.Commands, &cli.Command{
		Name:            "view",
		Usage:           "View test results from history",
		ArgsUsage:       "[ID|INDEX]",
		Action:          app.view,
		SkipFlagParsing: true,
		Description: `View test results from history.

Arguments:
  0           View last test run (default)
  -1          View 2nd last test run
  -2          View 3rd last test run
  <hex-id>    View test run matching the hex ID prefix

Examples:
  perfgo view           # View last test run
  perfgo view -1        # View 2nd last test run
  perfgo view -2        # View 3rd last test run
  perfgo view abc123    # View test run with ID starting with abc123

Display Priority:
  1. Protobuf profiles (perf.pb.gz)
  2. Perf stat outputs
  3. Test stdout/stderr
  4. Binaries (not displayed, only listed)`,
	})
	app.cli.Commands = append(app.cli.Commands, &cli.Command{
		Name:  "attach",
		Usage: "Attach performance profiling to Kubernetes pods or nodes",
		Subcommands: []*cli.Command{
			{
				Name:   "stat",
				Usage:  "Run perf stat on a pod or node",
				Action: app.attachStat,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "context",
						Usage: "Kubernetes context to use",
					},
					&cli.StringFlag{
						Name:  "pod",
						Usage: "Pod name to attach to (mutually exclusive with --node)",
					},
					&cli.StringFlag{
						Name:  "node",
						Usage: "Node name to attach to (mutually exclusive with --pod)",
					},
					&cli.StringFlag{
						Name:    "namespace",
						Aliases: []string{"n"},
						Usage:   "Kubernetes namespace (for pods, default: default)",
					},
					perf.StatEventFlag(),
					perf.StatDetailFlag(),
					&cli.StringFlag{
						Name:  "perf-image",
						Usage: "Container image for running perf",
						Value: defaultPerfImage,
					},
					perf.DurationFlag(),
				},
			},
			{
				Name:   "profile",
				Usage:  "Run perf record on a pod or node and generate pprof profile",
				Action: app.attachProfile,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "context",
						Usage: "Kubernetes context to use",
					},
					&cli.StringFlag{
						Name:  "pod",
						Usage: "Pod name to attach to (mutually exclusive with --pod)",
					},
					&cli.StringFlag{
						Name:  "node",
						Usage: "Node name to attach to (mutually exclusive with --pod)",
					},
					&cli.StringFlag{
						Name:    "namespace",
						Aliases: []string{"n"},
						Usage:   "Kubernetes namespace (for pods, default: default)",
					},
					perf.ProfileEventFlag(),
					perf.ProfileCountFlag(),
					&cli.StringFlag{
						Name:  "perf-image",
						Usage: "Container image for running perf",
						Value: defaultPerfImage,
					},
					perf.DurationFlag(),
				},
			},
			{
				Name:    "c2c",
				Aliases: []string{"cache-to-cache"},
				Usage:   "Run perf c2c on a pod or node to detect cache contention",
				Action:  app.attachC2C,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "context",
						Usage: "Kubernetes context to use",
					},
					&cli.StringFlag{
						Name:  "pod",
						Usage: "Pod name to attach to (mutually exclusive with --node)",
					},
					&cli.StringFlag{
						Name:  "node",
						Usage: "Node name to attach to (mutually exclusive with --pod)",
					},
					&cli.StringFlag{
						Name:    "namespace",
						Aliases: []string{"n"},
						Usage:   "Kubernetes namespace (for pods, default: default)",
					},
					&cli.StringFlag{
						Name:  "perf-image",
						Usage: "Container image for running perf",
						Value: defaultPerfImage,
					},
					perf.DurationFlag(),
				},
			},
			{
				Name:   "shell",
				Usage:  "Open an interactive shell in a privileged pod on the same node as the target pod",
				Action: app.attachShell,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "context",
						Usage: "Kubernetes context to use",
					},
					&cli.StringFlag{
						Name:  "pod",
						Usage: "Pod name to attach to (mutually exclusive with --node)",
					},
					&cli.StringFlag{
						Name:  "node",
						Usage: "Node name to attach to (mutually exclusive with --pod)",
					},
					&cli.StringFlag{
						Name:    "namespace",
						Aliases: []string{"n"},
						Usage:   "Kubernetes namespace (for pods, default: default)",
					},
					&cli.StringFlag{
						Name:  "perf-image",
						Usage: "Container image for running perf",
						Value: defaultPerfImage,
					},
				},
			},
		},
	})
	return app
}

func (a *App) Run(args []string) error {
	return a.cli.Run(args)
}

// SetVersion sets the version information for the CLI application
func (a *App) SetVersion(version, commit, date string) {
	a.cli.Version = version
	if commit != "none" && commit != "" {
		a.cli.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit[:8], date)
	}
}

func (a *App) testDefault(ctx *cli.Context) error {
	return a.runTest(ctx, "")
}

func (a *App) testStat(ctx *cli.Context) error {
	return a.runTest(ctx, "stat")
}

func (a *App) testProfile(ctx *cli.Context) error {
	return a.runTest(ctx, "profile")
}

func (a *App) testC2C(ctx *cli.Context) error {
	return a.runTest(ctx, "c2c")
}

func (a *App) runTest(ctx *cli.Context, perfMode string) error {
	startTime := time.Now()

	remoteHost := ctx.String("remote-host")
	keepArtifacts := ctx.Bool("keep")

	var perfEvent string
	var perfCount int
	var perfEvents []string
	var perfDetail bool
	var c2cEvent string
	var c2cCount int
	var c2cReportMode string
	var c2cShowAll bool

	if perfMode == "profile" {
		perfEvent = ctx.String("event")
		perfCount = ctx.Int("count")
	} else if perfMode == "stat" {
		perfEvents = ctx.StringSlice("event")
		perfDetail = ctx.Bool("detail")
	} else if perfMode == "c2c" {
		// Use default values for c2c
		c2cReportMode = "stdio"
		c2cShowAll = false
	}

	// Get additional arguments passed after flags (or after --)
	testArgs := ctx.Args().Slice()

	// Generate random 16-byte ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return fmt.Errorf("failed to generate test run ID: %w", err)
	}
	runID := hex.EncodeToString(idBytes)

	// Prepare history recording
	history := &model.History{
		ID:        runID,
		Type:      model.HistoryTypeTest,
		Timestamp: startTime,
		Args:      os.Args,
		Test:      &model.TestRun{},
	}

	// Track test binary path for artifact saving
	var testBinaryPath string
	// Track stdout and stderr content
	var stdoutContent, stderrContent string

	// Capture working directory
	if cwd, err := os.Getwd(); err == nil {
		history.WorkDir = cwd
	}

	// Capture git info (non-fatal if it fails)
	if commit, branch, err := a.getGitInfo(); err == nil {
		history.Git = &model.Git{
			Commit: commit,
			Branch: branch,
		}
	}

	// Create history directory early so artifacts can be written directly to it
	runDir, err := a.prepareHistoryDir(history)
	if err != nil {
		return fmt.Errorf("failed to prepare history directory: %w", err)
	}

	// Track final exit code
	var finalErr error
	defer func() {
		history.Duration = time.Since(startTime)
		if finalErr != nil {
			if exitErr, ok := finalErr.(*exec.ExitError); ok {
				history.ExitCode = exitErr.ExitCode()
			} else {
				history.ExitCode = 1
			}
		} else {
			history.ExitCode = 0
		}

		// Record the history (non-fatal if it fails)
		if err := a.recordHistory(history, runDir, testBinaryPath, stdoutContent, stderrContent); err != nil {
			a.logger.Warn().Err(err).Msg("Failed to record history")
		}

		// Clean up test binary after recording
		if testBinaryPath != "" {
			if err := os.Remove(testBinaryPath); err != nil {
				a.logger.Debug().Err(err).Str("binary", testBinaryPath).Msg("Failed to clean up test binary")
			}
		}
	}()

	if len(testArgs) > 0 {
		a.logger.Debug().Strs("args", testArgs).Msg("Additional test arguments")
	}

	// Validate that the first argument (if present) is a valid path or pattern
	if len(testArgs) < 1 {
		return fmt.Errorf("no package path specified: please provide a test path that resolves into a single package (e.g., '.' or './pkg/example')")
	}

	// the first args, always needs to be the test path
	if err := a.validateTestPath(testArgs[0]); err != nil {
		return err
	}

	// remove -- if given as separator
	if len(testArgs) > 1 && testArgs[1] == "--" {
		testArgs = append(testArgs[:1], testArgs[2:]...)
	}

	// Separate build args from runtime args
	buildArgs, runtimeArgs := a.separateTestArgs(testArgs)

	if len(buildArgs) > 0 {
		a.logger.Debug().Strs("build_args", buildArgs).Msg("Build-time arguments")
	}
	if len(runtimeArgs) > 0 {
		a.logger.Debug().Strs("runtime_args", runtimeArgs).Msg("Runtime arguments")
	}

	if remoteHost != "" {
		a.logger.Info().Str("host", remoteHost).Msg("Connecting to remote host")

		// Create SSH client for remote operations
		sshClient, err := ssh.New(a.logger, remoteHost)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to setup SSH connection")
			return err
		}
		defer sshClient.Close()

		remoteOS, remoteArch, err := sshClient.DetectSystem()
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to detect remote system")
			return err
		}

		// Store target information
		history.Target = &model.Target{
			RemoteHost: remoteHost,
			OS:         remoteOS,
			Arch:       remoteArch,
		}

		a.logger.Info().
			Str("os", remoteOS).
			Str("arch", remoteArch).
			Msg("Detected remote system")

		// Build test binary for remote system
		testBinary, err := a.buildTestBinary(remoteOS, remoteArch, buildArgs)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to build test binary")
			return err
		}
		testBinaryPath = testBinary

		a.logger.Info().Str("binary", testBinary).Msg("Test binary built successfully")

		// Get remote base directory for this repository
		remoteBaseDir, err := sshClient.GetRemoteRepositoryDir()
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to determine remote repository directory")
			return err
		}

		a.logger.Debug().Str("remoteBaseDir", remoteBaseDir).Msg("Using remote base directory")

		// Sync current directory to remote host
		remoteDir, err := sshClient.SyncDirectoryToRemote(remoteBaseDir)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to sync directory to remote host")
			return err
		}

		a.logger.Info().
			Str("local", ".").
			Str("remote", remoteDir).
			Msg("Directory synced to remote host")

		// Copy test binary to remote host
		remotePath, err := sshClient.CopyBinaryToRemote(testBinary, remoteBaseDir)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to copy binary to remote host")
			return err
		}

		a.logger.Info().
			Str("local", testBinary).
			Str("remote", remotePath).
			Msg("Test binary copied to remote host")

		// Clean up remote base directory after execution (unless --keep is specified)
		if !keepArtifacts {
			defer func() {
				// Clean up the entire base directory (includes working tree and binary)
				cleanupCmd := fmt.Sprintf("rm -rf %s", remoteBaseDir)
				if _, _, err := sshClient.RunCommand(cleanupCmd); err != nil {
					a.logger.Warn().Err(err).Str("path", remoteBaseDir).Msg("Failed to clean up remote base directory")
				} else {
					a.logger.Debug().Str("path", remoteBaseDir).Msg("Remote base directory cleaned up")
				}
			}()
		} else {
			a.logger.Info().
				Str("path", remoteBaseDir).
				Msg("Keeping remote artifacts (cleanup skipped)")
		}

		// Determine the package path to run tests in
		packagePath := a.getPackagePath(buildArgs)

		a.logger.Debug().Str("package", packagePath).Msg("Determined package path")

		// Execute the test binary remotely in the synced directory
		a.logger.Info().Str("path", remotePath).Msg("Executing tests on remote host")

		// Transform runtime args to use -test. prefix
		transformedArgs := a.transformTestFlags(runtimeArgs)

		if perfMode == "profile" {
			recordOpts := &perf.RecordOptions{
				Event: perfEvent,
				Count: perfCount,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				Record: &model.PerfRecord{
					Event: perfEvent,
					Count: perfCount,
				},
			}

			err := a.executeRemoteTestInDir(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, recordOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}

			// Copy back and process perf.data
			profilePath := filepath.Join(runDir, "perf.pb.gz")
			binaryArtifacts, err := perf.ProcessPerfData(a.logger, sshClient, remoteBaseDir, profilePath, runDir, nil, history.ID)
			if err != nil {
				a.logger.Error().Err(err).Msg("Failed to process performance data")
				finalErr = err
				return err
			}

			// Register binary artifacts
			for _, binArtifact := range binaryArtifacts {
				history.Artifacts = append(history.Artifacts, model.Artifact{
					Type: model.ArtifactTypeTestBinary,
					Size: binArtifact.Size,
					File: binArtifact.LocalPath,
				})
			}

			// Profile is written directly to history directory
		} else if perfMode == "stat" {
			var events []string
			if len(perfEvents) > 0 {
				events = perfEvents
			}
			statOpts := perf.StatOptions{
				Events: events,
				Detail: perfDetail,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				Stat: &model.PerfStat{
					Events: events,
					Detail: perfDetail,
				},
			}

			err := a.executeRemoteTestInDirWithStatOptions(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, statOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}
		} else if perfMode == "c2c" {
			c2cOpts := perf.C2COptions{
				Event: c2cEvent,
				Count: c2cCount,
			}

			reportOpts := perf.C2CReportOptions{
				Mode:    c2cReportMode,
				ShowAll: c2cShowAll,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				C2C: &model.PerfC2C{
					Event:      c2cEvent,
					Count:      c2cCount,
					ReportMode: c2cReportMode,
					ShowAll:    c2cShowAll,
				},
			}

			err := a.executeRemoteTestInDirWithC2COptions(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, c2cOpts, reportOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}

			// Process perf c2c data and generate report
			reportFilename, err := perf.ProcessC2CData(a.logger, sshClient, remoteBaseDir, runDir, reportOpts, history.ID)
			if err != nil {
				a.logger.Error().Err(err).Msg("Failed to process c2c data")
				finalErr = err
				return err
			}

			// Get report file size
			reportPath := filepath.Join(runDir, reportFilename)
			if reportInfo, err := os.Stat(reportPath); err == nil {
				history.Artifacts = append(history.Artifacts, model.Artifact{
					Type: model.ArtifactTypePerfC2CReport,
					Size: uint64(reportInfo.Size()),
					File: reportFilename,
				})
			}
		} else {
			err := a.executeRemoteTestInDir(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, nil, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}
		}
	} else {
		// Local test execution
		a.logger.Info().Msg("Running tests locally")

		// Capture local OS and architecture
		history.Target = &model.Target{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		}

		testBinary, err := a.buildTestBinary("", "", buildArgs)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to build test binary")
			return err
		}
		testBinaryPath = testBinary

		a.logger.Info().Str("binary", testBinary).Msg("Test binary built successfully")

		// Execute the test binary locally
		a.logger.Info().Str("path", testBinary).Msg("Executing tests locally")

		// Transform runtime args to use -test. prefix for local execution too
		transformedArgs := a.transformTestFlags(runtimeArgs)

		if perfMode == "profile" {
			recordOpts := &perf.RecordOptions{
				Event: perfEvent,
				Count: perfCount,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				Record: &model.PerfRecord{
					Event: perfEvent,
					Count: perfCount,
				},
			}

			err := a.executeLocalTest(testBinary, recordOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}

			// Process perf.data
			profilePath := filepath.Join(runDir, "perf.pb.gz")
			binaryArtifacts, err := perf.ConvertPerfToPprof(a.logger, "perf.data", profilePath, runDir, history.ID)
			if err != nil {
				a.logger.Error().Err(err).Msg("Failed to convert performance data to pprof")
				finalErr = err
				return err
			}

			// Register binary artifacts
			for _, binArtifact := range binaryArtifacts {
				history.Artifacts = append(history.Artifacts, model.Artifact{
					Type: model.ArtifactTypeTestBinary,
					Size: binArtifact.Size,
					File: binArtifact.LocalPath,
				})
			}

			// Profile is written directly to history directory
		} else if perfMode == "stat" {
			var events []string
			if len(perfEvents) > 0 {
				events = perfEvents
			}
			statOpts := perf.StatOptions{
				Events: events,
				Detail: perfDetail,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				Stat: &model.PerfStat{
					Events: events,
					Detail: perfDetail,
				},
			}

			err := a.executeLocalTestWithStatOptions(testBinary, statOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}
		} else if perfMode == "c2c" {
			c2cOpts := perf.C2COptions{
				Event:      c2cEvent,
				Count:      c2cCount,
				OutputPath: "perf.data",
			}

			reportOpts := perf.C2CReportOptions{
				Mode:    c2cReportMode,
				ShowAll: c2cShowAll,
			}

			// Store perf options in history
			history.Perf = &model.Perf{
				C2C: &model.PerfC2C{
					Event:      c2cEvent,
					Count:      c2cCount,
					ReportMode: c2cReportMode,
					ShowAll:    c2cShowAll,
				},
			}

			err := a.executeLocalTestWithC2COptions(testBinary, c2cOpts, reportOpts, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}

			// Convert perf.data to c2c report
			reportFilename, err := perf.ConvertPerfC2CToReport(a.logger, "perf.data", runDir, reportOpts, history.ID)
			if err != nil {
				a.logger.Error().Err(err).Msg("Failed to generate c2c report")
				finalErr = err
				return err
			}

			// Get report file size and register artifact
			reportPath := filepath.Join(runDir, reportFilename)
			if reportInfo, err := os.Stat(reportPath); err == nil {
				history.Artifacts = append(history.Artifacts, model.Artifact{
					Type: model.ArtifactTypePerfC2CReport,
					Size: uint64(reportInfo.Size()),
					File: reportFilename,
				})
			}
		} else {
			err := a.executeLocalTest(testBinary, nil, transformedArgs, &stdoutContent, &stderrContent)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}
		}
	}

	return nil
}
