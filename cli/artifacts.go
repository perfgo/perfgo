package cli

// This file contains artifact management functionality for saving
// test binaries and performance profiles to the history directory.

import (
	"crypto/sha256"
	"encoding/base32"
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

func (a *App) rewriteProfilePaths(profileFile, runDir, destBinary, originalBasename string) error {
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
	// Match based on the original basename (without hash prefix and .binary suffix)
	for _, mapping := range prof.Mapping {
		// Skip kernel mappings
		if strings.HasPrefix(mapping.File, "[") {
			continue
		}

		// Check if this mapping is for the test binary by comparing original basename
		mappingBasename := filepath.Base(mapping.File)
		if mappingBasename == originalBasename {
			oldPath := mapping.File
			// Update to use archived path
			mapping.File = destBinary
			a.logger.Debug().
				Str("old", oldPath).
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

func (a *App) saveArtifacts(runDir string, history *model.History, testBinaryPath string) error {
	// Save the test binary if provided
	if testBinaryPath != "" {
		if _, err := os.Stat(testBinaryPath); err == nil {
			// Read and hash the test binary
			data, err := os.ReadFile(testBinaryPath)
			if err != nil {
				a.logger.Warn().Err(err).Str("file", testBinaryPath).Msg("Failed to read test binary")
			} else {
				// Calculate SHA256 hash
				hashBytes := sha256.Sum256(data)
				hash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hashBytes[:]))

				// Construct filename with hash and original basename
				basename := filepath.Base(testBinaryPath)
				binaryFilename := hash + "." + basename + ".binary"
				destBinary := filepath.Join(runDir, binaryFilename)

				// Write to destination
				if err := os.WriteFile(destBinary, data, 0755); err != nil {
					a.logger.Warn().Err(err).Str("file", testBinaryPath).Msg("Failed to write test binary")
				} else {
					history.Artifacts = append(history.Artifacts, model.Artifact{
						Type: model.ArtifactTypeTestBinary,
						Size: uint64(len(data)),
						File: binaryFilename,
					})
					a.logger.Debug().
						Str("hash", hash).
						Str("dest", binaryFilename).
						Msg("Saved test binary")
				}
			}
		}
	}

	// Register perf.pb.gz profile if it exists (written directly to runDir)
	profileFile := filepath.Join(runDir, "perf.pb.gz")
	if info, err := os.Stat(profileFile); err == nil {
		// Find the test binary to rewrite paths
		var destBinary string
		var originalBasename string
		for _, artifact := range history.Artifacts {
			if artifact.Type == model.ArtifactTypeTestBinary {
				destBinary = filepath.Join(runDir, artifact.File)
				// Extract original basename from hash-based name: hash.basename.binary -> basename
				parts := strings.SplitN(artifact.File, ".", 3)
				if len(parts) >= 3 && strings.HasSuffix(artifact.File, ".binary") {
					// Remove the .binary suffix to get the original basename
					originalBasename = strings.TrimSuffix(parts[1]+"."+parts[2], ".binary")
				}
				break
			}
		}

		if destBinary != "" && originalBasename != "" {
			// Rewrite profile paths to point to saved binary
			if err := a.rewriteProfilePaths(profileFile, runDir, destBinary, originalBasename); err != nil {
				a.logger.Warn().Err(err).Msg("Failed to rewrite profile paths, using original")
			}
		}

		history.Artifacts = append(history.Artifacts, model.Artifact{
			Type: model.ArtifactTypePprofProfile,
			Size: uint64(info.Size()),
			File: "perf.pb.gz",
		})
		a.logger.Debug().Str("profile", profileFile).Msg("Registered pprof profile artifact")
	}

	return nil
}
