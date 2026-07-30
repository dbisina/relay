//go:build windows

// internal/detect/process_windows.go — enumerate processes with full command
// lines via CIM. tasklist cannot return the command line, which is exactly the
// field we need (agent CLIs run as `node …\cli.js`), so we query Win32_Process.

package detect

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/dbisina/relay/internal/process"
)

type procInfo struct {
	PID     int
	Name    string
	Cmdline string
	Cwd     string // working dir; unset on Windows (not exposed by Win32_Process)
}

// listProcesses returns running processes. Best-effort: on any failure the
// caller falls back to transcript-only detection.
func listProcesses() ([]procInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	// Force array output so a single-process result still parses as JSON array.
	const ps = `@(Get-CimInstance Win32_Process | Select-Object ProcessId,Name,CommandLine) | ConvertTo-Json -Compress -Depth 3`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	process.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var rows []struct {
		ProcessId   int    `json:"ProcessId"`
		Name        string `json:"Name"`
		CommandLine string `json:"CommandLine"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}

	procs := make([]procInfo, 0, len(rows))
	for _, r := range rows {
		procs = append(procs, procInfo{PID: r.ProcessId, Name: r.Name, Cmdline: r.CommandLine})
	}
	return procs, nil
}
