package mirror

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/cli/mirror/describe"
	"github.com/RedHatGov/bundle/pkg/cli/mirror/list"
	"github.com/RedHatGov/bundle/pkg/cli/mirror/version"
)

type MirrorOptions struct {
	*cli.RootOptions
	OutputDir       string
	ConfigPath      string
	SkipImagePin    bool
	ManifestsOnly   bool
	From            string
	ToMirror        string
	BuildxPlatforms []string
}

func NewMirrorCmd() *cobra.Command {
	o := MirrorOptions{}
	o.RootOptions = &cli.RootOptions{
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}

	// Configures a REST client getter factory from configs for mirroring releases.
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDiscoveryBurst(250)
	matchVersionKubeConfigFlags := kcmdutil.NewMatchVersionFlags(kubeConfigFlags)
	f := kcmdutil.NewFactory(matchVersionKubeConfigFlags)

	cmd := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "Manages mirrors per user configuration",
		Long: templates.LongDesc(`
			oc-mirror will create and publish user configured mirrors with
            a declarative configation input
		`),
		Example: templates.Examples(`
			# Mirror to a directory
			oc-mirror --config mirror-config.yaml file://mirror
			# Mirror to mirror publish
			oc-mirror --config mirror-config.yaml docker://localhost:5000
			# Publish a previously created mirror archive
			oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000
			# Replay a previous mirror
			oc-mirror --config mirror-config.yaml --replay=5 file://mirror
		`),
		PersistentPreRun:  o.LogfilePreRun,
		PersistentPostRun: o.LogfilePostRun,
		Args:              cobra.MinimumNArgs(1),
		SilenceErrors:     false,
		SilenceUsage:      false,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd, f, args))
			kcmdutil.CheckErr(o.Validate(cmd, f))
			kcmdutil.CheckErr(o.Run(cmd, f))
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.BoolVar(&o.SkipImagePin, "skip-image-pin", o.SkipImagePin, "Do not replace image tags with digest pins in operator catalogs")
	fs.StringVar(&o.From, "from", o.From, "The archive file path.")
	fs.BoolVar(&o.ManifestsOnly, "manifests-only", o.ManifestsOnly, "Generate manifests and do not mirror")
	fs.StringSliceVar(&o.BuildxPlatforms, "buildx-platforms", o.BuildxPlatforms,
		"If set, the command will invoke 'docker buildx build' to build a catalog manifest list "+
			"for the specified platforms, ex. linux/amd64, instead of 'podman build' for the host platform. "+
			"The 'buildx' plugin and accompanying configuration MUST be installed on the build host. "+
			"This list does NOT filter operator bundle manifest list platforms within the catalog")

	o.BindFlags(cmd.PersistentFlags())

	cmd.AddCommand(version.NewVersionCommand(f, o.RootOptions))
	cmd.AddCommand(list.NewListCommand(f, o.RootOptions))
	cmd.AddCommand(describe.NewDescribeCommand(f, o.RootOptions))

	return cmd
}

func (o *MirrorOptions) Complete(cmd *cobra.Command, f kcmdutil.Factory, args []string) error {

	destination := args[0]
	switch {
	case strings.Contains(destination, "file://"):
		o.OutputDir = strings.TrimPrefix(destination, "file://")
	case strings.Contains(destination, "docker://"):
		o.ToMirror = strings.TrimPrefix(destination, "docker://")
	}

	return nil
}

func (o *MirrorOptions) Validate(cmd *cobra.Command, f kcmdutil.Factory) error {
	switch {
	case len(o.From) > 0 && len(o.ToMirror) == 0:
		return fmt.Errorf("must specifiy a registry destination")
	case len(o.OutputDir) > 0 && len(o.ConfigPath) == 0:
		return fmt.Errorf("must specifiy a configuration file with --config")
	case len(o.ToMirror) > 0 && len(o.ConfigPath) == 0 && len(o.From) == 0:
		return fmt.Errorf("must specify --config or --from with registry destination")
	}

	// Attempt to login to registry
	if len(o.ToMirror) > 0 {
		logrus.Debug("Registry auth check not implemented")
	}

	if _, err := os.Stat(o.Dir); err != nil {
		return err
	}

	if len(o.OutputDir) > 0 {
		if _, err := os.Stat(o.OutputDir); err != nil {
			return err
		}
	}

	if len(o.From) > 0 {
		if _, err := os.Stat(o.From); err != nil {
			return err
		}
	}

	return nil
}

func (o *MirrorOptions) Run(cmd *cobra.Command, f kcmdutil.Factory) error {
	switch {
	case o.ManifestsOnly:
		logrus.Info("Not implemented yet")
	case len(o.OutputDir) > 0:
		return o.Create(cmd.Context(), cmd.PersistentFlags())
	case len(o.ToMirror) > 0 && len(o.From) > 0:
		return o.Publish(cmd.Context(), cmd, f)
	case len(o.ToMirror) > 0 && len(o.ConfigPath) > 0:

		// create temp workspace
		dir, err := ioutil.TempDir(o.Dir, "mirrortmp")
		if err != nil {
			return err
		}

		if !o.SkipCleanup {
			defer os.RemoveAll(dir)
		}

		o.OutputDir = dir
		if err := o.Create(cmd.Context(), cmd.PersistentFlags()); err != nil {
			return err
		}

		// run publish
		o.From = dir
		o.OutputDir = ""

		if err := o.Publish(cmd.Context(), cmd, f); err != nil {
			return err
		}
	}

	return nil
}
