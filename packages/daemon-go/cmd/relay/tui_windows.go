//go:build windows

package main

import "syscall"

func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
}
