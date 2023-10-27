package main

import (
	"os"

	"github.com/containers/buildah"
	"github.com/containers/storage/pkg/unshare"
	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	"golang.org/x/exp/slices"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func main() {
	// oc-mirror runs in rootless mode. It is therefore necessary to
	// ensure that oc-mirror is re-executed in a user namespace where
	// it has root privileges.
	if len(os.Args) > 0 && slices.Contains(os.Args[:], "--v2") {
		if buildah.InitReexec() {
			return
		}
		unshare.MaybeReexecUsingUserNamespace(false)
	}
	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())
}

func checkErr(err error) {
	if err != nil {
		kcmdutil.CheckErr(err)
	}
}
