package main

import (
	"os"

	cli "github.com/openshift/oc-mirror/v2/pkg/cli"
)

func main() {
	if err := cli.RunOcMirrorV2(); err != nil {
		os.Exit(1)
	}
}
