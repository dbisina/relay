//go:build !windows

// internal/detect/process_other.go — enumerate processes via `ps` on
// macOS/Linux. `command` gives the full argv, which is what the signature
// regexes match against.

package detect

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type procInfo struct {
	PID     int
	Name    string
	Cmdline string
	Cwd     string // working dir; unset (ps does not expose it)
}

// listProcesses returns running processes. Best-effort.
func listProcesses() ([]procInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	// pid + full command, no header. -ww disables column-width truncation.
	cmd := exec.CommandContext(ctx, "ps", "-axww", "-o", "pid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var procs []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp <= 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sp])
		if err != nil {
			continue
		}
		cmdline := strings.TrimSpace(line[sp+1:])
		name := cmdline
		if first := strings.IndexByte(cmdline, ' '); first > 0 {
			name = cmdline[:first]
		}
		procs = append(procs, procInfo{PID: pid, Name: filepath.Base(name), Cmdline: cmdline})
	}
	return procs, nil
}
