package fixtures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	releaseManifestsDir = "release-manifests"
	releaseVersionLabel = "io.openshift.release"
)

// Release builds the release payload image of releaseDir for one platform. Like an OpenShift
// release image, its layer holds the release-manifests directory, whose image-references file
// lists the release component images, and its release version is set as a label.
func Release(releaseDir string, platform v1.Platform) (v1.Image, error) {
	manifestsDir := filepath.Join(releaseDir, releaseManifestsDir)
	version, err := releaseVersion(manifestsDir)
	if err != nil {
		return nil, err
	}
	return build(nil, platform, []dirCopy{{src: manifestsDir, dest: releaseManifestsDir}},
		v1.Config{Labels: map[string]string{releaseVersionLabel: version}})
}

// releaseVersion reads the version from the release-metadata file of a release-manifests directory.
func releaseVersion(manifestsDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(manifestsDir, "release-metadata"))
	if err != nil {
		return "", err
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("failed to parse the release-metadata of %s: %w", manifestsDir, err)
	}
	if metadata.Version == "" {
		return "", fmt.Errorf("the release-metadata of %s has no version", manifestsDir)
	}
	return metadata.Version, nil
}
