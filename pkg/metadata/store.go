package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/remotes"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	oc "github.com/openshift/oc-mirror/pkg/cli/mirror/operatorcatalog"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
	"github.com/openshift/oc-mirror/pkg/operator"
)

// SyncMetadata copies Metadata from one Backend to another
func SyncMetadata(ctx context.Context, first storage.Backend, second storage.Backend) error {
	var meta v1alpha2.Metadata
	if err := first.ReadMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error reading metadata: %v", err)
	}
	// Add mirror as a new PastMirror
	if err := second.WriteMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}
	return nil
}

// UpdateMetadata runs some reconciliation functions on Metadata to ensure its state is consistent
// then uses the Backend to update the metadata storage medium.
func UpdateMetadata(
	ctx context.Context,
	backend storage.Backend,
	meta *v1alpha2.Metadata,
	workspace string,
	skipTLSVerify,
	plainHTTP bool,
	allCatalogs map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata,
) error {
	pastMeta := v1alpha2.NewMetadata()
	pastReleases := map[string]string{}
	merr := backend.ReadMetadata(ctx, &pastMeta, config.MetadataBasePath)
	if merr != nil && !errors.Is(merr, storage.ErrMetadataNotExist) {
		return merr
	} else if merr == nil {
		for _, ch := range pastMeta.PastMirror.Platforms {
			pastReleases[ch.ReleaseChannel] = ch.MinVersion
		}
	}

	mirror := meta.PastMirror
	// Store minimum versions for new catalogs
	klog.V(1).Info("Resolving operator metadata")
	var operatorErrs []error

	resolver, err := containerdregistry.NewResolver("", skipTLSVerify, plainHTTP, nil)
	if err != nil {
		return fmt.Errorf("error creating image resolver: %v", err)
	}
	cacheDir, err := os.MkdirTemp("", "imageset-catalog-registry-")
	if err != nil {
		return err
	}

	logger := logrus.New()

	logger.SetOutput(io.Discard)
	nullLogger := logrus.NewEntry(logger)

	reg, err := containerdregistry.NewRegistry(
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.SkipTLSVerify(skipTLSVerify),
		containerdregistry.WithPlainHTTP(plainHTTP),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
	if err != nil {
		return err
	}
	defer reg.Destroy()

	for _, operator := range mirror.Mirror.Operators {
		operatorMetas, err := resolveOperatorMetadata(ctx, operator, reg, resolver, workspace, allCatalogs[operator.Catalog])
		if err != nil {
			operatorErrs = append(operatorErrs, err)
			continue
		}

		meta.PastMirror.Operators = append(meta.PastMirror.Operators, operatorMetas...)
	}
	if len(operatorErrs) != 0 {
		return utilerrors.NewAggregate(operatorErrs)
	}

	// Store minimum versions for new release channels
	klog.V(1).Info("Resolving OCP release metadata")
	for _, channel := range mirror.Mirror.Platform.Channels {

		// Only collect the information
		// for heads only work flow for conversions
		// from ranges to heads only.
		if !channel.IsHeadsOnly() {
			continue
		}
		min, ok := pastReleases[channel.Name]
		if !ok {
			klog.V(2).Infof("channel %q not found, setting new min to %q", channel.Name, channel.MinVersion)
			min = channel.MinVersion
		}

		releaseMeta := v1alpha2.PlatformMetadata{}
		releaseMeta.ReleaseChannel = channel.Name
		releaseMeta.MinVersion = min
		meta.PastMirror.Platforms = append(meta.PastMirror.Platforms, releaseMeta)
	}

	// Add mirror as a new PastMirror
	if err := backend.WriteMetadata(ctx, meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	return nil
}

func resolveOperatorMetadata(
	ctx context.Context,
	ctlg v1alpha2.Operator,
	reg *containerdregistry.Registry,
	resolver remotes.Resolver,
	workspace string,
	catalogMetadataByPlatform map[oc.OperatorCatalogPlatform]oc.CatalogMetadata,
) (operatorMetas []v1alpha2.OperatorMetadata, err error) {

	// initialize return value
	operatorMetas = []v1alpha2.OperatorMetadata{}

	for platform := range catalogMetadataByPlatform {
		var operatorMeta v1alpha2.OperatorMetadata

		ctlgName, err := ctlg.GetUniqueName()
		if err != nil {
			return []v1alpha2.OperatorMetadata{}, err
		}
		operatorMeta.Catalog = ctlgName
		operatorMeta.SpecificTo = platform.String()

		// Stick to Catalog here because we
		// are referencing the source
		if !image.IsImagePinned(ctlg.Catalog) {
			if ctlg.IsFBCOCI() {
				ref, err := image.ParseReference(ctlg.Catalog)
				if err != nil {
					return []v1alpha2.OperatorMetadata{}, err
				}
				operatorMeta.ImagePin = ref.String()
			} else {
				ctlgPin, err := image.ResolveToPin(ctx, resolver, ctlg.Catalog)
				if err != nil {
					return []v1alpha2.OperatorMetadata{}, fmt.Errorf("error resolving catalog image %q: %v", ctlg.Catalog, err)
				}
				operatorMeta.ImagePin = ctlgPin
			}

		}

		var ic v1alpha2.IncludeConfig
		// Only collect the information
		// for heads only work flows for conversions from ranges
		// or full catalogs to heads only.
		if ctlg.IsHeadsOnly() {

			if ctlg.IsFBCOCI() {
				ctlgName = v1alpha2.OCITransportPrefix + "//" + ctlgName
			}
			// Determine the location of the created FBC
			tir, err := image.ParseReference(ctlgName)
			if err != nil {
				return []v1alpha2.OperatorMetadata{}, err
			}
			ctlgRef := tir.Ref
			ctlgLoc, err := operator.GenerateCatalogDir(ctlgRef)
			if err != nil {
				return []v1alpha2.OperatorMetadata{}, err
			}
			platformAsString := platform.String()
			var icLoc string
			if platformAsString == "" {
				icLoc = filepath.Join(workspace, config.CatalogsDir, ctlgLoc, config.IncludeConfigFile)
			} else {
				icLoc = filepath.Join(workspace, config.CatalogsDir, ctlgLoc, config.MultiDir, platformAsString, config.IncludeConfigFile)
			}
			includeFile, err := os.Open(icLoc)
			if err != nil {
				return []v1alpha2.OperatorMetadata{}, fmt.Errorf("error opening include config file: %v", err)
			}
			defer includeFile.Close()

			if err := ic.Decode(includeFile); err != nil {
				return []v1alpha2.OperatorMetadata{}, fmt.Errorf("error decoding include config file: %v", err)
			}

		}

		operatorMeta.IncludeConfig = ic

		operatorMetas = append(operatorMetas, operatorMeta)
	}

	return operatorMetas, nil
}
