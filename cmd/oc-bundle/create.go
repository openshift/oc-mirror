package main

import (
	"github.com/RedHatGov/bundle/pkg/bundle/create"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type createOpts struct {
	segSize    int64
	configPath string
	outputDir  string
}

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
	opts := createOpts{}
	cmd := &cobra.Command{
		Use:   "full",
		Short: "Create a full OCP related container image mirror",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()
			logrus.Infoln("Create full called")

			// Convert size to bytes
			segSizeBytes := opts.segSize * 1024 * 1024
			err := create.CreateFull(opts.configPath, opts.outputDir, rootOpts.dir, segSizeBytes)
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}

	f := cmd.Flags()
	//TODO convert to bytes with input + suffix
	f.Int64VarP(&opts.segSize, "archive-size", "s", 1000, "Size of each segemented archive in MB")
	f.StringVarP(&opts.configPath, "config", "c", "imageset-config.yaml", "Path to imageset configuration file")
	f.StringVarP(&opts.outputDir, "output", "o", ".", "Directory to output archived bundles")

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
			/*
				err := bundle.CreateDiff(rootOpts.dir)
				if err != nil {
					logrus.Fatal(err)
				}
			*/
		},
	}
}
