//go:build !linux

package main

import "syscall"

// sysProcAttr creates a non-linux compatible SysProcAttr, primarily removing the use of
// Pdeathsig which is only available on Linux. We Setpgid as true to tie the child process
// to the parent process as a workaround.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
