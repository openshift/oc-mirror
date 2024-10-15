package main

import (
	"fmt"
	"github.com/containers/buildah"
	"github.com/containers/storage/pkg/unshare"
	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"os"
)

func main() {
	if testE2E := os.Getenv("TEST_E2E"); len(testE2E) != 0 {
		fmt.Println("envar TEST_E2E detected - bypassing unshare")
	} else {
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
