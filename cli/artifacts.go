package cli

// This file contains artifact management functionality for saving
// test binaries and performance profiles to the history directory.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/pprof/profile"
	"github.com/perfgo/perfgo/model"
)

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
