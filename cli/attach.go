package cli

// This file contains the attach command for running performance profiling
// on Kubernetes pods or nodes.

import (
	"context"
	"fmt"
	"time"

	"github.com/perfgo/perfgo/cli/k8s"
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

	if podName != "" {
		targetType = "pod"
		target = podName

		// Verify pod exists
		a.logger.Info().
			Str("pod", podName).
			Str("namespace", namespace).
			Msg("Verifying pod exists")

		pods, err := k8sClient.GetPods(execCtx)
		if err != nil {
			return fmt.Errorf("failed to list pods: %w", err)
		}

		found := false
		for _, pod := range pods {
			if pod.Metadata.Name == podName {
				found = true
				a.logger.Info().
					Str("pod", podName).
					Str("node", pod.Spec.NodeName).
					Str("phase", pod.Status.Phase).
					Msg("Found pod")
				break
			}
		}

		if !found {
			return fmt.Errorf("pod %s not found in namespace %s", podName, namespace)
		}
	} else {
		targetType = "node"
		target = nodeName

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
