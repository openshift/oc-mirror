package main

import "syscall"

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Kill children if parent is dead
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
}
