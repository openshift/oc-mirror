package list

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

type ReleasesOptions struct {
	*cli.RootOptions
	Channels bool
	Versions bool
	Channel  string
}

func NewReleasesCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := ReleasesOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "releases",
		Short: "List available platform content and their version",
		Example: templates.Examples(`
			# Output OCP release channel list
            oc-mirror list releases --channels
			# List all OCP versions in a specified channel
			oc-mirror list releases --channel=stable-4.8 --versions
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
	fs.StringVar(&o.Channel, "channel", o.Channel, "List information for specified channel")

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *ReleasesOptions) Complete(cmd *cobra.Command, f kcmdutil.Factory, args []string) error {
	return nil
}

func (o *ReleasesOptions) Validate() error {
	return nil
}

func (o *ReleasesOptions) Run() error {
	logrus.Info("Not implemented")
	return nil
}
