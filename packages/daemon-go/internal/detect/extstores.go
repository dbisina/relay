// internal/detect/extstores.go — session readers for IDE-extension agents that
// store history as JSON objects (not JSONL) outside the home dir.
//
//   Continue : ~/.continue/sessions/<uuid>.json   (one object, history[])
//   Cline    : <vscode-globalStorage>/saoudrizwan.claude-dev/tasks/<id>/
//              ui_messages.json + task_metadata.json
//
// Kept separate from transcript.go (the JSONL readers) so the two evolve
// independently. Shared decoding helpers (decodeContent, joinText, cleanPrompt,
// jsonField, orderedSet) live in transcript.go.

package detect

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type intelCand struct {
	intel SessionIntel
	mod   time.Time
}

// recentIntel sorts newest-first and caps the result, matching the JSONL stores.
func recentIntel(cands []intelCand) []SessionIntel {
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	if len(cands) > maxTranscriptsPerProvider {
		cands = cands[:maxTranscriptsPerProvider]
	}
	out := make([]SessionIntel, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.intel)
	}
	return out
}

// ─── Continue ───────────────────────────────────────────────────────────────

func scanContinueSessions(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	dir := filepath.Join(home, ".continue", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	var cands []intelCand
	for _, e := range entries {
		// sessions.json is the index, not a transcript.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "sessions.json" {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if intel, ok := parseContinueSession(filepath.Join(dir, e.Name()), info.ModTime()); ok {
			cands = append(cands, intelCand{intel, info.ModTime()})
		}
	}
	return recentIntel(cands)
}

func parseContinueSession(path string, mod time.Time) (SessionIntel, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionIntel{}, false
	}
	var doc struct {
		SessionID          string `json:"sessionId"`
		Title              string `json:"title"`
		WorkspaceDirectory string `json:"workspaceDirectory"`
		ChatModelTitle     string `json:"chatModelTitle"`
		History            []struct {
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"history"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return SessionIntel{}, false
	}

	intel := SessionIntel{
		SessionID:      doc.SessionID,
		TranscriptPath: path,
		Model:          doc.ChatModelTitle,
		workDir:        doc.WorkspaceDirectory,
		lastActiveMs:   mod.UnixMilli(),
	}
	if intel.SessionID == "" {
		intel.SessionID = strings.TrimSuffix(filepath.Base(path), ".json")
	}

	files := newOrderedSet()
	var firstUser, lastUser, lastAssistant string
	for _, h := range doc.History {
		text, blocks := decodeContent(h.Message.Content)
		t := cleanPrompt(text + joinText(blocks))
		switch h.Message.Role {
		case "user":
			if t == "" {
				continue
			}
			if firstUser == "" {
				firstUser = t
			}
			lastUser = t
			intel.MessageCount++
		case "assistant":
			intel.MessageCount++
			if t != "" {
				lastAssistant = t
			}
			for _, b := range blocks {
				if b.Type == "tool_use" {
					if fp := jsonField(b.Input, "file_path"); fp != "" {
						files.add(fp)
					}
				}
			}
		}
	}
	intel.InitialPrompt = firstUser
	if intel.InitialPrompt == "" {
		intel.InitialPrompt = doc.Title // fall back to the session title
	}
	intel.LastPrompt = lastUser
	intel.LastActivity = lastAssistant
	intel.FilesTouched = files.slice()
	return intel, intel.MessageCount > 0 || intel.InitialPrompt != ""
}

// ─── Cline (VS Code extension: saoudrizwan.claude-dev) ────────────────────────

// appGlobalStorage returns a VS Code-family app's per-OS User/globalStorage dir
// (product = "Code", "Antigravity", "Cursor", …), derived from home so tests can
// point it at a temp tree.
func appGlobalStorage(home, product string) string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", product, "User", "globalStorage")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", product, "User", "globalStorage")
	default:
		return filepath.Join(home, ".config", product, "User", "globalStorage")
	}
}

// vscodeGlobalStorage returns stock VS Code's globalStorage dir.
func vscodeGlobalStorage(home string) string { return appGlobalStorage(home, "Code") }

func scanClineTasks(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	dir := filepath.Join(vscodeGlobalStorage(home), "saoudrizwan.claude-dev", "tasks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	var cands []intelCand
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskDir := filepath.Join(dir, e.Name())
		// Recency is keyed off ui_messages.json (always written, unlike metadata).
		info, err := os.Stat(filepath.Join(taskDir, "ui_messages.json"))
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if intel, ok := parseClineTask(taskDir, e.Name(), info.ModTime()); ok {
			cands = append(cands, intelCand{intel, info.ModTime()})
		}
	}
	return recentIntel(cands)
}

// parseClineTask reads a Cline task's ui_messages.json (clean task + assistant
// text) and task_metadata.json (model + files). The api_conversation_history is
// deliberately skipped — it is noisy with environment dumps and tool XML.
func parseClineTask(taskDir, id string, mod time.Time) (SessionIntel, bool) {
	uiData, err := os.ReadFile(filepath.Join(taskDir, "ui_messages.json"))
	if err != nil {
		return SessionIntel{}, false
	}
	var ui []struct {
		Ts   int64  `json:"ts"`
		Type string `json:"type"` // say | ask
		Say  string `json:"say"`  // task | text | …
		Text string `json:"text"`
	}
	if json.Unmarshal(uiData, &ui) != nil {
		return SessionIntel{}, false
	}

	intel := SessionIntel{
		SessionID:      id,
		TranscriptPath: filepath.Join(taskDir, "ui_messages.json"),
		lastActiveMs:   mod.UnixMilli(),
	}
	var lastAssistant string
	for _, m := range ui {
		if m.Ts > intel.lastActiveMs {
			intel.lastActiveMs = m.Ts // Cline ts is already unix ms
		}
		if m.Type != "say" {
			continue
		}
		switch m.Say {
		case "task":
			t := cleanPrompt(m.Text)
			if t == "" {
				continue
			}
			if intel.InitialPrompt == "" {
				intel.InitialPrompt = t
			}
			intel.LastPrompt = t
			intel.MessageCount++
		case "text":
			if t := strings.TrimSpace(m.Text); t != "" {
				lastAssistant = t
				intel.MessageCount++
			}
		}
	}
	intel.LastActivity = lastAssistant

	// task_metadata.json: authoritative model + touched files (+ tokens if present).
	if metaData, err := os.ReadFile(filepath.Join(taskDir, "task_metadata.json")); err == nil {
		var meta struct {
			FilesInContext json.RawMessage `json:"files_in_context"`
			ModelUsage     []struct {
				ModelID   string `json:"model_id"`
				TokensIn  int64  `json:"tokens_in"`
				TokensOut int64  `json:"tokens_out"`
			} `json:"model_usage"`
		}
		if json.Unmarshal(metaData, &meta) == nil {
			if n := len(meta.ModelUsage); n > 0 {
				intel.Model = meta.ModelUsage[n-1].ModelID
				for _, u := range meta.ModelUsage {
					intel.TokensIn += u.TokensIn
					intel.TokensOut += u.TokensOut
				}
			}
			intel.FilesTouched = parseClineFiles(meta.FilesInContext)
		}
	}
	return intel, intel.InitialPrompt != "" || intel.MessageCount > 0
}

// parseClineFiles tolerates files_in_context being either []string or a list of
// objects keyed by path/file/file_path (the shape has drifted across versions).
func parseClineFiles(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var ss []string
	if json.Unmarshal(raw, &ss) == nil {
		return dedupeNonEmpty(ss)
	}
	var objs []map[string]any
	if json.Unmarshal(raw, &objs) == nil {
		set := newOrderedSet()
		for _, o := range objs {
			for _, k := range []string{"path", "file", "file_path"} {
				if v, ok := o[k].(string); ok && v != "" {
					set.add(v)
					break
				}
			}
		}
		return set.slice()
	}
	return nil
}

func dedupeNonEmpty(in []string) []string {
	set := newOrderedSet()
	for _, s := range in {
		set.add(s)
	}
	return set.slice()
}

// ─── Cursor (~/.cursor/projects/<proj>/agent-transcripts/<id>/<id>.jsonl) ─────

func scanCursorTranscripts(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	// Cursor agent transcripts are JSONL nested under project/agent-transcripts/id/.
	root := filepath.Join(home, ".cursor", "projects")
	return scanJSONLTree(root, maxAge, parseCursorSession)
}

// parseCursorSession reuses the Claude line reader — Cursor lines are
// {role, message:{content:[blocks]}}, which parseClaudeSession already handles —
// but accepts only sessions with real turns (the tree walk also sees stray
// .jsonl that would otherwise pass on a filename-derived id alone).
func parseCursorSession(path string, mod time.Time) (SessionIntel, bool) {
	intel, _ := parseClaudeSession(path, mod)
	return intel, intel.MessageCount > 0
}

// ─── GitHub Copilot Chat (VS Code: workspaceStorage/<h>/chatSessions/*.json) ──

func scanCopilotChats(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	userDir := filepath.Dir(vscodeGlobalStorage(home)) // …/Code/User
	cutoff := time.Now().Add(-maxAge)
	var cands []intelCand

	// Per-workspace chat sessions (workDir resolved from workspace.json).
	wsRoot := filepath.Join(userDir, "workspaceStorage")
	if wsDirs, err := os.ReadDir(wsRoot); err == nil {
		for _, wd := range wsDirs {
			if !wd.IsDir() {
				continue
			}
			wsPath := filepath.Join(wsRoot, wd.Name())
			collectCopilotChats(filepath.Join(wsPath, "chatSessions"), vscodeWorkspaceFolder(wsPath), cutoff, &cands)
		}
	}
	// Folder-less (empty-window) sessions.
	collectCopilotChats(filepath.Join(userDir, "globalStorage", "emptyWindowChatSessions"), "", cutoff, &cands)
	return recentIntel(cands)
}

func collectCopilotChats(dir, workDir string, cutoff time.Time, cands *[]intelCand) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if intel, ok := parseCopilotChatSession(filepath.Join(dir, e.Name()), workDir, info.ModTime()); ok {
			*cands = append(*cands, intelCand{intel, info.ModTime()})
		}
	}
}

func parseCopilotChatSession(path, workDir string, mod time.Time) (SessionIntel, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionIntel{}, false
	}
	var doc struct {
		SessionID       string `json:"sessionId"`
		LastMessageDate int64  `json:"lastMessageDate"`
		Requests        []struct {
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			ModelID   string          `json:"modelId"`
			Timestamp int64           `json:"timestamp"`
			Response  json.RawMessage `json:"response"`
		} `json:"requests"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return SessionIntel{}, false
	}

	intel := SessionIntel{
		SessionID:      doc.SessionID,
		TranscriptPath: path,
		workDir:        workDir,
		lastActiveMs:   doc.LastMessageDate,
	}
	if intel.SessionID == "" {
		intel.SessionID = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	var firstUser, lastUser, lastAssistant string
	for _, r := range doc.Requests {
		if u := cleanPrompt(r.Message.Text); u != "" {
			if firstUser == "" {
				firstUser = u
			}
			lastUser = u
			intel.MessageCount++
		}
		if r.ModelID != "" {
			intel.Model = r.ModelID
		}
		if r.Timestamp > intel.lastActiveMs {
			intel.lastActiveMs = r.Timestamp
		}
		if a := copilotResponseText(r.Response); a != "" {
			lastAssistant = a
			intel.MessageCount++
		}
	}
	if intel.lastActiveMs == 0 {
		intel.lastActiveMs = mod.UnixMilli()
	}
	intel.InitialPrompt = firstUser
	intel.LastPrompt = lastUser
	intel.LastActivity = lastAssistant
	return intel, intel.MessageCount > 0
}

// copilotResponseText concatenates the markdown text blocks of a VS Code chat
// response, ignoring tool-invocation / reference blocks.
func copilotResponseText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range blocks {
		if v := markdownValue(blk); v != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(v)
		}
	}
	return strings.TrimSpace(b.String())
}

// markdownValue pulls the string out of a VS Code markdown block, tolerating the
// {content:{value}} and {value:string|{value}} shapes seen across versions.
func markdownValue(blk map[string]json.RawMessage) string {
	try := func(raw json.RawMessage) string {
		if len(raw) == 0 {
			return ""
		}
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
		var inner struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(raw, &inner) == nil {
			return inner.Value
		}
		return ""
	}
	if v := try(blk["content"]); v != "" {
		return v
	}
	return try(blk["value"])
}

// scanCopilot merges both Copilot stores: the standalone CLI event log (the
// richer, fresher source) and the VS Code Copilot Chat sessions.
func scanCopilot(home string, maxAge time.Duration) []SessionIntel {
	out := scanCopilotCLI(home, maxAge)
	return append(out, scanCopilotChats(home, maxAge)...)
}

// scanCopilotCLI reads the GitHub Copilot CLI event log at
// ~/.copilot/session-state/<id>/events.jsonl — a typed event stream
// ({type,data,timestamp}) covering user.message / assistant.message / tool calls.
func scanCopilotCLI(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	root := filepath.Join(home, ".copilot", "session-state")
	return scanJSONLTree(root, maxAge, parseCopilotCLISession)
}

func parseCopilotCLISession(path string, mod time.Time) (SessionIntel, bool) {
	f, err := os.Open(path)
	if err != nil {
		return SessionIntel{}, false
	}
	defer f.Close()

	intel := SessionIntel{TranscriptPath: path}
	files := newOrderedSet()
	var firstUser, lastUser, lastAssistant, intent string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)
	for sc.Scan() {
		var e struct {
			Type      string          `json:"type"`
			Timestamp string          `json:"timestamp"`
			Data      json.RawMessage `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if ms := parseTimeMs(e.Timestamp); ms > intel.lastActiveMs {
			intel.lastActiveMs = ms
		}
		switch e.Type {
		case "session.start":
			if sid := jsonField(e.Data, "sessionId"); sid != "" {
				intel.SessionID = sid
			}
			if m := jsonField(e.Data, "selectedModel"); m != "" {
				intel.Model = m
			}
		case "user.message":
			if t := cleanPrompt(jsonField(e.Data, "content")); t != "" {
				if firstUser == "" {
					firstUser = t
				}
				lastUser = t
				intel.MessageCount++
			}
		case "assistant.message":
			var d struct {
				Content      string `json:"content"`
				ToolRequests []struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"toolRequests"`
			}
			_ = json.Unmarshal(e.Data, &d)
			if t := strings.TrimSpace(d.Content); t != "" {
				lastAssistant = t
				intel.MessageCount++
			}
			for _, tr := range d.ToolRequests {
				addCopilotFile(tr.Name, tr.Arguments, files)
				if tr.Name == "report_intent" {
					if iv := jsonField(tr.Arguments, "intent"); iv != "" {
						intent = iv
					}
				}
			}
		case "tool.execution_start":
			name := jsonField(e.Data, "toolName")
			var d struct {
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(e.Data, &d)
			addCopilotFile(name, d.Arguments, files)
			if name == "report_intent" {
				if iv := jsonField(d.Arguments, "intent"); iv != "" {
					intent = iv
				}
			}
		}
	}

	if intel.SessionID == "" {
		intel.SessionID = filepath.Base(filepath.Dir(path)) // the <id> dir
	}
	if intel.lastActiveMs == 0 {
		intel.lastActiveMs = mod.UnixMilli()
	}
	intel.InitialPrompt = firstUser
	intel.LastPrompt = lastUser
	intel.LastActivity = lastAssistant
	if intel.LastActivity == "" && intent != "" {
		intel.LastActivity = "intent: " + intent // Copilot's self-reported activity
	}
	intel.FilesTouched = files.slice()
	return intel, intel.MessageCount > 0
}

// addCopilotFile pulls a file path out of a Copilot tool call's arguments.
func addCopilotFile(name string, args json.RawMessage, set *orderedSet) {
	for _, k := range []string{"path", "file_path", "filePath", "filename"} {
		if v := jsonField(args, k); v != "" {
			set.add(v)
			return
		}
	}
}

// vscodeWorkspaceFolder reads workspace.json's folder URI and turns it into a
// filesystem path, so a Copilot session can show which project it belongs to.
func vscodeWorkspaceFolder(wsDir string) string {
	data, err := os.ReadFile(filepath.Join(wsDir, "workspace.json"))
	if err != nil {
		return ""
	}
	var w struct {
		Folder string `json:"folder"`
	}
	if json.Unmarshal(data, &w) != nil || w.Folder == "" {
		return ""
	}
	u, err := url.Parse(w.Folder)
	if err != nil {
		return ""
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		p = u.Path
	}
	p = strings.TrimPrefix(p, "/") // strip leading / before a Windows drive
	return filepath.FromSlash(p)
}
