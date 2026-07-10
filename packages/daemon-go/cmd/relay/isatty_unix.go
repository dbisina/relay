//go:build !windows

package main

import "golang.org/x/sys/unix"

func isatty() bool {
	_, err := unix.IoctlGetTermios(0, ioctlReadTermios)
	return err == nil
}
