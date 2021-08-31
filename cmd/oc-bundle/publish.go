package main

import (
	"github.com/spf13/cobra"

	"github.com/RedHatGov/bundle/pkg/bundle/publish"
	"github.com/RedHatGov/bundle/pkg/cli"
)

func newPublishCmd(ro *cli.RootOptions) *cobra.Command {
	opts := publish.Options{
		RootOptions: ro,
	}

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish OCP related content to an internet-disconnected environment",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			checkErr(opts.Run(cmd.Context()))
		},
	}

	opts.BindFlags(cmd.PersistentFlags())

	return cmd
}
