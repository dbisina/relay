package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A representative Claude Code session transcript.
const fixtureJSONL = `{"type":"user","cwd":"/tmp/proj","sessionId":"sess-abc-123","timestamp":"2020-01-01T10:00:00Z","message":{"role":"user","content":"Build a login form <system-reminder>secret stuff</system-reminder>"}}
{"type":"assistant","sessionId":"sess-abc-123","timestamp":"2020-01-01T10:01:00Z","message":{"role":"assistant","model":"claude-opus-4-8","usage":{"input_tokens":1200,"output_tokens":300},"content":[{"type":"text","text":"Starting now"},{"type":"tool_use","name":"TodoWrite","input":{"todos":[{"content":"Write HTML","status":"completed","activeForm":"Writing HTML"},{"content":"Add validation","status":"in_progress","activeForm":"Adding validation"},{"content":"Wire submit","status":"pending","activeForm":"Wiring submit"}]}},{"type":"tool_use","name":"Edit","input":{"file_path":"src/login.tsx"}},{"type":"tool_use","name":"Skill","input":{"skill":"frontend-patterns"}},{"type":"tool_use","name":"mcp__figma__get_design","input":{}}]}}
{"type":"user","sessionId":"sess-abc-123","timestamp":"2020-01-01T10:02:00Z","message":{"role":"user","content":[{"type":"tool_result","text":""}]}}
{"type":"user","sessionId":"sess-abc-123","timestamp":"2020-01-01T10:03:00Z","message":{"role":"user","content":"Also add remember-me"}}
{"type":"assistant","sessionId":"sess-abc-123","timestamp":"2020-01-01T10:04:00Z","message":{"role":"assistant","content":[{"type":"text","text":"Added remember-me checkbox"}]}}
`

func writeFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-tmp-proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sess-abc-123.jsonl"), []byte(fixtureJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// A Codex rollout transcript: date-nested path, payload-wrapped lines,
// apply_patch function call, and a cumulative token_count event.
const codexFixtureJSONL = `{"type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/tmp/codexproj"},"timestamp":"2026-06-19T10:00:00Z"}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Refactor the auth module"}]},"timestamp":"2026-06-19T10:00:01Z"}
{"type":"response_item","payload":{"type":"function_call","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\n*** Update File: src/auth.go\n@@\n-old\n+new\n*** End Patch\"}"},"timestamp":"2026-06-19T10:00:02Z"}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Refactored auth module."}]},"timestamp":"2026-06-19T10:00:03Z"}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":2000,"output_tokens":500}}},"timestamp":"2026-06-19T10:00:04Z"}
`

// TestScanCodexNestedStore proves the date-nested rollout store is discovered
// (the old two-level walk missed it) and that codex intent is lifted as richly
// as Claude: prompt, last activity, apply_patch files, and cumulative tokens.
func TestScanCodexNestedStore(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "06", "19")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-06-19T10-00-00-codex-sess-1.jsonl"), []byte(codexFixtureJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	agents, err := Scan(Options{Home: home, IncludeProcesses: false, IncludeTranscripts: true, MaxAgeHours: 240})
	if err != nil {
		t.Fatal(err)
	}
	var codex *DetectedAgent
	for i := range agents {
		if agents[i].Provider == "codex" {
			codex = &agents[i]
			break
		}
	}
	if codex == nil {
		t.Fatalf("codex session not detected in nested store; got %d agents", len(agents))
	}
	if codex.WorkDir != "/tmp/codexproj" {
		t.Errorf("workDir = %q, want /tmp/codexproj", codex.WorkDir)
	}
	s := codex.Session
	if s == nil {
		t.Fatal("codex session intel missing")
	}
	if s.InitialPrompt != "Refactor the auth module" {
		t.Errorf("initialPrompt = %q", s.InitialPrompt)
	}
	if s.LastActivity != "Refactored auth module." {
		t.Errorf("lastActivity = %q", s.LastActivity)
	}
	if !contains(s.FilesTouched, "src/auth.go") {
		t.Errorf("filesTouched = %v, want it to include src/auth.go (from apply_patch)", s.FilesTouched)
	}
	if s.TokensIn != 2000 || s.TokensOut != 500 {
		t.Errorf("tokens = %d/%d, want 2000/500 (from token_count)", s.TokensIn, s.TokensOut)
	}
}

func TestScanTranscriptOnly(t *testing.T) {
	home := writeFixture(t)
	agents, err := Scan(Options{Home: home, IncludeProcesses: false, IncludeTranscripts: true, MaxAgeHours: 240})
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("want 1 agent, got %d", len(agents))
	}
	a := agents[0]
	if a.Provider != "claude" {
		t.Errorf("provider = %q, want claude", a.Provider)
	}
	if a.Running {
		t.Errorf("transcript-only agent should not be Running")
	}
	if a.WorkDir != "/tmp/proj" {
		t.Errorf("workDir = %q", a.WorkDir)
	}
	s := a.Session
	if s == nil {
		t.Fatal("session intel missing")
	}
	if s.InitialPrompt != "Build a login form" {
		t.Errorf("initialPrompt = %q (system-reminder not stripped?)", s.InitialPrompt)
	}
	if s.LastPrompt != "Also add remember-me" {
		t.Errorf("lastPrompt = %q", s.LastPrompt)
	}
	if s.LastActivity != "Added remember-me checkbox" {
		t.Errorf("lastActivity = %q", s.LastActivity)
	}
	if s.Model != "claude-opus-4-8" {
		t.Errorf("model = %q", s.Model)
	}
	if s.TokensIn != 1200 || s.TokensOut != 300 {
		t.Errorf("tokens = %d/%d, want 1200/300", s.TokensIn, s.TokensOut)
	}
	if s.MessageCount != 4 {
		t.Errorf("messageCount = %d, want 4 (tool_result turn skipped)", s.MessageCount)
	}
	if !contains(s.FilesTouched, "src/login.tsx") {
		t.Errorf("filesTouched = %v", s.FilesTouched)
	}
	if !contains(s.Skills, "frontend-patterns") {
		t.Errorf("skills = %v", s.Skills)
	}
	if !contains(s.Mcps, "figma") {
		t.Errorf("mcps = %v", s.Mcps)
	}
	if len(s.Plan) != 3 {
		t.Errorf("plan = %v, want 3 items", s.Plan)
	}
	if len(s.TasksRemaining) != 2 || s.TasksRemaining[0] != "Add validation" {
		t.Errorf("tasksRemaining = %v, want [Add validation, Wire submit]", s.TasksRemaining)
	}
}

// Codex rollout shape: role/content nested under "payload", input_text/output_text blocks.
const codexFixture = `{"timestamp":"2020-01-01T09:00:00Z","type":"session_meta","payload":{"id":"01codex","cwd":"/home/u/proj"}}
{"timestamp":"2020-01-01T09:00:05Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Refactor the auth module"}]}}
{"timestamp":"2020-01-01T09:00:10Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Split auth.go into smaller funcs"}]}}
{"timestamp":"2020-01-01T09:00:20Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Now add tests"}]}}
`

func TestScanCodexTranscript(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2020-01-01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-01codex.jsonl"), []byte(codexFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	sessions := scanCodexTranscripts(home, 240*time.Hour)
	if len(sessions) != 1 {
		t.Fatalf("want 1 codex session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.SessionID != "01codex" {
		t.Errorf("sessionId = %q, want 01codex (from payload)", s.SessionID)
	}
	if s.workDir != "/home/u/proj" {
		t.Errorf("workDir = %q", s.workDir)
	}
	if s.InitialPrompt != "Refactor the auth module" {
		t.Errorf("initialPrompt = %q (input_text not extracted?)", s.InitialPrompt)
	}
	if s.LastPrompt != "Now add tests" {
		t.Errorf("lastPrompt = %q", s.LastPrompt)
	}
	if s.LastActivity != "Split auth.go into smaller funcs" {
		t.Errorf("lastActivity = %q (output_text not extracted?)", s.LastActivity)
	}
	if s.MessageCount != 3 {
		t.Errorf("messageCount = %d, want 3", s.MessageCount)
	}
}

func TestRenderHandoff(t *testing.T) {
	home := writeFixture(t)
	agents, _ := Scan(Options{Home: home, IncludeProcesses: false, IncludeTranscripts: true, MaxAgeHours: 240})
	md := RenderHandoff(agents[0], "codex")
	for _, want := range []string{"Add validation", "Build a login form", "src/login.tsx", "frontend-patterns", "codex"} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered brief missing %q", want)
		}
	}
	// The files listed already have live edits on disk (worktree carries them
	// over) — the brief must say so, or a receiving agent may rewrite them from
	// scratch instead of reading and continuing the actual in-progress code.
	if !strings.Contains(md, "already contain") || !strings.Contains(md, "read") {
		t.Errorf("rendered brief does not instruct the agent to read existing edits before changing files:\n%s", md)
	}
}
