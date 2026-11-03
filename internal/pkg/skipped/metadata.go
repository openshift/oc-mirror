// Package skipped preserves operand platform exceptions for offline mirroring.
package skipped

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

// WriteOperandPlatforms replaces the previous run's decisions, even when empty.
// A present origin with no platforms means that the whole operand was skipped.
func WriteOperandPlatforms(workingDir string, platforms map[string][]v2alpha1.InstancePlatformFilter) error {
	if err := os.MkdirAll(filepath.Join(workingDir, config.InternalDir), 0o755); err != nil {
		return fmt.Errorf("create operand platform metadata directory: %w", err)
	}
	if platforms == nil {
		platforms = map[string][]v2alpha1.InstancePlatformFilter{}
	}
	data, err := json.Marshal(platforms)
	if err != nil {
		return fmt.Errorf("encode operand platform decisions: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workingDir, config.SkippedImagesPath), data, 0o644); err != nil { // #nosec G306 -- non-secret metadata must remain readable across users during disk-to-mirror.
		return fmt.Errorf("write operand platform decisions: %w", err)
	}
	return nil
}

// ReadOperandPlatforms allows older archives without a decision file.
func ReadOperandPlatforms(workingDir string) (map[string][]v2alpha1.InstancePlatformFilter, error) {
	platforms, err := parser.ParseJsonFile[map[string][]v2alpha1.InstancePlatformFilter](filepath.Join(workingDir, config.SkippedImagesPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("operand platform decisions: %w", err)
	}
	return platforms, nil
}
