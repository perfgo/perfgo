package k8s_test

import (
	"context"
	"fmt"
	"time"

	"github.com/perfgo/perfgo/cli/k8s"
)

func ExampleClient_GetNodes() {
	// Create a client for the current context
	client := k8s.New("", "")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all nodes in the cluster
	nodes, err := client.GetNodes(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print node information
	for _, node := range nodes {
		fmt.Printf("Node: %s (OS: %s, Arch: %s)\n",
			node.Metadata.Name,
			node.Status.NodeInfo.OperatingSystem,
			node.Status.NodeInfo.Architecture)
	}
}

func ExampleClient_GetPods() {
	// Create a client for the "production" context and "default" namespace
	client := k8s.New("production", "default")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all pods in the configured namespace
	pods, err := client.GetPods(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print pod information
	for _, pod := range pods {
		fmt.Printf("Pod: %s (Phase: %s, Node: %s)\n",
			pod.Metadata.Name,
			pod.Status.Phase,
			pod.Spec.NodeName)
	}
}

func ExampleClient_GetPodsInNamespace() {
	// Create a client for the current context
	client := k8s.New("", "")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all pods in a specific namespace
	pods, err := client.GetPodsInNamespace(ctx, "kube-system")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print pod information
	for _, pod := range pods {
		fmt.Printf("Pod: %s/%s\n",
			pod.Metadata.Namespace,
			pod.Metadata.Name)
	}
}

func ExampleNew() {
	// Use current context and default namespace
	client1 := k8s.New("", "")

	// Use specific context and default namespace
	client2 := k8s.New("production", "")

	// Use specific context and namespace
	client3 := k8s.New("production", "my-app")

	fmt.Printf("Client 1 - Context: %q, Namespace: %q\n", client1.Context(), client1.Namespace())
	fmt.Printf("Client 2 - Context: %q, Namespace: %q\n", client2.Context(), client2.Namespace())
	fmt.Printf("Client 3 - Context: %q, Namespace: %q\n", client3.Context(), client3.Namespace())

	// Output:
	// Client 1 - Context: "", Namespace: ""
	// Client 2 - Context: "production", Namespace: ""
	// Client 3 - Context: "production", Namespace: "my-app"
}
