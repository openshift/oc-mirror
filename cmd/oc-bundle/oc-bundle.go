package main

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("oc-bundle", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := bundle.NewCmdBundle(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
