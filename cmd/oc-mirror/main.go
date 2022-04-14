package main

import (
	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	"k8s.io/klog/v2"
)

func main() {
	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())
}

func checkErr(err error) {
	if err != nil {
		klog.Fatal(err)
	}
}
