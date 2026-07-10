// internal/detect/render.go — turn captured session intel into a portable
// handoff brief a receiving agent can resume from.
//
// This mirrors the continuation-contract shape used by the handoff engine
// (DO NOT REDO first, then goal / next / tasks / context), but is built purely
// from observed evidence rather than a signed snapshot, since the source agent
// was never under Relay's control.

package detect

import (
	"fmt"
	"strings"
	"time"
)

// RenderHandoff renders a Markdown brief for porting this agent's work to
// `target` (a provider name, may be empty for "any agent").
func RenderHandoff(a DetectedAgent, target string) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# Continuation brief (adopted session)\n\n")
	w("_Captured by Relay from a %s session you were already running", a.DisplayName)
	if target != "" {
		w(", to be continued by %s", target)
	}
	w("._\n\n")

	s := a.Session
	if s == nil {
		w("> No on-disk transcript was available for this agent; only its running\n")
		w("> process was detected. Re-run the task from the original prompt.\n")
		return b.String()
	}

	if len(s.TasksRemaining) > 0 {
		w("## Do next (remaining tasks)\n")
		for _, t := range s.TasksRemaining {
			w("- [ ] %s\n", t)
		}
		w("\n")
	}

	w("## Original goal\n%s\n\n", fallback(s.InitialPrompt, "(unknown — no first prompt captured)"))

	if s.LastPrompt != "" && s.LastPrompt != s.InitialPrompt {
		w("## Most recent instruction\n%s\n\n", s.LastPrompt)
	}
	if s.LastActivity != "" {
		w("## Where it left off\n%s\n\n", s.LastActivity)
	}

	if len(s.Plan) > 0 {
		w("## Full plan (as the source agent tracked it)\n")
		for _, p := range s.Plan {
			done := " "
			if !contains(s.TasksRemaining, p) {
				done = "x"
			}
			w("- [%s] %s\n", done, p)
		}
		w("\n")
	}

	if len(s.FilesTouched) > 0 {
		w("## Files in flight\n")
		for _, f := range s.FilesTouched {
			w("- `%s`\n", f)
		}
		w("\n")
	}

	if len(s.Skills) > 0 {
		w("## Skills loaded\n%s\n\n", strings.Join(s.Skills, ", "))
	}
	if len(s.Mcps) > 0 {
		w("## MCP servers connected\n%s\n\n", strings.Join(s.Mcps, ", "))
	}

	w("## Provenance\n")
	w("- Source: %s (%s)\n", a.DisplayName, a.Provider)
	if s.Model != "" {
		w("- Model: %s\n", s.Model)
	}
	if a.WorkDir != "" {
		w("- Working dir: `%s`\n", a.WorkDir)
	}
	w("- Messages: %d · tokens in/out: %d/%d\n", s.MessageCount, s.TokensIn, s.TokensOut)
	if a.LastActive > 0 {
		w("- Last active: %s\n", time.UnixMilli(a.LastActive).Format(time.RFC3339))
	}
	w("- Transcript: `%s`\n", s.TranscriptPath)
	return b.String()
}

func fallback(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
