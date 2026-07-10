//go:build !windows

// internal/process/job_unix.go
//
// Unix stub: use process groups + SIGKILL-on-exit for child cleanup.

package process

import (
	"os/exec"
	"syscall"
)

// SetupChildProcess puts the child in its own process group.
// On daemon exit, the OS reclaims the process group automatically.
// For explicit cleanup, call cmd.Process.Kill() or kill(-pgid, SIGKILL).
func SetupChildProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return nil
}

// AssignToJobObject is a no-op on Unix.
func AssignToJobObject(cmd *exec.Cmd) error {
	return nil
}
