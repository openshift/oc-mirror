package additional

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
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
func (o *LocalStorageCollector) AdditionalImagesCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var allImages []v1alpha3.CopyImageSchema

	if o.Opts.Mode == mirrorToDisk {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			imgRef := img.Name
			var src string
			var dest string
			if !strings.Contains(imgRef, "://") {
				src = dockerProtocol + imgRef
			} else {
				src = imgRef
				transportAndRef := strings.Split(imgRef, "://")
				imgRef = transportAndRef[1]
			}

			pathWithoutDNS, err := pathWithoutDNS(imgRef)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}

			if isImageByDigest(imgRef) {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS}, "/")
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})

		}
	}

	if o.Opts.Mode == diskToMirror {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			var src string
			var dest string

			if !strings.HasPrefix(img.Name, ociProtocol) {

				imgRef := img.Name
				transportAndRef := strings.Split(imgRef, "://")
				if len(transportAndRef) > 1 {
					imgRef = transportAndRef[1]
				}

				pathWithoutDNS, err := pathWithoutDNS(imgRef)
				if err != nil {
					o.Log.Error("%s", err.Error())
					return nil, err
				}

				if isImageByDigest(imgRef) {
					src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
					dest = strings.Join([]string{o.Opts.Destination, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
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

func isImageByDigest(imgRef string) bool {
	return strings.Contains(imgRef, "@")
}

func pathWithoutDNS(imgRef string) (string, error) {

	var imageName []string
	if isImageByDigest(imgRef) {
		imageNameSplit := strings.Split(imgRef, "@")
		imageName = strings.Split(imageNameSplit[0], "/")
	} else {
		imageName = strings.Split(imgRef, "/")
	}

	if len(imageName) > 2 {
		return strings.Join(imageName[1:], "/"), nil
	} else if len(imageName) == 1 {
		return imageName[0], nil
	} else {
		return "", fmt.Errorf("unable to parse image %s correctly", imgRef)
	}
}

func imageHash(imgRef string) string {
	var hash string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		hash = strings.Split(imgSplit[1], ":")[1]
	}

	return hash
}
