package list

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

type OperatorsOptions struct {
	*cli.RootOptions
	Channels bool
	Versions bool
	Channel  string
	Package  string
}

func NewOperatorsCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := OperatorsOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "operators",
		Short: "List available operator content and their version",
		Example: templates.Examples(`
			# Output default operator channles
			oc-mirror list operators --channels
			# List all operators in a channel
			oc-mirror list operators --channel=channel-name --packages
			# List all available versions for a specified operator
			oc-mirror list operators --channel=channel-name --package=operator-name --versions
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd, f, args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run())
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&o.Channels, "channels", o.Channels, "List all channel information")
	fs.BoolVar(&o.Versions, "versions", o.Versions, "List all version information")
	fs.StringVar(&o.Package, "package", o.Package, "List information for specified package")
	fs.StringVar(&o.Channel, "channel", o.Channel, "List information for specified channel")

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *OperatorsOptions) Complete(cmd *cobra.Command, f kcmdutil.Factory, args []string) error {
	return nil
}

func (o *OperatorsOptions) Validate() error {
	return nil
}

func (o *OperatorsOptions) Run() error {
	logrus.Info("Not implemented")
	return nil
}
