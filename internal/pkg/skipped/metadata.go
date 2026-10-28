// Package skipped persists images a mirror-to-disk run intentionally did not
// mirror, so disk-to-mirror makes the same decision instead of failing on a
// cache miss.
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

// WriteSkippedImages records the skipped images under workingDir/internal/.
// An empty list is valid: it records that nothing was skipped.
func WriteSkippedImages(workingDir string, images []v2alpha1.SkippedImage) error {
	internalDir := filepath.Join(workingDir, config.InternalDir)
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		return fmt.Errorf("unable to create internal directory: %w", err)
	}

	if images == nil {
		images = []v2alpha1.SkippedImage{}
	}

	data, err := json.MarshalIndent(images, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal skipped images: %w", err)
	}

	path := filepath.Join(workingDir, config.SkippedImagesPath)
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // image references are not sensitive info
		return fmt.Errorf("unable to write skipped images: %w", err)
	}

	return nil
}

// ReadSkippedImages reads the record under workingDir/internal/. A missing
// file is not an error: older archives recorded nothing.
func ReadSkippedImages(workingDir string) ([]v2alpha1.SkippedImage, error) {
	path := filepath.Join(workingDir, config.SkippedImagesPath)

	images, err := parser.ParseJsonFile[[]v2alpha1.SkippedImage](path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("skipped images: %w", err)
	}

	return images, nil
}

func Origins(images []v2alpha1.SkippedImage) map[string]struct{} {
	origins := make(map[string]struct{}, len(images))
	for _, img := range images {
		origins[img.Origin] = struct{}{}
	}
	return origins
}
