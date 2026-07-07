package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

const versionMismatchWarn = "It is recommended to use the same oc-mirror version for both mirror-to-disk and disk-to-mirror"

// VersionMetadata records which oc-mirror build produced a mirror-to-disk archive.
type VersionMetadata struct {
	GitVersion string `json:"gitVersion"`
	GitCommit  string `json:"gitCommit"`
}

// NewVersionMetadata builds metadata from the current binary version info.
func NewVersionMetadata(info Info) VersionMetadata {
	return VersionMetadata{
		GitVersion: info.GitVersion,
		GitCommit:  info.GitCommit,
	}
}

// WriteVersionMetadata writes version metadata under workingDir/internal/.
func WriteVersionMetadata(workingDir string, info Info) error {
	internalDir := filepath.Join(workingDir, config.InternalDir)
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		return fmt.Errorf("unable to create internal directory: %w", err)
	}

	meta := NewVersionMetadata(info)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal version metadata: %w", err)
	}

	path := filepath.Join(workingDir, config.VersionMetadataPath)
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // version metadata is not sensitive info
		return fmt.Errorf("unable to write version metadata: %w", err)
	}

	return nil
}

// ReadVersionMetadata reads version metadata from the given file path.
func ReadVersionMetadata(path string) (VersionMetadata, error) {
	meta, err := parser.ParseJsonFile[VersionMetadata](path)
	if err != nil {
		return VersionMetadata{}, fmt.Errorf("version metadata: %w", err)
	}
	return meta, nil
}

// CheckVersionMetadata returns a warning message when the archive metadata is missing,
// unreadable, or does not match the current binary. An empty string means no warning.
func CheckVersionMetadata(workingDir string, current Info) string {
	path := filepath.Join(workingDir, config.VersionMetadataPath)

	meta, err := ReadVersionMetadata(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Sprintf("No oc-mirror version metadata found in this archive. Mirror-to-disk may have been run with an older oc-mirror release that did not record version information. %s.", versionMismatchWarn)
		}
		return fmt.Sprintf("Unable to read oc-mirror version metadata from %s: %v", path, err)
	}

	if meta.GitVersion == current.GitVersion && meta.GitCommit == current.GitCommit {
		return ""
	}

	return fmt.Sprintf(
		"oc-mirror version mismatch: this archive was created with version %s (commit %s), but the current binary is version %s (commit %s). %s.",
		meta.GitVersion,
		meta.GitCommit,
		current.GitVersion,
		current.GitCommit,
		versionMismatchWarn,
	)
}
