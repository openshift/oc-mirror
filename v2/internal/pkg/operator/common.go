package operator

import (
	"context"
	"fmt"
	"hash/fnv"
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
	Log                clog.PluggableLoggerInterface
	LogsDir            string
	Mirror             mirror.MirrorInterface
	Manifest           manifest.ManifestInterface
	Config             v2alpha1.ImageSetConfiguration
	Opts               mirror.CopyOptions
	LocalStorageFQDN   string
	destReg            string
	ctlgHandler        catalogHandlerInterface
	generateV1DestTags bool
}

func WithV1Tags(o CollectorInterface) CollectorInterface {
	switch impl := o.(type) {
	case *FilterCollector:
		impl.generateV1DestTags = true
	case *LocalStorageCollector:
		impl.generateV1DestTags = true
	}
	return o
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

// cachedCatalog returns the reference to the filtered catalog in the local oc-mirror cache
// The filtered cached catalog reference is computed from:
// * `catalog` (`v2alpha1.Operator`): the reference to the catalog in the imageSetConfig along with targetCatalog and targetTag if set
// * the `filteredTag`: which is the expected tag to be used for the filtered catalog.
func (o OperatorCollector) cachedCatalog(catalog v2alpha1.Operator, filteredTag string) (string, error) {
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

	src = src + ":" + filteredTag

	return src, nil
}

// catalogDigest: method used during diskToMirror in order to discover the catalog's digest from a reference by tag.
// It queries the cache registry instead of the registry set in the `catalog` reference
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
		src = src + ":" + srcImgSpec.Algorithm + "-" + srcImgSpec.Digest
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
				if img.RebuiltTag != "" {
					src = src + ":" + img.RebuiltTag
				} else {
					src = src + ":" + img.TargetTag
				}
				dest = dest + ":" + img.TargetTag
			case imgSpec.Tag == "":
				if img.RebuiltTag != "" {
					src = src + ":" + img.RebuiltTag
				} else {
					src = src + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest
				}
				//TODO remove me when the migration from oc-mirror v1 to v2 ends
				if o.generateV1DestTags {
					if img.OriginFromOperatorCatalogOnDisk {
						dest = dest + ":" + imgSpec.Digest[0:6]
					} else {
						hasher := fnv.New32a()
						hasher.Reset()
						_, err = hasher.Write([]byte(imgSpec.Reference))
						if err != nil {
							return result, fmt.Errorf("couldn't generate v1 tag for image (%s), skipping ", imgSpec.ReferenceWithTransport)
						}
						dest = dest + ":" + fmt.Sprintf("%x", hasher.Sum32())
					}
				} else {
					dest = dest + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest
				}
			default:
				if img.RebuiltTag != "" {
					src = src + ":" + img.RebuiltTag
				} else {
					src = src + ":" + imgSpec.Tag
				}
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
					result = append(result, v2alpha1.CopyImageSchema{Origin: imgSpec.ReferenceWithTransport, Source: src, Destination: dest, Type: img.Type, RebuiltTag: img.RebuiltTag})
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
			switch {
			// applies only to catalogs
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetCatalog) > 0:
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.TargetCatalog}, "/")
			case img.Type == v2alpha1.TypeOperatorCatalog && imgSpec.Transport == ociProtocol:
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.Name}, "/")
			default:
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/")
			}

			// add the tag for src and dest
			switch {
			// applies only to catalogs
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0:
				dest = dest + ":" + img.TargetTag
			case imgSpec.Tag == "" && imgSpec.Transport == ociProtocol:
				dest = dest + "::latest"
			case imgSpec.IsImageByDigestOnly():
				dest = dest + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest
			case imgSpec.IsImageByTagAndDigest(): // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
				// use tag only for dest, but pull by digest
				o.Log.Warn(collectorPrefix+"%s has both tag and digest : using digest to pull, but tag only for mirroring", imgSpec.Reference)
				src = imgSpec.Transport + strings.Join([]string{imgSpec.Domain, imgSpec.PathComponent}, "/") + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest
				dest = dest + ":" + imgSpec.Tag
			default:
				dest = dest + ":" + imgSpec.Tag
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)

			if _, found := alreadyIncluded[img.Image]; !found {
				result = append(result, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: imgSpec.ReferenceWithTransport, Type: img.Type, RebuiltTag: img.RebuiltTag})
				// OCPBUGS-37948 + CLID-196
				// Keep a copy of the catalog image in local cache for delete workflow
				if img.Type == v2alpha1.TypeOperatorCatalog && o.Opts.Mode == mirror.MirrorToMirror {
					cacheDest := strings.Replace(dest, o.destinationRegistry(), o.LocalStorageFQDN, 1)
					result = append(result, v2alpha1.CopyImageSchema{Source: src, Destination: cacheDest, Origin: imgSpec.ReferenceWithTransport, Type: img.Type, RebuiltTag: img.RebuiltTag})

				}
				alreadyIncluded[img.Image] = struct{}{}
			}

		}
	}
	return result, nil
}

func (o OperatorCollector) dispatchImagesForM2M(images map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	var alreadyIncluded map[string]struct{} = make(map[string]struct{})
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			if img.Image == "" { // OCPBUGS-31622 skipping empty related images
				continue
			}
			var copies []v2alpha1.CopyImageSchema
			var err error
			switch img.Type {
			case v2alpha1.TypeOperatorCatalog:
				dispatcher := CatalogImageDispatcher{
					log:                 o.Log,
					cacheRegistry:       dockerProtocol + o.LocalStorageFQDN,
					destinationRegistry: o.Opts.Destination,
				}
				copies, err = dispatcher.dispatch(img)
				if err != nil {
					// OCPBUGS-33081 - skip if parse error (i.e semver and other)
					o.Log.Warn("%v : SKIPPING", err)
					continue
				}
			default:
				dispatcher := OtherImageDispatcher{
					log:                 o.Log,
					cacheRegistry:       dockerProtocol + o.LocalStorageFQDN,
					destinationRegistry: o.Opts.Destination,
				}
				copies, err = dispatcher.dispatch(img)
				if err != nil {
					// OCPBUGS-33081 - skip if parse error (i.e semver and other)
					o.Log.Warn("%v : SKIPPING", err)
					continue
				}
			}

			if _, found := alreadyIncluded[img.Image]; !found {
				result = append(result, copies...)
				alreadyIncluded[img.Image] = struct{}{}
			}
		}
	}
	return result, nil

}

type OtherImageDispatcher struct {
	imageDispatcher
	log                 clog.PluggableLoggerInterface
	destinationRegistry string
	cacheRegistry       string
}

func (d OtherImageDispatcher) dispatch(img v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var src, dest string
	copies := []v2alpha1.CopyImageSchema{}
	imgSpec, err := image.ParseRef(img.Image)
	if err != nil {
		return copies, err
	}

	src = imgSpec.ReferenceWithTransport
	dest = strings.Join([]string{d.destinationRegistry, imgSpec.PathComponent}, "/")
	switch {
	case imgSpec.Tag == "" && imgSpec.Transport == ociProtocol:
		dest = dest + ":latest"
	case imgSpec.IsImageByDigestOnly():
		dest = dest + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest
	case imgSpec.IsImageByTagAndDigest(): // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
		// use tag only for dest, but pull by digest
		d.log.Warn(collectorPrefix+"%s has both tag and digest : using digest to pull, but tag only for mirroring", imgSpec.Reference)
		src = imgSpec.Transport + strings.Join([]string{imgSpec.Domain, imgSpec.PathComponent}, "/") + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest
		dest = dest + ":" + imgSpec.Tag
	default:
		dest = dest + ":" + imgSpec.Tag
	}
	copies = append(copies, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: imgSpec.ReferenceWithTransport, Type: img.Type})
	return copies, nil
}

type CatalogImageDispatcher struct {
	imageDispatcher
	log                 clog.PluggableLoggerInterface
	destinationRegistry string
	cacheRegistry       string
}

func (d CatalogImageDispatcher) dispatch(img v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	imgSpec, err := image.ParseRef(img.Image)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}
	var toCacheImage, fromRebuiltImage, toDestImage string
	toCacheImage = saveCtlgToCacheRef(imgSpec, img, d.cacheRegistry)
	fromRebuiltImage = rebuiltCtlgRef(imgSpec, img, d.cacheRegistry)
	toDestImage = destCtlgRef(imgSpec, img, d.destinationRegistry)
	cacheCopy := v2alpha1.CopyImageSchema{
		Source:      imgSpec.ReferenceWithTransport,
		Destination: toCacheImage,
		Origin:      imgSpec.ReferenceWithTransport,
		RebuiltTag:  img.RebuiltTag,
		Type:        img.Type,
	}
	destCopy := v2alpha1.CopyImageSchema{
		Source:      fromRebuiltImage,
		Destination: toDestImage,
		Origin:      imgSpec.ReferenceWithTransport,
		RebuiltTag:  img.RebuiltTag,
		Type:        img.Type,
	}

	return []v2alpha1.CopyImageSchema{cacheCopy, destCopy}, nil
}

// used by CatalogImageDispatcher.dispatch()
func saveCtlgToCacheRef(spec image.ImageSpec, img v2alpha1.RelatedImage, cacheRegistry string) string {
	var saveCtlgDest string
	switch {
	case len(img.TargetCatalog) > 0:
		saveCtlgDest = strings.Join([]string{cacheRegistry, img.TargetCatalog}, "/")
	case spec.Transport == ociProtocol:
		saveCtlgDest = strings.Join([]string{cacheRegistry, img.Name}, "/")
	default:
		saveCtlgDest = strings.Join([]string{cacheRegistry, spec.PathComponent}, "/")
	}

	// add the tag for src and dest
	switch {
	case len(img.TargetTag) > 0:
		saveCtlgDest = saveCtlgDest + ":" + img.TargetTag
	case spec.Tag == "" && spec.Transport == ociProtocol:
		saveCtlgDest = saveCtlgDest + ":latest"
	case spec.IsImageByDigestOnly():
		saveCtlgDest = saveCtlgDest + ":" + spec.Algorithm + "-" + spec.Digest
	case spec.IsImageByTagAndDigest():
		saveCtlgDest = saveCtlgDest + ":" + spec.Tag
	default:
		saveCtlgDest = saveCtlgDest + ":" + spec.Tag
	}
	return saveCtlgDest
}

// used by CatalogImageDispatcher.dispatch()
func rebuiltCtlgRef(spec image.ImageSpec, img v2alpha1.RelatedImage, cacheRegistry string) string {
	var rebuiltCtlgSrc string
	switch {
	// applies only to catalogs
	case len(img.TargetCatalog) > 0:
		rebuiltCtlgSrc = strings.Join([]string{cacheRegistry, img.TargetCatalog}, "/")
	case spec.Transport == ociProtocol:
		rebuiltCtlgSrc = strings.Join([]string{cacheRegistry, img.Name}, "/")
	default:
		rebuiltCtlgSrc = strings.Join([]string{cacheRegistry, spec.PathComponent}, "/")
	}

	// add the tag for src and dest
	switch {
	// applies only to catalogs
	case img.RebuiltTag != "":
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":" + img.RebuiltTag
	case len(img.TargetTag) > 0:
	case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0:
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":" + img.TargetTag
	case spec.Tag == "" && spec.Transport == ociProtocol:
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":latest"
	case spec.IsImageByDigestOnly():
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":" + spec.Algorithm + "-" + spec.Digest
	case spec.IsImageByTagAndDigest(): // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
		// use tag only for dest, but pull by digest
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":" + spec.Tag
	default:
		rebuiltCtlgSrc = rebuiltCtlgSrc + ":" + spec.Tag
	}
	return rebuiltCtlgSrc
}

// used by CatalogImageDispatcher.dispatch()
func destCtlgRef(spec image.ImageSpec, img v2alpha1.RelatedImage, destinationRegistry string) string {
	var dest string
	switch {
	// applies only to catalogs
	case len(img.TargetCatalog) > 0:
		dest = strings.Join([]string{destinationRegistry, img.TargetCatalog}, "/")
	case spec.Transport == ociProtocol:
		dest = strings.Join([]string{destinationRegistry, img.Name}, "/")
	default:
		dest = strings.Join([]string{destinationRegistry, spec.PathComponent}, "/")
	}

	// add the tag for src and dest
	switch {
	// applies only to catalogs

	case len(img.TargetTag) > 0:
		dest = dest + ":" + img.TargetTag
	case spec.Tag == "" && spec.Transport == ociProtocol:
		dest = dest + ":latest"
	case spec.IsImageByDigestOnly():
		dest = dest + ":" + spec.Algorithm + "-" + spec.Digest
	case spec.IsImageByTagAndDigest(): // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
		// use tag only for dest, but pull by digest
		dest = dest + ":" + spec.Tag
	default:
		dest = dest + ":" + spec.Tag

	}
	return dest
}
