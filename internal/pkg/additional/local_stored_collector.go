package additional

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type LocalStorageCollector struct {
	Log                clog.PluggableLoggerInterface
	Mirror             mirror.MirrorInterface
	Manifest           manifest.ManifestInterface
	Config             v2alpha1.ImageSetConfiguration
	Opts               mirror.CopyOptions
	LocalStorageFQDN   string
	destReg            string
	generateV1DestTags bool
}

func WithV1Tags(o CollectorInterface) CollectorInterface {
	switch impl := o.(type) {
	case *LocalStorageCollector:
		impl.generateV1DestTags = true
	}
	return o
}

func (o LocalStorageCollector) destinationRegistry() string {
	if o.destReg == "" {
		if o.Opts.Mode == mirror.DiskToMirror || o.Opts.Mode == mirror.MirrorToMirror {
			o.destReg = strings.TrimPrefix(o.Opts.Destination, dockerProtocol)
		} else {
			o.destReg = o.LocalStorageFQDN
		}
	}
	return o.destReg
}

// AdditionalImagesCollector - this looks into the additional images field
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded in oci format
func (o LocalStorageCollector) AdditionalImagesCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {

	var allImages []v2alpha1.CopyImageSchema

	o.Log.Debug(collectorPrefix+"setting copy option o.Opts.MultiArch=%s when collecting releases image", o.Opts.MultiArch)
	for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
		var src, dest, tmpSrc, tmpDest string

		origin := img.Name

		imgSpec, err := image.ParseRef(img.Name)
		if err != nil {
			// OCPBUGS-33081 - skip if parse error (i.e semver and other)
			o.Log.Warn("%v : SKIPPING", err)
			continue
		}

		if img.TargetRepo != "" && !v2alpha1.IsValidPathComponent(img.TargetRepo) {
			o.Log.Warn("invalid targetRepo %s for image %s : SKIPPING", img.TargetRepo, img.Name)
			continue
		}

		targetRepo := imgSpec.PathComponent
		if img.TargetRepo != "" {
			targetRepo = img.TargetRepo
		}

		targetTag := imgSpec.Tag
		if img.TargetTag != "" {
			targetTag = img.TargetTag
		}

		switch {
		case o.Opts.IsMirrorToDisk(), o.Opts.IsMirrorToMirror():
			tmpSrc, tmpDest = o.buildMirrorToDiskPaths(img, imgSpec, targetRepo, targetTag)
		case o.Opts.IsDiskToMirror(), o.Opts.IsDelete():
			tmpSrc, tmpDest = o.buildDiskToMirrorPaths(img, imgSpec, targetRepo, targetTag)
		}
		if tmpSrc == "" || tmpDest == "" {
			o.Log.Error(collectorPrefix+"unable to determine src %s or dst %s for %s", tmpSrc, tmpDest, img.Name)
			return allImages, fmt.Errorf("unable to determine src %s or dst %s for %s", tmpSrc, tmpDest, img.Name)
		}
		srcSpec, err := image.ParseRef(tmpSrc) // makes sure this ref is valid, and adds transport if needed
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return nil, err
		}
		src = srcSpec.ReferenceWithTransport

		destSpec, err := image.ParseRef(tmpDest) // makes sure this ref is valid, and adds transport if needed
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return nil, err
		}
		dest = destSpec.ReferenceWithTransport

		o.Log.Debug(collectorPrefix+"source %s", src)
		o.Log.Debug(collectorPrefix+"destination %s", dest)

		allImages = append(allImages, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: origin, Type: v2alpha1.TypeGeneric})
	}
	return allImages, nil
}

// buildMirrorToDiskPaths constructs source and destination paths for mirror-to-disk and mirror-to-mirror operations
func (o LocalStorageCollector) buildMirrorToDiskPaths(img v2alpha1.Image, imgSpec image.ImageSpec, targetRepo, targetTag string) (string, string) {
	tmpSrc := imgSpec.ReferenceWithTransport
	var tmpDest string

	if imgSpec.Transport != dockerProtocol {
		// oci image
		// Although fetching the digest of the oci image (using o.Manifest.GetDigest) might work in mirrorToDisk and mirrorToMirror
		// it will not work during diskToMirror as the oci image might not be on the disk any longer
		tag := latestTag
		if img.TargetTag != "" {
			tag = targetTag
		}
		tmpDest = fmt.Sprintf("%s/%s:%s", o.destinationRegistry(), strings.TrimPrefix(targetRepo, "/"), tag)
		return tmpSrc, tmpDest
	}

	// Docker protocol
	switch {
	case imgSpec.IsImageByTagAndDigest():
		// OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
		// use tag only for both src and dest
		o.Log.Warn(collectorPrefix+"%s has both tag and digest : using digest to pull, but tag only for mirroring", imgSpec.Reference)
		tmpSrc = fmt.Sprintf("%s/%s@%s:%s", imgSpec.Domain, imgSpec.PathComponent, imgSpec.Algorithm, imgSpec.Digest)
		tmpDest = fmt.Sprintf("%s/%s:%s", o.destinationRegistry(), targetRepo, targetTag)
	case imgSpec.IsImageByDigestOnly() && img.TargetTag == "":
		tmpDest = fmt.Sprintf("%s/%s:%s-%s", o.destinationRegistry(), targetRepo, imgSpec.Algorithm, imgSpec.Digest)
	default:
		tmpDest = fmt.Sprintf("%s/%s:%s", o.destinationRegistry(), targetRepo, targetTag)
	}

	return tmpSrc, tmpDest
}

// buildDiskToMirrorPaths constructs source and destination paths for disk-to-mirror operations
func (o LocalStorageCollector) buildDiskToMirrorPaths(img v2alpha1.Image, imgSpec image.ImageSpec, targetRepo, targetTag string) (string, string) {
	// Docker protocol
	var tmpSrc, tmpDest string

	if imgSpec.Transport != dockerProtocol {
		// oci image
		tag := latestTag
		if targetTag != "" {
			tag = targetTag
		}
		tmpSrc = fmt.Sprintf("%s/%s:%s", o.LocalStorageFQDN, strings.TrimPrefix(targetRepo, "/"), tag)
		tmpDest = fmt.Sprintf("%s/%s:%s", o.Opts.Destination, strings.TrimPrefix(targetRepo, "/"), tag)
		return tmpSrc, tmpDest
	}

	switch {
	case imgSpec.IsImageByDigestOnly() && img.TargetTag == "" && o.generateV1DestTags:
		tmpSrc = fmt.Sprintf("%s/%s:%s-%s", o.LocalStorageFQDN, targetRepo, imgSpec.Algorithm, imgSpec.Digest)
		tmpDest = fmt.Sprintf("%s/%s:%s", o.Opts.Destination, targetRepo, latestTag)
	case imgSpec.IsImageByDigestOnly() && img.TargetTag == "":
		digestTag := imgSpec.Algorithm + "-" + imgSpec.Digest
		tmpSrc = fmt.Sprintf("%s/%s:%s", o.LocalStorageFQDN, targetRepo, digestTag)
		tmpDest = fmt.Sprintf("%s/%s:%s", o.Opts.Destination, targetRepo, digestTag)
	default:
		// OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
		if imgSpec.IsImageByTagAndDigest() {
			o.Log.Warn(collectorPrefix+"%s has both tag and digest : using tag only", imgSpec.Reference)
		}
		tmpSrc = fmt.Sprintf("%s/%s:%s", o.LocalStorageFQDN, targetRepo, targetTag)
		tmpDest = fmt.Sprintf("%s/%s:%s", o.Opts.Destination, targetRepo, targetTag)
	}

	return tmpSrc, tmpDest
}
