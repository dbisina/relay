// internal/detect/detect.go — discover AI coding agents already running on
// this machine (that Relay did NOT spawn) and lift their session intent.
//
// Why this exists: the orchestrator only knows about agents it launches as
// child processes. Relay's headline promise is the opposite — notice the
// Claude Code / Codex / Copilot session you already started (possibly under a
// different account or app), read what it is doing, and stage it for handoff
// to another agent before its quota runs out.
//
// Two evidence sources, fused:
//   1. Running processes   — OS process list matched against known agent CLIs.
//   2. On-disk transcripts — agents persist full session logs (Claude Code
//      writes ~/.claude/projects/<cwd>/<session>.jsonl). These carry the
//      initial prompt, plan, tasks, files, skills, MCPs and token usage — the
//      richest, most reliable signal, available even after the process exits.
//
// The package is stdlib-only (no new dependency) and never mutates anything.
// Redaction is injected by the caller so secrets never cross the API boundary.

package detect

import (
	"os"
	"sort"
	"strings"
	"time"
)

// liveRecency: a transcript touched within this window is treated as a live
// session when no owning process can be pinned down.
const liveRecency = 10 * time.Minute

// DetectedAgent is one agent discovered on this machine.
type DetectedAgent struct {
	ID          string        `json:"id"`          // stable: provider:session or provider:pid
	Provider    string        `json:"provider"`    // claude, codex, ...
	DisplayName string        `json:"displayName"` // "Claude Code"
	Surface     string        `json:"surface"`     // terminal | desktop | ide | unknown
	PID         int           `json:"pid"`         // 0 if transcript-only (process gone)
	Running     bool          `json:"running"`     // process currently alive
	Cmdline     string        `json:"cmdline"`     // redacted command line, if a process matched
	WorkDir     string        `json:"workDir"`     // cwd, if known
	Account     string        `json:"account"`     // best-effort account/org label
	DetectedVia []string      `json:"detectedVia"` // any of: process, transcript
	LastActive  int64         `json:"lastActive"`  // unix ms of most recent activity
	Session     *SessionIntel `json:"session,omitempty"`
}

// SessionIntel is the intent extracted from an agent's on-disk session.
// These fields are the raw material a continuation handoff is built from.
type SessionIntel struct {
	SessionID      string   `json:"sessionId"`
	TranscriptPath string   `json:"transcriptPath"`
	Model          string   `json:"model,omitempty"`
	InitialPrompt  string   `json:"initialPrompt,omitempty"`
	LastPrompt     string   `json:"lastPrompt,omitempty"`
	LastActivity   string   `json:"lastActivity,omitempty"` // last assistant text, truncated
	Plan           []string `json:"plan,omitempty"`         // full todo list (latest TodoWrite)
	TasksRemaining []string `json:"tasksRemaining,omitempty"`
	FilesTouched   []string `json:"filesTouched,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Mcps           []string `json:"mcps,omitempty"`
	TokensIn       int64    `json:"tokensIn"`
	TokensOut      int64    `json:"tokensOut"`
	MessageCount   int      `json:"messageCount"`

	// Correlation-only fields (not serialized): used to fuse transcript intel
	// with a matching running process.
	workDir      string
	account      string
	lastActiveMs int64
}

// Options configures a scan.
type Options struct {
	Home               string              // override home dir; "" → os.UserHomeDir
	MaxAgeHours        int                 // transcript recency window; 0 → 24
	IncludeProcesses   bool                // match running processes
	IncludeTranscripts bool                // read on-disk session stores
	Redact             func(string) string // scrub sensitive strings; nil → identity
}

// DefaultOptions returns a scan that uses both evidence sources over the last day.
func DefaultOptions() Options {
	return Options{MaxAgeHours: 24, IncludeProcesses: true, IncludeTranscripts: true}
}

// Scan discovers agents on this machine. Best-effort: a failure in one source
// (no process tool, unreadable transcript dir) degrades gracefully rather than
// failing the whole scan.
func Scan(opts Options) ([]DetectedAgent, error) {
	if opts.MaxAgeHours <= 0 {
		opts.MaxAgeHours = 24
	}
	if !opts.IncludeProcesses && !opts.IncludeTranscripts {
		opts.IncludeProcesses, opts.IncludeTranscripts = true, true
	}
	scrub := opts.Redact
	if scrub == nil {
		scrub = func(s string) string { return s }
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	maxAge := time.Duration(opts.MaxAgeHours) * time.Hour

	// Pass 1: running processes. These are honest about liveness (a real PID)
	// but carry no intent — we can't read a process's cwd portably, so we can't
	// link a bare process to the session it owns.
	var procAgents []*DetectedAgent
	if opts.IncludeProcesses {
		procs, _ := listProcesses() // best-effort
		for _, p := range procs {
			for _, sig := range signatures {
				if !sig.matchProcess(p) {
					continue
				}
				procAgents = append(procAgents, &DetectedAgent{
					ID:          sig.Provider + ":pid:" + itoa(p.PID),
					Provider:    sig.Provider,
					DisplayName: sig.DisplayName,
					Surface:     sig.Surface,
					PID:         p.PID,
					Running:     true,
					Cmdline:     truncate(scrub(p.Cmdline), 240),
					WorkDir:     p.Cwd, // empty unless the OS scanner can resolve it
					DetectedVia: []string{"process"},
				})
				break // one signature per process
			}
		}
	}

	// Pass 2: on-disk transcripts. These carry the intent. "Running" is inferred
	// from recency, since the owning process can't be pinned down on every OS.
	now := time.Now()
	var transAgents []*DetectedAgent
	if opts.IncludeTranscripts {
		for _, sig := range signatures {
			if sig.scanTranscripts == nil {
				continue
			}
			for _, intel := range sig.scanTranscripts(home, maxAge) {
				intel := intel
				redactIntel(&intel, scrub)
				transAgents = append(transAgents, &DetectedAgent{
					ID:          sig.Provider + ":" + shortID(intel.SessionID),
					Provider:    sig.Provider,
					DisplayName: sig.DisplayName,
					Surface:     sig.Surface,
					Running:     now.Sub(time.UnixMilli(intel.lastActiveMs)) < liveRecency,
					WorkDir:     intel.workDir,
					Account:     scrub(intel.account),
					DetectedVia: []string{"transcript"},
					LastActive:  intel.lastActiveMs,
					Session:     &intel,
				})
			}
		}
	}

	// Fuse a process into a session ONLY when the link is provable: same
	// provider and equal, known working directories. (Possible on OSes whose
	// scanner resolves cwd; a no-op where it can't.) Fused processes are not
	// listed again on their own.
	consumed := map[*DetectedAgent]bool{}
	for _, ta := range transAgents {
		if ta.WorkDir == "" {
			continue
		}
		for _, pa := range procAgents {
			if consumed[pa] || pa.Provider != ta.Provider || pa.WorkDir == "" {
				continue
			}
			if strings.EqualFold(pa.WorkDir, ta.WorkDir) {
				ta.PID = pa.PID
				ta.Running = true
				ta.Cmdline = pa.Cmdline
				ta.DetectedVia = appendUniq(ta.DetectedVia, "process")
				consumed[pa] = true
				break
			}
		}
	}

	// Providers with transcript evidence: suppress their leftover bare-process
	// cards — we'd be guessing which process owns which session.
	hasTranscript := map[string]bool{}
	for _, ta := range transAgents {
		hasTranscript[ta.Provider] = true
	}

	out := make([]DetectedAgent, 0, len(transAgents)+len(procAgents))
	for _, ta := range transAgents {
		out = append(out, *ta)
	}
	// Collapse bare-process cards to one per provider: Electron-based IDEs
	// (Antigravity, etc.) spawn many matching processes, and without a session
	// they carry no distinguishing intel — one "running" card is enough.
	procSeen := map[string]bool{}
	for _, pa := range procAgents {
		if consumed[pa] || hasTranscript[pa.Provider] || procSeen[pa.Provider] {
			continue
		}
		procSeen[pa.Provider] = true
		out = append(out, *pa)
	}

	// Running first, then most-recently active.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Running != out[j].Running {
			return out[i].Running
		}
		return out[i].LastActive > out[j].LastActive
	})
	return out, nil
}

func redactIntel(in *SessionIntel, scrub func(string) string) {
	in.InitialPrompt = truncate(scrub(in.InitialPrompt), 600)
	in.LastPrompt = truncate(scrub(in.LastPrompt), 600)
	in.LastActivity = truncate(scrub(in.LastActivity), 600)
	for i := range in.Plan {
		in.Plan[i] = scrub(in.Plan[i])
	}
	for i := range in.TasksRemaining {
		in.TasksRemaining[i] = scrub(in.TasksRemaining[i])
	}
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func shortID(id string) string {
	if id == "" {
		return "unknown"
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
