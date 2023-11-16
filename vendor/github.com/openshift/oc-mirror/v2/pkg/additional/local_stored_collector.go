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

const (
	hashTruncLen int = 12
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

	if o.Opts.IsMirrorToDisk() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			imgRef := img.Name
			var src string
			var dest string
			if !strings.Contains(imgRef, "://") {
				src = dockerProtocol + imgRef
			} else {
				src = imgRef
				imgRef = image.RefWithoutTransport(imgRef)
			}
			pathWithoutDNS, err := image.PathWithoutDNS(imgRef)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}

			if image.IsImageByDigest(imgRef) {
				pathWithoutDNSNoDigest := image.PathWithoutDigest(pathWithoutDNS)
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNSNoDigest + ":" + image.Hash(imgRef)[:hashTruncLen]}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS}, "/")
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})

		}
	}

	if o.Opts.IsDiskToMirror() {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			var src string
			var dest string

			if !strings.HasPrefix(img.Name, ociProtocol) {

				imgRef := image.RefWithoutTransport(img.Name)

				pathWithoutDNS, err := image.PathWithoutDNS(imgRef)
				if err != nil {
					o.Log.Error("%s", err.Error())
					return nil, err
				}

				if image.IsImageByDigest(imgRef) {
					pathWithoutDNSNoDigest := image.PathWithoutDigest(pathWithoutDNS)
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNSNoDigest + ":" + image.Hash(imgRef)[:hashTruncLen]}, "/")
					dest = strings.Join([]string{o.Opts.Destination, pathWithoutDNSNoDigest + ":" + image.Hash(imgRef)[:hashTruncLen]}, "/")
				} else {
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS}, "/")
					dest = strings.Join([]string{o.Opts.Destination, pathWithoutDNS}, "/")
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
			allImages = append(allImages, v1alpha3.CopyImageSchema{Origin: img.Name, Source: src, Destination: dest})
		}
	}
	return allImages, nil
}
