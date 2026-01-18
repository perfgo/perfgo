package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"github.com/perfgo/perfgo/model"
	"github.com/perfgo/perfgo/perfscript"
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

		// Setup SSH multiplexing for all remote operations
		controlPath, err := a.setupSSHMultiplexing(remoteHost)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to setup SSH multiplexing")
			return err
		}
		defer a.cleanupSSHMultiplexing(remoteHost, controlPath)

		remoteOS, remoteArch, err := a.detectRemoteSystemWithControlPath(remoteHost, controlPath)
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
		remoteBaseDir, err := a.getRemoteRepositoryDir(remoteHost, controlPath)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to determine remote repository directory")
			return err
		}

		a.logger.Debug().Str("remoteBaseDir", remoteBaseDir).Msg("Using remote base directory")

		// Sync current directory to remote host
		remoteDir, err := a.syncDirectoryToRemote(remoteHost, controlPath, remoteBaseDir)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to sync directory to remote host")
			return err
		}

		a.logger.Info().
			Str("local", ".").
			Str("remote", remoteDir).
			Msg("Directory synced to remote host")

		// Copy test binary to remote host
		remotePath, err := a.copyBinaryToRemote(remoteHost, controlPath, testBinary, remoteBaseDir)
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
				if _, err := a.runRemoteCommand(remoteHost, controlPath, cleanupCmd); err != nil {
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
			err := a.executeRemoteTestInDir(remoteHost, controlPath, remotePath, remoteDir, remoteBaseDir, packagePath, "profile", perfEvent, transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}

			// Copy back and process perf.data
			if err := a.processPerfData(remoteHost, controlPath, remoteBaseDir); err != nil {
				a.logger.Error().Err(err).Msg("Failed to process performance data")
				finalErr = err
				return err
			}

			// Note: artifacts will be saved after recordTestRun creates the directory
		} else if perfMode == "stat" {
			err := a.executeRemoteTestInDir(remoteHost, controlPath, remotePath, remoteDir, remoteBaseDir, packagePath, "stat", strings.Join(perfEvents, ","), transformedArgs, testRun)
			if err != nil {
				a.logger.Error().Err(err).Msg("Remote test execution failed")
				finalErr = err
				return err
			}
		} else {
			err := a.executeRemoteTestInDir(remoteHost, controlPath, remotePath, remoteDir, remoteBaseDir, packagePath, "", "", transformedArgs, testRun)
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

func (a *App) processPerfData(host, controlPath, remoteBaseDir string) error {
	remotePerfData := fmt.Sprintf("%s/perf.data", remoteBaseDir)

	a.logger.Info().
		Str("remote", remotePerfData).
		Msg("Processing performance data on remote host")

	// Run perf script remotely and capture output
	perfScriptCmd := fmt.Sprintf("perf script -i %s", remotePerfData)
	scriptOutput, err := a.runRemoteCommand(host, controlPath, perfScriptCmd)
	if err != nil {
		return fmt.Errorf("failed to run perf script remotely: %w", err)
	}

	// Save the perf script output locally
	scriptFile := "perf.script"
	if err := os.WriteFile(scriptFile, []byte(scriptOutput), 0644); err != nil {
		return fmt.Errorf("failed to write perf script output: %w", err)
	}

	a.logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return a.parsePerfScript(strings.NewReader(scriptOutput))
}

func (a *App) convertPerfToPprof(perfDataPath string) error {
	a.logger.Info().Str("input", perfDataPath).Msg("Processing performance data locally")

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

	a.logger.Info().Str("file", scriptFile).Msg("Performance script output saved")

	// Parse and summarize the perf script output
	return a.parsePerfScript(bytes.NewReader(output))
}

func (a *App) parsePerfScript(scriptOutput io.Reader) error {
	a.logger.Info().Msg("Parsing perf script output")

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

	a.logger.Info().
		Str("profile", profileFile).
		Str("script", "perf.script").
		Int("samples", len(prof.Sample)).
		Int("functions", len(prof.Function)).
		Int("locations", len(prof.Location)).
		Msg("Performance profile created")

	a.logger.Info().Msgf("View profile with: go tool pprof %s", profileFile)

	return nil
}

func (a *App) getGitInfo() (commit, branch string, err error) {
	// Get current commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get git commit: %w", err)
	}
	commit = strings.TrimSpace(string(output))

	// Get current branch
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err = cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get git branch: %w", err)
	}
	branch = strings.TrimSpace(string(output))

	return commit, branch, nil
}

func (a *App) saveArtifacts(runDir string, testRun *model.TestRun, testBinaryPath string) error {
	cwd, _ := os.Getwd()
	
	// Save the test binary if provided
	if testBinaryPath != "" {
		if info, err := os.Stat(testBinaryPath); err == nil && !info.IsDir() {
			destBinary := filepath.Join(runDir, filepath.Base(testBinaryPath))
			if err := a.copyFile(testBinaryPath, destBinary); err != nil {
				a.logger.Warn().Err(err).Str("file", testBinaryPath).Msg("Failed to copy test binary")
			} else {
				testRun.Artifacts = append(testRun.Artifacts, model.Artifact{
					Type: model.ArtifactTypeTestBinary,
					Size: uint64(info.Size()),
					File: filepath.Base(testBinaryPath),
				})
				a.logger.Debug().Str("dest", destBinary).Msg("Saved test binary")
			}
		}
	}
	
	// Save perf.pb.gz profile if it exists
	profileFile := filepath.Join(cwd, "perf.pb.gz")
	if info, err := os.Stat(profileFile); err == nil {
		destProfile := filepath.Join(runDir, "perf.pb.gz")
		
		// Find the test binary to rewrite paths
		var destBinary string
		for _, artifact := range testRun.Artifacts {
			if artifact.Type == model.ArtifactTypeTestBinary {
				destBinary = filepath.Join(runDir, artifact.File)
				break
			}
		}
		
		if destBinary != "" {
			// Rewrite profile paths to point to saved binary
			if err := a.rewriteProfilePaths(profileFile, runDir, destBinary); err != nil {
				a.logger.Warn().Err(err).Msg("Failed to rewrite profile paths, copying original")
				// Fall back to copying original
				if err := a.copyFile(profileFile, destProfile); err != nil {
					return fmt.Errorf("failed to copy profile: %w", err)
				}
			}
		} else {
			// No binary found, just copy the profile
			if err := a.copyFile(profileFile, destProfile); err != nil {
				return fmt.Errorf("failed to copy profile: %w", err)
			}
		}
		
		testRun.Artifacts = append(testRun.Artifacts, model.Artifact{
			Type: model.ArtifactTypePprofProfile,
			Size: uint64(info.Size()),
			File: "perf.pb.gz",
		})
		a.logger.Debug().Str("dest", destProfile).Msg("Saved pprof profile")
	}
	
	return nil
}

func (a *App) rewriteProfilePaths(profileFile, runDir, destBinary string) error {
	// Read the profile
	f, err := os.Open(profileFile)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer f.Close()

	prof, err := profile.Parse(f)
	if err != nil {
		return fmt.Errorf("failed to parse profile: %w", err)
	}

	// Update all mappings to point to archived binary
	testBinaryBasename := filepath.Base(destBinary)
	for _, mapping := range prof.Mapping {
		// Skip kernel mappings
		if strings.HasPrefix(mapping.File, "[") {
			continue
		}
		
		// Check if this mapping is for the test binary
		mappingBasename := filepath.Base(mapping.File)
		if mappingBasename == testBinaryBasename {
			// Update to use archived path
			mapping.File = destBinary
			a.logger.Debug().
				Str("old", mapping.File).
				Str("new", destBinary).
				Msg("Updated mapping path")
		}
	}

	// Write updated profile
	destProfile := filepath.Join(runDir, "perf.pb.gz")
	outFile, err := os.Create(destProfile)
	if err != nil {
		return fmt.Errorf("failed to create output profile: %w", err)
	}
	defer outFile.Close()

	if err := prof.Write(outFile); err != nil {
		return fmt.Errorf("failed to write updated profile: %w", err)
	}

	return nil
}

func (a *App) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

func (a *App) getRemoteRepositoryDir(host, controlPath string) (string, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Get repository base name from directory
	repoBaseName := filepath.Base(cwd)

	// Get git repository root to create a stable hash
	gitRootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	gitRootOut, err := gitRootCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git root: %w", err)
	}
	gitRoot := strings.TrimSpace(string(gitRootOut))

	// Create a hash of the git root path for uniqueness
	hash := sha256.Sum256([]byte(gitRoot))
	pathHash := hex.EncodeToString(hash[:])[:8] // Use 8 chars for readability

	// Construct repository identifier
	repoIdent := fmt.Sprintf("%s-%s", repoBaseName, pathHash)

	// Get remote cache directory path
	cacheDir, err := a.getRemoteCacheDir(host, controlPath)
	if err != nil {
		return "", fmt.Errorf("failed to get remote cache directory: %w", err)
	}

	// Construct full path
	remoteBaseDir := fmt.Sprintf("%s/repositories/%s", cacheDir, repoIdent)

	return remoteBaseDir, nil
}

func (a *App) getRemoteCacheDir(host, controlPath string) (string, error) {
	// Query remote host for XDG_CACHE_HOME or default
	getCacheDirCmd := `
if [ -n "$XDG_CACHE_HOME" ]; then
    echo "$XDG_CACHE_HOME/perfgo"
elif [ -n "$HOME" ]; then
    echo "$HOME/.cache/perfgo"
else
    echo "/tmp/perfgo"
fi
`
	cacheDir, err := a.runRemoteCommand(host, controlPath, getCacheDirCmd)
	if err != nil {
		return "", fmt.Errorf("failed to determine remote cache directory: %w", err)
	}

	return strings.TrimSpace(cacheDir), nil
}

func (a *App) getPackagePath(buildArgs []string) string {
	// Look for package path in build args
	// Package paths typically look like: ./..., ./pkg/..., ./cmd/api, etc.
	// Default to current directory if not specified
	for _, arg := range buildArgs {
		// Check if it's a package path (not a flag)
		if !strings.HasPrefix(arg, "-") {
			// Clean up the path
			path := strings.TrimPrefix(arg, "./")
			path = strings.TrimSuffix(path, "/...")

			// If it's just "..." or empty, use current directory
			if path == "" || path == "..." {
				return "."
			}

			return path
		}
	}

	// Default to current directory
	return "."
}

func (a *App) separateTestArgs(args []string) (buildArgs, runtimeArgs []string) {
	// Build-only flags (used during go test -c)
	buildOnlyFlags := map[string]bool{
		"-tags":       true,
		"-race":       true,
		"-msan":       true,
		"-asan":       true,
		"-cover":      true,
		"-covermode":  true,
		"-coverpkg":   true,
		"-gcflags":    true,
		"-ldflags":    true,
		"-asmflags":   true,
		"-gccgoflags": true,
		"-mod":        true,
		"-modfile":    true,
		"-overlay":    true,
		"-pkgdir":     true,
		"-toolexec":   true,
		"-work":       true,
	}

	buildArgs = []string{}
	runtimeArgs = []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Skip package paths (they're only for build, not execution)
		if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
			arg == "..." || strings.Contains(arg, "/...") {
			buildArgs = append(buildArgs, arg)
			continue
		}

		// Check if it's a build-only flag
		if buildOnlyFlags[arg] {
			buildArgs = append(buildArgs, arg)
			// Some flags take a value, include it
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				buildArgs = append(buildArgs, args[i])
			}
			continue
		}

		// Check if it's a build-only flag with = syntax (e.g., -tags=foo)
		flagName := arg
		if idx := strings.Index(arg, "="); idx > 0 {
			flagName = arg[:idx]
		}
		if buildOnlyFlags[flagName] {
			buildArgs = append(buildArgs, arg)
			continue
		}

		// Everything else is a runtime arg
		runtimeArgs = append(runtimeArgs, arg)
	}

	return buildArgs, runtimeArgs
}

func (a *App) transformTestFlags(args []string) []string {
	transformed := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Skip if already has -test. prefix
		if strings.HasPrefix(arg, "-test.") {
			transformed = append(transformed, arg)
			continue
		}

		// Transform short flags to -test. prefix
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			// Handle -flag=value format
			if idx := strings.Index(arg, "="); idx > 0 {
				flagName := arg[1:idx] // Remove leading - and get flag name
				value := arg[idx:]     // Keep =value part
				transformed = append(transformed, fmt.Sprintf("-test.%s%s", flagName, value))
			} else {
				// Handle -flag format (might have separate value)
				flagName := arg[1:] // Remove leading -
				transformed = append(transformed, fmt.Sprintf("-test.%s", flagName))
			}
		} else {
			// Not a flag, keep as-is (could be a flag value)
			transformed = append(transformed, arg)
		}
	}

	return transformed
}

func (a *App) detectRemoteSystem(host string) (string, string, error) {
	// Setup SSH multiplexing
	controlPath, err := a.setupSSHMultiplexing(host)
	if err != nil {
		return "", "", fmt.Errorf("failed to setup SSH multiplexing: %w", err)
	}
	defer a.cleanupSSHMultiplexing(host, controlPath)

	return a.detectRemoteSystemWithControlPath(host, controlPath)
}

func (a *App) detectRemoteSystemWithControlPath(host, controlPath string) (string, string, error) {
	// Detect OS
	osName, err := a.runRemoteCommand(host, controlPath, "uname -s")
	if err != nil {
		return "", "", fmt.Errorf("failed to detect OS: %w", err)
	}

	// Detect architecture
	arch, err := a.runRemoteCommand(host, controlPath, "uname -m")
	if err != nil {
		return "", "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	// Normalize OS name to Go's GOOS format
	osName = strings.ToLower(strings.TrimSpace(osName))
	if osName == "darwin" {
		osName = "darwin"
	} else if osName == "linux" {
		osName = "linux"
	}

	// Normalize architecture to Go's GOARCH format
	arch = strings.TrimSpace(arch)
	switch arch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	case "i386", "i686":
		arch = "386"
	case "armv7l":
		arch = "arm"
	}

	return osName, arch, nil
}

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

func (a *App) executeLocalTest(binaryPath, perfMode, perfEvent string, args []string, testRun *model.TestRun) error {
	a.logger.Debug().
		Str("binary", binaryPath).
		Str("perfMode", perfMode).
		Str("perfEvent", perfEvent).
		Strs("args", args).
		Msg("Starting local test execution")

	var cmd *exec.Cmd

	if perfMode == "profile" {
		// Build perf command: perf record -g --call-graph fp -e <event> -o perf.data -- <binary> <args>
		perfArgs := []string{"record", "-g", "--call-graph", "fp", "-e", perfEvent, "-o", "perf.data", "--", binaryPath}
		perfArgs = append(perfArgs, args...)
		cmd = exec.Command("perf", perfArgs...)

		a.logger.Info().
			Str("event", perfEvent).
			Msg("Wrapping test execution with perf record")
	} else if perfMode == "stat" {
		// Build perf command: perf stat -e <events> -- <binary> <args>
		perfArgs := []string{"stat"}
		if perfEvent != "" {
			// Split comma-separated events
			events := strings.Split(perfEvent, ",")
			for _, event := range events {
				perfArgs = append(perfArgs, "-e", strings.TrimSpace(event))
			}
		}
		perfArgs = append(perfArgs, "--", binaryPath)
		perfArgs = append(perfArgs, args...)
		cmd = exec.Command("perf", perfArgs...)

		a.logger.Info().
			Str("events", perfEvent).
			Msg("Wrapping test execution with perf stat")
	} else {
		// Execute the test binary directly with arguments
		cmd = exec.Command(binaryPath, args...)
	}

	// Capture stdout and stderr for history
	var stdoutBuf, stderrBuf bytes.Buffer
	
	// Create multi-writers to both capture and display output
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Save captured output to testRun
		testRun.StdoutFile = stdoutBuf.String()
		testRun.StderrFile = stderrBuf.String()
		
		// Test failures are expected to return non-zero exit codes
		// Check if it's an ExitError (test failed) vs other errors
		if exitErr, ok := err.(*exec.ExitError); ok {
			a.logger.Info().
				Int("exit_code", exitErr.ExitCode()).
				Msg("Tests completed with failures")
			return fmt.Errorf("tests failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute test: %w", err)
	}

	// Save captured output to testRun
	testRun.StdoutFile = stdoutBuf.String()
	testRun.StderrFile = stderrBuf.String()

	if perfMode == "profile" {
		a.logger.Info().Str("output", "perf.data").Msg("Performance data collected")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) syncDirectoryToRemote(host, controlPath, remoteBaseDir string) (string, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if we're in a git repository
	gitCheckCmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := gitCheckCmd.Run(); err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	// Use the worktree subdirectory within the base directory
	remoteDir := fmt.Sprintf("%s/worktree", remoteBaseDir)

	a.logger.Info().
		Str("local", cwd).
		Str("remote", remoteDir).
		Msg("Syncing git working tree to remote host")

	// Create remote directory
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	if _, err := a.runRemoteCommand(host, controlPath, mkdirCmd); err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Create a tar archive of the current working tree (including uncommitted changes)
	// and pipe it directly to the remote host
	a.logger.Debug().Msg("Creating archive of working tree")

	// Use git ls-files to get all tracked files (with current modifications)
	// and git ls-files --others to get untracked files (respecting .gitignore)
	// Then tar them all and pipe through SSH
	archiveCmd := exec.Command("sh", "-c",
		"(git ls-files -z; git ls-files --others --exclude-standard -z) | tar --null -T - -czf -",
	)

	// Pipe directly to SSH and extract on remote
	sshCmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		host,
		fmt.Sprintf("cd %s && tar -xzf -", remoteDir),
	)

	// Connect the archive output to ssh input
	pipe, err := archiveCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create pipe: %w", err)
	}
	sshCmd.Stdin = pipe

	var archiveStderr, sshStderr bytes.Buffer
	archiveCmd.Stderr = &archiveStderr
	sshCmd.Stderr = &sshStderr

	// Start both commands
	if err := sshCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start SSH: %w (stderr: %s)", err, sshStderr.String())
	}

	if err := archiveCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start archive: %w (stderr: %s)", err, archiveStderr.String())
	}

	// Wait for archive to finish
	if err := archiveCmd.Wait(); err != nil {
		return "", fmt.Errorf("archive failed: %w (stderr: %s)", err, archiveStderr.String())
	}

	// Wait for ssh to finish
	if err := sshCmd.Wait(); err != nil {
		return "", fmt.Errorf("failed to extract on remote: %w (stderr: %s)", err, sshStderr.String())
	}

	a.logger.Debug().Msg("Working tree synced successfully")

	return remoteDir, nil
}

func (a *App) executeRemoteTestInDir(host, controlPath, remotePath, remoteDir, remoteBaseDir, packagePath, perfMode, perfEvent string, args []string, testRun *model.TestRun) error {
	// Construct the full working directory path
	workDir := remoteDir
	if packagePath != "." && packagePath != "" {
		workDir = fmt.Sprintf("%s/%s", remoteDir, packagePath)
	}

	a.logger.Debug().
		Str("host", host).
		Str("binary", remotePath).
		Str("sync_dir", remoteDir).
		Str("work_dir", workDir).
		Str("package", packagePath).
		Str("perfMode", perfMode).
		Str("perfEvent", perfEvent).
		Strs("args", args).
		Msg("Starting remote test execution")

	var remoteCmd string

	if perfMode == "profile" {
		// Wrap with perf record
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		remoteCmd = fmt.Sprintf("cd %s && perf record -g --call-graph fp -e %s -o %s -- %s",
			workDir, perfEvent, perfDataPath, remotePath)

		a.logger.Info().
			Str("event", perfEvent).
			Str("output", perfDataPath).
			Msg("Wrapping remote test execution with perf record")
	} else if perfMode == "stat" {
		// Wrap with perf stat
		remoteCmd = fmt.Sprintf("cd %s && perf stat", workDir)
		if perfEvent != "" {
			// Split comma-separated events
			events := strings.Split(perfEvent, ",")
			for _, event := range events {
				remoteCmd += fmt.Sprintf(" -e %s", strings.TrimSpace(event))
			}
		}
		remoteCmd += fmt.Sprintf(" -- %s", remotePath)

		a.logger.Info().
			Str("events", perfEvent).
			Msg("Wrapping remote test execution with perf stat")
	} else {
		// Direct execution without perf
		remoteCmd = fmt.Sprintf("cd %s && %s", workDir, remotePath)
	}

	if len(args) > 0 {
		// Append arguments to the remote command
		for _, arg := range args {
			// Simple shell escaping - wrap in single quotes and escape any single quotes
			escapedArg := strings.ReplaceAll(arg, "'", "'\\''")
			remoteCmd += fmt.Sprintf(" '%s'", escapedArg)
		}
	}

	// Execute the test binary remotely
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		host,
		remoteCmd,
	)

	// Capture stdout and stderr for history
	var stdoutBuf, stderrBuf bytes.Buffer
	
	// Create multi-writers to both capture and display output
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Save captured output to testRun
		testRun.StdoutFile = stdoutBuf.String()
		testRun.StderrFile = stderrBuf.String()
		
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

	// Save captured output to testRun
	testRun.StdoutFile = stdoutBuf.String()
	testRun.StderrFile = stderrBuf.String()

	if perfMode == "profile" {
		perfDataPath := fmt.Sprintf("%s/perf.data", remoteBaseDir)
		a.logger.Info().
			Str("output", perfDataPath).
			Msg("Performance data collected on remote host")
	}

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) executeRemoteTest(host, controlPath, remotePath string, args []string) error {
	a.logger.Debug().
		Str("host", host).
		Str("binary", remotePath).
		Strs("args", args).
		Msg("Starting remote test execution")

	// Build the remote command with arguments
	// Need to properly quote the remote path and arguments
	remoteCmd := remotePath
	if len(args) > 0 {
		// Append arguments to the remote command
		for _, arg := range args {
			// Simple shell escaping - wrap in single quotes and escape any single quotes
			escapedArg := strings.ReplaceAll(arg, "'", "'\\''")
			remoteCmd += fmt.Sprintf(" '%s'", escapedArg)
		}
	}

	// Execute the test binary remotely
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		host,
		remoteCmd,
	)

	// Connect stdout and stderr to display test output in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
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

	a.logger.Info().Msg("Tests completed successfully")
	return nil
}

func (a *App) copyBinaryToRemote(host, controlPath, localPath, remoteBaseDir string) (string, error) {
	// Store binary in the base directory
	remotePath := fmt.Sprintf("%s/%s", remoteBaseDir, filepath.Base(localPath))

	a.logger.Info().
		Str("local", localPath).
		Str("remote", remotePath).
		Msg("Copying binary to remote host")

	// Ensure the remote base directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteBaseDir)
	if _, err := a.runRemoteCommand(host, controlPath, mkdirCmd); err != nil {
		return "", fmt.Errorf("failed to create remote base directory: %w", err)
	}

	// Use scp with the SSH multiplexing control path
	cmd := exec.Command("scp",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		localPath,
		fmt.Sprintf("%s:%s", host, remotePath),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.logger.Debug().
		Str("command", cmd.String()).
		Msg("Executing scp")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to copy binary: %w (stderr: %s)", err, stderr.String())
	}

	// Make the binary executable on the remote host
	chmodCmd := fmt.Sprintf("chmod +x %s", remotePath)
	if _, err := a.runRemoteCommand(host, controlPath, chmodCmd); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	a.logger.Debug().Str("path", remotePath).Msg("Binary made executable")

	return remotePath, nil
}

func (a *App) getControlSocketDir() string {
	// Try XDG_RUNTIME_DIR first (preferred for runtime sockets)
	// Keep path short to avoid Unix socket path length limits (104-108 chars)
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "perfgo")
	}

	// Fall back to XDG_CONFIG_HOME or ~/.config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		if home := os.Getenv("HOME"); home != "" {
			configHome = filepath.Join(home, ".config")
		}
	}

	if configHome != "" {
		return filepath.Join(configHome, "perfgo")
	}

	// Last resort: use temp directory
	return filepath.Join(os.TempDir(), "perfgo")
}

func (a *App) setupSSHMultiplexing(host string) (string, error) {
	// Get control socket directory using XDG standards
	controlDir := a.getControlSocketDir()

	// Create the control directory if it doesn't exist
	if err := os.MkdirAll(controlDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create control directory: %w", err)
	}

	// Create a short hash of the host to avoid Unix socket path length limits
	// Unix domain sockets have a path length limit (typically 104-108 chars)
	hash := sha256.Sum256([]byte(host))
	hostHash := hex.EncodeToString(hash[:])[:12] // Use first 12 chars of hash

	// Create control path with short identifier
	socketName := fmt.Sprintf("ssh-%s", hostHash)
	controlPath := filepath.Join(controlDir, socketName)

	a.logger.Debug().
		Str("host", host).
		Str("hostHash", hostHash).
		Str("controlDir", controlDir).
		Str("controlPath", controlPath).
		Int("pathLength", len(controlPath)).
		Msg("Setting up SSH multiplexing")

	// Establish the master connection
	cmd := exec.Command("ssh",
		"-o", "ControlMaster=auto",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlPersist=30s",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-f", // Run in background
		"-N", // Don't execute a remote command
		host,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to establish SSH master connection: %w (stderr: %s)", err, stderr.String())
	}

	a.logger.Debug().Str("host", host).Msg("SSH master connection established")
	return controlPath, nil
}

func (a *App) cleanupSSHMultiplexing(host, controlPath string) {
	a.logger.Debug().Str("controlPath", controlPath).Msg("Cleaning up SSH multiplexing")

	// Close the master connection
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-O", "exit",
		host,
	)
	_ = cmd.Run() // Ignore errors on cleanup

	// Remove the control socket file if it still exists
	_ = os.Remove(controlPath)
}

func (a *App) runRemoteCommand(host, controlPath, command string) (string, error) {
	cmd := exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlMaster=no",
		host,
		command,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.logger.Debug().
		Str("host", host).
		Str("command", command).
		Msg("Running remote command")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

type testRunEntry struct {
	testRun  model.TestRun
	fullPath string
}

func (a *App) list(ctx *cli.Context) error {
	filterPath := ctx.String("path")
	limit := ctx.Int("limit")

	// Get git repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))

	perfgoRoot := filepath.Join(repoRoot, ".perfgo")

	// Check if .perfgo directory exists
	if _, err := os.Stat(perfgoRoot); os.IsNotExist(err) {
		fmt.Println("No test runs found")
		fmt.Printf("Test runs are saved to %s/history/<timestamp>-<commit>-<id>/\n", perfgoRoot)
		return nil
	}

	// Collect test runs
	var testRunEntries []testRunEntry
	err = filepath.WalkDir(perfgoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			testRunPath := filepath.Join(path, "testrun.json")
			if _, err := os.Stat(testRunPath); err == nil {
				testRun, err := a.parseTestRunJSON(testRunPath)
				if err != nil {
					a.logger.Warn().Err(err).Str("path", testRunPath).Msg("Failed to parse testrun.json")
					return nil
				}

				entry := testRunEntry{
					testRun:  testRun,
					fullPath: path,
				}

				// Apply path filter if specified
				if filterPath == "" || strings.Contains(testRun.WorkDir, filterPath) {
					testRunEntries = append(testRunEntries, entry)
				}
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk .perfgo directory: %w", err)
	}

	if len(testRunEntries) == 0 {
		if filterPath != "" {
			fmt.Printf("No test runs found matching path: %s\n", filterPath)
		} else {
			fmt.Println("No test runs found")
		}
		return nil
	}

	// Sort by timestamp (newest first)
	sort.Slice(testRunEntries, func(i, j int) bool {
		return testRunEntries[i].testRun.Timestamp.After(testRunEntries[j].testRun.Timestamp)
	})

	// Apply limit
	displayRuns := testRunEntries
	if limit > 0 && limit < len(displayRuns) {
		displayRuns = displayRuns[:limit]
	}

	fmt.Printf("\n=== Test Runs (%d total) ===\n\n", len(testRunEntries))

	for _, entry := range displayRuns {
		tr := entry.testRun
		timestamp := tr.Timestamp.Format("2006-01-02 15:04:05")

		// Format duration
		duration := tr.Duration.Round(time.Millisecond)

		// Determine status indicator
		status := "✓"
		if tr.ExitCode != 0 {
			status = "✗"
		}

		// Format args (skip the program name)
		args := ""
		if len(tr.Args) > 1 {
			args = strings.Join(tr.Args[1:], " ")
		}

		// Show short ID (first 8 chars)
		shortID := tr.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		fmt.Printf("%s  %s  [%s]  exit=%d  id=%s\n", status, timestamp, duration, tr.ExitCode, shortID)
		if args != "" {
			fmt.Printf("   Args: %s\n", args)
		}
		if tr.WorkDir != "" {
			fmt.Printf("   Path: %s\n", tr.WorkDir)
		}
		if tr.RemoteHost != "" {
			fmt.Printf("   Remote: %s", tr.RemoteHost)
			if tr.OS != "" && tr.Arch != "" {
				fmt.Printf(" (%s/%s)", tr.OS, tr.Arch)
			}
			fmt.Println()
		} else if tr.OS != "" && tr.Arch != "" {
			fmt.Printf("   Local: %s/%s\n", tr.OS, tr.Arch)
		}
		if tr.Commit != "" {
			shortCommit := tr.Commit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			fmt.Printf("   Commit: %s", shortCommit)
			if tr.Branch != "" {
				fmt.Printf(" (%s)", tr.Branch)
			}
			fmt.Println()
		}
		if len(tr.Artifacts) > 0 {
			for _, artifact := range tr.Artifacts {
				var typeName string
				switch artifact.Type {
				case model.ArtifactTypePprofProfile:
					typeName = "profile"
				case model.ArtifactTypeTestBinary:
					typeName = "binary"
				}
				fmt.Printf("   %s: %s (%.1f KB)\n", typeName, artifact.File, float64(artifact.Size)/1024)
			}
		}
		fmt.Printf("   %s\n", entry.fullPath)
		fmt.Println()
	}

	fmt.Println("\nView test output: cat <path>/stdout.txt")
	fmt.Println("View profile: go tool pprof <path>/perf.pb.gz")

	return nil
}

func (a *App) parseTestRunJSON(testRunPath string) (model.TestRun, error) {
	data, err := os.ReadFile(testRunPath)
	if err != nil {
		return model.TestRun{}, err
	}

	var testRun model.TestRun
	if err := json.Unmarshal(data, &testRun); err != nil {
		return model.TestRun{}, err
	}

	return testRun, nil
}

func (a *App) recordTestRun(testRun *model.TestRun, testBinaryPath string) error {
	// Get repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoName := filepath.Base(repoRoot)
	
	// Store repo name in testRun
	testRun.Repo = repoName

	// Get relative path from repo root
	relPath := "."
	if testRun.WorkDir != "" {
		if rel, err := filepath.Rel(repoRoot, testRun.WorkDir); err == nil {
			relPath = rel
		}
	}
	
	// Update WorkDir to be relative to repo root
	testRun.WorkDir = relPath

	// Create directory in .perfgo/history/<timestamp>-<commit>-<id>
	timestamp := testRun.Timestamp.Format("20060102-150405")
	shortCommit := testRun.Commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	shortID := testRun.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	
	runName := fmt.Sprintf("%s-%s-%s", timestamp, shortCommit, shortID)
	runDir := filepath.Join(repoRoot, ".perfgo", "history", runName)

	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Write stdout to file if present
	if testRun.StdoutFile != "" {
		stdoutPath := filepath.Join(runDir, "stdout.txt")
		if err := os.WriteFile(stdoutPath, []byte(testRun.StdoutFile), 0644); err != nil {
			return fmt.Errorf("failed to write stdout: %w", err)
		}
		testRun.StdoutFile = "stdout.txt" // Store relative filename
	}

	// Write stderr to file if present
	if testRun.StderrFile != "" {
		stderrPath := filepath.Join(runDir, "stderr.txt")
		if err := os.WriteFile(stderrPath, []byte(testRun.StderrFile), 0644); err != nil {
			return fmt.Errorf("failed to write stderr: %w", err)
		}
		testRun.StderrFile = "stderr.txt" // Store relative filename
	}

	// Archive artifacts if they exist
	if err := a.saveArtifacts(runDir, testRun, testBinaryPath); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to save some artifacts")
		// Don't fail the test run on artifact errors
	}

	// Write test run metadata
	metadataPath := filepath.Join(runDir, "testrun.json")
	metadataJSON, err := json.MarshalIndent(testRun, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal test run: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write test run metadata: %w", err)
	}

	a.logger.Debug().Str("dir", runDir).Str("id", testRun.ID).Msg("Recorded test run")
	return nil
}
