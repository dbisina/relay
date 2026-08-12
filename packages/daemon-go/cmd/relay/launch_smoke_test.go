package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestInteractiveLaunchSmoke spawns a real terminal window via the production
// openInTerminalIn and asserts it ran the command in the requested working
// directory (proving the interactive-adopt `cd` works) with the requested env
// exported to the child (proving CLAUDE_CONFIG_DIR-style account switching
// reaches the launched CLI).
//
// Opt-in: it opens a GUI terminal, so it is skipped unless RELAY_MANUAL_LAUNCH
// is set. Run with:
//
//	RELAY_MANUAL_LAUNCH=1 go test ./cmd/relay/ -run TestInteractiveLaunchSmoke -v
//
// The spawned window stays open (the production launcher holds it with `pause`
// so a real adopted session is inspectable); this test leaves it for you to see
// and close. It uses its own temp dir with best-effort cleanup rather than
// t.TempDir, whose auto-RemoveAll would race that still-open window.
func TestInteractiveLaunchSmoke(t *testing.T) {
	if os.Getenv("RELAY_MANUAL_LAUNCH") == "" {
		t.Skip("set RELAY_MANUAL_LAUNCH=1 to run (opens a real terminal window)")
	}
	dir, err := os.MkdirTemp("", "relay-smoke-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	// Best-effort: the open window may hold dir until the user closes it, so a
	// failed RemoveAll is not a test failure.
	defer func() { _ = os.RemoveAll(dir) }()

	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		// echo writes the cwd marker; `set RELAY_MARK` lists the child's live env
		// (inherited, not %-expanded), so it proves the env crossed the process
		// boundary the same way CLAUDE_CONFIG_DIR does in production.
		cmd = "cmd"
		args = []string{"/c", "echo ok>relay_ok.txt & set RELAY_MARK>relay_env.txt"}
	default:
		cmd = "bash"
		args = []string{"-c", "echo ok > relay_ok.txt; echo RELAY_MARK=$RELAY_MARK > relay_env.txt"}
	}

	if err := openInTerminalIn(dir, cmd, args, []string{"RELAY_MARK=hello"}, "relay-smoke"); err != nil {
		t.Fatalf("openInTerminalIn: %v", err)
	}

	okPath := filepath.Join(dir, "relay_ok.txt")
	envPath := filepath.Join(dir, "relay_env.txt")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, okErr := os.Stat(okPath)
		_, envErr := os.Stat(envPath)
		if okErr == nil && envErr == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if _, err := os.Stat(okPath); err != nil {
		t.Fatalf("cwd marker not written under %s — the terminal did not cd into the workDir", dir)
	}
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("env marker missing: %v", err)
	}
	if !strings.Contains(string(env), "hello") {
		t.Fatalf("env not exported to the launched process: got %q, want it to contain RELAY_MARK=hello", string(env))
	}
	t.Logf("OK: terminal ran in %s with env exported (%s)", dir, strings.TrimSpace(string(env)))
}
