package main

import (
	"github.com/containers/buildah"
	"github.com/containers/storage/pkg/unshare"
	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func main() {
	if buildah.InitReexec() {
		return
	}
	unshare.MaybeReexecUsingUserNamespace(false)

	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())
}

func checkErr(err error) {
	if err != nil {
		kcmdutil.CheckErr(err)
	}
}
