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
	Log              clog.PluggableLoggerInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v2alpha1.ImageSetConfiguration
	Opts             mirror.CopyOptions
	LocalStorageFQDN string
	destReg          string
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

	if o.Opts.IsMirrorToDisk() || o.Opts.IsMirrorToMirror() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			imgSpec, err := image.ParseRef(img.Name)
			if err != nil {
				// OCPBUGS-33081 - skip if parse error (i.e semver and other)
				o.Log.Warn("%v : SKIPPING", err)
				continue
			}

			var src string
			var dest string
			src = imgSpec.ReferenceWithTransport

			if imgSpec.IsImageByDigestOnly() {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent + ":" + imgSpec.Digest}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			}

			o.Log.Debug(collectorPrefix+"source %s", src)
			o.Log.Debug(collectorPrefix+"destination %s", dest)

			// OCPBUGS-33196 - check source image for tag and digest
			// skip mirroring
			if imgSpec.IsImageByTagAndDigest() {
				o.Log.Warn(collectorPrefix+"%s has both tag and digest : SKIPPING", imgSpec.Reference)
			} else {
				allImages = append(allImages, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: src, Type: v2alpha1.TypeGeneric})
			}
		}
	}

	if o.Opts.IsDiskToMirror() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			var src string
			var dest string

			if !strings.HasPrefix(img.Name, ociProtocol) {
				imgSpec, err := image.ParseRef(img.Name)
				if err != nil {
					o.Log.Error(errMsg, err.Error())
					return nil, err
				}

				if imgSpec.IsImageByDigestOnly() {
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Digest}, "/")
					dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + ":" + imgSpec.Digest}, "/")
				} else {
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
					dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
				}

			} else {
				src = img.Name
				transportAndPath := strings.Split(img.Name, "://")
				dest = dockerProtocol + strings.Join([]string{o.Opts.Destination, transportAndPath[1]}, "/")
			}

			if src == "" || dest == "" {
				o.Log.Error(collectorPrefix+"unable to determine src %s or dst %s for %s", src, dest, img.Name)
				return allImages, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
			}

			o.Log.Debug(collectorPrefix+"source %s", src)
			o.Log.Debug(collectorPrefix+"destination %s", dest)
			allImages = append(allImages, v2alpha1.CopyImageSchema{Origin: img.Name, Source: src, Destination: dest, Type: v2alpha1.TypeGeneric})
		}
	}
	return allImages, nil
}
