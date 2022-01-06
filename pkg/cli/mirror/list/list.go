package list

import (
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/cli"
)

func NewListCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available platform and operator content and their version",
		Example: templates.Examples(`
			# Output operator catalogs
			oc-mirror list operators --version=4.9

			# Output OCP release channel list
			oc-mirror list releases --channels --version=4.9

			# List all OCP versions in a specified channel
			oc-mirror list releases --channel=stable-4.8

			# List all operators in a catalog
			oc-mirror list operators --catalog=catalog-name

			# List all available versions for a specified operator
			oc-mirror list operators --catalog=catalog-name --channel=channel-name --package=operator-name

			# List updates between remote and current workspace
			oc-mirror list updates --config mirror-config.yaml
		`),
		Run: kcmdutil.DefaultSubCommandRun(ro.IOStreams.ErrOut),
	}

	cmd.AddCommand(NewOperatorsCommand(f, ro))
	cmd.AddCommand(NewReleasesCommand(f, ro))
	cmd.AddCommand(NewUpdatesCommand(f, ro))

	return cmd
}
