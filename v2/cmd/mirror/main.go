package main

import (
	"os"

	cli "github.com/openshift/oc-mirror/v2/pkg/cli"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
)

func main() {

	// setup pluggable logger
	// feel free to plugin you own logger
	// just use the PluggableLoggerInterface
	// in the file pkg/log/logger.go

	log := clog.New("info")

	rootCmd := cli.NewMirrorCmd(log)
	err := rootCmd.Execute()
	if err != nil {
		log.Error("[Executor] %v ", err)
		os.Exit(1)
	}
}
