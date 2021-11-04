package version

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type VersionOptions struct {
	*cli.RootOptions
}

func NewVersionCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := VersionOptions{
		ro,
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Output version",
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run())
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *VersionOptions) Validate() error {
	return nil
}

func (o *VersionOptions) Run() error {
	logrus.Info("Not implemented")
	return nil
}
