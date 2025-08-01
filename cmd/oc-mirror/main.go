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
	"slices"
	"strings"
	"syscall"

	"k8s.io/klog"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

	cliV1 "github.com/openshift/oc-mirror/pkg/cli/mirror"
)

//go:embed data/*
var mirrorV2 embed.FS

func main() {
	if slices.Contains(os.Args, "--v2") {
		if err := runOcMirrorV2(os.Args); err != nil {
			var exitErr *exec.ExitError
			if err != nil && errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Printf("failed to run oc-mirror: %s\n", err.Error())
			if strings.Contains(err.Error(), "permission denied") {
				tmpdir, ok := os.LookupEnv("TMPDIR")
				if !ok {
					tmpdir = "/tmp"
				}
				fmt.Printf("The tmp dir %q might be mounted as `noexec`. Please set TMPDIR to a filesystem with exec permissions.\n", tmpdir)
			}
			os.Exit(1)
		}
	} else {
		rootCmd := cliV1.NewMirrorCmd()
		kcmdutil.CheckErr(rootCmd.Execute())
	}
}

func runOcMirrorV2(args []string) error {
	tmpdir, err := os.MkdirTemp("", "oc-mirror-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	path := filepath.Join(tmpdir, "oc-mirror")
	klog.V(5).Infof("Unpacking v2 binary to %s", path)

	binaryV2, err := mirrorV2.ReadFile("data/oc-mirror-v2")
	if err != nil {
		return fmt.Errorf("failed to read v2 binary: %w", err)
	}

	v2File, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create v2 binary: %w", err)
	}
	if _, err := io.Copy(v2File, bytes.NewReader(binaryV2)); err != nil {
		v2File.Close()
		return fmt.Errorf("failed to write v2 binary: %w", err)
	}
	// We must close the binary file before we can execute it
	v2File.Close()

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

	return cmd.Run()
}
