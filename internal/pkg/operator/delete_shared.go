package operator

import (
	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// packagesWithoutVersionBounds returns a copy of the operator filter that keeps
// selected package names but includes every channel and version of those
// packages. Channel lists and version bounds would hide sibling bundles that
// still share related images with the version being deleted.
func packagesWithoutVersionBounds(op v2alpha1.Operator) v2alpha1.Operator {
	out := op
	out.Full = true
	out.Packages = make([]v2alpha1.IncludePackage, len(op.Packages))
	for i, pkg := range op.Packages {
		out.Packages[i] = v2alpha1.IncludePackage{Name: pkg.Name}
	}
	return out
}

func hasVersionConstraints(op v2alpha1.Operator) bool {
	for _, pkg := range op.Packages {
		if pkg.MinVersion != "" || pkg.MaxVersion != "" {
			return true
		}
		for _, ch := range pkg.Channels {
			if ch.MinVersion != "" || ch.MaxVersion != "" {
				return true
			}
		}
	}
	return false
}

// destIdentity is the mirrored destination repository plus digest (or tag).
// Source registries that share a path and digest map to the same destination.
func destIdentity(ref string) string {
	spec, err := image.ParseRef(ref)
	if err != nil {
		return ref
	}
	if spec.PathComponent == "" {
		return ref
	}
	if spec.Digest != "" {
		algo := spec.Algorithm
		if algo == "" {
			algo = "sha256"
		}
		return spec.PathComponent + "@" + algo + ":" + spec.Digest
	}
	if spec.Tag != "" {
		return spec.PathComponent + ":" + spec.Tag
	}
	return spec.PathComponent
}

func destIdentitySet(refs ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		out[destIdentity(ref)] = struct{}{}
	}
	return out
}

func mergeBundleNames(sets ...map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	for _, set := range sets {
		for name := range set {
			out[name] = struct{}{}
		}
	}
	return out
}

// excludeRelatedImagesSharedWithSiblingBundles drops related images from the
// delete set when another (non-deleted) bundle of the same package still
// references the same destination image (OCPBUGS-61236). Bundle images for the
// versions being deleted are always kept in the delete set.
func excludeRelatedImagesSharedWithSiblingBundles(
	log clog.PluggableLoggerInterface,
	deleteRelated map[string][]v2alpha1.RelatedImage,
	deleteDC *declcfg.DeclarativeConfig,
	packageWideDC *declcfg.DeclarativeConfig,
	unionDeleteBundles map[string]struct{},
	siblingPresent func(bundle declcfg.Bundle) bool,
) map[string][]v2alpha1.RelatedImage {
	if deleteDC == nil || packageWideDC == nil || len(deleteRelated) == 0 {
		return deleteRelated
	}

	deleteBundles, deleteBundleImages := deleteBundleIdentity(deleteDC)
	deleteBundles = mergeBundleNames(deleteBundles, unionDeleteBundles)
	keepImages := siblingKeepImages(packageWideDC, deleteBundles, siblingPresent)
	if len(keepImages) == 0 {
		return deleteRelated
	}
	return filterSharedRelatedImages(log, deleteRelated, deleteBundleImages, keepImages)
}

func deleteBundleIdentity(dc *declcfg.DeclarativeConfig) (names, images map[string]struct{}) {
	names = make(map[string]struct{}, len(dc.Bundles))
	images = make(map[string]struct{}, len(dc.Bundles))
	for _, b := range dc.Bundles {
		names[b.Name] = struct{}{}
		if id := destIdentity(b.Image); id != "" {
			images[id] = struct{}{}
		}
	}
	return names, images
}

func siblingKeepImages(
	packageWideDC *declcfg.DeclarativeConfig,
	deleteBundles map[string]struct{},
	siblingPresent func(bundle declcfg.Bundle) bool,
) map[string]struct{} {
	keep := make(map[string]struct{})
	for _, b := range packageWideDC.Bundles {
		if _, deleting := deleteBundles[b.Name]; deleting {
			continue
		}
		if siblingPresent != nil && !siblingPresent(b) {
			continue
		}
		for _, ri := range b.RelatedImages {
			if id := destIdentity(ri.Image); id != "" {
				keep[id] = struct{}{}
			}
		}
	}
	return keep
}

func mustKeepInDeleteSet(img v2alpha1.RelatedImage, deleteBundleImages map[string]struct{}) bool {
	if img.Type == v2alpha1.TypeOperatorBundle || img.Image == "" {
		return true
	}
	_, isBundleImg := deleteBundleImages[destIdentity(img.Image)]
	return isBundleImg
}

func filterSharedRelatedImages(
	log clog.PluggableLoggerInterface,
	deleteRelated map[string][]v2alpha1.RelatedImage,
	deleteBundleImages, keepImages map[string]struct{},
) map[string][]v2alpha1.RelatedImage {
	filtered := make(map[string][]v2alpha1.RelatedImage, len(deleteRelated))
	skipped := 0
	for bundleName, images := range deleteRelated {
		kept := make([]v2alpha1.RelatedImage, 0, len(images))
		for _, img := range images {
			if mustKeepInDeleteSet(img, deleteBundleImages) {
				kept = append(kept, img)
				continue
			}
			if _, shared := keepImages[destIdentity(img.Image)]; shared {
				skipped++
				log.Info("skipping delete of related image %s (still referenced by another operator version)", img.Image)
				continue
			}
			kept = append(kept, img)
		}
		if len(kept) > 0 {
			filtered[bundleName] = kept
		}
	}
	if skipped > 0 {
		log.Info("excluded %d related image(s) from delete set because they are shared with other operator versions", skipped)
	}
	return filtered
}
