//go:build windows

// internal/process/job_windows.go
//
// Windows Job Object supervisor.
// Every child process gets assigned to a Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE. When the Relay daemon process exits
// (crash, kill, or clean shutdown), the OS closes the handle → all agents die.
// No orphaned CLI processes. This is the primary reason the daemon is in Go.

package process

import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	globalJob     windows.Handle
	globalJobOnce sync.Once
	globalJobErr  error
)

// initGlobalJob creates the daemon-lifetime Job Object once.
func initGlobalJob() {
	globalJob, globalJobErr = windows.CreateJobObject(nil, nil)
	if globalJobErr != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, globalJobErr = windows.SetInformationJobObject(
		globalJob,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if globalJobErr != nil {
		windows.CloseHandle(globalJob)
		globalJob = 0
	}
}

// SetupChildProcess configures SysProcAttr so the child starts in a new
// process group (prevents Ctrl-C propagation) and can be assigned to the
// Job Object after Start().
func SetupChildProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &windows.SysProcAttr{}
	}
	// New process group: Ctrl-C in the terminal does not kill the agent.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	return nil
}

// AssignToJobObject must be called immediately after cmd.Start().
// It attaches the new process to the global kill-on-close Job Object.
func AssignToJobObject(cmd *exec.Cmd) error {
	globalJobOnce.Do(initGlobalJob)
	if globalJobErr != nil {
		// Degrade gracefully: no Job Object, but warn.
		return fmt.Errorf("job object init failed (non-fatal): %w", globalJobErr)
	}
	if globalJob == 0 {
		return nil
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return fmt.Errorf("OpenProcess pid=%d: %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(handle)

	if err := windows.AssignProcessToJobObject(globalJob, handle); err != nil {
		// ERROR_ACCESS_DENIED: process is already in a job (common in CI).
		// Not fatal — skip silently.
		if err == windows.ERROR_ACCESS_DENIED {
			return nil
		}
		return fmt.Errorf("AssignProcessToJobObject pid=%d: %w", cmd.Process.Pid, err)
	}
	return nil
}
