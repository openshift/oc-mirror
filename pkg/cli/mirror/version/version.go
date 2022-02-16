package version

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	apimachineryversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/component-base/version"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/pkg/cli"
)

type VersionOptions struct {
	*cli.RootOptions
	Output string
	Short  bool
}

// Version is a struct for version information
type Version struct {
	ClientVersion *apimachineryversion.Info `json:"clientVersion,omitempty" yaml:"clientVersion,omitempty"`
}

func NewVersionCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := VersionOptions{
		RootOptions: ro,
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Output version",
		Example: templates.Examples(`
			# Get oc-mirror version
			oc-mirror version
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Run())
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&o.Short, "short", o.Short, "If true, print just the version number")
	fs.StringVar(&o.Output, "output", o.Output, "One of 'yaml' or 'json'.")
	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

// Validate validates the provided options
func (o *VersionOptions) Validate() error {
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		return errors.New(`--output must be 'yaml' or 'json'`)
	}

	return nil
}

// Run executes version command
func (o *VersionOptions) Run() error {
	var versionInfo Version

	clientVersion := version.Get()
	versionInfo.ClientVersion = &clientVersion

	switch o.Output {
	case "":
		if o.Short {
			fmt.Fprintf(o.Out, "Client Version: %s\n", clientVersion.GitVersion)
		} else {
			fmt.Fprintf(o.Out, "Client Version: %#v\n", clientVersion)
		}
	case "yaml":
		marshalled, err := yaml.Marshal(&versionInfo)
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(marshalled))
	case "json":
		marshalled, err := json.MarshalIndent(&versionInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(marshalled))
	default:
		// There is a bug in the program if we hit this case.
		// However, we follow a policy of never panicking.
		return fmt.Errorf("VersionOptions were not validated: --output=%q should have been rejected", o.Output)
	}

	return nil
}
