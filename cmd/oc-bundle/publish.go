package main

import (
	bundle "github.com/RedHatGov/bundle/pkg/bundle/publish"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish OCP related content to an internet-disconnected environment",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			cleanup := setupFileHook(rootOpts.dir)
			defer cleanup()
			logrus.Infoln("Publish Was Called")
			err := bundle.Publish(rootOpts.dir)
			if err != nil {
				logrus.Fatal(err)
			}
		},
	}
	cmd.PersistentFlags().StringVar(&bundle.PublishOpts.FromBundle, "from-bundle", "", "The bundle archive filename.")
	cmd.PersistentFlags().StringVar(&bundle.PublishOpts.ToMirror, "to-mirror", "", "The URL to the destination mirror registry")

	return cmd
}
