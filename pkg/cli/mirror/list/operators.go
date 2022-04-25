package list

import (
	"errors"
	"fmt"
	"io"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
)

type OperatorsOptions struct {
	*cli.RootOptions
	Catalog  string
	Package  string
	Channel  string
	Version  string
	Catalogs bool
}

func NewOperatorsCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := OperatorsOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "operators",
		Short: "List available operator catalog content and versions",
		Example: templates.Examples(`
		    # List available operator catalog release versions
			oc-mirror list operators

			# Output default operator catalogs for OpenShift release 4.8
			oc-mirror list operators --catalogs --version=4.8

			# List all operator packages in a catalog
			oc-mirror list operators --catalog=catalog-name

			# List all channels in an operator package
			oc-mirror list operators --catalog=catalog-name --package=package-name

			# List all available versions for a specified operator in a channel
			oc-mirror list operators --catalog=catalog-name --package=operator-name --channel=channel-name
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd))
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&o.Catalogs, "catalogs", o.Catalogs, "List available catalogs for an OpenShift release version, requires --version")
	fs.StringVar(&o.Catalog, "catalog", o.Catalog, "List information for a specified catalog")
	fs.StringVar(&o.Package, "package", o.Package, "List information for a specified package")
	fs.StringVar(&o.Channel, "channel", o.Channel, "List information for a specified channel")
	fs.StringVar(&o.Version, "version", o.Version, "Specify an OpenShift release version")

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *OperatorsOptions) Complete() error {
	if len(o.Version) > 0 {
		o.Catalogs = true
	}
	return nil
}

func (o *OperatorsOptions) Validate() error {
	if len(o.Version) == 0 && o.Catalogs {
		return errors.New("must specify --version with --catalogs")
	}
	if len(o.Channel) > 0 && (len(o.Package) == 0 || len(o.Catalog) == 0) {
		return errors.New("must specify --catalog and --package with --channel")
	}
	if len(o.Package) > 0 && len(o.Catalog) == 0 {
		return errors.New("must specify --catalog with --package")
	}
	return nil
}

func (o *OperatorsOptions) Run(cmd *cobra.Command) error {

	w := o.IOStreams.Out
	ctx := cmd.Context()

	// Process cases from most specific to most broad
	switch {
	case len(o.Channel) > 0:
		// Print Version for all bundles in a channel
		var ch model.Channel
		lc := action.ListChannels{
			IndexReference: o.Catalog,
			PackageName:    o.Package,
		}
		res, err := lc.Run(ctx)
		if err != nil {
			return err
		}
		// Find target channel for searching
		for _, c := range res.Channels {
			if c.Name == o.Channel {
				ch = c
				break
			}
		}

		if _, err := fmt.Fprintln(w, "VERSIONS"); err != nil {
			return err
		}
		// List all bundle versions in channel
		for _, bndl := range ch.Bundles {
			if _, err := fmt.Fprintln(w, bndl.Version); err != nil {
				return err
			}
		}
	case len(o.Package) > 0:
		lc := action.ListChannels{
			IndexReference: o.Catalog,
			PackageName:    o.Package,
		}
		res, err := lc.Run(ctx)
		if err != nil {
			return err
		}
		if err := res.WriteColumns(o.IOStreams.Out); err != nil {
			return err
		}
	case len(o.Catalog) > 0:
		lp := action.ListPackages{
			IndexReference: o.Catalog,
		}
		res, err := lp.Run(ctx)
		if err != nil {
			return fmt.Errorf("failed to list operators, please check catalog name - %s : %w", o.Catalog, err)
		}
		if err := res.WriteColumns(o.IOStreams.Out); err != nil {
			return err
		}
	case o.Catalogs:
		if _, err := fmt.Fprintln(w, "Available OpenShift OperatorHub catalogs:"); err != nil {
			return err
		}
		if err := o.listCatalogs(w); err != nil {
			return err
		}
	default:

		vm, err := image.GetTagsFromImage(catalogs[0])
		if err != nil {
			return err
		}

		fmt.Fprintln(w, "Available OpenShift OperatorHub catalog versions:")

		for v := range vm {

			if _, err := fmt.Fprintf(w, "  %s\n", v); err != nil {
				return err
			}

		}
	}

	return nil
}

var catalogs = []string{
	"registry.redhat.io/redhat/redhat-operator-index",
	"registry.redhat.io/redhat/certified-operator-index",
	"registry.redhat.io/redhat/community-operator-index",
	"registry.redhat.io/redhat/redhat-marketplace-index",
}

func (o *OperatorsOptions) listCatalogs(w io.Writer) error {

	if _, err := fmt.Fprintf(w, "OpenShift %s:\n", o.Version); err != nil {
		return err
	}
	for _, catalog := range catalogs {
		versions, err := image.GetTagsFromImage(catalog)
		if err != nil {
			fmt.Fprintf(w, "Failed to get catalog version details: %s", err)
			continue
		}

		if versions["v"+o.Version] > 0 {
			fmt.Fprintf(w, "%s:v%s\n", catalog, o.Version)
		} else {
			fmt.Fprintf(w, "Invalid catalog reference, please check version: %s:v%s\n", catalog, o.Version)
		}
	}
	return nil
}
