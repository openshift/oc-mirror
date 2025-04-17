package main

import (
	"os"
	"slices"

	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

	cliV1 "github.com/openshift/oc-mirror/pkg/cli/mirror"
	cliV2 "github.com/openshift/oc-mirror/v2/pkg/cli"
)

func main() {
	if slices.Contains(os.Args, "--v2") {
		err := cliV2.RunOcMirrorV2()
		kcmdutil.CheckErr(err)
	} else {
		rootCmd := cliV1.NewMirrorCmd()
		kcmdutil.CheckErr(rootCmd.Execute())
	}
}
