package cli

// This file contains the attach command for running performance profiling
// on Kubernetes pods or nodes.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/perfgo/perfgo/cli/k8s"
	"github.com/perfgo/perfgo/cli/perf"
	"github.com/perfgo/perfgo/cli/ssh"
	"github.com/urfave/cli/v2"
)

func (a *App) attachStat(ctx *cli.Context) error {
	return a.runAttach(ctx, "stat")
}

func (a *App) attachProfile(ctx *cli.Context) error {
	return a.runAttach(ctx, "profile")
}

func (a *App) runAttach(ctx *cli.Context, mode string) error {
	// Get flags
	kubeContext := ctx.String("context")
	podName := ctx.String("pod")
	nodeName := ctx.String("node")
	namespace := ctx.String("namespace")
	perfImage := ctx.String("perf-image")
	duration := ctx.Int("duration")

	var perfEvent string
	var perfEvents []string
	if mode == "profile" {
		perfEvent = ctx.String("event")
	} else if mode == "stat" {
		perfEvents = ctx.StringSlice("event")
		perfEvent = strings.Join(perfEvents, ",")
	}

	// Validate that exactly one of --pod or --node is specified
	if podName == "" && nodeName == "" {
		return fmt.Errorf("either --pod or --node must be specified")
	}
	if podName != "" && nodeName != "" {
		return fmt.Errorf("--pod and --node are mutually exclusive, specify only one")
	}

	// Set default namespace if targeting a pod
	if podName != "" && namespace == "" {
		namespace = "default"
	}

	// Create Kubernetes client
	k8sClient := k8s.New(kubeContext, namespace)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create temporary directory for SSH keys
	tempDir, err := os.MkdirTemp("", "perfgo-ssh-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			a.logger.Warn().Err(err).Str("path", tempDir).Msg("Failed to remove temporary directory")
		} else {
			a.logger.Debug().Str("path", tempDir).Msg("Cleaned up temporary SSH keys")
		}
	}()

	var targetNodeName string

	if podName != "" {
		// Get the specific pod to extract container IDs and node information
		a.logger.Info().
			Str("pod", podName).
			Str("namespace", namespace).
			Msg("Getting pod information")

		pod, err := k8sClient.GetPod(execCtx, podName)
		if err != nil {
			return fmt.Errorf("failed to get pod: %w", err)
		}

		targetNodeName = pod.Spec.NodeName

		a.logger.Info().
			Str("pod", podName).
			Str("node", targetNodeName).
			Str("phase", pod.Status.Phase).
			Msg("Found pod")

		// Extract container IDs
		containerIDs := k8s.GetContainerIDs(pod)
		if len(containerIDs) > 0 {
			a.logger.Info().
				Interface("container_ids", containerIDs).
				Msg("Extracted container IDs")
		} else {
			a.logger.Warn().Msg("No container IDs found in pod status")
		}

		// Generate random suffix for pod name
		suffixBytes := make([]byte, 4)
		if _, err := rand.Read(suffixBytes); err != nil {
			return fmt.Errorf("failed to generate random suffix: %w", err)
		}
		randomSuffix := hex.EncodeToString(suffixBytes)

		// Create a privileged perf pod on the same node
		perfPodName := fmt.Sprintf("perfgo-%s-%s", podName, randomSuffix)
		a.logger.Info().
			Str("perf_pod", perfPodName).
			Str("image", perfImage).
			Str("node", targetNodeName).
			Msg("Creating privileged perf pod")

		if err := k8sClient.CreatePrivilegedPod(execCtx, perfPodName, perfImage, targetNodeName); err != nil {
			return fmt.Errorf("failed to create privileged perf pod: %w", err)
		}

		a.logger.Info().
			Str("perf_pod", perfPodName).
			Msg("Privileged perf pod created successfully")

		// Wait for pod to be ready
		a.logger.Info().
			Str("perf_pod", perfPodName).
			Msg("Waiting for perf pod to be ready")

		if err := k8sClient.WaitForPodReady(execCtx, perfPodName); err != nil {
			return fmt.Errorf("failed to wait for perf pod to be ready: %w", err)
		}

		a.logger.Info().
			Str("perf_pod", perfPodName).
			Msg("Perf pod is ready")

		// Set up SSH keys in the pod
		privateKeyPath, hostKeyPath, err := a.setupSSHKeys(execCtx, k8sClient, perfPodName, namespace, tempDir)
		if err != nil {
			return fmt.Errorf("failed to setup SSH keys: %w", err)
		}

		// Create SSH client to the perf pod
		a.logger.Info().Msg("Creating SSH client to perf pod")
		proxyCmd := fmt.Sprintf("kubectl exec -i -n %s %s -- bash -c '/usr/sbin/sshd -i 2> /dev/null'", namespace, perfPodName)
		sshHost := fmt.Sprintf("root@%s", perfPodName)

		sshClient, err := ssh.New(a.logger, sshHost,
			ssh.WithIdentityFile(privateKeyPath),
			ssh.WithKnownHostsFile(hostKeyPath),
			ssh.WithProxyCommand(proxyCmd),
			ssh.WithExtraOptions("IdentitiesOnly=yes"),
		)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %w", err)
		}
		defer sshClient.Close()

		// Find PIDs for the container IDs
		a.logger.Info().Msg("Finding PIDs for container IDs")
		pids, err := a.findPIDsForContainers(sshClient, containerIDs)
		if err != nil {
			return fmt.Errorf("failed to find PIDs for containers: %w", err)
		}

		a.logger.Info().
			Interface("pids", pids).
			Msg("Found PIDs for containers")

		// Flatten all PIDs into a single list for perf
		allPIDs := []string{}
		for _, pidList := range pids {
			allPIDs = append(allPIDs, pidList...)
		}

		if len(allPIDs) == 0 {
			return fmt.Errorf("no PIDs found for containers")
		}

		// Run perf stat or perf record
		if mode == "stat" {
			if err := a.executePerfStat(sshClient, allPIDs, perfEvent, duration); err != nil {
				return fmt.Errorf("failed to execute perf stat: %w", err)
			}
		} else if mode == "profile" {
			if err := a.executePerfRecord(sshClient, allPIDs, perfEvent, duration); err != nil {
				return fmt.Errorf("failed to execute perf record: %w", err)
			}
		}

	} else {
		targetNodeName = nodeName

		// Verify node exists
		a.logger.Info().
			Str("node", nodeName).
			Msg("Verifying node exists")

		nodes, err := k8sClient.GetNodes(execCtx)
		if err != nil {
			return fmt.Errorf("failed to list nodes: %w", err)
		}

		found := false
		for _, node := range nodes {
			if node.Metadata.Name == nodeName {
				found = true
				a.logger.Info().
					Str("node", nodeName).
					Str("os", node.Status.NodeInfo.OperatingSystem).
					Str("arch", node.Status.NodeInfo.Architecture).
					Msg("Found node")
				break
			}
		}

		if !found {
			return fmt.Errorf("node %s not found in cluster", nodeName)
		}
	}

	return nil
}

// setupSSHKeys sets up SSH keys in the perf pod for SSH access.
// Returns the paths to the private key and host public key files.
func (a *App) setupSSHKeys(ctx context.Context, client *k8s.Client, podName, namespace, tempDir string) (string, string, error) {
	a.logger.Info().
		Str("pod", podName).
		Msg("Setting up SSH keys in perf pod")

	// Create SSH directories
	a.logger.Debug().Msg("Creating SSH directories")
	if _, err := client.ExecCommand(ctx, podName, []string{"mkdir", "-p", "/etc/ssh", "/root/.ssh"}); err != nil {
		return "", "", fmt.Errorf("failed to create SSH directories: %w", err)
	}

	// Set proper permissions on .ssh directory
	if _, err := client.ExecCommand(ctx, podName, []string{"chmod", "700", "/root/.ssh"}); err != nil {
		return "", "", fmt.Errorf("failed to set permissions on .ssh directory: %w", err)
	}

	// Generate SSH host key (ed25519)
	a.logger.Debug().Msg("Generating SSH host key (ed25519)")
	if _, err := client.ExecCommand(ctx, podName, []string{
		"ssh-keygen", "-t", "ed25519", "-f", "/etc/ssh/ssh_host_ed25519_key", "-N", "",
	}); err != nil {
		return "", "", fmt.Errorf("failed to generate SSH host key: %w", err)
	}

	// Generate SSH user key for root (ed25519)
	a.logger.Debug().Msg("Generating SSH user key for root (ed25519)")
	if _, err := client.ExecCommand(ctx, podName, []string{
		"ssh-keygen", "-t", "ed25519", "-f", "/root/.ssh/id_ed25519", "-N", "",
	}); err != nil {
		return "", "", fmt.Errorf("failed to generate SSH user key: %w", err)
	}

	// Copy public key to authorized_keys
	a.logger.Debug().Msg("Setting up authorized_keys")
	if _, err := client.ExecCommand(ctx, podName, []string{
		"sh", "-c", "cat /root/.ssh/id_ed25519.pub > /root/.ssh/authorized_keys",
	}); err != nil {
		return "", "", fmt.Errorf("failed to setup authorized_keys: %w", err)
	}

	// Set proper permissions on authorized_keys
	if _, err := client.ExecCommand(ctx, podName, []string{"chmod", "600", "/root/.ssh/authorized_keys"}); err != nil {
		return "", "", fmt.Errorf("failed to set permissions on authorized_keys: %w", err)
	}

	// Read back the private key
	a.logger.Debug().Msg("Reading private key from pod")
	privateKey, err := client.ReadFileFromPod(ctx, podName, "/root/.ssh/id_ed25519")
	if err != nil {
		return "", "", fmt.Errorf("failed to read private key: %w", err)
	}

	// Read back the host public key
	a.logger.Debug().Msg("Reading host public key from pod")
	hostPublicKey, err := client.ReadFileFromPod(ctx, podName, "/etc/ssh/ssh_host_ed25519_key.pub")
	if err != nil {
		return "", "", fmt.Errorf("failed to read host public key: %w", err)
	}

	// Save keys in temporary directory
	a.logger.Debug().Str("tempDir", tempDir).Msg("Saving SSH keys to temporary directory")

	// Save private key
	privateKeyPath := filepath.Join(tempDir, "id_ed25519")
	if err := os.WriteFile(privateKeyPath, []byte(privateKey), 0600); err != nil {
		return "", "", fmt.Errorf("failed to save private key: %w", err)
	}

	a.logger.Debug().
		Str("path", privateKeyPath).
		Msg("Private key saved to temporary directory")

	// Save host public key for known_hosts with pod name prepended
	hostKeyPath := filepath.Join(tempDir, "known_hosts")
	hostKeyWithName := fmt.Sprintf("%s %s", podName, hostPublicKey)
	if err := os.WriteFile(hostKeyPath, []byte(hostKeyWithName), 0644); err != nil {
		return "", "", fmt.Errorf("failed to save host public key: %w", err)
	}

	a.logger.Debug().
		Str("path", hostKeyPath).
		Msg("Host public key saved to temporary directory")

	return privateKeyPath, hostKeyPath, nil
}

// findPIDsForContainers finds all PIDs associated with the given container IDs.
// Returns a map of container name to list of PIDs.
func (a *App) findPIDsForContainers(client *ssh.Client, containerIDs map[string]string) (map[string][]string, error) {
	pids := make(map[string][]string)

	for containerName, containerID := range containerIDs {
		a.logger.Debug().
			Str("container", containerName).
			Str("container_id", containerID).
			Msg("Searching for PIDs")

		// Search for processes with this container ID in their cgroup
		// The cgroup file contains the container ID in various formats depending on the runtime
		findCmd := fmt.Sprintf("grep -l '%s' /proc/*/cgroup 2>/dev/null | cut -d/ -f3 | sort -u", containerID)
		output, err := client.RunCommand(findCmd)
		if err != nil {
			a.logger.Warn().
				Err(err).
				Str("container", containerName).
				Msg("Failed to find PIDs for container")
			continue
		}

		// Parse PIDs from output
		pidList := []string{}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if line != "" && line != "self" && line != "thread-self" {
				pidList = append(pidList, line)
			}
		}

		if len(pidList) > 0 {
			pids[containerName] = pidList
			a.logger.Debug().
				Str("container", containerName).
				Strs("pids", pidList).
				Msg("Found PIDs for container")
		} else {
			a.logger.Debug().
				Str("container", containerName).
				Msg("No PIDs found for container")
		}
	}

	return pids, nil
}

// executePerfStat runs perf stat on the specified PIDs via SSH.
func (a *App) executePerfStat(client *ssh.Client, pids []string, events string, duration int) error {
	a.logger.Info().
		Strs("pids", pids).
		Str("events", events).
		Int("duration", duration).
		Msg("Running perf stat on PIDs")

	// Build perf stat command
	var eventList []string
	if events != "" {
		eventList = strings.Split(events, ",")
	}

	statOpts := perf.StatOptions{
		Events:   eventList,
		PIDs:     pids,
		Duration: duration,
	}
	perfCmd := perf.BuildStatCommand(statOpts)

	a.logger.Debug().Str("command", perfCmd).Msg("Executing perf stat command")

	stdout, stderr, err := client.RunCommandWithStderr(perfCmd)
	if err != nil {
		return fmt.Errorf("perf stat failed: %w", err)
	}

	// Display the perf stat output (perf stat writes to stderr)
	output := stderr
	if output == "" {
		output = stdout
	}
	
	fmt.Println("\nPerf stat output:")
	fmt.Println(output)

	a.logger.Info().Msg("Perf stat completed successfully")
	return nil
}

// executePerfRecord runs perf record on the specified PIDs via SSH.
func (a *App) executePerfRecord(client *ssh.Client, pids []string, event string, duration int) error {
	a.logger.Info().
		Strs("pids", pids).
		Str("event", event).
		Int("duration", duration).
		Msg("Running perf record on PIDs")

	// Build perf record command
	perfDataPath := "/tmp/perf.data"
	recordOpts := perf.RecordOptions{
		Event:      event,
		PIDs:       pids,
		Duration:   duration,
		OutputPath: perfDataPath,
	}
	perfCmd := perf.BuildRecordCommand(recordOpts)

	a.logger.Debug().Str("command", perfCmd).Msg("Executing perf record command")

	output, err := client.RunCommand(perfCmd)
	if err != nil {
		return fmt.Errorf("perf record failed: %w", err)
	}

	// Display the output
	if output != "" {
		fmt.Println(output)
	}

	a.logger.Info().
		Str("remote_path", perfDataPath).
		Msg("Performance data collected on remote host")

	// Process perf.data and convert to pprof
	remoteBaseDir := "/tmp"
	if err := perf.ProcessPerfData(a.logger, client, remoteBaseDir); err != nil {
		return fmt.Errorf("failed to process performance data: %w", err)
	}

	return nil
}
