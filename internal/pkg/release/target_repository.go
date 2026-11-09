package release

import (
	"fmt"
	"strings"

	"go.podman.io/image/v5/docker/reference"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type targetRepository struct{ field, repo string }

func platformTargetRepositories(platform v2alpha1.Platform) []targetRepository {
	return []targetRepository{
		{"targetImageRepo", platform.TargetImageRepo},
		{"targetComponentRepo", platform.TargetComponentRepo},
	}
}

// ValidateTargetRepositories checks repository syntax when loading an ISC or DISC.
// Destination-dependent checks are deferred until the CLI knows the workflow and registry.
func ValidateTargetRepositories(platform v2alpha1.Platform) []error {
	var errs []error
	for _, target := range platformTargetRepositories(platform) {
		if target.repo != "" && (len(target.repo) > reference.NameTotalLengthMax || !v2alpha1.IsValidPathComponent(target.repo)) {
			errs = append(errs, fmt.Errorf("platform.%s %q must be a lowercase repository path of at most %d characters without a transport, tag, or digest", target.field, target.repo, reference.NameTotalLengthMax))
		}
	}
	return errs
}

// ValidatePlatformDestination checks the complete names and nesting limits once
// the destination (or mirror-to-disk cache registry) is known. Syntax is checked
// separately by ValidateTargetRepositories during configuration loading.
func ValidatePlatformDestination(platform v2alpha1.Platform, opts mirror.CopyOptions) error {
	if platform.TargetImageRepo == "" && platform.TargetComponentRepo == "" {
		return nil
	}
	destination := strings.TrimPrefix(opts.Destination, consts.DockerProtocol)
	if opts.IsMirrorToDisk() {
		destination = opts.LocalStorageFQDN
	}
	for _, target := range platformTargetRepositories(platform) {
		if err := validateTargetRepository(platform, opts, destination, target); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetRepository(platform v2alpha1.Platform, opts mirror.CopyOptions, destination string, target targetRepository) error {
	if target.repo == "" {
		return nil
	}
	ref, err := reference.WithName(destination + "/" + target.repo)
	if err != nil {
		return fmt.Errorf("platform.%s %q with destination %q is invalid: %w", target.field, target.repo, destination, err)
	}
	if limit := opts.Global.MaxNestedPaths; limit > 0 && strings.Count(reference.Path(ref), "/")+1 > limit {
		return fmt.Errorf("platform.%s %q with destination %q exceeds --max-nested-paths=%d; choose a repository and destination namespace that fit the limit", target.field, target.repo, opts.Destination, limit)
	}
	if platform.Graph {
		return validateGraphRepository(destination, ref.Name(), target.field, opts)
	}
	return nil
}

// Graph construction can push to the original graph repository before the batch
// applies nesting limits. Reserve both locations without changing graph routing.
func validateGraphRepository(destination, targetRepository, field string, opts mirror.CopyOptions) error {
	graphRef := destination + "/" + graphImageName + ":latest"
	graphRefs := []string{graphRef}
	if !opts.IsMirrorToDisk() && opts.Global.MaxNestedPaths > 0 {
		flattened, err := image.WithMaxNestedPaths(graphRef, opts.Global.MaxNestedPaths)
		if err != nil {
			return err
		}
		graphRefs = append(graphRefs, flattened)
	}
	for _, graphRef := range graphRefs {
		graphSpec, err := image.ParseRef(graphRef)
		if err != nil {
			return err
		}
		if targetRepository == graphSpec.Name {
			return fmt.Errorf("platform.%s repository %q conflicts with the Cincinnati graph repository %q", field, targetRepository, graphSpec.Name)
		}
	}
	return nil
}

// ValidatePlatformImageDestinations prevents repository overrides from assigning
// different images to the same tag. Identical digest-pinned images may share it.
func ValidatePlatformImageDestinations(platform v2alpha1.Platform, images []v2alpha1.CopyImageSchema) error {
	if platform.TargetImageRepo == "" && platform.TargetComponentRepo == "" {
		return nil
	}
	origins := make(map[string]v2alpha1.CopyImageSchema, len(images))
	for _, img := range images {
		img.Origin = strings.TrimPrefix(img.Origin, consts.DockerProtocol)
		previous, exists := origins[img.Destination]
		if exists && previous.Origin != img.Origin {
			if sameImageDigest(previous.Origin, img.Origin) {
				continue
			}
			return fmt.Errorf("conflicting destination %q for %q (%s) and %q (%s)", img.Destination,
				previous.Origin, imageDestinationConfigSource(platform, previous.Type), img.Origin, imageDestinationConfigSource(platform, img.Type))
		}
		origins[img.Destination] = img
	}
	return nil
}

// A tag-only reference does not establish identity with a digest-pinned reference.
func sameImageDigest(first, second string) bool {
	firstSpec, firstErr := image.ParseRef(first)
	secondSpec, secondErr := image.ParseRef(second)
	return firstErr == nil && secondErr == nil && firstSpec.Digest != "" &&
		firstSpec.Digest == secondSpec.Digest && firstSpec.Algorithm == secondSpec.Algorithm
}

func imageDestinationConfigSource(platform v2alpha1.Platform, imageType v2alpha1.ImageType) string {
	switch imageType {
	case v2alpha1.TypeOCPRelease:
		if repo := platform.TargetImageRepo; repo != "" {
			return fmt.Sprintf("platform.targetImageRepo %q", repo)
		}
		return "platform release payload (default repository)"
	case v2alpha1.TypeOCPReleaseContent, v2alpha1.TypeKubeVirtContainer:
		if repo := platform.TargetComponentRepo; repo != "" {
			return fmt.Sprintf("platform.targetComponentRepo %q", repo)
		}
		return "platform release component (default repository)"
	case v2alpha1.TypeCincinnatiGraph:
		return "platform.graph"
	case v2alpha1.TypeGeneric:
		return "additionalImages"
	case v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorRelatedImage:
		return "operators"
	case v2alpha1.TypeHelmImage:
		return "helm"
	case v2alpha1.TypeInvalid:
		return "unknown image type"
	}
	return fmt.Sprintf("unknown image type %d", imageType)
}
