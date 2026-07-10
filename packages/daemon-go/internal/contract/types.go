// internal/contract/types.go

package contract

import "time"

// ContinuationContract is the Markdown-first signed handoff artifact.
// Primacy: DO NOT REDO section appears first when serialised.
type ContinuationContract struct {
	// Identity
	ContractID string    `json:"contractId"`
	SessionID  string    `json:"sessionId"`
	TaskID     string    `json:"taskId"`
	CreatedAt  time.Time `json:"createdAt"`
	Version    int       `json:"version"` // schema version, currently 2

	// Continuity
	TaskGoal   string   `json:"taskGoal"`
	NextAction string   `json:"nextAction"`
	DoNotRedo  []string `json:"doNotRedo"` // primacy items — never repeat these

	// Rich session intent (v2). Captured live by the orchestrator or lifted from
	// a detected agent's transcript (internal/detect). These give the receiving
	// agent the full picture — what was asked, the plan, what is left, which
	// skills were in play, and the code that was mid-edit — not just the next step.
	InitialPrompt  string         `json:"initialPrompt,omitempty"`  // the original ask, verbatim
	Plan           []string       `json:"plan,omitempty"`           // full plan as the source agent tracked it
	TasksRemaining []string       `json:"tasksRemaining,omitempty"` // plan items not yet complete
	SkillsLoaded   []string       `json:"skillsLoaded,omitempty"`   // skills available to the session
	SkillsInUse    []string       `json:"skillsInUse,omitempty"`    // skills actively invoked
	SkillsToUse    []string       `json:"skillsToUse,omitempty"`    // skills the plan still calls for
	InFlightCode   []InFlightFile `json:"inFlightCode,omitempty"`   // files mid-edit + truncated snippets

	// Acceptance
	AcceptanceAssertions []string `json:"acceptanceAssertions"` // must all pass to mark task done

	// File context
	FileManifest []FileEntry `json:"fileManifest"` // key files + their last-modified SHA

	// Decisions & constraints
	Decisions   []Decision   `json:"decisions"`
	Constraints []Constraint `json:"constraints"`

	// Commit anchors
	BaseCommitSHA     string `json:"baseCommitSha"`
	SnapshotCommitSHA string `json:"snapshotCommitSha"`

	// Signing (HMAC-SHA256)
	Signature string `json:"signature,omitempty"` // hex; empty before signing
}

// FileEntry records a file path and its content SHA at snapshot time.
type FileEntry struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Modified bool   `json:"modified"` // true if changed since BaseCommitSHA
}

// InFlightFile is a file that was mid-edit, with a truncated snippet of the
// in-progress code so the receiving agent can pick up the exact thread.
type InFlightFile struct {
	Path    string `json:"path"`
	Snippet string `json:"snippet,omitempty"` // truncated; never a full file
}

// Decision records an architectural or implementation decision made this session.
type Decision struct {
	Summary   string `json:"summary"`
	Rationale string `json:"rationale,omitempty"`
}

// Constraint is a hard rule the receiving agent must not violate.
type Constraint struct {
	Rule   string `json:"rule"`
	Source string `json:"source,omitempty"` // e.g. "PRD §SEC-3"
}
