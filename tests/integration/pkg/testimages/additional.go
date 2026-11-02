package testimages

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// additionalImage is a synthetic image used as additional image (or as image
// referenced by a Helm chart). Its content is irrelevant: each one only reproduces
// the manifest structure that some tests rely on.
type additionalImage struct {
	repoTag    string
	mediaTypes mediaTypes
	// platforms of the image. A single platform produces an image, several an index
	// where each platform has a distinct layer.
	platforms []v1.Platform
	// layers is the number of layers of each image.
	layers int
	// unsigned images don't get a cosign signature.
	unsigned bool
}

var linuxARMv7 = v1.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}

var additionalImages = []additionalImage{
	{
		// OCI index for the four OpenShift architectures, to test platform filtering
		// (sparse manifests) of additional images.
		repoTag:    "oc-mirror/multi-platform-image:latest",
		mediaTypes: ociMediaTypes,
		platforms:  []v1.Platform{linuxAMD64, linuxARM64, linuxS390X, linuxPPC64LE},
		layers:     1,
	},
	{
		// Single-platform OCI manifest (no index), to test platform filters on
		// images that aren't multi-arch.
		repoTag:    "oc-mirror/single-manifest-image:latest",
		mediaTypes: ociMediaTypes,
		platforms:  []v1.Platform{linuxAMD64},
		layers:     1,
	},
	{
		// Unsigned Docker manifest list (not OCI), outside of the oc-mirror namespace.
		// Used to test manifest list handling (e.g. dry-run sub-manifest mappings) and
		// the rejection of unsigned images by trust policies.
		repoTag:    "unsigned/docker-manifest-list:latest",
		mediaTypes: dockerMediaTypes,
		platforms:  []v1.Platform{linuxARM64, linuxPPC64LE},
		layers:     1,
		unsigned:   true,
	},
	{
		// Docker image with many layers, to test parallel layer copies.
		repoTag:    "oc-mirror/oc-mirror-dev:multilayers-latest",
		mediaTypes: dockerMediaTypes,
		platforms:  []v1.Platform{linuxAMD64},
		layers:     15,
	},
	{
		// Docker manifest list referenced by the test-chart Helm chart, including a
		// platform variant (arm/v7), to test platform filtering of Helm images.
		repoTag:    "oc-mirror/test-chart:1.0.0",
		mediaTypes: dockerMediaTypes,
		platforms:  []v1.Platform{linuxAMD64, linuxARM64, linuxARMv7},
		layers:     1,
	},
}

// buildAdditionalImages builds and pushes the additionalImages.
func (b *builder) buildAdditionalImages() error {
	for _, ai := range additionalImages {
		if err := b.buildAdditionalImage(ai); err != nil {
			return fmt.Errorf("additional image %s: %w", ai.repoTag, err)
		}
	}
	return nil
}

func (b *builder) buildAdditionalImage(ai additionalImage) error {
	build := func(platform v1.Platform) (v1.Image, error) {
		spec := imageSpec{platform: platform, mediaTypes: ai.mediaTypes}
		for i := range ai.layers {
			content := fmt.Sprintf("%s %s layer %d\n", ai.repoTag, platform.String(), i)
			spec.layers = append(spec.layers, []layerFile{{path: fmt.Sprintf("layer-%d", i), content: []byte(content)}})
		}
		return buildImage(spec)
	}
	push := b.pushImage
	pushIndex := b.pushIndex
	if ai.unsigned {
		push = b.pushUnsignedImage
		pushIndex = b.pushUnsignedIndex
	}

	if len(ai.platforms) == 1 {
		img, err := build(ai.platforms[0])
		if err != nil {
			return err
		}
		return push(ai.repoTag, img)
	}

	var images []v1.Image
	for _, platform := range ai.platforms {
		img, err := build(platform)
		if err != nil {
			return err
		}
		images = append(images, img)
	}
	idx, err := newIndex(ai.mediaTypes, ai.platforms, images)
	if err != nil {
		return err
	}
	return pushIndex(ai.repoTag, idx)
}
