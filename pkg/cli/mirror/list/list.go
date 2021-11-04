package list

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

func NewListCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available platform and operator content and their version",
		Example: templates.Examples(`
			# Output default operator channles
			oc-mirror list operators --channels
			# Output OCP release channel list
            oc-mirror list releases --channels
			# List all OCP versions in a specified channel
			oc-mirror list releases --channel=stable-4.8 --versions
			# List all operators in a channel
			oc-mirror list operators --channel=channel-name --packages
			# List all available versions for a specified operator
			oc-mirror list operators --channel=channel-name --package=operator-name --versions
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
