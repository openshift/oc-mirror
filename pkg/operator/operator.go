package operator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/admin/catalog"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	imgmirror "github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/config/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

type OperatorOptions struct {
	RootDestDir string
	DryRun      bool
	Cleanup     bool
	SkipTLS     bool
}

func NewOperatorOptions() *OperatorOptions {
	return &OperatorOptions{}
}

func (o *OperatorOptions) mktempDir() (string, error) {
	dir := filepath.Join(o.RootDestDir, fmt.Sprintf("operators.%d", time.Now().Unix()))
	return dir, os.MkdirAll(dir, os.ModePerm)
}

// Full mirrors each catalog image in its entirety to the <RootDestDir>/src directory.
func (o *OperatorOptions) Full(_ context.Context, cfg v1alpha1.ImageSetConfiguration) (err error) {

	tmp, err := o.mktempDir()
	if err != nil {
		return err
	}
	if o.Cleanup {
		defer func() {
			if err := os.RemoveAll(tmp); err != nil {
				logrus.Error(err)
			}
		}()
	}

	for _, ctlg := range cfg.Mirror.Operators {
		stream := genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		}
		opts := catalog.NewMirrorCatalogOptions(stream)
		opts.DryRun = o.DryRun
		opts.FileDir = filepath.Join(o.RootDestDir, config.SourcePath)
		opts.ManifestDir = filepath.Join(tmp, fmt.Sprintf("manifests-%s-%d", opts.SourceRef.Ref.Name, time.Now().Unix()))
		opts.SecurityOptions.Insecure = o.SkipTLS

		ref, err := imgreference.Parse(ctlg.Catalog)
		if err != nil {
			return err
		}
		ref = ref.DockerClientDefaults()

		args := []string{
			// The source is the catalog image itself.
			ctlg.Catalog,
			// The destination is within <RootDestDir>/src/v2/<image-name>.
			refToFileScheme(ref),
		}
		if err := opts.Complete(&cobra.Command{}, args); err != nil {
			return err
		}

		// TODO(estroz): the mirrorer needs to be set after Complete() is called
		// because the default ImageMirrorer does not set FileDir from opts.
		// This isn't great because the default ImageMirrorer is a closure
		// and may contain configuration that gets overridden here.
		if opts.ImageMirrorer, err = newMirrorerFunc(opts); err != nil {
			return err
		}

		if err := opts.Validate(); err != nil {
			return err
		}

		if err := opts.Run(); err != nil {
			return err
		}
	}

	return nil
}

// Input refs to the mirror library must be prefixed with "file://" if a local file ref.
func refToFileScheme(ref imgreference.DockerImageReference) string {
	return "file://" + filepath.Join(ref.Namespace, ref.Name)
}

// Copied from https://github.com/openshift/oc/blob/4df50be4d929ce036c4f07893c07a1782eadbbba/pkg/cli/admin/catalog/mirror.go#L284-L313
// Hoping this can be temporary, and `oc adm mirror catalog` libs support index.yaml direct mirroring.

func newMirrorerFunc(opts *catalog.MirrorCatalogOptions) (catalog.ImageMirrorerFunc, error) {
	allmanifestsFilter := imagemanifest.FilterOptions{FilterByOS: ".*"}
	if err := allmanifestsFilter.Validate(); err != nil {
		return nil, err
	}

	return func(mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {
		mappings := []imgmirror.Mapping{}
		for from, to := range mapping {
			mappings = append(mappings, imgmirror.Mapping{
				Source:      from,
				Destination: to,
			})
		}

		a := imgmirror.NewMirrorImageOptions(opts.IOStreams)
		a.SkipMissing = true
		a.ContinueOnError = true
		a.DryRun = opts.DryRun
		a.SecurityOptions = opts.SecurityOptions
		a.FileDir = opts.FileDir
		// because images in the catalog are statically referenced by digest,
		// we do not allow filtering for mirroring. this may change if sparse manifestlists are allowed
		// by registries, or if multi-arch management moves into images that can be rewritten on mirror (i.e. the bundle
		// images themselves, not the images referenced inside of the bundle images).
		a.FilterOptions = allmanifestsFilter
		a.ParallelOptions = opts.ParallelOptions
		a.KeepManifestList = true
		a.Mappings = mappings
		a.SkipMultipleScopes = true

		if err := a.Validate(); err != nil {
			fmt.Fprintf(opts.IOStreams.ErrOut, "error configuring image mirroring: %v\n", err)
			return nil
		}

		if err := a.Run(); err != nil {
			fmt.Fprintf(opts.IOStreams.ErrOut, "error mirroring image: %v\n", err)
			return nil
		}

		return nil
	}, nil
}
