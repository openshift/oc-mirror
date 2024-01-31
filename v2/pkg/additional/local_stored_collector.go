package additional

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type LocalStorageCollector struct {
	Log              clog.PluggableLoggerInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v1alpha2.ImageSetConfiguration
	Opts             mirror.CopyOptions
	LocalStorageFQDN string
}

// AdditionalImagesCollector - this looks into the additional images field
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded in oci format
func (o LocalStorageCollector) AdditionalImagesCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var allImages []v1alpha3.CopyImageSchema

	if o.Opts.IsMirrorToDisk() || o.Opts.IsPrepare() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			imgSpec, err := image.ParseRef(img.Name)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}
			var src string
			var dest string
			src = imgSpec.ReferenceWithTransport

			if imgSpec.IsImageByDigest() {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Digest[:hashTruncLen]}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest, Origin: src, Type: v1alpha2.TypeGeneric})

		}
	}

	if o.Opts.IsDiskToMirror() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			var src string
			var dest string

			if !strings.HasPrefix(img.Name, ociProtocol) {
				imgSpec, err := image.ParseRef(img.Name)
				if err != nil {
					o.Log.Error("%s", err.Error())
					return nil, err
				}

				if imgSpec.IsImageByDigest() {
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Digest[:hashTruncLen]}, "/")
					dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + ":" + imgSpec.Digest[:hashTruncLen]}, "/")
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
				return allImages, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Origin: img.Name, Source: src, Destination: dest, Type: v1alpha2.TypeGeneric})
		}
	}
	return allImages, nil
}
