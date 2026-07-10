// internal/fsm/machine.go
//
// HandoffMachine — 7-state FSM with atomic disk persistence.
//
// State file: .relay/session.json
// Atomic write: write temp → fsync → rename (same NTFS/ext4 volume).
// Idempotency: ENVELOPE_BUILT checks SHA-256 of built envelope before rebuild.

package fsm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionRecord is the persisted FSM state.
type SessionRecord struct {
	SessionID    string    `json:"sessionId"`
	TaskID       string    `json:"taskId"`
	State        FsmState  `json:"state"`
	Provider     string    `json:"provider"`
	EnvelopeHash string    `json:"envelopeHash,omitempty"` // for ENVELOPE_BUILT idempotency
	EnvelopePath string    `json:"envelopePath,omitempty"`
	CommitSHA    string    `json:"commitSha,omitempty"`
	ErrorMsg     string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// HandoffMachine manages FSM state transitions with atomic disk persistence.
type HandoffMachine struct {
	mu       sync.Mutex
	record   SessionRecord
	stateDir string // e.g. .relay/
}

// NewHandoffMachine loads or initialises the FSM from stateDir.
func NewHandoffMachine(stateDir, sessionID, taskID, provider string) (*HandoffMachine, error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("fsm: mkdir %s: %w", stateDir, err)
	}
	m := &HandoffMachine{stateDir: stateDir}

	path := m.statePath()
	data, err := os.ReadFile(path)
	if err == nil {
		if jerr := json.Unmarshal(data, &m.record); jerr != nil {
			return nil, fmt.Errorf("fsm: parse state file: %w", jerr)
		}
		return m, nil
	}

	// Fresh session
	m.record = SessionRecord{
		SessionID: sessionID,
		TaskID:    taskID,
		State:     StateRunning,
		Provider:  provider,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := m.persist(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *HandoffMachine) statePath() string {
	return filepath.Join(m.stateDir, "session.json")
}

// State returns the current FSM state (lock-free read for logging).
func (m *HandoffMachine) State() FsmState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.record.State
}

// Record returns a copy of the current session record.
func (m *HandoffMachine) Record() SessionRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := m.record
	return cp
}

// Transition validates and applies a state transition, then persists.
func (m *HandoffMachine) Transition(to FsmState, mutate func(*SessionRecord)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.record.State
	if !CanTransition(from, to) {
		return fmt.Errorf("fsm: invalid transition %s → %s", from, to)
	}

	m.record.State = to
	m.record.UpdatedAt = time.Now().UTC()
	if mutate != nil {
		mutate(&m.record)
	}

	return m.persist()
}

// TransitionError moves to ERROR with a message, regardless of current state.
func (m *HandoffMachine) TransitionError(reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record.State = StateError
	m.record.ErrorMsg = reason
	m.record.UpdatedAt = time.Now().UTC()
	return m.persist()
}

// ExecuteSnapshot transitions RUNNING→PAUSING→SNAPSHOTTED.
// commit is the git commit SHA after snapshotting.
func (m *HandoffMachine) ExecuteSnapshot(commitSHA string) error {
	if err := m.Transition(StatePausing, nil); err != nil {
		return err
	}
	return m.Transition(StateSnapshotted, func(r *SessionRecord) {
		r.CommitSHA = commitSHA
	})
}

// ExecuteEnvelopeBuild transitions SNAPSHOTTED→ENVELOPE_BUILT.
// Idempotent: if the file at envelopePath already has the expected hash, skips.
func (m *HandoffMachine) ExecuteEnvelopeBuild(envelopePath string) error {
	m.mu.Lock()
	expectedHash := m.record.EnvelopeHash
	m.mu.Unlock()

	if expectedHash != "" {
		// Verify existing envelope
		hash, err := sha256File(envelopePath)
		if err == nil && hash == expectedHash {
			// Already built — idempotent skip
			return nil
		}
	}

	hash, err := sha256File(envelopePath)
	if err != nil {
		return fmt.Errorf("fsm: hash envelope: %w", err)
	}

	return m.Transition(StateEnvBuilt, func(r *SessionRecord) {
		r.EnvelopePath = envelopePath
		r.EnvelopeHash = hash
	})
}

// ExecuteDispatch transitions ENVELOPE_BUILT→DISPATCHED.
func (m *HandoffMachine) ExecuteDispatch() error {
	return m.Transition(StateDispatched, nil)
}

// ExecuteResume transitions DISPATCHED→RESUMING→RUNNING.
func (m *HandoffMachine) ExecuteResume(newProvider string) error {
	if err := m.Transition(StateResuming, func(r *SessionRecord) {
		r.Provider = newProvider
	}); err != nil {
		return err
	}
	return m.Transition(StateRunning, nil)
}

// ExecuteEmergencySnapshot transitions directly to SNAPSHOTTED from any
// non-terminal state, used when the 60s safe-pause timeout fires.
func (m *HandoffMachine) ExecuteEmergencySnapshot(branchName, commitSHA string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Allow from RUNNING or PAUSING (timeout may fire after PAUSING entered)
	switch m.record.State {
	case StateRunning, StatePausing:
		// ok
	default:
		return fmt.Errorf("fsm: emergency snapshot not allowed from state %s", m.record.State)
	}

	m.record.State = StateSnapshotted
	m.record.CommitSHA = commitSHA
	m.record.EnvelopeHash = "" // will be rebuilt
	m.record.UpdatedAt = time.Now().UTC()
	return m.persist()
}

// persist writes the state record atomically: tmp → fsync → rename.
// Caller must hold m.mu.
func (m *HandoffMachine) persist() error {
	data, err := json.MarshalIndent(m.record, "", "  ")
	if err != nil {
		return fmt.Errorf("fsm: marshal state: %w", err)
	}

	target := m.statePath()
	tmp := target + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("fsm: open tmp: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsm: write tmp: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsm: fsync tmp: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("fsm: close tmp: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("fsm: rename tmp→state: %w", err)
	}

	return nil
}

// sha256File returns the lowercase hex SHA-256 of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
