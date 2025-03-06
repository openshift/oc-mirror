package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/blobinfocache/none"
	"github.com/containers/image/v5/types"
	containersarchive "github.com/containers/storage/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"
)

func NewMetadataPopulator(log log.PluggableLoggerInterface, opts mirror.CopyOptions) MetadataPopulator {
	return &metadataPopulator{
		log:  log,
		opts: opts,
	}
}

type metadataPopulator struct {
	log  log.PluggableLoggerInterface
	opts mirror.CopyOptions
}

func (m *metadataPopulator) PopulateMetadata(ctx context.Context, result v2alpha1.CatalogFilterResult) error {
	return m.updateOperatorMetadata(ctx, result)
}

func (m *metadataPopulator) updateOperatorMetadata(ctx context.Context, ctlgFilterResult v2alpha1.CatalogFilterResult) error {
	filteredDC, err := declcfg.LoadFS(ctx, os.DirFS(ctlgFilterResult.FilteredConfigPath))
	if err != nil {
		return err
	}

	// Let's use filteredDC and metadataPropertyType to
	// hydrate channel heads that lack metadata?
	if err := m.hydrateChannelHeadMetadata(ctx, filteredDC, ctlgFilterResult.MetadataPropertyType); err != nil {
		return err
	}

	if err := os.RemoveAll(ctlgFilterResult.FilteredConfigPath); err != nil {
		return err
	}
	if err := declcfg.WriteFS(*filteredDC, ctlgFilterResult.FilteredConfigPath, declcfg.WriteJSON, ".json"); err != nil {
		return err
	}
	return nil
}

func (m *metadataPopulator) hydrateChannelHeadMetadata(ctx context.Context, dc *declcfg.DeclarativeConfig, metadataPropertyType string) error {
	channelHeadsNeedingMetadata := sets.New[string]()
	if err := func() error {
		m, err := declcfg.ConvertToModel(*dc)
		if err != nil {
			return err
		}
		for _, mpkg := range m {
			for _, mch := range mpkg.Channels {
				b, err := mch.Head()
				if err != nil {
					return err
				}
				hasMetadata := false
				for _, p := range b.Properties {
					if p.Type == property.TypeBundleObject || p.Type == property.TypeCSVMetadata {
						hasMetadata = true
						break
					}
				}
				if hasMetadata {
					continue
				}
				channelHeadsNeedingMetadata.Insert(b.Name)
			}
		}
		return nil
	}(); err != nil {
		return fmt.Errorf("failed to determine operator catalog channel heads: %w", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(int(m.opts.ParallelImages))

	for i, b := range dc.Bundles {
		if !channelHeadsNeedingMetadata.Has(b.Name) {
			continue
		}

		eg.Go(func() error {
			csv, err := m.getBundleCSV(egCtx, b)
			if err != nil {
				return fmt.Errorf("failed to extract CSV from bundle %q: %w", b.Name, err)
			}

			switch metadataPropertyType {
			case property.TypeBundleObject:
				csvJson, err := json.Marshal(csv)
				if err != nil {
					return err
				}
				dc.Bundles[i].Properties = append(dc.Bundles[i].Properties, property.MustBuildBundleObject(csvJson))
			case property.TypeCSVMetadata:
				dc.Bundles[i].Properties = append(dc.Bundles[i].Properties, property.MustBuildCSVMetadata(*csv))
			default:
				panic(fmt.Sprintf("unknown property type: %s", metadataPropertyType))
			}
			return nil
		})
	}
	return eg.Wait()
}

func (m *metadataPopulator) getSourceImageRef(bundleImageRef string) (reference.Named, error) {
	r, err := reference.ParseNamed(bundleImageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bundle image ref: %w", err)
	}
	// If mode is m2m, we'll use the image reference defined in the catalog
	if m.opts.IsMirrorToMirror() {
		return r, nil
	}

	// Otherwise, we assume that the image has already been pulled into a local cache
	cachedRef := fmt.Sprintf("%s/%s", m.opts.LocalStorageFQDN, reference.Path(r))
	switch t := r.(type) {
	case reference.NamedTagged:
		cachedRef = fmt.Sprintf("%s:%s", cachedRef, t.Tag())
	case reference.Digested:
		cachedRef = fmt.Sprintf("%s:%s-%s", cachedRef, t.Digest().Algorithm(), t.Digest().Encoded())
	default:
		return nil, fmt.Errorf("reference %q must include a tag or digest", bundleImageRef)
	}
	return reference.ParseNamed(cachedRef)
}

func (m *metadataPopulator) getBundleCSV(ctx context.Context, b declcfg.Bundle) (*v1alpha1.ClusterServiceVersion, error) {
	bundleImgRef, err := m.getSourceImageRef(b.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to determine source image ref: %w", err)
	}

	sourceCtx, err := m.opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}
	if !m.opts.IsMirrorToMirror() {
		sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	dockerImgRef, err := docker.NewReference(bundleImgRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker layout reference: %w", err)
	}
	dockerImg, err := dockerImgRef.NewImage(ctx, sourceCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create oci layout image: %w", err)
	}
	dockerImgSrc, err := dockerImgRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create oci layout image source: %w", err)
	}

	tmpDir, err := os.MkdirTemp(m.opts.Global.WorkingDir, ".operator-bundle-unpack-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	for _, layerInfo := range dockerImg.LayerInfos() {
		layerReader, _, err := dockerImgSrc.GetBlob(ctx, layerInfo, none.NoCache)
		if err != nil {
			return nil, fmt.Errorf("failed to get blob: %w", err)
		}
		defer layerReader.Close()

		if _, err := containersarchive.ApplyLayer(tmpDir, layerReader); err != nil {
			return nil, fmt.Errorf("failed to apply layer: %w", err)
		}
	}

	manifestsDir := filepath.Join(tmpDir, "manifests")
	manifestEntries, err := os.ReadDir(manifestsDir)
	if err != nil {
		return nil, err
	}
	for _, manifestEntry := range manifestEntries {
		if manifestEntry.IsDir() {
			continue
		}

		fileData, err := os.ReadFile(filepath.Join(manifestsDir, manifestEntry.Name()))
		if err != nil {
			return nil, err
		}

		var po v1.PartialObjectMetadata
		if err := yaml.Unmarshal(fileData, &po); err != nil {
			return nil, err
		}

		if po.Kind != "ClusterServiceVersion" {
			continue
		}

		var csv v1alpha1.ClusterServiceVersion
		if err := yaml.Unmarshal(fileData, &csv); err != nil {
			return nil, err
		}
		return &csv, nil
	}
	return nil, fmt.Errorf("no ClusterServiceVersion found in bundle %q (%s)", b.Name, b.Image)
}
