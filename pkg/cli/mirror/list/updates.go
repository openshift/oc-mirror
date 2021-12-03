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

	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

type UpdatesOptions struct {
	*cli.RootOptions
	ConfigPath string
}

func NewUpdatesCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := UpdatesOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "updates",
		Short: "List available updates from upstream sources",
		Example: templates.Examples(`
			# List updates between remote and current workspace
			oc-mirror list updates --config mirror-config.yaml
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	fs := cmd.Flags()
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	return cmd
}

func (o *UpdatesOptions) Validate() error {
	if len(o.ConfigPath) == 0 {
		return fmt.Errorf("must specify config using --config")
	}
	return nil
}

func (o *UpdatesOptions) Run(ctx context.Context) error {
	cfg, err := config.LoadConfig(o.ConfigPath)
	if err != nil {
		return err
	}

	path := filepath.Join(o.Dir, config.SourceDir)
	backend, err := storage.ByConfig(path, cfg.StorageConfig)
	if err != nil {
		return fmt.Errorf("error opening backend: %v", err)
	}

	var meta v1alpha1.Metadata
	switch err := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath); {
	case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
		return err
	case err != nil && errors.Is(err, storage.ErrMetadataNotExist):
		return fmt.Errorf("no metadata detected")
	default:
		if len(cfg.Mirror.OCP.Channels) != 0 {
			if err := o.releaseUpdates(ctx, cfg, meta); err != nil {
				return err
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

func (o UpdatesOptions) releaseUpdates(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata) error {
	uuid := uuid.New()
	// QUESTION(jpower): Handle multiple arch?
	arch := "amd64"
	logrus.Info("Getting release update information")
	for _, ch := range cfg.Mirror.OCP.Channels {
		url := cincinnati.UpdateUrl
		if ch.Name == "okd" {
			url = cincinnati.OkdUpdateURL
		}

		client, upstream, err := cincinnati.NewClient(url, uuid)
		if err != nil {
			return err
		}

		// Find the last release downloads if no downloads
		// have been made in the target channel list
		// all versions
		var vers []semver.Version
		lastCh, ver, err := cincinnati.FindLastRelease(meta, ch.Name)
		switch {
		case err != nil && !errors.Is(err, cincinnati.ErrNoPreviousRelease):
			return err
		case err != nil:
			vers, err = client.GetVersions(ctx, upstream, ch.Name)
			if err != nil {
				return err
			}
		default:
			latest, err := client.GetChannelLatest(ctx, upstream, arch, ch.Name)
			if err != nil {
				return err
			}
			logrus.Debugf("Finding releases between %s and %s", ver.String(), latest.String())
			_, _, upgrades, err := client.CalculateUpgrades(ctx, upstream, arch, lastCh, ch.Name, ver, latest)
			if err != nil {
				return err
			}
			for _, upgrade := range upgrades {
				vers = append(vers, upgrade.Version)
			}
		}

		if err := o.writeReleaseColumns(vers, ch.Name); err != nil {
			return err
		}

	}
	return nil
}

func (o UpdatesOptions) operatorUpdates(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata) error {
	logrus.Info("Getting operator update information")
	dstDir, err := os.MkdirTemp(o.Dir, "updatetmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dstDir)

	// Find last operator catalog digest
	var pin string
	for _, mirror := range meta.PastMirrors {
		for _, op := range mirror.Operators {
			if len(op.ImagePin) != 0 {
				pin = op.ImagePin
				break
			}
		}
	}

	// QUESTION(jpower): TLS must be configurable?
	reg, err := containerdregistry.NewRegistry(
		containerdregistry.SkipTLS(false),
		containerdregistry.WithCacheDir(filepath.Join(dstDir, "cache")),
	)
	defer reg.Destroy()
	if err != nil {
		return err
	}
	for _, ctlg := range cfg.Mirror.Operators {
		catLogger := logrus.WithField("catalog", ctlg.Catalog)
		diff := action.Diff{
			Registry:      reg,
			NewRefs:       []string{ctlg.Catalog},
			Logger:        catLogger,
			IncludeConfig: ctlg.DiffIncludeConfig,
		}
		if len(pin) != 0 {
			diff.OldRefs = []string{pin}
		}
		dc, err := diff.Run(ctx)
		if err != nil {
			return err
		}

		if err := o.writeCatalogColumns(*dc); err != nil {
			return err
		}
	}
	return nil
}

func (o UpdatesOptions) writeReleaseColumns(upgrades []semver.Version, channel string) error {
	tw := tabwriter.NewWriter(o.IOStreams.Out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "TARGET CHANNEL:\t%s\n", channel); err != nil {
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

func (o UpdatesOptions) writeCatalogColumns(dc declcfg.DeclarativeConfig) error {
	tw := tabwriter.NewWriter(o.IOStreams.Out, 0, 4, 2, ' ', 0)
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
