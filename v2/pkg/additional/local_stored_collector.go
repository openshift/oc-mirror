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
			// no transport was provided, assume docker://
			if !strings.Contains(src, "://") {
				src = dockerProtocol + imgRef
			} else {
				transportAndRef := strings.Split(imgRef, "://")
				// because we are reusing this to construct dest
				imgRef = transportAndRef[1]
			}

			if isImageByDigest(imgRef) {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imageName(imgRef) + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgRef}, "/")
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})

		}
	}

	if o.Opts.Mode == diskToMirror {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			// TODO Make this more complete
			// This logic will be useful for operators and releases
			// strip the domain name from the img.Name
			var src string
			var dest string

			if !strings.HasPrefix(img.Name, ociProtocol) {

				domainAndPathComps := img.Name
				// pathComponents := img.Name
				// temporarily strip out the transport
				transportAndRef := strings.Split(domainAndPathComps, "://")
				if len(transportAndRef) > 1 {
					domainAndPathComps = transportAndRef[1]
				}
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.Name}, "/")

				if isImageByDigest(img.Name) {
					dest = strings.Join([]string{o.Opts.Destination, imageName(img.Name) + ":" + imageHash(img.Name)[:hashTruncLen]}, "/")
				} else {
					dest = strings.Join([]string{o.Opts.Destination, img.Name}, "/")
				}

				// the following is for having the destination without the initial domain name => later
				// domainAndPathCompsArray := strings.Split(domainAndPathComps, "/")
				// if len(domainAndPathCompsArray) > 2 {
				// 	pathComponents = strings.Join(domainAndPathCompsArray[1:], "/")
				// } else {
				// 	return allImages, fmt.Errorf("unable to parse image %s correctly", img.Name)
				// }
				// src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathComponents}, "/")
				// dst = strings.Join([]string{o.Opts.Destination, pathComponents}, "/") // already has a transport protocol

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

func imageName(imgRef string) string {
	var imageName string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		imageName = imgSplit[0]
	}

	return imageName
}

func imageHash(imgRef string) string {
	var hash string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		hash = strings.Split(imgSplit[1], ":")[1]
	}

	return hash
}
