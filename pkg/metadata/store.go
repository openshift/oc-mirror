package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/remotes"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
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
func UpdateMetadata(ctx context.Context, backend storage.Backend, meta *v1alpha2.Metadata, workspace string, skipTLSVerify, plainHTTP bool) error {
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
	logrus.Debugf("Resolving operator metadata")
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
	logger.SetOutput(ioutil.Discard)
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
		operatorMeta, err := resolveOperatorMetadata(ctx, operator, reg, resolver, workspace)
		if err != nil {
			operatorErrs = append(operatorErrs, err)
			continue
		}

		meta.PastMirror.Operators = append(meta.PastMirror.Operators, operatorMeta)
	}
	if len(operatorErrs) != 0 {
		return utilerrors.NewAggregate(operatorErrs)
	}

	// Store minimum versions for new release channels
	logrus.Debugf("Resolving OCP release metadata")
	for _, channel := range mirror.Mirror.Platform.Channels {

		// Only collect the information
		// for heads only work flow for conversions
		// from ranges to heads only.
		if !channel.IsHeadsOnly() {
			continue
		}
		min, ok := pastReleases[channel.Name]
		if !ok {
			logrus.Debugf("channel %q not found, setting new min to %q", channel.Name, channel.MinVersion)
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

func resolveOperatorMetadata(ctx context.Context, ctlg v1alpha2.Operator, reg *containerdregistry.Registry, resolver remotes.Resolver, workspace string) (operatorMeta v1alpha2.OperatorMetadata, err error) {
	ctlgName, err := ctlg.GetUniqueName()
	if err != nil {
		return v1alpha2.OperatorMetadata{}, err
	}
	operatorMeta.Catalog = ctlgName

	// Stick to Catalog here because we
	// are referencing the source
	ctlgPin := ctlg.Catalog
	if !image.IsImagePinned(ctlg.Catalog) {
		ctlgPin, err = image.ResolveToPin(ctx, resolver, ctlg.Catalog)
		if err != nil {
			return v1alpha2.OperatorMetadata{}, fmt.Errorf("error resolving catalog image %q: %v", ctlg.Catalog, err)
		}
	}
	operatorMeta.ImagePin = ctlgPin

	var ic v1alpha2.IncludeConfig
	// Only collect the information
	// for heads only work flows for conversions from ranges
	// or full catalogs to heads only.
	// TODO(jpower432): The include config is already generated
	// during catalog processing. Would be better to write it to disk
	// with the FBC and unmarshal it into the struct instead of generating
	// it again on diff
	var converter operator.IncludeConfigConverter
	hasInclude := len(ctlg.Packages) != 0
	if ctlg.IsHeadsOnly() {

		if hasInclude {
			converter = operator.NewIncludeStrategy(ctlg.IncludeConfig)
		} else {
			converter = operator.NewCatalogStrategy()
		}

		// Determine the location of the created FBC
		ctlgRef, err := imgreference.Parse(ctlgName)
		if err != nil {
			return v1alpha2.OperatorMetadata{}, err
		}
		dcLoc, err := operator.GenerateCatalogDir(ctlgRef)
		if err != nil {
			return v1alpha2.OperatorMetadata{}, err
		}
		dcLoc = filepath.Join(workspace, config.CatalogsDir, dcLoc, config.IndexDir)
		dc, err := action.Render{
			Registry:       reg,
			Refs:           []string{dcLoc},
			AllowedRefMask: action.RefAll,
		}.Run(ctx)
		if err != nil {
			return v1alpha2.OperatorMetadata{}, err
		}
		ic, err = converter.ConvertDCToIncludeConfig(*dc)
		if err != nil {
			return v1alpha2.OperatorMetadata{}, err
		}
	}

	operatorMeta.IncludeConfig = ic

	return operatorMeta, nil
}
