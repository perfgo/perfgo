package cli

// This file contains artifact management functionality for saving
// test binaries and performance profiles to the history directory.

import (
	"crypto/sha256"
	"encoding/base32"
	"os"
	"path/filepath"
	"strings"

	"github.com/perfgo/perfgo/model"
)

func (a *App) saveArtifacts(runDir string, history *model.History, testBinaryPath string) error {
	// Save the test binary if not already archived by ConvertPerfToPprof/ProcessPerfData.
	if testBinaryPath != "" {
		alreadyArchived := false
		for _, artifact := range history.Artifacts {
			if artifact.Type == model.ArtifactTypeTestBinary {
				alreadyArchived = true
				break
			}
		}

		if !alreadyArchived {
			if _, err := os.Stat(testBinaryPath); err == nil {
				data, err := os.ReadFile(testBinaryPath)
				if err != nil {
					a.logger.Warn().Err(err).Str("file", testBinaryPath).Msg("Failed to read test binary")
				} else {
					hashBytes := sha256.Sum256(data)
					hash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hashBytes[:]))
					basename := filepath.Base(testBinaryPath)
					binaryFilename := hash + "." + basename + ".binary"
					destBinary := filepath.Join(runDir, binaryFilename)

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
	}

	// Register perf.pb.gz profile if it exists (written directly to runDir).
	profileFile := filepath.Join(runDir, "perf.pb.gz")
	if info, err := os.Stat(profileFile); err == nil {
		history.Artifacts = append(history.Artifacts, model.Artifact{
			Type: model.ArtifactTypePprofProfile,
			Size: uint64(info.Size()),
			File: "perf.pb.gz",
		})
		a.logger.Debug().Str("profile", profileFile).Msg("Registered pprof profile artifact")
	}

	return nil
}
