package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	_ "embed"

	"github.com/openshift/oc-mirror/pkg/cli/mirror"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

//go:embed oc-mirror-v*
var binaryV2 []byte

func main() {
	if isV2() {
		if err := runOcMirrorV2Cmd(os.Args[1:]); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
		}
		return
	}
	rootCmd := mirror.NewMirrorCmd()
	checkErr(rootCmd.Execute())
}

func checkErr(err error) {
	if err != nil {
		kcmdutil.CheckErr(err)
	}
}

func isV2() bool {
	return len(os.Args) > 0 && slices.Contains(os.Args[:], "--v2")
}

func runOcMirrorV2Cmd(args []string) error {
	tmpdir, err := os.MkdirTemp("", "oc-mirror")
	if err != nil {
		return fmt.Errorf("failed to create tmpdir: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	path := filepath.Join(tmpdir, "oc-mirror")
	klog.V(5).Infof("Unpacking v2 binary to %s", path)
	v2File, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
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
