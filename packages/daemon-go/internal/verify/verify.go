// internal/verify/verify.go — the verifier gate.
//
// Between handoffs and after pipeline nodes, Relay can run a set of acceptance
// checks (shell commands such as `go build ./...` or `go test ./...`). If any
// fail, the work is not accepted — the orchestrator retries on a fallback or
// surfaces the failure rather than declaring a broken state "done". This turns a
// linear chain into a self-correcting loop.

package verify

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dbisina/relay/internal/process"
)

// Check is the result of running one acceptance command.
type Check struct {
	Cmd    string `json:"cmd"`
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"` // combined stdout+stderr, truncated
}

// Result aggregates all checks.
type Result struct {
	Checks    []Check `json:"checks"`
	AllPassed bool    `json:"allPassed"`
}

// Failed returns the commands that did not pass.
func (r Result) Failed() []string {
	var out []string
	for _, c := range r.Checks {
		if !c.OK {
			out = append(out, c.Cmd)
		}
	}
	return out
}

// Executor runs a single command in workDir and reports its combined output and
// whether it succeeded (exit 0). Injectable so the gate logic is unit-testable
// without spawning real shells.
type Executor func(cmd string) (output string, ok bool)

// RunWith evaluates cmds with a supplied executor. Empty cmds → AllPassed true
// (nothing to check is not a failure). This is the pure, testable core.
func RunWith(cmds []string, exec Executor) Result {
	res := Result{AllPassed: true}
	for _, cmd := range cmds {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		out, ok := exec(cmd)
		if !ok {
			res.AllPassed = false
		}
		res.Checks = append(res.Checks, Check{Cmd: cmd, OK: ok, Output: truncate(out, 2000)})
	}
	return res
}

// Run evaluates cmds as real shell commands in workDir. Each command runs under
// the OS shell so pipes/operators work; a non-zero exit (or spawn failure) is a
// failed check.
func Run(ctx context.Context, workDir string, cmds []string) Result {
	return RunWith(cmds, func(cmd string) (string, bool) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(cctx, "cmd", "/C", cmd)
		} else {
			c = exec.CommandContext(cctx, "sh", "-c", cmd)
		}
		c.Dir = workDir
		process.HideWindow(c)
		var buf bytes.Buffer
		c.Stdout = &buf
		c.Stderr = &buf
		err := c.Run()
		return buf.String(), err == nil
	})
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
