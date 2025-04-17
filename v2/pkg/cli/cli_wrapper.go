package cli

import (
	"os"
	"runtime/pprof"
	"slices"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli"
	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/spf13/cobra"
)

/*
V2Cmd is exposed only temporary and will be removed in the future, please do not use it.
This func exists only to redirect the flow to the v2 version of the cli while v1 is still supported
*/
func V2Cmd(loglevel string) *cobra.Command {
	log := clog.New(loglevel)
	return cli.NewMirrorCmd(log)
}

func RunOcMirrorV2() error {
	cpuProfArg := slices.Contains(os.Args, "--cpu-prof")
	memProfArg := slices.Contains(os.Args, "--mem-prof")

	var cpuProfileFile *os.File
	if cpuProfArg {
		var err error
		cpuProfileFile, err = cpuProf()
		if err != nil {
			stopCloseCpuProf(cpuProfileFile)
			return err
		}
	}

	// Setup pluggable logger. Feel free to plugin you own logger just use the
	// PluggableLoggerInterface in the file pkg/log/logger.go
	log := clog.New("info")
	rootCmd := cli.NewMirrorCmd(log)
	err := rootCmd.Execute()
	if cpuProfArg {
		stopCloseCpuProf(cpuProfileFile)
	}

	if memProfArg {
		if memProfErr := memProf(); memProfErr != nil {
			if err == nil {
				err = memProfErr
			} else {
				log.Error("write memory profile: %s", memProfErr.Error())
			}
		}
	}
	if err != nil {
		log.Error("[Executor] %v ", err)
		exitCode := exitCodeFromError(err)
		os.Exit(exitCode)
	}

	return nil
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(cli.CodeExiter); ok {
		return e.ExitCode()
	}
	return errcode.GenericErr
}

func cpuProf() (*os.File, error) {
	var cpuProfileFile *os.File
	var err error

	cpuProfileFile, err = os.Create("cpu.prof")
	if err != nil {
		return nil, err
	}

	if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
		return cpuProfileFile, err
	}

	return cpuProfileFile, nil
}

func stopCloseCpuProf(cpuProfileFile *os.File) {
	pprof.StopCPUProfile()
	cpuProfileFile.Close()
}

func memProf() error {
	memProfileFile, err := os.Create("mem.prof")
	if err != nil {
		return err
	}
	defer memProfileFile.Close()

	return pprof.WriteHeapProfile(memProfileFile)
}
