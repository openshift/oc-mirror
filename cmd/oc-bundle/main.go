package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containers/buildah"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/RedHatGov/bundle/pkg/cli"
)

func main() {

	// Rootless buildah
	if buildah.InitReexec() {
		return
	}

	// This attempts to configure klog (used by vendored Kubernetes code) not
	// to log anything.
	// Handle k8s.io/klog
	var fsv1 flag.FlagSet
	klogv1.InitFlags(&fsv1)
	checkErr(fsv1.Set("stderrthreshold", "4"))
	klogv1.SetOutput(ioutil.Discard)
	// Handle k8s.io/klog/v2
	var fsv2 flag.FlagSet
	klogv2.InitFlags(&fsv2)
	checkErr(fsv2.Set("stderrthreshold", "4"))
	klogv2.SetOutput(ioutil.Discard)

	rootCmd := newRootCmd()
	checkErr(rootCmd.Execute())
}

func newRootCmd() *cobra.Command {
	opts := cli.RootOptions{
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}

	cmd := &cobra.Command{
		Use:               filepath.Base(os.Args[0]),
		Short:             "Manages container images for internet-disconnected systems",
		Long:              "TODO",
		PersistentPreRun:  opts.LogfilePreRun,
		PersistentPostRun: opts.LogfilePostRun,
		SilenceErrors:     true,
		SilenceUsage:      true,
	}

	cmd.AddCommand(
		newCreateCmd(&opts),
		newPublishCmd(&opts),
	)

	opts.BindFlags(cmd.PersistentFlags())

	return cmd
}

func checkErr(err error) {
	if err != nil {
		logrus.Fatal(err)
	}
}
