package additional

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
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
			o.destReg = strings.TrimPrefix(o.Opts.Destination, consts.DockerProtocol)
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
		var src, dest, tmpSrc, tmpDest, origin string

		imgSpec, err := image.ParseRef(img.Name)
		if err != nil {
			// OCPBUGS-33081 - skip if parse error (i.e semver and other)
			o.Log.Warn("%v : SKIPPING", err)
			continue
		}
		if o.Opts.IsMirrorToDisk() || o.Opts.IsMirrorToMirror() {

			tmpSrc = imgSpec.ReferenceWithTransport
			origin = img.Name
			if imgSpec.Transport == consts.DockerProtocol {
				if imgSpec.IsImageByDigestOnly() {
					tmpDest = strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/") + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest
				} else if imgSpec.IsImageByTagAndDigest() { // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
					// use tag only for both src and dest
					o.Log.Warn(collectorPrefix+"%s has both tag and digest : using digest to pull, but tag only for mirroring", imgSpec.Reference)
					tmpSrc = strings.Join([]string{imgSpec.Domain, imgSpec.PathComponent}, "/") + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest
					tmpDest = strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
				} else {
					tmpDest = strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
				}
			} else { // oci image
				// Although fetching the digest of the oci image (using o.Manifest.GetDigest) might work in mirrorToDisk and mirrorToMirror
				// it will not work during diskToMirror as the oci image might not be on the disk any longer
				tmpDest = strings.Join([]string{o.destinationRegistry(), strings.TrimPrefix(imgSpec.PathComponent, "/")}, "/") + ":latest"
			}

		} else if o.Opts.IsDiskToMirror() {
			origin = img.Name
			imgSpec, err := image.ParseRef(img.Name)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return nil, err
			}

			if imgSpec.Transport == consts.DockerProtocol {

				if imgSpec.IsImageByDigestOnly() {
					tmpSrc = strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest}, "/")
					if o.generateV1DestTags {
						tmpDest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + ":latest"}, "/")

					} else {
						tmpDest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + ":" + imgSpec.Algorithm + "-" + imgSpec.Digest}, "/")
					}
				} else if imgSpec.IsImageByTagAndDigest() { // OCPBUGS-33196 + OCPBUGS-37867- check source image for tag and digest
					// use tag only for both src and dest
					o.Log.Warn(collectorPrefix+"%s has both tag and digest : using tag only", imgSpec.Reference)
					tmpSrc = strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
					tmpDest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
				} else {
					tmpSrc = strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
					tmpDest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
				}

			} else {
				tmpSrc = strings.Join([]string{o.LocalStorageFQDN, strings.TrimPrefix(imgSpec.PathComponent, "/")}, "/") + ":latest"
				tmpDest = strings.Join([]string{o.Opts.Destination, strings.TrimPrefix(imgSpec.PathComponent, "/")}, "/") + ":latest"
			}

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
