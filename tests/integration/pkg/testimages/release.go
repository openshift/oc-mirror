package testimages

import (
	"fmt"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	// releaseRepo is the repository prefix of release payloads and their components.
	releaseRepo = "oc-mirror/release"
	// releaseTag is the tag of every release payload and component.
	releaseTag = "v0.0.1"
)

// releasePayload describes a fake OpenShift release payload and the single component
// image its image-references point to.
type releasePayload struct {
	// name is the payload repository under releaseRepo, and the directory under
	// release/ holding its release-manifests.
	name string
	// platforms of the payload. A single platform produces an image, several an index.
	platforms []v1.Platform
	// component is the component repository under releaseRepo.
	component string
	// componentPlatforms of the component. A single platform produces an image, several an index.
	componentPlatforms []v1.Platform
}

var releasePayloads = []releasePayload{
	{
		name:               "test-release-index",
		platforms:          []v1.Platform{linuxAMD64},
		component:          "test-image",
		componentPlatforms: []v1.Platform{linuxAMD64},
	},
	{
		name:               "test-release-index-multi",
		platforms:          []v1.Platform{linuxAMD64, linuxARM64, linuxS390X, linuxPPC64LE},
		component:          "test-image-multi",
		componentPlatforms: []v1.Platform{linuxAMD64, linuxARM64},
	},
}

// buildReleaseImages builds and pushes, for each releasePayloads entry, its component
// image and its payload image, whose only layer holds the release-manifests directory
// found in dir/<payload name>. Payloads are signed for oc-mirror's release signature
// verification (see signRelease).
func (b *builder) buildReleaseImages(dir string) error {
	for _, p := range releasePayloads {
		component := imageSpec{
			mediaTypes: ociMediaTypes,
			labels:     map[string]string{"io.openshift.release.component": "alpine"},
			layers:     [][]layerFile{{{path: "simple-release", content: []byte(releaseRepo + "/" + p.component + "\n")}}},
		}
		if err := b.pushImageForPlatforms(releaseRepo+"/"+p.component+":"+releaseTag, component, p.componentPlatforms); err != nil {
			return fmt.Errorf("release component %s: %w", p.component, err)
		}

		files, err := filesFromDir(filepath.Join(dir, p.name), "", []string{"release-manifests"}, b.inventory.render)
		if err != nil {
			return fmt.Errorf("release payload %s: %w", p.name, err)
		}
		payload := imageSpec{
			mediaTypes: ociMediaTypes,
			labels: map[string]string{
				"io.openshift.release":                   "4.18.0-0.ocp-2025-03-24-022732",
				"io.openshift.release.base-image-digest": "sha256:0ed55ee636f5703b212337ff8a82833c5dfc4553069dc3f6cd52564cc0adcd08",
			},
			layers: [][]layerFile{files},
		}
		repoTag := releaseRepo + "/" + p.name + ":" + releaseTag
		if err := b.pushImageForPlatforms(repoTag, payload, p.platforms); err != nil {
			return fmt.Errorf("release payload %s: %w", p.name, err)
		}
		digest, err := b.inventory.Digest(repoTag)
		if err != nil {
			return err
		}
		if err := b.signRelease(repoTag, digest); err != nil {
			return fmt.Errorf("release payload %s: %w", p.name, err)
		}
	}
	return nil
}

// pushImageForPlatforms builds spec as a single image if there is one platform, or
// as an image index otherwise, and pushes it as repoTag.
func (b *builder) pushImageForPlatforms(repoTag string, spec imageSpec, platforms []v1.Platform) error {
	if len(platforms) == 1 {
		spec.platform = platforms[0]
		img, err := buildImage(spec)
		if err != nil {
			return err
		}
		return b.pushImage(repoTag, img)
	}
	idx, err := buildMultiArchImage(spec, platforms)
	if err != nil {
		return err
	}
	return b.pushIndex(repoTag, idx)
}
