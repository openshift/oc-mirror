package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/RedHatGov/bundle/pkg/bundle/create"
)

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create image mirror bundles of OCP related resources",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCreateFullCmd())
	cmd.AddCommand(newCreateDiffCmd())
	return cmd
}

func newCreateFullCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "full",
		Short: "Create a full OCP related container image mirror",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()
			logrus.Infoln("Create full called")
			err := create.CreateFull(rootOpts.configPath, rootOpts.dir, rootOpts.dryRun, rootOpts.skipTLS)
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}

	return cmd
}

func newCreateDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Create a differential OCP related container image mirror updates",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()
			logrus.Infoln("Create Diff called")
			err := create.CreateDiff(rootOpts.configPath, rootOpts.dir)
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}
}
