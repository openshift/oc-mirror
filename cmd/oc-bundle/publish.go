package main

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/RedHatGov/bundle/pkg/bundle/publish"
	"github.com/RedHatGov/bundle/pkg/cli"
)

func newPublishCmd(ro *cli.RootOptions) *cobra.Command {
	opts := publish.Options{
		RootOptions: ro,
	}

	// Configures a REST client getter factory from configs for mirroring releases.
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDiscoveryBurst(250)
	matchVersionKubeConfigFlags := kcmdutil.NewMatchVersionFlags(kubeConfigFlags)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish OCP related content to an internet-disconnected environment",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			f := kcmdutil.NewFactory(matchVersionKubeConfigFlags)
			checkErr(opts.Run(cmd.Context(), cmd, f))
		},
	}

	kubeConfigFlags.AddFlags(cmd.Flags())
	matchVersionKubeConfigFlags.AddFlags(cmd.Flags())
	opts.BindFlags(cmd.Flags())

	return cmd
}
