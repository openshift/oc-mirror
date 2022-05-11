package main

import (
	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func main() {
	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())
}

func checkErr(err error) {
	if err != nil {
		kcmdutil.CheckErr(err)
	}
}
