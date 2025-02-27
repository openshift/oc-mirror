package main

import (
	"os"
	"runtime/pprof"
	"slices"

	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func main() {
	cpuProfArg := slices.Contains(os.Args, "--cpu-prof")
	memProfArg := slices.Contains(os.Args, "--mem-prof")

	var cpuProfileFile, memProfileFile *os.File
	var err error

	if cpuProfArg {
		cpuProfileFile, err = cpuProf()
		defer cpuProfileFile.Close()
		defer pprof.StopCPUProfile()
		if err != nil {
			os.Exit(1)
		}
	}

	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())

	if memProfArg {
		memProfileFile, err = memProf()
		defer memProfileFile.Close()
		if err != nil {
			os.Exit(1)
		}
	}
}

func checkErr(err error) {
	if err != nil {
		kcmdutil.CheckErr(err)
	}
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

func memProf() (*os.File, error) {
	var memProfileFile *os.File
	var err error

	memProfileFile, err = os.Create("mem.prof")
	if err != nil {
		return nil, err
	}

	if err := pprof.WriteHeapProfile(memProfileFile); err != nil {
		return memProfileFile, err
	}

	return memProfileFile, nil
}
