package list

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/spf13/cobra"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/operator"
)

const (
	// Known catalog registries
	redhatCatalogRegistry      string = "registry.redhat.io/redhat/redhat-operator-index"
	communityCatalogRegistry   string = "registry.redhat.io/redhat/community-operator-index"
	certifiedCatalogRegistry   string = "registry.redhat.io/redhat/certified-operator-index"
	marketplaceCatalogRegistry string = "registry.redhat.io/redhat/redhat-marketplace-index"
)

type listOperatorsOptions struct {
	catalogs   bool
	catalog    string
	operator   string
	channel    string
	version    string
	globalOpts *mirror.CopyOptions
}

func NewListOperatorsCommand(log log.PluggableLoggerInterface, globalOpts *mirror.CopyOptions) *cobra.Command {
	opts := &listOperatorsOptions{globalOpts: globalOpts}

	cmd := &cobra.Command{
		Use:   "operators",
		Short: "List available operator catalog content and versions",
		Example: templates.Examples(`
            # Output default operator catalogs for OpenShift release 4.8
            oc-mirror --v2 list operators --catalogs --version=4.8

            # List all operator packages in a catalog
            oc-mirror --v2 list operators --catalog=catalog-name

            # List all channels in an operator package
            oc-mirror --v2 list operators --catalog=catalog-name --package=package-name

            # List all available versions for a specified operator in a channel
            oc-mirror --v2 list operators --catalog=catalog-name --package=operator-name --channel=channel-name
    `),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(opts.version) > 0 {
				opts.catalogs = true
			}
			if len(opts.catalog) > 0 {
				if _, err := reference.ParseNamed(opts.catalog); err != nil {
					return fmt.Errorf("%q: %w", opts.catalog, err)
				}
			}
			if len(opts.operator) > 0 && len(opts.catalog) == 0 {
				return errors.New("must specify --catalog with --package")
			}
			if len(opts.channel) > 0 && len(opts.operator) == 0 {
				return errors.New("must specify --catalog and --package with --channel")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// NOTE: we don't want help output on errors from here onwards
			cmd.SilenceUsage = true
			return run(cmd.Context(), log, opts)
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&opts.catalogs, "catalogs", false, "List available catalogs for an OpenShift release version. Requires --version.")
	fs.StringVar(&opts.version, "version", "", "Specify an OpenShift release version.")
	fs.StringVar(&opts.catalog, "catalog", "", "List information for a specified catalog.")
	fs.StringVar(&opts.operator, "package", "", "List information for a specified package. Requires --catalog.")
	fs.StringVar(&opts.channel, "channel", "", "List information for a specified channel. Requires --catalog and --package.")

	cmd.MarkFlagsRequiredTogether("catalogs", "version")
	cmd.MarkFlagsMutuallyExclusive("catalog", "catalogs")
	cmd.MarkFlagsOneRequired("catalog", "catalogs")

	return cmd
}

func run(ctx context.Context, log log.PluggableLoggerInterface, opts *listOperatorsOptions) error {
	var catalog model.Model
	if !opts.catalogs {
		var err error
		catalog, err = downloadCatalog(ctx, log, opts.catalog, *opts.globalOpts)
		if err != nil {
			return fmt.Errorf("failed to load catalog: %w", err)
		}
		log.Debug("Loaded catalog %s", opts.catalog)
	}

	var err error
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	switch {
	case len(opts.channel) > 0:
		pkg, ok := catalog[opts.operator]
		if !ok {
			return fmt.Errorf("operator %q not found in catalog", opts.operator)
		}
		ch, ok := pkg.Channels[opts.channel]
		if !ok {
			return fmt.Errorf("channel %q not found for operator %q", opts.channel, opts.operator)
		}
		err = listBundles(tw, ch)
	case len(opts.operator) > 0:
		pkg, ok := catalog[opts.operator]
		if !ok {
			return fmt.Errorf("operator %q not found in catalog", opts.operator)
		}
		err = listChannels(tw, pkg)
	case len(opts.catalog) > 0:
		err = listOperators(tw, catalog)
	case len(opts.version) > 0:
		err = listCatalogsForVersion(ctx, log, tw, opts.version, *opts.globalOpts)
	}
	tw.Flush()

	return err
}

func downloadCatalog(ctx context.Context, log log.PluggableLoggerInterface, catalog string, opts mirror.CopyOptions) (model.Model, error) {
	handler := operator.CatalogHandler{
		Log:      log,
		Mirror:   mirror.New(mirror.NewMirrorCopy(), nil),
		Manifest: manifest.New(log),
	}

	// CLID-27 ensure we pick up oci:// (on disk) catalogs
	imgSpec, err := image.ParseRef(catalog)
	if err != nil {
		return nil, err
	}

	indexDir, err := os.MkdirTemp("", "oc-mirror-list-")
	if err != nil {
		return nil, fmt.Errorf("failed to create catalog extraction dir: %w", err)
	}
	defer os.RemoveAll(indexDir)

	if err := handler.EnsureCatalogInOCIFormat(ctx, imgSpec, catalog, indexDir, opts); err != nil {
		return nil, err
	}

	dcPath, err := handler.ExtractOCIConfigLayers(imgSpec, indexDir)
	if err != nil {
		return nil, err
	}
	log.Debug("catalog extracted to %s", dcPath)

	dc, err := handler.GetDeclarativeConfig(ctx, dcPath)
	if err != nil {
		return nil, err
	}

	m, err := declcfg.ConvertToModel(*dc)
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog model: %w", err)
	}

	return m, nil
}

func listBundles(w io.Writer, channel *model.Channel) error {
	fmt.Fprintln(w, "VERSIONS")
	for _, name := range slices.Sorted(maps.Keys(channel.Bundles)) {
		fmt.Fprintf(w, "%s\n", channel.Bundles[name].Version)
	}

	return nil
}

func listChannels(w io.Writer, pkg *model.Package) error {
	if pkg.DefaultChannel != nil {
		fmt.Fprintln(w, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL")
		fmt.Fprintf(w, "%s\t%s\t%s\n", pkg.Name, getDisplayName(pkg), pkg.DefaultChannel.Name)
		fmt.Fprintln(w, "")
	}

	fmt.Fprintln(w, "PACKAGE\tCHANNEL\tHEAD")
	for _, name := range slices.Sorted(maps.Keys(pkg.Channels)) {
		ch := pkg.Channels[name]
		var head string
		if h, err := ch.Head(); err != nil {
			head = fmt.Sprintf("ERROR: %s", err)
		} else {
			head = h.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", pkg.Name, ch.Name, head)
	}

	return nil
}

func listOperators(w io.Writer, catalogModel model.Model) error {
	fmt.Fprintln(w, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL")
	for _, name := range slices.Sorted(maps.Keys(catalogModel)) {
		op := catalogModel[name]
		defChannelName := ""
		if op.DefaultChannel != nil {
			defChannelName = op.DefaultChannel.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", op.Name, getDisplayName(op), defChannelName)
	}

	return nil
}

func getDisplayName(pkg *model.Package) string {
	// In the OLMv0 data model, the display name is not a property of the
	// operator/package but is instead buried within a CSV bundle metadata.
	// The OLM teams' advice to recover that info is to use the value from the
	// highest version bundle in the package.
	var highest *model.Bundle
	for _, ch := range pkg.Channels {
		for _, bundle := range ch.Bundles {
			if highest == nil || bundle.Version.GT(highest.Version) {
				highest = bundle
			}
		}
	}

	if highest == nil {
		return ""
	}

	// Try to get the display name from the "olm.csv.metadata" property from some operator's bundle.
	for _, prop := range highest.Properties {
		if prop.Type != property.TypeCSVMetadata {
			continue
		}
		var val property.CSVMetadata
		if err := json.Unmarshal(prop.Value, &val); err == nil {
			return val.DisplayName
		}
	}

	return ""
}

func listCatalogsForVersion(ctx context.Context, log log.PluggableLoggerInterface, w io.Writer, version string, opts mirror.CopyOptions) error {
	fmt.Fprintln(w, "Available OpenShift OperatorHub catalogs:")
	fmt.Fprintf(w, "OpenShift %s:\n", version)

	var errs []error
	for _, catalog := range []string{redhatCatalogRegistry, certifiedCatalogRegistry, communityCatalogRegistry, marketplaceCatalogRegistry} {
		ref := fmt.Sprintf("%s:v%s", catalog, version)
		log.Debug("Checking whether catalog %q exists", ref)
		exists, err := taggedCatalogExists(ctx, ref, opts)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to check catalog %q versions: %w", catalog, err))
			continue
		}
		if exists {
			fmt.Fprintf(w, "%s\n", ref)
		} else {
			fmt.Fprintf(w, "Invalid catalog reference %q, please check version\n", ref)
		}
	}

	return errors.Join(errs...)
}

func taggedCatalogExists(ctx context.Context, catalog string, opts mirror.CopyOptions) (bool, error) {
	ref, err := docker.ParseReference("//" + catalog)
	if err != nil {
		return false, fmt.Errorf("failed to parse catalog reference: %w", err)
	}
	src, err := ref.NewImageSource(ctx, opts.Global.NewSystemContext())
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "manifest unknown") || strings.Contains(msg, "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to load catalog image: %w", err)
	}
	defer src.Close()
	// If the tagged image has a valid manifest, then we know the tag is valid
	// without having to go through its tag list (which might be huge)
	return true, nil
}
