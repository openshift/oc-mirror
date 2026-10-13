package main

import (
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

	cli "github.com/openshift/oc-mirror/pkg/cli/mirror"
)

func main() {
	rootCmd := cli.NewMirrorCmd()
	kcmdutil.CheckErr(rootCmd.Execute())
}
