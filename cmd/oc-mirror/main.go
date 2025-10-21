package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"syscall"

	"k8s.io/klog"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli"
	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

//go:embed data/*
var mirrorV1 embed.FS

//nolint:cyclop // some of this will go away when v1 is removed
func main() {
	useV1 := slices.Contains(os.Args, "--v1")
	useV2 := slices.Contains(os.Args, "--v2")
	switch {
	case useV1 && useV2:
		klog.Fatal("the flags --v1 and --v2 are mutually exclusive")
	case useV1:
		if err := runOcMirrorV1(os.Args); err != nil {
			klog.Errorf("failed to run oc-mirror: %s", err.Error())
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			os.Exit(1)
		}
	// Do not complain about version flag in case of `oc-mirror version`
	case slices.Contains(os.Args, "version"):
		fallthrough
	case useV2:
		if err := runOcMirrorV2(); err != nil {
			exitCode := exitCodeFromError(err)
			os.Exit(exitCode)
		}
	default:
		if slices.Contains(os.Args, "list") || slices.Contains(os.Args, "describe") || slices.Contains(os.Args, "init") {
			klog.Error("⚠️  the sub-commands `list`, `describe` and `init` are only implemented in oc-mirror v1, so please use --v1 for these sub-commands until some replacement is provided")
		}
		klog.Fatal("⚠️  the use of the flag --v1 or --v2 is mandatory, please use --v2 for the supported oc-mirror version or --v1 to continue using the deprecated version")
	}
}

func runOcMirrorV1(args []string) error {
	tmpdir, err := os.MkdirTemp("", "oc-mirror-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	path := filepath.Join(tmpdir, "oc-mirror")
	klog.V(5).Infof("Unpacking v2 binary to %s", path)

	binaryV1, err := mirrorV1.ReadFile("data/oc-mirror-v1")
	if err != nil {
		return fmt.Errorf("failed to read v1 binary: %w", err)
	}

	v1File, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create v1 binary: %w", err)
	}
	if _, err := io.Copy(v1File, bytes.NewReader(binaryV1)); err != nil {
		v1File.Close()
		return fmt.Errorf("failed to write v1 binary: %w", err)
	}
	// We must close the binary file before we can execute it
	v1File.Close()

	cmd := exec.Cmd{
		Path: path,
		SysProcAttr: &syscall.SysProcAttr{
			// Kill children if parent is dead
			Pdeathsig: syscall.SIGKILL,
			Setpgid:   true,
		},
		Args:   args,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	err = cmd.Run()
	if err != nil && strings.Contains(err.Error(), "permission denied") {
		tmpdir, ok := os.LookupEnv("TMPDIR")
		if !ok {
			tmpdir = "/tmp"
		}
		klog.Errorf("The tmp dir %q might be mounted as `noexec`. Please set TMPDIR to a filesystem with exec permissions.", tmpdir)
	}
	//nolint:wrapcheck // we don't need nor want to wrap this error
	return err
}

func runOcMirrorV2() error {
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
