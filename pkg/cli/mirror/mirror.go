package mirror

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/describe"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/list"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/version"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

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
			# Publish to a registry and add a a top-level namespace
			oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000/namespace
			# Replay a previous mirror
			oc-mirror --config mirror-config.yaml --replay=5 file://mirror
		`),
		PersistentPreRun:  o.LogfilePreRun,
		PersistentPostRun: o.LogfilePostRun,
		Args:              cobra.MinimumNArgs(1),
		SilenceErrors:     false,
		SilenceUsage:      false,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd, f))
		},
	}

	o.BindFlags(cmd.Flags())
	o.RootOptions.BindFlags(cmd.PersistentFlags())

	cmd.AddCommand(version.NewVersionCommand(f, o.RootOptions))
	cmd.AddCommand(list.NewListCommand(f, o.RootOptions))
	cmd.AddCommand(describe.NewDescribeCommand(f, o.RootOptions))

	return cmd
}

func (o *MirrorOptions) Complete(args []string) error {

	destination := args[0]
	splitIdx := strings.Index(destination, "://")
	if splitIdx == -1 {
		return fmt.Errorf("no scheme delimiter in destination argument")
	}
	typStr, ref := destination[:splitIdx], destination[splitIdx+3:]

	switch typStr {
	case "file":
		ref = filepath.Clean(ref)
		if ref == "" {
			ref = "."
		}
		o.OutputDir = ref
		// If the destination is on disk, made the output dir the
		// parent dir for the workspace
		o.Dir = filepath.Join(o.OutputDir, o.Dir)
	case "docker":
		mirror, err := imagesource.ParseReference(ref)
		if err != nil {
			return err
		}
		o.ToMirror = mirror.Ref.Registry
		o.UserNamespace = mirror.Ref.AsRepository().RepositoryName()
		if mirror.Ref.ID != "" || mirror.Ref.Tag != "" {
			return fmt.Errorf("destination registry must consist of registry host and namespace(s) only")
		}
	default:
		return fmt.Errorf("unknown destination scheme %q", typStr)
	}

	return nil
}

func (o *MirrorOptions) Validate() error {
	switch {
	case len(o.From) > 0 && len(o.ToMirror) == 0:
		return fmt.Errorf("must specifiy a registry destination")
	case len(o.OutputDir) > 0 && len(o.ConfigPath) == 0:
		return fmt.Errorf("must specifiy a configuration file with --config")
	case len(o.ToMirror) > 0 && len(o.ConfigPath) == 0 && len(o.From) == 0:
		return fmt.Errorf("must specify --config or --from with registry destination")
	}

	// Attempt to login to registry
	// FIXME(jpower): CheckPushPermissions is slated for deprecation
	// must replace with its replacement
	logrus.Infof("Checking push permissions for %s", o.ToMirror)
	stringRef := fmt.Sprintf("%s/oc-mirror", o.ToMirror)
	if len(o.ToMirror) > 0 {
		ref, err := name.ParseReference(stringRef)
		if err != nil {
			return err
		}
		if err := remote.CheckPushPermission(ref, authn.DefaultKeychain, o.createRT()); err != nil {
			return fmt.Errorf("error checking push permissions for %s: %v", o.ToMirror, err)
		}
	}

	if len(o.From) > 0 {
		if _, err := os.Stat(o.From); err != nil {
			return err
		}
	}

	return nil
}

func (o *MirrorOptions) Run(cmd *cobra.Command, f kcmdutil.Factory) (err error) {
	if o.OutputDir != "" {
		if err := os.MkdirAll(o.OutputDir, 0755); err != nil {
			return err
		}
	}

	switch {
	case o.ManifestsOnly:
		logrus.Info("Not implemented yet")
	case len(o.OutputDir) > 0 && o.From == "":
		return o.Create(cmd.Context(), cmd.PersistentFlags())
	case len(o.ToMirror) > 0 && len(o.From) > 0:
		return o.Publish(cmd.Context(), cmd, f)
	case len(o.ToMirror) > 0 && len(o.ConfigPath) > 0:

		dir := o.OutputDir
		if dir == "" {
			// create temp workspace
			if dir, err = ioutil.TempDir(".", "mirrortmp"); err != nil {
				return err
			}
		}

		fmt.Fprintf(o.IOStreams.Out, "workspace: %s\n", dir)

		o.OutputDir = dir
		if err := o.Create(cmd.Context(), cmd.PersistentFlags()); err != nil {
			return err
		}

		// run publish
		o.From = dir
		o.OutputDir = ""

		if err := o.Publish(cmd.Context(), cmd, f); err != nil {
			fmt.Fprintf(o.IOStreams.ErrOut, "Image Publish:\nERROR: publishing operation failed: %v\nTo retry this operation run \"oc-mirror --from %s docker://%s\"\n", err, o.From, o.ToMirror)
			return kcmdutil.ErrExit
		}

		// Remove tmp directory
		if !o.SkipCleanup {
			fmt.Fprintln(o.IOStreams.Out, "cleaning up workspace")
			os.RemoveAll(dir)
		}
	}

	return nil
}

func (o *MirrorOptions) createRT() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: o.DestSkipTLS}
	return transport
}
