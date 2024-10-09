package operator

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type OperatorCollector struct {
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v2alpha1.ImageSetConfiguration
	Opts             mirror.CopyOptions
	LocalStorageFQDN string
	destReg          string
	ctlgHandler      catalogHandlerInterface
}

func (o OperatorCollector) destinationRegistry() string {
	if o.destReg == "" {
		if o.Opts.Mode == mirror.DiskToMirror || o.Opts.Mode == mirror.MirrorToMirror {
			o.destReg = strings.TrimPrefix(o.Opts.Destination, dockerProtocol)
		} else {
			o.destReg = o.LocalStorageFQDN
		}
	}
	return o.destReg
}

func isMultiManifestIndex(oci v2alpha1.OCISchema) bool {
	return len(oci.Manifests) > 1
}

func (o OperatorCollector) catalogDigest(ctx context.Context, catalog v2alpha1.Operator) (string, error) {
	var src string

	srcImgSpec, err := image.ParseRef(catalog.Catalog)
	if err != nil {
		return "", fmt.Errorf("unable to determine cached reference for catalog %s: %v", catalog.Catalog, err)
	}

	// prepare the src and dest references
	switch {
	case len(catalog.TargetCatalog) > 0:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, catalog.TargetCatalog}, "/")
	case srcImgSpec.Transport == ociProtocol:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, path.Base(srcImgSpec.Reference)}, "/")
	default:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, srcImgSpec.PathComponent}, "/")
	}

	switch {
	case len(catalog.TargetTag) > 0: // applies only to catalogs
		src = src + ":" + catalog.TargetTag
	case srcImgSpec.Tag == "" && srcImgSpec.Digest != "":
		src = src + ":" + srcImgSpec.Digest
	case srcImgSpec.Tag == "" && srcImgSpec.Digest == "" && srcImgSpec.Transport == ociProtocol:
		src = src + ":latest"
	default:
		src = src + ":" + srcImgSpec.Tag
	}

	imgSpec, err := image.ParseRef(src)
	if err != nil {
		o.Log.Error(errMsg, err.Error())
		return "", err
	}

	sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return "", err
	}
	// OCPBUGS-37948 : No TLS verification when getting manifests from the cache registry
	if strings.Contains(src, o.Opts.LocalStorageFQDN) { // when copying from cache, use HTTP
		sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	catalogDigest, err := o.Manifest.GetDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		o.Log.Error(errMsg, err.Error())
		return "", err
	}
	return catalogDigest, nil
}

func (o OperatorCollector) prepareD2MCopyBatch(images map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	var alreadyIncluded map[string]struct{} = make(map[string]struct{})
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string
			// OCPBUGS-31622 skipping empty related images
			if img.Image == "" {
				continue
			}
			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				// OCPBUGS-33081 - skip if parse error (i.e semver and other)
				o.Log.Warn("mirroring skipped : %v", err)
				continue
			}

			// prepare the src and dest references
			switch {
			// applies only to catalogs
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetCatalog) > 0:
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.TargetCatalog}, "/")
				dest = strings.Join([]string{o.Opts.Destination, img.TargetCatalog}, "/")
			case imgSpec.Transport == ociProtocol:
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.Name}, "/")
				dest = strings.Join([]string{o.Opts.Destination, img.Name}, "/")
			default:
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/")
				dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/")
			}

			// add the tag for src and dest
			switch {
			// applies only to catalogs
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0:
				src = src + ":" + img.TargetTag
				dest = dest + ":" + img.TargetTag
			case imgSpec.Tag == "":
				src = src + ":" + imgSpec.Digest
				dest = dest + ":" + imgSpec.Digest
			default:
				src = src + ":" + imgSpec.Tag
				dest = dest + ":" + imgSpec.Tag
			}
			if src == "" || dest == "" {
				return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Image)
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			if img.Type == v2alpha1.TypeOperatorCatalog && o.Opts.Function == "delete" {
				o.Log.Debug("delete mode, catalog index %s : SKIPPED", img.Image)
			} else {
				if _, found := alreadyIncluded[img.Image]; !found {
					result = append(result, v2alpha1.CopyImageSchema{Origin: imgSpec.ReferenceWithTransport, Source: src, Destination: dest, Type: img.Type})
					alreadyIncluded[img.Image] = struct{}{}
				}
			}
		}
	}
	return result, nil
}

func (o OperatorCollector) prepareM2DCopyBatch(images map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	var alreadyIncluded map[string]struct{} = make(map[string]struct{})
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string
			if img.Image == "" { // OCPBUGS-31622 skipping empty related images
				continue
			}
			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				// OCPBUGS-33081 - skip if parse error (i.e semver and other)
				o.Log.Warn("%v : SKIPPING", err)
				continue
			}

			src = imgSpec.ReferenceWithTransport
			if img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetCatalog) > 0 {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.TargetCatalog}, "/")
			} else if img.Type == v2alpha1.TypeOperatorCatalog && imgSpec.Transport == ociProtocol {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.Name}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/")
			}
			if img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0 {
				dest = dest + ":" + img.TargetTag
			} else if imgSpec.Tag == "" && imgSpec.Transport == ociProtocol {
				dest = dest + ":latest"
			} else if imgSpec.IsImageByDigestOnly() {
				dest = dest + ":" + imgSpec.Digest
			} else if imgSpec.IsImageByTagAndDigest() { // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
				// use tag only for dest, but pull by digest
				o.Log.Warn(collectorPrefix+"%s has both tag and digest : using digest to pull, but tag only for mirroring", imgSpec.Reference)

				src = imgSpec.Transport + strings.Join([]string{imgSpec.Domain, imgSpec.PathComponent}, "/") + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest
				dest = dest + ":" + imgSpec.Tag
			} else {
				dest = dest + ":" + imgSpec.Tag
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)

			if _, found := alreadyIncluded[img.Image]; !found {
				result = append(result, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: imgSpec.ReferenceWithTransport, Type: img.Type})
				// OCPBUGS-37948 + CLID-196
				// Keep a copy of the catalog image in local cache for delete workflow
				if img.Type == v2alpha1.TypeOperatorCatalog && o.Opts.Mode == mirror.MirrorToMirror {
					cacheDest := strings.Replace(dest, o.destinationRegistry(), o.LocalStorageFQDN, 1)
					result = append(result, v2alpha1.CopyImageSchema{Source: src, Destination: cacheDest, Origin: imgSpec.ReferenceWithTransport, Type: img.Type})

				}
				alreadyIncluded[img.Image] = struct{}{}
			}

		}
	}
	return result, nil
}
