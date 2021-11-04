package describe

import (
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

type DescribeOptions struct {
	*cli.RootOptions
}

func NewDescribeCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := DescribeOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Pretty print the contents of mirror metadata",
		Example: templates.Examples(`
			oc-mirror describe mirror_seq1_00000.tar
		`),
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd, f, args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run())
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *DescribeOptions) Complete(cmd *cobra.Command, f kcmdutil.Factory, args []string) error {
	return nil
}

func (o *DescribeOptions) Validate() error {
	return nil
}

func (o *DescribeOptions) Run() error {
	logrus.Info("Not implemented")
	return nil
}
