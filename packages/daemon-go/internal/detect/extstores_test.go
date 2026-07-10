package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const continueFixture = `{
  "sessionId": "cont-1",
  "title": "Fix the navbar",
  "workspaceDirectory": "/home/u/web",
  "chatModelTitle": "GPT-4o",
  "history": [
    {"message": {"role": "user", "content": "Fix the navbar overflow"}},
    {"message": {"role": "assistant", "content": [{"type": "text", "text": "Adjusted flex-wrap"}, {"type": "tool_use", "name": "edit", "input": {"file_path": "src/nav.tsx"}}]}},
    {"message": {"role": "user", "content": "Now make it sticky"}}
  ]
}`

func TestScanContinueSessions(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".continue", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cont-1.json"), []byte(continueFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scanContinueSessions(home, 240*time.Hour)
	if len(s) != 1 {
		t.Fatalf("want 1 continue session, got %d", len(s))
	}
	got := s[0]
	if got.InitialPrompt != "Fix the navbar overflow" {
		t.Errorf("initialPrompt = %q", got.InitialPrompt)
	}
	if got.LastPrompt != "Now make it sticky" {
		t.Errorf("lastPrompt = %q", got.LastPrompt)
	}
	if got.LastActivity != "Adjusted flex-wrap" {
		t.Errorf("lastActivity = %q", got.LastActivity)
	}
	if got.Model != "GPT-4o" {
		t.Errorf("model = %q", got.Model)
	}
	if got.workDir != "/home/u/web" {
		t.Errorf("workDir = %q", got.workDir)
	}
	if !contains(got.FilesTouched, "src/nav.tsx") {
		t.Errorf("filesTouched = %v", got.FilesTouched)
	}
	if got.MessageCount != 3 {
		t.Errorf("messageCount = %d, want 3", got.MessageCount)
	}
}

const clineUIFixture = `[
 {"ts":1779124931753,"type":"say","say":"task","text":"Refactor the auth service"},
 {"ts":1779124931800,"type":"say","say":"checkpoint_created","text":""},
 {"ts":1779124932000,"type":"say","say":"text","text":"Split into smaller modules"},
 {"ts":1779124933000,"type":"ask","ask":"followup","text":"Which file first?"}
]`

const clineMetaFixture = `{"files_in_context":[{"path":"src/auth.ts"}],"model_usage":[{"ts":1,"model_id":"gpt-5","tokens_in":1000,"tokens_out":200}]}`

func TestScanClineTasks(t *testing.T) {
	home := t.TempDir()
	taskDir := filepath.Join(vscodeGlobalStorage(home), "saoudrizwan.claude-dev", "tasks", "1779124931753")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "ui_messages.json"), []byte(clineUIFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "task_metadata.json"), []byte(clineMetaFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	s := scanClineTasks(home, 240*time.Hour)
	if len(s) != 1 {
		t.Fatalf("want 1 cline task, got %d", len(s))
	}
	got := s[0]
	if got.InitialPrompt != "Refactor the auth service" {
		t.Errorf("initialPrompt = %q", got.InitialPrompt)
	}
	if got.LastActivity != "Split into smaller modules" {
		t.Errorf("lastActivity = %q", got.LastActivity)
	}
	if got.Model != "gpt-5" {
		t.Errorf("model = %q (from model_usage)", got.Model)
	}
	if got.TokensIn != 1000 || got.TokensOut != 200 {
		t.Errorf("tokens = %d/%d, want 1000/200", got.TokensIn, got.TokensOut)
	}
	if !contains(got.FilesTouched, "src/auth.ts") {
		t.Errorf("filesTouched = %v (from files_in_context)", got.FilesTouched)
	}
	if got.MessageCount != 2 {
		t.Errorf("messageCount = %d, want 2 (task + text)", got.MessageCount)
	}
}

const cursorFixture = `{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nbuild a login form\n</user_query>"}]}}
{"role":"assistant","message":{"content":[{"type":"text","text":"Done, added LoginForm.tsx"}]}}
{"role":null,"message":{}}
`

func TestScanCursorTranscripts(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".cursor", "projects", "empty-window", "agent-transcripts", "abc123")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "abc123.jsonl"), []byte(cursorFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scanCursorTranscripts(home, 240*time.Hour)
	if len(s) != 1 {
		t.Fatalf("want 1 cursor session, got %d", len(s))
	}
	if s[0].InitialPrompt != "build a login form" {
		t.Errorf("initialPrompt = %q (user_query not unwrapped?)", s[0].InitialPrompt)
	}
	if s[0].LastActivity != "Done, added LoginForm.tsx" {
		t.Errorf("lastActivity = %q", s[0].LastActivity)
	}
	if s[0].MessageCount != 2 {
		t.Errorf("messageCount = %d, want 2", s[0].MessageCount)
	}
}

const copilotChatFixture = `{"sessionId":"cop-1","lastMessageDate":1779000000000,"requests":[
 {"message":{"text":"build the android app"},"modelId":"gpt-4o","timestamp":1779000000000,"response":[{"kind":"markdownContent","content":{"value":"Building now"}}]}
]}`

func TestScanCopilotChats(t *testing.T) {
	home := t.TempDir()
	userDir := filepath.Dir(vscodeGlobalStorage(home))
	ws := filepath.Join(userDir, "workspaceStorage", "ws1")
	if err := os.MkdirAll(filepath.Join(ws, "chatSessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "workspace.json"), []byte(`{"folder":"file:///c%3A/Users/u/app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "chatSessions", "s1.json"), []byte(copilotChatFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scanCopilotChats(home, 240*time.Hour)
	if len(s) != 1 {
		t.Fatalf("want 1 copilot session, got %d", len(s))
	}
	got := s[0]
	if got.InitialPrompt != "build the android app" {
		t.Errorf("initialPrompt = %q", got.InitialPrompt)
	}
	if got.LastActivity != "Building now" {
		t.Errorf("lastActivity = %q (markdown block not read?)", got.LastActivity)
	}
	if got.Model != "gpt-4o" {
		t.Errorf("model = %q", got.Model)
	}
	if !strings.Contains(got.workDir, "app") {
		t.Errorf("workDir = %q (workspace.json folder not resolved?)", got.workDir)
	}
}

const copilotCLIFixture = `{"type":"session.start","timestamp":"2026-06-19T11:40:19Z","data":{"sessionId":"cli-1","selectedModel":"gpt-5-mini"}}
{"type":"user.message","timestamp":"2026-06-19T11:40:56Z","data":{"content":"fix the build"}}
{"type":"assistant.message","timestamp":"2026-06-19T11:41:14Z","data":{"content":"","toolRequests":[{"name":"report_intent","arguments":{"intent":"Diagnosing build error"}}]}}
{"type":"tool.execution_start","timestamp":"2026-06-19T11:41:15Z","data":{"toolName":"str_replace","arguments":{"path":"main.go"}}}
{"type":"assistant.message","timestamp":"2026-06-19T11:41:20Z","data":{"content":"Fixed the import","toolRequests":[]}}
`

func TestScanCopilotCLI(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".copilot", "session-state", "cli-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(copilotCLIFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scanCopilotCLI(home, 240*time.Hour)
	if len(s) != 1 {
		t.Fatalf("want 1 copilot CLI session, got %d", len(s))
	}
	got := s[0]
	if got.InitialPrompt != "fix the build" {
		t.Errorf("initialPrompt = %q", got.InitialPrompt)
	}
	if got.Model != "gpt-5-mini" {
		t.Errorf("model = %q (from session.start)", got.Model)
	}
	if got.LastActivity != "Fixed the import" {
		t.Errorf("lastActivity = %q", got.LastActivity)
	}
	if !contains(got.FilesTouched, "main.go") {
		t.Errorf("filesTouched = %v (tool args path)", got.FilesTouched)
	}
	if got.MessageCount != 2 {
		t.Errorf("messageCount = %d, want 2", got.MessageCount)
	}
}

// TestScanClineThroughScan proves the provider surfaces via the top-level Scan
// as a transcript-only agent (no process, recency-driven).
func TestScanClineThroughScan(t *testing.T) {
	home := t.TempDir()
	taskDir := filepath.Join(vscodeGlobalStorage(home), "saoudrizwan.claude-dev", "tasks", "t1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "ui_messages.json"), []byte(clineUIFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	agents, err := Scan(Options{Home: home, IncludeProcesses: false, IncludeTranscripts: true, MaxAgeHours: 240})
	if err != nil {
		t.Fatal(err)
	}
	var cl *DetectedAgent
	for i := range agents {
		if agents[i].Provider == "cline" {
			cl = &agents[i]
			break
		}
	}
	if cl == nil {
		t.Fatalf("cline not surfaced via Scan; got %d agents", len(agents))
	}
	if cl.Session == nil || cl.Session.InitialPrompt != "Refactor the auth service" {
		t.Errorf("cline intel wrong: %+v", cl.Session)
	}
}
