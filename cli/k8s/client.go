package k8s

// Package k8s provides a client interface to Kubernetes clusters via kubectl.
// It manages kubectl command execution with context and namespace configuration.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	Phase      string           `json:"phase"`
	Conditions []PodCondition   `json:"conditions,omitempty"`
	PodIP      string           `json:"podIP,omitempty"`
	HostIP     string           `json:"hostIP,omitempty"`
	StartTime  *time.Time       `json:"startTime,omitempty"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
}

// PodCondition represents a condition of the pod.
type PodCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// ContainerStatus contains container status information.
type ContainerStatus struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
	State ContainerState `json:"state"`
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
