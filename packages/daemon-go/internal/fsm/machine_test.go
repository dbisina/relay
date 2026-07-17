// internal/fsm/machine_test.go — HandoffMachine persistence: stale-record
// adoption guard, Clear on clean exit, transition validity.

package fsm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRecord(t *testing.T, dir string, rec SessionRecord) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFreshMachineStartsRunning(t *testing.T) {
	dir := t.TempDir()
	m, err := NewHandoffMachine(dir, "s1", "t1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	rec := m.Record()
	if rec.State != StateRunning || rec.SessionID != "s1" || rec.Provider != "claude" {
		t.Errorf("fresh record wrong: %+v", rec)
	}
}

func TestAdoptsOwnNonTerminalRecord(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, SessionRecord{
		SessionID: "s1", TaskID: "t1", State: StateDispatched,
		Provider: "claude", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	m, err := NewHandoffMachine(dir, "s1", "t1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if m.State() != StateDispatched {
		t.Errorf("expected to adopt DISPATCHED record, got %s", m.State())
	}
}

func TestDiscardsTerminalRecord(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, SessionRecord{
		SessionID: "s1", TaskID: "t1", State: StateError, ErrorMsg: "boom",
		Provider: "claude", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	// Same session id, but the record is terminal: must start fresh, otherwise
	// the first handoff fails with "invalid transition ERROR → PAUSING".
	m, err := NewHandoffMachine(dir, "s1", "t1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if m.State() != StateRunning {
		t.Errorf("expected fresh RUNNING after terminal record, got %s", m.State())
	}
	if err := m.Transition(StatePausing, nil); err != nil {
		t.Errorf("transition after discard should work: %v", err)
	}
}

func TestDiscardsForeignSessionRecord(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, SessionRecord{
		SessionID: "old-session", TaskID: "t0", State: StateRunning,
		Provider: "codex", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	m, err := NewHandoffMachine(dir, "s2", "t2", "claude")
	if err != nil {
		t.Fatal(err)
	}
	rec := m.Record()
	if rec.SessionID != "s2" || rec.Provider != "claude" {
		t.Errorf("stale foreign record leaked into new session: %+v", rec)
	}
}

func TestClearRemovesStateFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewHandoffMachine(dir, "s1", "t1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session.json")); !os.IsNotExist(err) {
		t.Error("session.json should be gone after Clear")
	}
	// Clear on an already-missing file is not an error.
	if err := m.Clear(); err != nil {
		t.Errorf("second Clear should be a no-op: %v", err)
	}
}

func TestInvalidTransitionRejected(t *testing.T) {
	dir := t.TempDir()
	m, err := NewHandoffMachine(dir, "s1", "t1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Transition(StateDispatched, nil); err == nil {
		t.Error("RUNNING → DISPATCHED should be invalid")
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(StateError) {
		t.Error("ERROR must be terminal")
	}
	for _, s := range []FsmState{StateRunning, StatePausing, StateSnapshotted, StateEnvBuilt, StateDispatched, StateResuming} {
		if IsTerminal(s) {
			t.Errorf("%s must not be terminal", s)
		}
	}
}
