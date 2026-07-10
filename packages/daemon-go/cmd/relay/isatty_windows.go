//go:build windows

package main

import goIsatty "github.com/mattn/go-isatty"

func isatty() bool {
	return goIsatty.IsTerminal(0)
}
