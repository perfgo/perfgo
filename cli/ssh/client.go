package ssh

// Package ssh provides SSH multiplexing and remote command execution
// functionality for perfgo. It manages persistent SSH connections,
// file synchronization, and remote command execution.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// Client manages an SSH connection to a specific remote host.
type Client struct {
	logger         zerolog.Logger
	host           string
	controlPath    string
	identityFile   string
	knownHostsFile string
	proxyCommand   string
	extraOptions   []string
}

// SSHOption is a function that configures an SSH client.
type SSHOption func(*Client)

// WithIdentityFile sets the identity file (private key) to use for authentication.
func WithIdentityFile(path string) SSHOption {
	return func(c *Client) {
		c.identityFile = path
	}
}

// WithKnownHostsFile sets the known hosts file to use for host verification.
func WithKnownHostsFile(path string) SSHOption {
	return func(c *Client) {
		c.knownHostsFile = path
	}
}

// WithProxyCommand sets a proxy command for the SSH connection.
func WithProxyCommand(command string) SSHOption {
	return func(c *Client) {
		c.proxyCommand = command
	}
}

// WithExtraOptions adds extra SSH options to the connection.
func WithExtraOptions(options ...string) SSHOption {
	return func(c *Client) {
		c.extraOptions = append(c.extraOptions, options...)
	}
}

// New creates a new SSH client and establishes a multiplexed connection to the host.
func New(logger zerolog.Logger, host string, opts ...SSHOption) (*Client, error) {
	c := &Client{
		logger: logger,
		host:   host,
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Setup SSH multiplexing
	controlPath, err := c.setupMultiplexing()
	if err != nil {
		return nil, fmt.Errorf("failed to setup SSH multiplexing: %w", err)
	}
	c.controlPath = controlPath

	return c, nil
}

// Close closes the SSH connection and cleans up the control socket.
func (c *Client) Close() {
	c.logger.Debug().Str("controlPath", c.controlPath).Msg("Cleaning up SSH multiplexing")

	// Close the master connection
	args := []string{
		"-o", fmt.Sprintf("ControlPath=%s", c.controlPath),
		"-O", "exit",
		c.host,
	}
	cmd := exec.Command("ssh", args...)
	_ = cmd.Run() // Ignore errors on cleanup

	// Remove the control socket file if it still exists
	_ = os.Remove(c.controlPath)
}

// RunCommand executes a command on the remote host and returns the output.
func (c *Client) RunCommand(command string) (string, error) {
	args := c.buildSSHArgs()
	args = append(args, c.host, command)

	cmd := exec.Command("ssh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.logger.Debug().
		Str("host", c.host).
		Str("command", command).
		Msg("Running remote command")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommandWithStderr executes a command on the remote host and returns both stdout and stderr.
func (c *Client) RunCommandWithStderr(command string) (stdout, stderr string, err error) {
	args := c.buildSSHArgs()
	args = append(args, c.host, command)

	cmd := exec.Command("ssh", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	c.logger.Debug().
		Str("host", c.host).
		Str("command", command).
		Msg("Running remote command")

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("command failed: %w (stderr: %s)", err, stderrBuf.String())
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// buildSSHArgs constructs the SSH arguments with all configured options.
func (c *Client) buildSSHArgs() []string {
	args := []string{}

	// Add control path options if using multiplexing
	if c.controlPath != "" {
		args = append(args,
			"-o", fmt.Sprintf("ControlPath=%s", c.controlPath),
			"-o", "ControlMaster=no",
		)
	}

	// Add identity file if specified
	if c.identityFile != "" {
		args = append(args, "-i", c.identityFile)
	}

	// Add known hosts file if specified
	if c.knownHostsFile != "" {
		args = append(args, "-o", fmt.Sprintf("UserKnownHostsFile=%s", c.knownHostsFile))
	}

	// Add proxy command if specified
	if c.proxyCommand != "" {
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s", c.proxyCommand))
	}

	// Add extra options
	for _, opt := range c.extraOptions {
		args = append(args, "-o", opt)
	}

	return args
}

// DetectSystem detects the OS and architecture of the remote system.
func (c *Client) DetectSystem() (string, string, error) {
	// Detect OS
	osName, err := c.RunCommand("uname -s")
	if err != nil {
		return "", "", fmt.Errorf("failed to detect OS: %w", err)
	}

	// Detect architecture
	arch, err := c.RunCommand("uname -m")
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

// GetRemoteRepositoryDir determines the remote directory path for the current repository.
func (c *Client) GetRemoteRepositoryDir() (string, error) {
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
	cacheDir, err := c.getRemoteCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get remote cache directory: %w", err)
	}

	// Construct full path
	remoteBaseDir := fmt.Sprintf("%s/repositories/%s", cacheDir, repoIdent)

	return remoteBaseDir, nil
}

// SyncDirectoryToRemote syncs the current git working tree to the remote host.
func (c *Client) SyncDirectoryToRemote(remoteBaseDir string) (string, error) {
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

	c.logger.Info().
		Str("local", cwd).
		Str("remote", remoteDir).
		Msg("Syncing git working tree to remote host")

	// Create remote directory
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	if _, err := c.RunCommand(mkdirCmd); err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Create a tar archive of the current working tree (including uncommitted changes)
	// and pipe it directly to the remote host
	c.logger.Debug().Msg("Creating archive of working tree")

	// Use git ls-files to get all tracked files (with current modifications)
	// and git ls-files --others to get untracked files (respecting .gitignore)
	// Then tar them all and pipe through SSH
	archiveCmd := exec.Command("sh", "-c",
		"(git ls-files -z; git ls-files --others --exclude-standard -z) | tar --null -T - -czf -",
	)

	// Pipe directly to SSH and extract on remote
	args := c.buildSSHArgs()
	args = append(args, c.host, fmt.Sprintf("cd %s && tar -xzf -", remoteDir))
	sshCmd := exec.Command("ssh", args...)

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

	c.logger.Debug().Msg("Working tree synced successfully")

	return remoteDir, nil
}

// CopyBinaryToRemote copies a local binary to the remote host and makes it executable.
func (c *Client) CopyBinaryToRemote(localPath, remoteBaseDir string) (string, error) {
	// Store binary in the base directory
	remotePath := fmt.Sprintf("%s/%s", remoteBaseDir, filepath.Base(localPath))

	c.logger.Info().
		Str("local", localPath).
		Str("remote", remotePath).
		Msg("Copying binary to remote host")

	// Ensure the remote base directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteBaseDir)
	if _, err := c.RunCommand(mkdirCmd); err != nil {
		return "", fmt.Errorf("failed to create remote base directory: %w", err)
	}

	// Use scp with the SSH multiplexing control path
	args := c.buildSSHArgs()
	args = append(args, localPath, fmt.Sprintf("%s:%s", c.host, remotePath))
	cmd := exec.Command("scp", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.logger.Debug().
		Str("command", cmd.String()).
		Msg("Executing scp")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to copy binary: %w (stderr: %s)", err, stderr.String())
	}

	// Make the binary executable on the remote host
	chmodCmd := fmt.Sprintf("chmod +x %s", remotePath)
	if _, err := c.RunCommand(chmodCmd); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	c.logger.Debug().Str("path", remotePath).Msg("Binary made executable")

	return remotePath, nil
}

// Host returns the remote host this client is connected to.
func (c *Client) Host() string {
	return c.host
}

// ControlPath returns the SSH control socket path.
func (c *Client) ControlPath() string {
	return c.controlPath
}

// setupMultiplexing establishes an SSH master connection for multiplexing.
func (c *Client) setupMultiplexing() (string, error) {
	// Get control socket directory using XDG standards
	controlDir := c.getControlSocketDir()

	// Create the control directory if it doesn't exist
	if err := os.MkdirAll(controlDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create control directory: %w", err)
	}

	// Create a short hash of the host to avoid Unix socket path length limits
	// Unix domain sockets have a path length limit (typically 104-108 chars)
	hash := sha256.Sum256([]byte(c.host))
	hostHash := hex.EncodeToString(hash[:])[:12] // Use first 12 chars of hash

	// Create control path with short identifier
	socketName := fmt.Sprintf("ssh-%s", hostHash)
	controlPath := filepath.Join(controlDir, socketName)

	c.logger.Debug().
		Str("host", c.host).
		Str("hostHash", hostHash).
		Str("controlDir", controlDir).
		Str("controlPath", controlPath).
		Int("pathLength", len(controlPath)).
		Msg("Setting up SSH multiplexing")

	// Establish the master connection
	args := []string{
		"-o", "ControlMaster=auto",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlPersist=30s",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}

	// Add identity file if specified
	if c.identityFile != "" {
		args = append(args, "-i", c.identityFile)
	}

	// Add known hosts file if specified
	if c.knownHostsFile != "" {
		args = append(args, "-o", fmt.Sprintf("UserKnownHostsFile=%s", c.knownHostsFile))
	}

	// Add proxy command if specified
	if c.proxyCommand != "" {
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s", c.proxyCommand))
	}

	// Add extra options
	for _, opt := range c.extraOptions {
		args = append(args, "-o", opt)
	}

	args = append(args,
		"-f", // Run in background
		"-N", // Don't execute a remote command
		c.host,
	)

	cmd := exec.Command("ssh", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to establish SSH master connection: %w (stderr: %s)", err, stderr.String())
	}

	c.logger.Debug().Str("host", c.host).Msg("SSH master connection established")
	return controlPath, nil
}

// getControlSocketDir returns the directory to use for SSH control sockets.
func (c *Client) getControlSocketDir() string {
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

// getRemoteCacheDir determines the cache directory on the remote host.
func (c *Client) getRemoteCacheDir() (string, error) {
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
	cacheDir, err := c.RunCommand(getCacheDirCmd)
	if err != nil {
		return "", fmt.Errorf("failed to determine remote cache directory: %w", err)
	}

	return strings.TrimSpace(cacheDir), nil
}
