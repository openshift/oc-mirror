package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"slices"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli"
	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func main() {
	if err := RunOcMirrorV2(); err != nil {
		exitCode := exitCodeFromError(err)
		os.Exit(exitCode)
	}
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
				log.Error("%s", memProfErr.Error())
			}
		}
	}
	if err != nil {
		log.Error("[Executor] %v ", err)
	}

	return err
}

func cpuProf() (*os.File, error) {
	var cpuProfileFile *os.File
	var err error

	cpuProfileFile, err = os.Create("cpu.prof")
	if err != nil {
		return nil, fmt.Errorf("failed to create cpu.prof file: %w", err)
	}

	if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
		return cpuProfileFile, fmt.Errorf("failed to start cpu profiling: %w", err)
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
		return fmt.Errorf("failed to create mem.prof file: %w", err)
	}
	defer memProfileFile.Close()

	if err := pprof.WriteHeapProfile(memProfileFile); err != nil {
		return fmt.Errorf("failed to write mem profile: %w", err)
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
