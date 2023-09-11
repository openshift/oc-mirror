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

func NewWithLocalStorage(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	localStorageFQDN string,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: localStorageFQDN}
}

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

			imgName := img.Name
			src := ""
			if !strings.Contains(src, "://") { // no transport was provided, assume docker://
				src = dockerProtocol + imgName
			} else {
				transportAndRef := strings.Split(imgName, "://")
				imgName = transportAndRef[1] // because we are reusing this to construct dest
			}

			dest := dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgName}, "/")
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
			src := ""
			dst := ""

			if !strings.HasPrefix(img.Name, ociProtocol) {

				domainAndPathComps := img.Name
				// pathComponents := img.Name
				// temporarily strip out the transport
				transportAndRef := strings.Split(domainAndPathComps, "://")
				if len(transportAndRef) > 1 {
					domainAndPathComps = transportAndRef[1]
				}
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.Name}, "/")
				dst = strings.Join([]string{o.Opts.Destination, img.Name}, "/")

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
				dst = dockerProtocol + strings.Join([]string{o.Opts.Destination, transportAndPath[1]}, "/")
			}

			if src == "" || dst == "" {
				return allImages, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dst, img.Name)
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dst)
			allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dst})
		}
	}
	return allImages, nil
}

// customImageParser - simple image string parser
