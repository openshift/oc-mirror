package operator

import (
	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// packagesWithoutVersionBounds returns a copy of the operator filter that keeps
// package/channel names but drops min/max version bounds so sibling versions
// remain visible when computing shared related images for delete.
func packagesWithoutVersionBounds(op v2alpha1.Operator) v2alpha1.Operator {
	out := op
	out.Packages = make([]v2alpha1.IncludePackage, len(op.Packages))
	for i, pkg := range op.Packages {
		p := pkg
		p.MinVersion = ""
		p.MaxVersion = ""
		if len(pkg.Channels) > 0 {
			p.Channels = make([]v2alpha1.IncludeChannel, len(pkg.Channels))
			for j, ch := range pkg.Channels {
				c := ch
				c.MinVersion = ""
				c.MaxVersion = ""
				p.Channels[j] = c
			}
		}
		out.Packages[i] = p
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

// excludeRelatedImagesSharedWithSiblingBundles drops related images from the
// delete set when another (non-deleted) bundle of the same package still
// references the same image digest (OCPBUGS-61236). Bundle images for the
// versions being deleted are always kept in the delete set.
func excludeRelatedImagesSharedWithSiblingBundles(
	log clog.PluggableLoggerInterface,
	deleteRelated map[string][]v2alpha1.RelatedImage,
	deleteDC *declcfg.DeclarativeConfig,
	packageWideDC *declcfg.DeclarativeConfig,
) map[string][]v2alpha1.RelatedImage {
	if deleteDC == nil || packageWideDC == nil || len(deleteRelated) == 0 {
		return deleteRelated
	}

	deleteBundles := make(map[string]struct{}, len(deleteDC.Bundles))
	deleteBundleImages := make(map[string]struct{}, len(deleteDC.Bundles))
	for _, b := range deleteDC.Bundles {
		deleteBundles[b.Name] = struct{}{}
		deleteBundleImages[b.Image] = struct{}{}
	}

	keepImages := make(map[string]struct{})
	for _, b := range packageWideDC.Bundles {
		if _, deleting := deleteBundles[b.Name]; deleting {
			continue
		}
		for _, ri := range b.RelatedImages {
			keepImages[ri.Image] = struct{}{}
		}
	}
	if len(keepImages) == 0 {
		return deleteRelated
	}

	filtered := make(map[string][]v2alpha1.RelatedImage, len(deleteRelated))
	skipped := 0
	for bundleName, images := range deleteRelated {
		kept := make([]v2alpha1.RelatedImage, 0, len(images))
		for _, img := range images {
			// Always delete the bundle manifest for the targeted version.
			if img.Type == v2alpha1.TypeOperatorBundle || img.Image == "" {
				kept = append(kept, img)
				continue
			}
			if _, isBundleImg := deleteBundleImages[img.Image]; isBundleImg {
				kept = append(kept, img)
				continue
			}
			if _, shared := keepImages[img.Image]; shared {
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
