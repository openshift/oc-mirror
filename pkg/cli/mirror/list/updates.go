package list

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
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
			kcmdutil.CheckErr(o.Complete(cmd, f, args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run())
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	fs := cmd.Flags()
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")

	return cmd
}

func (o *UpdatesOptions) Complete(cmd *cobra.Command, f kcmdutil.Factory, args []string) error {
	return nil
}

func (o *UpdatesOptions) Validate() error {
	return nil
}

func (o *UpdatesOptions) Run() error {
	logrus.Info("Not implemented")
	return nil
}
