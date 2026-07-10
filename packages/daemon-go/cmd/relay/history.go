// history.go — the time machine.
//
// Every Relay session leaves two trails: the hash-chained audit log (the handoff
// story — who ran when and why) and, when the working dir is a git repo, a commit
// per snapshot. The time machine surfaces both, lets the user diff any commit,
// and rewinds to one NON-DESTRUCTIVELY (a new branch at the snapshot) so current
// work is never lost.

package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/dbisina/relay/internal/audit"
)

type historyItem struct {
	Seq      int64  `json:"seq"`
	Ts       string `json:"ts"`
	Event    string `json:"event"`
	Provider string `json:"provider,omitempty"`
	Summary  string `json:"summary"`
}

// buildHistory distils the audit log into the handoff timeline, keeping the
// lifecycle events (handoff, dispatch, account switch, snapshot, pipeline) and
// dropping low-signal chatter.
func buildHistory(auditPath string) []historyItem {
	entries, _ := audit.Tail(auditPath, 400)
	out := make([]historyItem, 0, len(entries))
	for _, e := range entries {
		msg, _ := e.Data["msg"].(string)
		keep := e.Event == "handoff" || e.Event == "worktree_create" || e.Event == "envelope_signature_fail"
		if !keep && e.Event == "system" {
			for _, kw := range []string{"handoff", "dispatched", "account", "pipeline", "completed", "failover"} {
				if strings.Contains(msg, kw) {
					keep = true
					break
				}
			}
		}
		if !keep {
			continue
		}
		summary := msg
		if summary == "" {
			if p, ok := e.Data["path"].(string); ok {
				summary = p
			} else {
				summary = e.Event
			}
		}
		prov, _ := e.Data["provider"].(string)
		out = append(out, historyItem{
			Seq:      e.Seq,
			Ts:       e.Timestamp.Format(time.RFC3339),
			Event:    e.Event,
			Provider: prov,
			Summary:  summary,
		})
	}
	return out
}

type commitItem struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	When    string `json:"when"`
}

// safeRefRe guards a user-supplied git ref against argument abuse. git args are
// passed as separate argv (no shell), but we still reject anything that isn't a
// plain ref and never let it start with a dash.
var safeRefRe = regexp.MustCompile(`^[0-9A-Za-z._/-]{4,}$`)

func isSafeRef(sha string) bool {
	return sha != "" && !strings.HasPrefix(sha, "-") && safeRefRe.MatchString(sha)
}

func runGit(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	return string(out), err
}

func isGitRepo(workDir string) bool {
	out, err := runGit(workDir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// gitCommits returns the recent commit trail of the working dir (empty if not a
// git repo).
func gitCommits(workDir string) []commitItem {
	if !isGitRepo(workDir) {
		return nil
	}
	out, err := runGit(workDir, "log", "-n", "60", "--pretty=format:%H\x1f%h\x1f%s\x1f%cI")
	if err != nil {
		return nil
	}
	var items []commitItem
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\x1f")
		if len(parts) < 4 {
			continue
		}
		items = append(items, commitItem{SHA: parts[0], Short: parts[1], Subject: parts[2], When: parts[3]})
	}
	return items
}

// gitDiff returns the diff for one commit (stat + patch, size-bounded).
func gitDiff(workDir, sha string) (map[string]interface{}, error) {
	if !isSafeRef(sha) {
		return nil, fmt.Errorf("invalid commit ref")
	}
	if !isGitRepo(workDir) {
		return nil, fmt.Errorf("working dir is not a git repo")
	}
	out, err := runGit(workDir, "show", "--stat", "-p", "--no-color", sha)
	if err != nil {
		return nil, fmt.Errorf("git show failed (unknown commit?)")
	}
	const maxDiff = 200_000
	if len(out) > maxDiff {
		out = out[:maxDiff] + "\n… (diff truncated)"
	}
	return map[string]interface{}{"sha": sha, "diff": out}, nil
}

// gitRewind creates a branch at the snapshot — non-destructive, so the user can
// recover that point without discarding current work.
func gitRewind(workDir, sha string) (map[string]interface{}, error) {
	if !isSafeRef(sha) {
		return nil, fmt.Errorf("invalid commit ref")
	}
	if !isGitRepo(workDir) {
		return nil, fmt.Errorf("working dir is not a git repo")
	}
	short := sha
	if len(short) > 8 {
		short = short[:8]
	}
	branch := "relay-rewind-" + short
	if _, err := runGit(workDir, "branch", branch, sha); err != nil {
		return nil, fmt.Errorf("could not create branch %s (already exists or unknown commit)", branch)
	}
	return map[string]interface{}{
		"branch": branch,
		"hint":   fmt.Sprintf("git switch %s   # to inspect; your current work is untouched", branch),
	}, nil
}
