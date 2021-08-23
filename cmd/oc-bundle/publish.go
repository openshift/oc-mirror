package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/RedHatGov/bundle/pkg/bundle/publish"
)

type publishOpts struct {
	*rootOpts

	archivePath string
	toMirror    string
}

func newPublishCmd(ro *rootOpts) *cobra.Command {
	opts := publishOpts{
		rootOpts: ro,
	}

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish OCP related content to an internet-disconnected environment",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, _ []string) {
			cleanup := setupFileHook(opts.dir)
			defer cleanup()

			if err := publish.Publish(cmd.Context(), opts.dir, opts.archivePath, opts.toMirror); err != nil {
				logrus.Fatal(err)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&opts.archivePath, "archive", "", "The archive file path.")
	cmd.PersistentFlags().StringVar(&opts.toMirror, "to-mirror", "", "The URL to the destination mirror registry")

	return cmd
}
