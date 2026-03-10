package operator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	filter "github.com/sherine-k/catalog-filter/pkg/filter/mirror-config/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

type catalogHandler struct {
	Log clog.PluggableLoggerInterface
}

func (o catalogHandler) getDeclarativeConfig(filePath string) (*declcfg.DeclarativeConfig, error) {
	return declcfg.LoadFS(context.Background(), os.DirFS(filePath))
}

func saveDeclarativeConfig(fbc declcfg.DeclarativeConfig, path string) error {
	return declcfg.WriteFS(fbc, path, declcfg.WriteJSON, ".json")
}

func filterFromImageSetConfig(iscCatalogFilter v2alpha1.Operator) (filter.FilterConfiguration, error) {
	catFilter := filter.FilterConfiguration{
		TypeMeta: v1.TypeMeta{
			Kind:       "FilterConfiguration",
			APIVersion: "olm.operatorframework.io/filter/mirror/v1alpha1",
		},
		Packages: []filter.Package{},
	}

	if len(iscCatalogFilter.Packages) == 0 {
		return catFilter, catFilter.Validate()
	}

	for _, op := range iscCatalogFilter.Packages {
		p := filter.Package{
			Name:           op.Name,
			DefaultChannel: op.DefaultChannel,
		}
		if op.MinVersion != "" {
			p.VersionRange = ">=" + op.MinVersion
		}
		if op.MaxVersion != "" {
			p.VersionRange += " <=" + op.MaxVersion
		}
		if len(op.Channels) > 0 {
			p.Channels = []filter.Channel{}
			for _, ch := range op.Channels {
				filterChan := filter.Channel{
					Name: ch.Name,
				}

				if ch.MinVersion != "" {
					filterChan.VersionRange = ">=" + ch.MinVersion
				}
				if ch.MaxVersion != "" {
					filterChan.VersionRange += " <=" + ch.MaxVersion
				}
				p.Channels = append(p.Channels, filterChan)
			}
		}
		catFilter.Packages = append(catFilter.Packages, p)
	}

	return catFilter, catFilter.Validate()
}

func filterCatalog(ctx context.Context, operatorCatalog declcfg.DeclarativeConfig, iscCatalogFilter v2alpha1.Operator) (*declcfg.DeclarativeConfig, error) {
	config, err := filterFromImageSetConfig(iscCatalogFilter)
	if err != nil {
		return nil, err
	}
	ctlgFilter := filter.NewMirrorFilter(config, []filter.FilterOption{filter.InFull(iscCatalogFilter.Full)}...)
	return ctlgFilter.FilterCatalog(ctx, &operatorCatalog)
}

func (o catalogHandler) getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error) {
	var errs []error
	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	for _, bundle := range dc.Bundles {
		ris, err := handleRelatedImages(bundle, bundle.Package, copyImageSchemaMap)
		if err != nil {
			o.Log.Warn("%s SKIPPING bundle %s of operator %s", err.Error(), bundle.Name, bundle.Package)
			errs = append(errs, err)
			continue
		}
		relatedImages[bundle.Name] = ris
	}

	if len(relatedImages) == 0 {
		errs = append(errs, errors.New("no related images found"))
	}

	return relatedImages, errors.Join(errs...)
}

func handleRelatedImages(bundle declcfg.Bundle, operatorName string, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) ([]v2alpha1.RelatedImage, error) {
	var relatedImages []v2alpha1.RelatedImage
	for _, ri := range bundle.RelatedImages {
		if strings.Contains(ri.Image, consts.OciProtocol) {
			return relatedImages, fmt.Errorf("invalid image: %s 'oci' is not supported in operator catalogs", ri.Image)
		}
		relatedImage := v2alpha1.RelatedImage{
			Name:  ri.Name,
			Image: ri.Image,
		}
		if ri.Image == bundle.Image {
			relatedImage.Type = v2alpha1.TypeOperatorBundle
		} else {
			relatedImage.Type = v2alpha1.TypeOperatorRelatedImage
		}

		imgSpec, err := image.ParseRef(ri.Image)
		if err != nil {
			return relatedImages, fmt.Errorf("error parsing image %s: %w", ri.Image, err)
		}

		operators := copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport]

		if _, found := operators[operatorName]; !found {
			if operators == nil {
				copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport] = make(map[string]struct{})
			}
			copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport][operatorName] = struct{}{}
		}

		bundles := copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport]
		if _, found := bundles[bundle.Name]; !found {
			if bundles == nil {
				copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport] = make(map[string]string)
			}
			copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport][bundle.Image] = bundle.Name
		}

		relatedImages = append(relatedImages, relatedImage)
	}

	return relatedImages, nil
}
