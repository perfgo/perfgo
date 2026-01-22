package k8s

// Package k8s provides a client interface to Kubernetes clusters via kubectl.
// It manages kubectl command execution with context and namespace configuration.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client manages kubectl commands for a specific Kubernetes context and namespace.
type Client struct {
	kubeContext string
	namespace   string
}

// Node represents a Kubernetes node.
type Node struct {
	Metadata NodeMetadata `json:"metadata"`
	Status   NodeStatus   `json:"status"`
}

// NodeMetadata contains node metadata.
type NodeMetadata struct {
	Name              string            `json:"name"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// NodeStatus contains node status information.
type NodeStatus struct {
	Conditions []NodeCondition `json:"conditions,omitempty"`
	NodeInfo   NodeInfo        `json:"nodeInfo"`
}

// NodeCondition represents a condition of the node.
type NodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// NodeInfo contains node system information.
type NodeInfo struct {
	KubeletVersion          string `json:"kubeletVersion"`
	OSImage                 string `json:"osImage"`
	OperatingSystem         string `json:"operatingSystem"`
	Architecture            string `json:"architecture"`
	ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
}

// NodeList represents a list of nodes.
type NodeList struct {
	Items []Node `json:"items"`
}

// Pod represents a Kubernetes pod.
type Pod struct {
	Metadata PodMetadata `json:"metadata"`
	Spec     PodSpec     `json:"spec"`
	Status   PodStatus   `json:"status"`
}

// PodMetadata contains pod metadata.
type PodMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// PodSpec contains pod specification.
type PodSpec struct {
	NodeName   string      `json:"nodeName,omitempty"`
	Containers []Container `json:"containers"`
}

// Container represents a container in a pod.
type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// PodStatus contains pod status information.
type PodStatus struct {
	Phase             string            `json:"phase"`
	Conditions        []PodCondition    `json:"conditions,omitempty"`
	PodIP             string            `json:"podIP,omitempty"`
	HostIP            string            `json:"hostIP,omitempty"`
	StartTime         *time.Time        `json:"startTime,omitempty"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty"`
}

// PodCondition represents a condition of the pod.
type PodCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// ContainerStatus contains container status information.
type ContainerStatus struct {
	Name        string         `json:"name"`
	Ready       bool           `json:"ready"`
	State       ContainerState `json:"state"`
	ContainerID string         `json:"containerID,omitempty"`
}

// ContainerState represents the state of a container.
type ContainerState struct {
	Running    *ContainerStateRunning    `json:"running,omitempty"`
	Waiting    *ContainerStateWaiting    `json:"waiting,omitempty"`
	Terminated *ContainerStateTerminated `json:"terminated,omitempty"`
}

// ContainerStateRunning represents a running container state.
type ContainerStateRunning struct {
	StartedAt time.Time `json:"startedAt"`
}

// ContainerStateWaiting represents a waiting container state.
type ContainerStateWaiting struct {
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ContainerStateTerminated represents a terminated container state.
type ContainerStateTerminated struct {
	ExitCode   int       `json:"exitCode"`
	Reason     string    `json:"reason,omitempty"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

// PodList represents a list of pods.
type PodList struct {
	Items []Pod `json:"items"`
}

// New creates a new Kubernetes client for the specified context and namespace.
// If kubeContext is empty, the current context will be used.
// If namespace is empty, the default namespace will be used.
func New(kubeContext, namespace string) *Client {
	return &Client{
		kubeContext: kubeContext,
		namespace:   namespace,
	}
}

// GetNodes retrieves all nodes in the cluster.
func (c *Client) GetNodes(ctx context.Context) ([]Node, error) {
	args := []string{"get", "nodes", "-o", "json"}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	output, err := c.runKubectl(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var nodeList NodeList
	if err := json.Unmarshal([]byte(output), &nodeList); err != nil {
		return nil, fmt.Errorf("failed to parse nodes response: %w", err)
	}

	return nodeList.Items, nil
}

// GetPods retrieves all pods in the configured namespace.
// If no namespace was specified in New(), it uses the default namespace.
func (c *Client) GetPods(ctx context.Context) ([]Pod, error) {
	args := []string{"get", "pods", "-o", "json"}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	// Add namespace if specified
	if c.namespace != "" {
		args = append(args, "-n", c.namespace)
	}

	output, err := c.runKubectl(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	var podList PodList
	if err := json.Unmarshal([]byte(output), &podList); err != nil {
		return nil, fmt.Errorf("failed to parse pods response: %w", err)
	}

	return podList.Items, nil
}

// GetPodsInNamespace retrieves all pods in the specified namespace.
// This allows querying a different namespace than the one configured in the client.
func (c *Client) GetPodsInNamespace(ctx context.Context, namespace string) ([]Pod, error) {
	args := []string{"get", "pods", "-o", "json", "-n", namespace}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	output, err := c.runKubectl(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods in namespace %s: %w", namespace, err)
	}

	var podList PodList
	if err := json.Unmarshal([]byte(output), &podList); err != nil {
		return nil, fmt.Errorf("failed to parse pods response: %w", err)
	}

	return podList.Items, nil
}

// GetPod retrieves a specific pod by name in the configured namespace.
func (c *Client) GetPod(ctx context.Context, name string) (*Pod, error) {
	args := []string{"get", "pod", name, "-o", "json"}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	// Add namespace if specified
	if c.namespace != "" {
		args = append(args, "-n", c.namespace)
	}

	output, err := c.runKubectl(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", name, err)
	}

	var pod Pod
	if err := json.Unmarshal([]byte(output), &pod); err != nil {
		return nil, fmt.Errorf("failed to parse pod response: %w", err)
	}

	return &pod, nil
}

// GetContainerIDs extracts all container IDs from a pod.
// Returns a map of container name to container ID.
// Container IDs have the runtime prefix (e.g., "containerd://", "docker://") stripped.
func GetContainerIDs(pod *Pod) map[string]string {
	containerIDs := make(map[string]string)

	// Process init containers
	for _, status := range pod.Status.InitContainerStatuses {
		if status.ContainerID != "" {
			containerIDs[status.Name] = stripContainerIDPrefix(status.ContainerID)
		}
	}

	// Process regular containers
	for _, status := range pod.Status.ContainerStatuses {
		if status.ContainerID != "" {
			containerIDs[status.Name] = stripContainerIDPrefix(status.ContainerID)
		}
	}

	return containerIDs
}

// stripContainerIDPrefix removes the runtime prefix from container IDs.
// For example, "containerd://abc123" becomes "abc123".
func stripContainerIDPrefix(containerID string) string {
	// Container IDs from Kubernetes are in format: <runtime>://<id>
	// Common runtimes: containerd, docker, cri-o
	if idx := strings.Index(containerID, "://"); idx != -1 {
		return containerID[idx+3:]
	}
	return containerID
}

// CreatePrivilegedPod creates a new privileged pod using kubectl run.
// The pod will run an infinite sleep command and be scheduled on the specified node.
// It uses the host PID namespace and runs with privileged security context.
func (c *Client) CreatePrivilegedPod(ctx context.Context, name, image, nodeName string) error {
	overrides := fmt.Sprintf(`{
		"metadata": {
			"labels": {
				"app.kubernetes.io/name": "perfgo",
				"app.kubernetes.io/component": "perf-profiler",
				"app.kubernetes.io/managed-by": "perfgo"
			}
		},
		"spec": {
			"hostPID": true,
			"nodeName": "%s",
			"containers": [{
				"name": "%s",
				"image": "%s",
				"command": ["sleep", "infinity"],
				"securityContext": {
					"privileged": true
				}
			}]
		}
	}`, nodeName, name, image)

	args := []string{
		"run", name,
		"--image=" + image,
		"--restart=Never",
		"--overrides=" + overrides,
	}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	// Add namespace if specified
	if c.namespace != "" {
		args = append(args, "-n", c.namespace)
	}

	_, err := c.runKubectl(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to create privileged pod %s: %w", name, err)
	}

	return nil
}

// WaitForPodReady waits for a pod to be in the Running phase and ready.
// It polls the pod status until it's ready or the context is cancelled.
func (c *Client) WaitForPodReady(ctx context.Context, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %s to be ready: %w", name, ctx.Err())
		case <-ticker.C:
			pod, err := c.GetPod(ctx, name)
			if err != nil {
				continue // Pod might not exist yet
			}

			if pod.Status.Phase == "Running" {
				// Check if all containers are ready
				allReady := true
				for _, status := range pod.Status.ContainerStatuses {
					if !status.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					return nil
				}
			}
		}
	}
}

// ExecCommand executes a command in a pod container.
func (c *Client) ExecCommand(ctx context.Context, podName string, command []string) (string, error) {
	args := []string{}

	// Add context if specified
	if c.kubeContext != "" {
		args = append(args, "--context", c.kubeContext)
	}

	// Add namespace if specified
	if c.namespace != "" {
		args = append(args, "-n", c.namespace)
	}

	// Add exec command
	args = append(args, "exec", podName, "--")
	args = append(args, command...)

	output, err := c.runKubectl(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to exec command in pod %s: %w", podName, err)
	}

	return output, nil
}

// ReadFileFromPod reads a file from a pod using kubectl exec cat.
func (c *Client) ReadFileFromPod(ctx context.Context, podName, filePath string) (string, error) {
	return c.ExecCommand(ctx, podName, []string{"cat", filePath})
}

// DeletePod deletes a pod by name in the configured namespace.
func (c *Client) DeletePod(ctx context.Context, name string) error {
	args := []string{"delete", "pod", name}

	// Add context if specified
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	// Add namespace if specified
	if c.namespace != "" {
		args = append(args, "-n", c.namespace)
	}

	// Add grace period to delete immediately
	args = append(args, "--grace-period=0", "--force")

	_, err := c.runKubectl(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", name, err)
	}

	return nil
}

// Context returns the Kubernetes context this client is configured for.
func (c *Client) Context() string {
	return c.kubeContext
}

// Namespace returns the namespace this client is configured for.
func (c *Client) Namespace() string {
	return c.namespace
}

// runKubectl executes a kubectl command with the given arguments.
func (c *Client) runKubectl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
