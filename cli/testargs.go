package cli

// This file contains argument processing utilities for separating
// build-time and runtime test arguments.

import (
	"fmt"
	"strings"
)

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
