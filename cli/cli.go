package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/perfgo/perfgo/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

const AppName = "perfgo"

type App struct {
	logger zerolog.Logger
	cli    *cli.App
}

func New() *App {

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
					&cli.StringSliceFlag{
						Name:    "event",
						Aliases: []string{"e"},
						Usage:   "Event to measure (can be specified multiple times)",
					},
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
					&cli.StringFlag{
						Name:    "event",
						Aliases: []string{"e"},
						Usage:   "Event to record (default: cycles:u)",
						Value:   "cycles:u",
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
					&cli.StringSliceFlag{
						Name:    "event",
						Aliases: []string{"e"},
						Usage:   "Event to measure (can be specified multiple times)",
					},
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
						Name:    "event",
						Aliases: []string{"e"},
						Usage:   "Event to record (default: cycles:u)",
						Value:   "cycles:u",
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

func (a *App) testDefault(ctx *cli.Context) error {
	return a.runTest(ctx, "")
}

func (a *App) testStat(ctx *cli.Context) error {
	return a.runTest(ctx, "stat")
}

func (a *App) testProfile(ctx *cli.Context) error {
	return a.runTest(ctx, "profile")
}

func (a *App) runTest(ctx *cli.Context, perfMode string) error {
	startTime := time.Now()

	remoteHost := ctx.String("remote-host")
	keepArtifacts := ctx.Bool("keep")

	var perfEvent string
	var perfEvents []string

	if perfMode == "profile" {
		perfEvent = ctx.String("event")
	} else if perfMode == "stat" {
		perfEvents = ctx.StringSlice("event")
	}

	// Get additional arguments passed after flags (or after --)
	testArgs := ctx.Args().Slice()

	// Generate random 16-byte ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return fmt.Errorf("failed to generate test run ID: %w", err)
	}
	runID := hex.EncodeToString(idBytes)

	// Prepare test run recording
	testRun := &model.TestRun{
		ID:        runID,
		Timestamp: startTime,
		Args:      os.Args,
	}

	// Track test binary path for artifact saving
	var testBinaryPath string

	// Capture working directory
	if cwd, err := os.Getwd(); err == nil {
		testRun.WorkDir = cwd
	}

	// Capture git info (non-fatal if it fails)
	if commit, branch, err := a.getGitInfo(); err == nil {
		testRun.Commit = commit
		testRun.Branch = branch
	}

	// Track final exit code
	var finalErr error
	defer func() {
		testRun.Duration = time.Since(startTime)
		if finalErr != nil {
			if exitErr, ok := finalErr.(*exec.ExitError); ok {
				testRun.ExitCode = exitErr.ExitCode()
			} else {
				testRun.ExitCode = 1
			}
		} else {
			testRun.ExitCode = 0
		}

		// Record the test run (non-fatal if it fails)
		if err := a.recordTestRun(testRun, testBinaryPath); err != nil {
			a.logger.Warn().Err(err).Msg("Failed to record test run")
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

		// Store remote host information
		testRun.RemoteHost = remoteHost

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

		// Store OS and architecture information
		testRun.OS = remoteOS
		testRun.Arch = remoteArch

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
				if _, err := sshClient.RunCommand(cleanupCmd); err != nil {
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
			err := a.executeRemoteTestInDir(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, "profile", perfEvent, transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}

			// Copy back and process perf.data
			if err := a.processPerfData(sshClient, remoteBaseDir); err != nil {
				a.logger.Error().Err(err).Msg("Failed to process performance data")
				finalErr = err
				return err
			}

			// Note: artifacts will be saved after recordTestRun creates the directory
		} else if perfMode == "stat" {
			err := a.executeRemoteTestInDir(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, "stat", strings.Join(perfEvents, ","), transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}
		} else {
			err := a.executeRemoteTestInDir(sshClient, remotePath, remoteDir, remoteBaseDir, packagePath, "", "", transformedArgs, testRun)
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
		testRun.OS = runtime.GOOS
		testRun.Arch = runtime.GOARCH

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
			err := a.executeLocalTest(testBinary, "profile", perfEvent, transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}

			// Process perf.data
			if err := a.convertPerfToPprof("perf.data"); err != nil {
				a.logger.Error().Err(err).Msg("Failed to convert performance data to pprof")
				finalErr = err
				return err
			}

			// Note: artifacts will be saved after recordTestRun creates the directory
		} else if perfMode == "stat" {
			err := a.executeLocalTest(testBinary, "stat", strings.Join(perfEvents, ","), transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}
		} else {
			err := a.executeLocalTest(testBinary, "", "", transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Local test execution failed")
				finalErr = err
				return err
			}
		}
	}

	return nil
}
