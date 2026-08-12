// internal/detect/transcript.go — read agent session logs off disk and
// distil them into SessionIntel.
//
// Claude Code persists one JSONL file per session under
// ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl. Each line is a record:
// user turns, assistant turns (with token usage), and tool calls. Tool calls
// are gold — TodoWrite reveals the plan and remaining tasks, Edit/Write reveal
// touched files, Skill reveals loaded skills, and mcp__server__tool names
// reveal connected MCP servers. We parse tolerantly: unknown shapes are
// skipped, never fatal.

package detect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxTranscriptsPerProvider = 20

// recentTurnWindow bounds how many trailing user/assistant turns a session
// carries into a handoff. Enough to convey the current thread of work without
// shipping the entire transcript (which can be hundreds of turns).
const recentTurnWindow = 16

// tailTurns returns the last recentTurnWindow entries of turns (all of them when
// shorter). It copies, so the caller's backing array can be released.
func tailTurns(turns []Turn) []Turn {
	if len(turns) <= recentTurnWindow {
		if len(turns) == 0 {
			return nil
		}
		out := make([]Turn, len(turns))
		copy(out, turns)
		return out
	}
	out := make([]Turn, recentTurnWindow)
	copy(out, turns[len(turns)-recentTurnWindow:])
	return out
}

// ─── line schema (tolerant) ──────────────────────────────────────────────────

type rawLine struct {
	Type      string          `json:"type"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Role      string          `json:"role"`    // some logs put role at top level
	Content   json.RawMessage `json:"content"` // some logs put content at top level
	Message   *rawMessage     `json:"message"`
	Payload   *rawPayload     `json:"payload"` // Codex wraps role/content (+ session meta) here
}

// rawPayload is the Codex rollout wrapper: response_item lines carry
// {role, content} or {type:"function_call", name, arguments}; session_meta lines
// carry {cwd, id}; event_msg token_count lines carry usage under {info}.
type rawPayload struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Cwd       string          `json:"cwd"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`      // item subtype: message | function_call | reasoning
	Name      string          `json:"name"`      // function_call tool: shell | apply_patch | …
	Arguments json.RawMessage `json:"arguments"` // function_call args (often a JSON string)
}

type rawMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type todoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

var sysReminderRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// Slash-command invocations are logged as <command-name>/foo</command-name> …
// wrappers; strip them so the real prompt surfaces instead of command noise.
var commandTagRe = regexp.MustCompile(`(?s)<(command-message|command-name|command-args|command-contents|local-command-stdout|local-command-stderr)>.*?</(command-message|command-name|command-args|command-contents|local-command-stdout|local-command-stderr)>`)

// Codex prepends <environment_context>…; Cline appends <environment_details>… —
// machine-injected scaffolding, not the user's prompt. Strip so the real ask
// surfaces (a turn that becomes empty after stripping is skipped entirely).
var envContextRe = regexp.MustCompile(`(?s)<(environment_context|environment_details)>.*?</(environment_context|environment_details)>`)

// Cursor wraps the actual prompt in <user_query>…</user_query> and attaches
// context in <additional_data>/<user_info> blocks. Drop the context blocks
// wholesale, but only unwrap user_query/user_instructions (keep their text).
var cursorDropRe = regexp.MustCompile(`(?s)<(additional_data|user_info|attached_files)>.*?</(additional_data|user_info|attached_files)>`)
var cursorTagRe = regexp.MustCompile(`</?(user_query|user_instructions)>`)

// ─── Claude Code ──────────────────────────────────────────────────────────────

func scanClaudeTranscripts(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	root := filepath.Join(home, ".claude", "projects")
	return scanJSONLStore(root, maxAge, parseClaudeSession)
}

// scanJSONLStore walks a two-level store (root/<project>/<session>.jsonl),
// parses each recent file with parse, and returns the newest N sessions.
func scanJSONLStore(root string, maxAge time.Duration, parse func(path string, mod time.Time) (SessionIntel, bool)) []SessionIntel {
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)

	type cand struct {
		intel SessionIntel
		mod   time.Time
	}
	var cands []cand
	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		dir := filepath.Join(root, proj.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil || info.ModTime().Before(cutoff) {
				continue
			}
			intel, ok := parse(filepath.Join(dir, f.Name()), info.ModTime())
			if ok {
				cands = append(cands, cand{intel, info.ModTime()})
			}
		}
	}
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

func parseClaudeSession(path string, mod time.Time) (SessionIntel, bool) {
	f, err := os.Open(path)
	if err != nil {
		return SessionIntel{}, false
	}
	defer f.Close()

	intel := SessionIntel{TranscriptPath: path}
	skills := newOrderedSet()
	mcps := newOrderedSet()
	files := newOrderedSet()
	var firstUser, lastUser, lastAssistant string
	var latestTodos []todoItem
	var turns []Turn

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 32*1024*1024)
	for sc.Scan() {
		var rl rawLine
		if json.Unmarshal(sc.Bytes(), &rl) != nil {
			continue
		}
		if rl.Cwd != "" {
			intel.workDir = rl.Cwd
		}
		if rl.SessionID != "" {
			intel.SessionID = rl.SessionID
		}
		if ms := parseTimeMs(rl.Timestamp); ms > intel.lastActiveMs {
			intel.lastActiveMs = ms
		}

		role := rl.Role
		content := rl.Content
		var usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		}
		if rl.Message != nil {
			if rl.Message.Role != "" {
				role = rl.Message.Role
			}
			if rl.Message.Content != nil {
				content = rl.Message.Content
			}
			if rl.Message.Model != "" {
				intel.Model = rl.Message.Model
			}
			usage = rl.Message.Usage
		}

		text, blocks := decodeContent(content)

		switch role {
		case "user":
			t := cleanPrompt(text + joinText(blocks))
			if t == "" {
				continue // pure tool_result turn
			}
			if firstUser == "" {
				firstUser = t
			}
			lastUser = t
			turns = append(turns, Turn{Role: "user", Text: t})
			intel.MessageCount++
		case "assistant":
			intel.MessageCount++
			if usage != nil {
				intel.TokensIn += usage.InputTokens
				intel.TokensOut += usage.OutputTokens
			}
			if at := strings.TrimSpace(text + joinText(blocks)); at != "" {
				lastAssistant = at
				turns = append(turns, Turn{Role: "assistant", Text: at})
			}
			for _, b := range blocks {
				if b.Type != "tool_use" {
					continue
				}
				if server := mcpServer(b.Name); server != "" {
					mcps.add(server)
				}
				switch b.Name {
				case "Skill":
					if sk := jsonField(b.Input, "skill"); sk != "" {
						skills.add(sk)
					}
				case "Edit", "Write", "MultiEdit", "NotebookEdit":
					if fp := jsonField(b.Input, "file_path"); fp != "" {
						files.add(fp)
					}
				case "TodoWrite":
					if todos := parseTodos(b.Input); len(todos) > 0 {
						latestTodos = todos
					}
				}
			}
		}
	}

	if intel.SessionID == "" {
		intel.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if intel.lastActiveMs == 0 {
		intel.lastActiveMs = mod.UnixMilli()
	}
	intel.InitialPrompt = firstUser
	intel.LastPrompt = lastUser
	intel.LastActivity = lastAssistant
	intel.RecentTurns = tailTurns(turns)
	intel.Skills = skills.slice()
	intel.Mcps = mcps.slice()
	intel.FilesTouched = files.slice()
	for _, td := range latestTodos {
		label := td.Content
		if label == "" {
			label = td.ActiveForm
		}
		intel.Plan = append(intel.Plan, label)
		if td.Status != "completed" {
			intel.TasksRemaining = append(intel.TasksRemaining, label)
		}
	}

	return intel, intel.MessageCount > 0 || intel.SessionID != ""
}

// ─── Codex (best-effort generic) ──────────────────────────────────────────────

func scanCodexTranscripts(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	// Codex rollout logs nest by date: ~/.codex/sessions/<YYYY>/<MM>/<DD>/rollout-*.jsonl.
	// A fixed two-level walk misses them entirely, so walk the whole tree.
	root := filepath.Join(home, ".codex", "sessions")
	return scanJSONLTree(root, maxAge, parseGenericSession)
}

// scanJSONLTree walks an arbitrarily-nested store, parsing every recent *.jsonl
// file. Used for stores (like Codex) that bucket sessions by date rather than the
// flat root/<project>/<session>.jsonl layout scanJSONLStore assumes.
func scanJSONLTree(root string, maxAge time.Duration, parse func(path string, mod time.Time) (SessionIntel, bool)) []SessionIntel {
	cutoff := time.Now().Add(-maxAge)
	type cand struct {
		intel SessionIntel
		mod   time.Time
	}
	var cands []cand
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil //nolint:nilerr // tolerate unreadable subtrees
		}
		info, ierr := d.Info()
		if ierr != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		if intel, ok := parse(p, info.ModTime()); ok {
			cands = append(cands, cand{intel, info.ModTime()})
		}
		return nil
	})
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

// codexPatchFileRe pulls file paths out of an apply_patch payload's header lines.
var codexPatchFileRe = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

// codexFunctionFiles extracts touched files from a Codex function_call: apply_patch
// headers name the files directly; other tools are ignored (best-effort).
func codexFunctionFiles(name string, args json.RawMessage) []string {
	if len(args) == 0 {
		return nil
	}
	// Codex usually serialises arguments as a JSON string; unwrap if so.
	var s string
	if json.Unmarshal(args, &s) != nil {
		s = string(args)
	}
	var out []string
	for _, m := range codexPatchFileRe.FindAllStringSubmatch(s, -1) {
		if f := strings.TrimSpace(m[1]); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// scanCumulativeUsage pulls cumulative token usage from a Codex token_count line,
// tolerant of the {payload:{info:{total_token_usage|input_tokens}}} shapes seen
// across versions. Returns ok=false when no usage is present.
func scanCumulativeUsage(raw []byte) (int64, int64, bool) {
	if !bytes.Contains(raw, []byte("input_tokens")) && !bytes.Contains(raw, []byte("output_tokens")) {
		return 0, 0, false
	}
	var probe struct {
		Payload struct {
			Info struct {
				Total struct {
					In  int64 `json:"input_tokens"`
					Out int64 `json:"output_tokens"`
				} `json:"total_token_usage"`
				In  int64 `json:"input_tokens"`
				Out int64 `json:"output_tokens"`
			} `json:"info"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return 0, 0, false
	}
	if probe.Payload.Info.Total.In > 0 || probe.Payload.Info.Total.Out > 0 {
		return probe.Payload.Info.Total.In, probe.Payload.Info.Total.Out, true
	}
	if probe.Payload.Info.In > 0 || probe.Payload.Info.Out > 0 {
		return probe.Payload.Info.In, probe.Payload.Info.Out, true
	}
	return 0, 0, false
}

// parseGenericSession handles the common {role,content} / {message:{role,content}}
// JSONL shape without provider-specific tool semantics.
func parseGenericSession(path string, mod time.Time) (SessionIntel, bool) {
	f, err := os.Open(path)
	if err != nil {
		return SessionIntel{}, false
	}
	defer f.Close()

	intel := SessionIntel{TranscriptPath: path, SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	files := newOrderedSet()
	var firstUser, lastUser, lastAssistant string
	var turns []Turn

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)
	for sc.Scan() {
		var rl rawLine
		if json.Unmarshal(sc.Bytes(), &rl) != nil {
			continue
		}
		if rl.Cwd != "" {
			intel.workDir = rl.Cwd
		}
		if ms := parseTimeMs(rl.Timestamp); ms > intel.lastActiveMs {
			intel.lastActiveMs = ms
		}
		role := rl.Role
		content := rl.Content
		if rl.Message != nil {
			if rl.Message.Role != "" {
				role = rl.Message.Role
			}
			if rl.Message.Content != nil {
				content = rl.Message.Content
			}
		}
		// Codex rollout: role/content (and session meta) live under payload.
		if rl.Payload != nil {
			if rl.Payload.Role != "" {
				role = rl.Payload.Role
			}
			if rl.Payload.Content != nil {
				content = rl.Payload.Content
			}
			if rl.Payload.Cwd != "" {
				intel.workDir = rl.Payload.Cwd
			}
			if rl.Payload.ID != "" {
				intel.SessionID = rl.Payload.ID
			}
			// Codex edits arrive as apply_patch function calls, not Edit tool_use.
			if rl.Payload.Type == "function_call" {
				for _, fp := range codexFunctionFiles(rl.Payload.Name, rl.Payload.Arguments) {
					files.add(fp)
				}
			}
		}
		// Cumulative token usage (Codex token_count events; harmless elsewhere).
		if in, out, ok := scanCumulativeUsage(sc.Bytes()); ok {
			intel.TokensIn, intel.TokensOut = in, out
		}
		text, blocks := decodeContent(content)
		t := cleanPrompt(text + joinText(blocks))
		switch role {
		case "user":
			if t == "" {
				continue
			}
			if firstUser == "" {
				firstUser = t
			}
			lastUser = t
			turns = append(turns, Turn{Role: "user", Text: t})
			intel.MessageCount++
		case "assistant":
			intel.MessageCount++
			if t != "" {
				lastAssistant = t
				turns = append(turns, Turn{Role: "assistant", Text: t})
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
	if intel.lastActiveMs == 0 {
		intel.lastActiveMs = mod.UnixMilli()
	}
	intel.InitialPrompt = firstUser
	intel.LastPrompt = lastUser
	intel.LastActivity = lastAssistant
	intel.RecentTurns = tailTurns(turns)
	intel.FilesTouched = files.slice()
	return intel, intel.MessageCount > 0
}

// ─── decoding helpers ─────────────────────────────────────────────────────────

// decodeContent handles content that is either a JSON string or an array of
// typed blocks.
func decodeContent(raw json.RawMessage) (string, []contentBlock) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return "", nil
	}
	switch t[0] {
	case '"':
		var s string
		_ = json.Unmarshal(t, &s)
		return s, nil
	case '[':
		var blocks []contentBlock
		_ = json.Unmarshal(t, &blocks)
		return "", blocks
	}
	return "", nil
}

func joinText(blocks []contentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		// "text" = Anthropic; input_text/output_text = OpenAI/Codex content blocks.
		if (blk.Type == "text" || blk.Type == "input_text" || blk.Type == "output_text") && blk.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

func cleanPrompt(s string) string {
	s = sysReminderRe.ReplaceAllString(s, "")
	s = commandTagRe.ReplaceAllString(s, "")
	s = envContextRe.ReplaceAllString(s, "")
	s = cursorDropRe.ReplaceAllString(s, "")
	s = cursorTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func mcpServer(toolName string) string {
	if !strings.HasPrefix(toolName, "mcp__") {
		return ""
	}
	parts := strings.SplitN(strings.TrimPrefix(toolName, "mcp__"), "__", 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

func jsonField(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return ""
}

func parseTodos(raw json.RawMessage) []todoItem {
	var wrap struct {
		Todos []todoItem `json:"todos"`
	}
	if json.Unmarshal(raw, &wrap) != nil {
		return nil
	}
	return wrap.Todos
}

func parseTimeMs(ts string) int64 {
	if ts == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// ─── ordered set ──────────────────────────────────────────────────────────────

type orderedSet struct {
	seen map[string]bool
	list []string
}

func newOrderedSet() *orderedSet { return &orderedSet{seen: map[string]bool{}} }

func (o *orderedSet) add(v string) {
	v = strings.TrimSpace(v)
	if v == "" || o.seen[v] {
		return
	}
	o.seen[v] = true
	o.list = append(o.list, v)
}

func (o *orderedSet) slice() []string {
	if len(o.list) == 0 {
		return nil
	}
	return o.list
}
