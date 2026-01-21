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
	perfEvent := ctx.String("event")
	perfImage := ctx.String("perf-image")

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

	var target string
	var targetType string
	var targetNodeName string

	if podName != "" {
		targetType = "pod"
		target = podName

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
		privateKeyPath, hostKeyPath, err := a.setupSSHKeys(execCtx, k8sClient, perfPodName, namespace)
		if err != nil {
			return fmt.Errorf("failed to setup SSH keys: %w", err)
		}

		// Build SSH connection command with all necessary parameters
		sshCmd := fmt.Sprintf(`ssh -i %s -o IdentitiesOnly=yes -o UserKnownHostsFile=%s -o ProxyCommand="kubectl exec -i -n %s %s -- bash -c '/usr/sbin/sshd -i 2> /dev/null'" root@%s`,
			privateKeyPath, hostKeyPath, namespace, perfPodName, perfPodName)

		fmt.Printf("\nSSH connection command:\n%s\n\n", sshCmd)

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

	} else {
		targetType = "node"
		target = nodeName
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

	// Execute the attach operation
	if mode == "stat" {
		a.logger.Info().
			Str("target_type", targetType).
			Str("target", target).
			Str("event", perfEvent).
			Msg("Running perf stat attach")

		// TODO: Implement actual perf stat attach logic
		fmt.Printf("Attaching perf stat to %s %s\n", targetType, target)
		if perfEvent != "" {
			fmt.Printf("Events: %s\n", perfEvent)
		}
		return fmt.Errorf("perf stat attach not yet implemented")
	} else if mode == "profile" {
		a.logger.Info().
			Str("target_type", targetType).
			Str("target", target).
			Str("event", perfEvent).
			Msg("Running perf profile attach")

		// TODO: Implement actual perf profile attach logic
		fmt.Printf("Attaching perf profile to %s %s\n", targetType, target)
		fmt.Printf("Event: %s\n", perfEvent)
		return fmt.Errorf("perf profile attach not yet implemented")
	}

	return nil
}

// setupSSHKeys sets up SSH keys in the perf pod for SSH access.
// Returns the paths to the private key and host public key files.
func (a *App) setupSSHKeys(ctx context.Context, client *k8s.Client, podName, namespace string) (string, string, error) {
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

	// Save keys locally
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh", "perfgo")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create local SSH directory: %w", err)
	}

	// Save private key
	privateKeyPath := filepath.Join(sshDir, fmt.Sprintf("%s_%s", namespace, podName))
	if err := os.WriteFile(privateKeyPath, []byte(privateKey), 0600); err != nil {
		return "", "", fmt.Errorf("failed to save private key: %w", err)
	}

	a.logger.Info().
		Str("path", privateKeyPath).
		Msg("Private key saved locally")

	// Save host public key for known_hosts with pod name prepended
	hostKeyPath := filepath.Join(sshDir, fmt.Sprintf("%s_%s.pub", namespace, podName))
	hostKeyWithName := fmt.Sprintf("%s %s", podName, hostPublicKey)
	if err := os.WriteFile(hostKeyPath, []byte(hostKeyWithName), 0644); err != nil {
		return "", "", fmt.Errorf("failed to save host public key: %w", err)
	}

	a.logger.Info().
		Str("path", hostKeyPath).
		Msg("Host public key saved locally")

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
			a.logger.Warn().
				Str("container", containerName).
				Msg("No PIDs found for container")
		}
	}

	return pids, nil
}
