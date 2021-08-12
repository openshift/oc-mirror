package main

import (
	"github.com/spf13/cobra"

	"github.com/RedHatGov/bundle/pkg/bundle/create"
	"github.com/RedHatGov/bundle/pkg/cli"
)

func newCreateCmd(ro *cli.RootOptions) *cobra.Command {

	opts := create.Options{
		RootOptions: ro,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create image mirror bundles of OCP related resources",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newCreateFullCmd(&opts),
		newCreateDiffCmd(&opts),
	)

	opts.BindFlags(cmd.PersistentFlags())

	return cmd
}

func newCreateFullCmd(o *create.Options) *cobra.Command {

	return &cobra.Command{
		Use:   "full",
		Short: "Create a full OCP related container image mirror",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			checkErr(o.RunFull(cmd.Context()))
		},
	}
}

func newCreateDiffCmd(o *create.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Create a differential OCP related container image mirror updates",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			checkErr(o.RunDiff(cmd.Context()))
		},
	}
}
