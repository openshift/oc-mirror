package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/RedHatGov/bundle/pkg/bundle/create"
)

type createOpts struct {
	*rootOpts

	outputDir  string
	configPath string
}

func newCreateCmd(ro *rootOpts) *cobra.Command {

	opts := createOpts{
		rootOpts: ro,
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

	cmd.PersistentFlags().StringVarP(&opts.configPath, "config", "c", "imageset-config.yaml", "Path to imageset configuration file")
	cmd.PersistentFlags().StringVarP(&opts.outputDir, "output", "o", ".", "output directory for archives")

	return cmd
}

func newCreateFullCmd(o *createOpts) *cobra.Command {

	return &cobra.Command{
		Use:   "full",
		Short: "Create a full OCP related container image mirror",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			cleanup := setupFileHook(o.dir)
			defer cleanup()
			logrus.Infoln("Create full called")

			err := create.CreateFull(cmd.Context(), o.configPath, o.dir, o.outputDir, o.dryRun, o.skipTLS, o.skipCleanup)
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}
}

func newCreateDiffCmd(o *createOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Create a differential OCP related container image mirror updates",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			cleanup := setupFileHook(o.dir)
			defer cleanup()
			logrus.Infoln("Create Diff called")

			err := create.CreateDiff(cmd.Context(), o.configPath, o.dir, o.outputDir, o.dryRun, o.skipTLS, o.skipCleanup)
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}
}
