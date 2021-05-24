package main

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/bundle"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	rootOpts struct {
		dir      string
		logLevel string
	}
)

func main() {
	flags := pflag.NewFlagSet("oc-bundle", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := NewCmdBundle(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func NewCmdBundle(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage OCP release, operators, and additional container images for internet-disconnected systems",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBundleCreateCmd())
	cmd.AddCommand(newBundlePublishCmd())

	return cmd
}

func newBundleCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create image mirror bundles of OCP related resources",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBundleCreateFullCmd())
	cmd.AddCommand(newBundleCreateDiffCmd())
	return cmd
}

func newBundlePublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish",
		Short: "Publish OCP related content to an internet-disconnected environment",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()

			err := bundle.Publish(rootOpts.dir)
			if err != nil {
				logrus.Fatal(err)
			}
		},
	}
}

func newBundleCreateFullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "full",
		Short: "Create a full OCP related container image mirror",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()
			err := bundle.CreateFull(rootOpts.dir)
			if err != nil {
				logrus.Fatal(err)
			}
		},
	}
}

func newBundleCreateDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bundle",
		Short: "Create local OCP release bundle",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()

			err := bundle.CreateDiff(rootOpts.dir)
			if err != nil {
				logrus.Fatal(err)
			}
		},
	}
}
