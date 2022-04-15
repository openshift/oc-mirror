package list

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

type UpdatesOptions struct {
	*cli.RootOptions
	ConfigPath    string
	FilterOptions []string
}

func NewUpdatesCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := UpdatesOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "updates",
		Short: "List available updates in upgrade graph from upstream sources",
		Long: templates.LongDesc(`
		List available updates in the upgrade graph for releases and operators from upstream sources
		based on current state. A storage configuration must be specified to use this command.
	`),
		Example: templates.Examples(`
			# List updates between remote and current workspace
			oc-mirror list updates --config mirror-config.yaml
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	fs := cmd.Flags()
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.StringSliceVar(&o.FilterOptions, "filter-options", o.FilterOptions, "An architecture list to control the release image"+
		"picked when multiple variants are available")

	// TODO(jpower432): Make this flag visible again once release architecture selection
	// has been more thouroughly vetted
	if err := fs.MarkHidden("filter-options"); err != nil {
		logrus.Panic(err.Error())
	}
	return cmd
}

func (o *UpdatesOptions) Complete() error {
	if len(o.FilterOptions) == 0 {
		o.FilterOptions = []string{v1alpha2.DefaultPlatformArchitecture}
	}
	return nil
}

func (o *UpdatesOptions) Validate() error {
	if len(o.ConfigPath) == 0 {
		return fmt.Errorf("must specify config using --config")
	}
	for _, arch := range o.FilterOptions {
		if _, ok := cincinnati.SupportedArchs[arch]; !ok {
			return fmt.Errorf("architecture %q is not a supported release architecture", arch)
		}
	}
	return nil
}

func (o *UpdatesOptions) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig(o.ConfigPath)
	if err != nil {
		return err
	}

	path := filepath.Join(o.Dir, config.SourceDir)
	backend, err := storage.ByConfig(path, cfg.StorageConfig)
	if err != nil {
		return fmt.Errorf("error opening backend: %v", err)
	}

	var meta v1alpha2.Metadata
	switch err := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath); {
	case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
		return err
	case err != nil && errors.Is(err, storage.ErrMetadataNotExist):
		return fmt.Errorf("no metadata detected")
	default:
		for _, arch := range o.FilterOptions {
			if len(cfg.Mirror.Platform.Channels) != 0 {
				if err := o.releaseUpdates(ctx, arch, cfg, meta.PastMirror); err != nil {
					return err
				}
			}
		}

		if len(cfg.Mirror.Operators) != 0 {
			if err := o.operatorUpdates(ctx, cfg, meta); err != nil {
				return err
			}
		}
	}

	return nil
}

func (o UpdatesOptions) releaseUpdates(ctx context.Context, arch string, cfg v1alpha2.ImageSetConfiguration, last v1alpha2.PastMirror) error {
	logrus.Info("Getting release update information")
	lastMaxVersion := map[string]semver.Version{}
	for _, ch := range last.Mirror.Platform.Channels {
		version, err := semver.Parse(ch.MaxVersion)
		if err != nil {
			return err
		}
		lastMaxVersion[ch.Name] = version
	}

	// Find the latest version is each channel being requested and plot upgrade graph between the old
	// versions if available
	id := uuid.New()

	for _, ch := range cfg.Mirror.Platform.Channels {

		var c cincinnati.Client
		var err error
		if ch.Type == v1alpha2.TypeOKD {
			c, err = cincinnati.NewOKDClient(id)
		} else {
			c, err = cincinnati.NewOCPClient(id)
		}
		if err != nil {
			return err
		}
		latest, err := cincinnati.GetChannelMinOrMax(ctx, c, arch, ch.Name, false)
		if err != nil {
			return err
		}
		ver, found := lastMaxVersion[ch.Name]
		if !found {
			ver = latest
		}
		_, _, upgrades, err := cincinnati.GetUpdates(ctx, c, arch, ch.Name, ver, latest)
		if err != nil {
			return err
		}

		var vers []semver.Version
		for _, upgrade := range upgrades {
			vers = append(vers, upgrade.Version)
		}

		if err := o.writeReleaseColumns(vers, arch, ch.Name); err != nil {
			return err
		}
	}
	return nil
}

func (o UpdatesOptions) operatorUpdates(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, meta v1alpha2.Metadata) error {
	logrus.Info("Getting operator update information")
	dstDir, err := os.MkdirTemp(o.Dir, "updatetmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dstDir)

	// Find last operator catalog digest
	var pin string

	for _, op := range meta.PastMirror.Operators {
		if len(op.ImagePin) != 0 {
			pin = op.ImagePin
			break
		}
	}

	reg, err := containerdregistry.NewRegistry(
		containerdregistry.SkipTLSVerify(false),
		containerdregistry.WithCacheDir(filepath.Join(dstDir, "cache")),
	)
	defer reg.Destroy()
	if err != nil {
		return err
	}
	for _, ctlg := range cfg.Mirror.Operators {
		catLogger := logrus.WithField("catalog", ctlg.Catalog)
		dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
		if err != nil {
			return err
		}
		diff := action.Diff{
			Registry:      reg,
			NewRefs:       []string{ctlg.Catalog},
			Logger:        catLogger,
			IncludeConfig: dic,
		}
		if len(pin) != 0 {
			diff.OldRefs = []string{pin}
		}
		dc, err := diff.Run(ctx)
		if err != nil {
			return err
		}

		if err := o.writeCatalogColumns(*dc, ctlg.Catalog); err != nil {
			return err
		}
	}
	return nil
}

func (o UpdatesOptions) writeReleaseColumns(upgrades []semver.Version, arch, channel string) error {
	if len(upgrades) == 0 {
		if _, err := fmt.Fprintf(os.Stdout, "No updates found for release channel %s\n", channel); err != nil {
			return err
		}
		return nil
	}
	tw := tabwriter.NewWriter(o.IOStreams.Out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "CHANNEL:\t%s\n", channel); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "ARCHITECTURE:\t%s\n", arch); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "VERSIONS"); err != nil {
		return err
	}
	for _, upgrade := range upgrades {
		if _, err := fmt.Fprintf(tw, "%s\n", upgrade); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func (o UpdatesOptions) writeCatalogColumns(dc declcfg.DeclarativeConfig, catalog string) error {
	if len(dc.Packages) == 0 {
		if _, err := fmt.Fprintf(os.Stdout, "No updates found for catalog %s\n", catalog); err != nil {
			return err
		}
		return nil
	}
	tw := tabwriter.NewWriter(o.IOStreams.Out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "Listing update for catalog:\t%s", catalog); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "PACKAGE\tCHANNEL\tBUNDLE\tREPLACES"); err != nil {
		return err
	}
	mod, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return err
	}

	pkgs := []model.Package{}
	for _, pkg := range mod {
		pkgs = append(pkgs, *pkg)
	}

	bundles := []model.Bundle{}
	for _, pkg := range pkgs {
		for _, ch := range pkg.Channels {
			for _, b := range ch.Bundles {
				bundles = append(bundles, *b)
			}
		}
	}

	for _, b := range bundles {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", b.Package.Name, b.Channel.Name, b.Name, b.Replaces); err != nil {
			return err
		}
	}

	return tw.Flush()
}
