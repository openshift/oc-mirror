package main

import (
	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
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
