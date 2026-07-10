//go:build linux

package main

import "golang.org/x/sys/unix"

// ioctlReadTermios is the ioctl request that reads the terminal attributes.
const ioctlReadTermios = unix.TCGETS
