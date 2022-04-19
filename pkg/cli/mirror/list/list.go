package list

import (
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/oc-mirror/pkg/cli"
)

func NewListCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available platform and operator content and their versions.",
		Run:   kcmdutil.DefaultSubCommandRun(ro.IOStreams.ErrOut),
	}

	cmd.AddCommand(NewOperatorsCommand(f, ro))
	cmd.AddCommand(NewReleasesCommand(f, ro))
	cmd.AddCommand(NewUpdatesCommand(f, ro))

	return cmd
}
