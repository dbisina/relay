// internal/adapter/stub.go
//
// CLIAdapter — robust base for CLI-based providers (Codex, Antigravity, OpenCode,
// Copilot, Continue, Cline). It aims for parity with the Claude adapter:
//   - optional structured line parser (per-provider JSON event schema)
//   - generic token/usage extraction
//   - git-commit safe-pause synthesis (the canonical clean handoff point)
//   - stdin replies (StdinReplier) so any CLI provider can answer a prompt
//   - orchestrator-signalled idle safe-pause as a fallback
//
// Each provider embeds CLIAdapter via a New*Adapter constructor that sets the
// binary, args, injection semantics, and (where the CLI emits structured output)
// a line parser. Providers without structured output degrade to plain text plus
// a best-effort token parser.

package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/dbisina/relay/internal/retry"
	"sync"
	"time"

	"github.com/dbisina/relay/internal/process"
)

const (
	cliSafePauseTimeout = 45 * time.Second
	cliLineBufferSize   = 1 * 1024 * 1024 // 1 MB — agents emit large JSON lines
)

// gitCommitRe matches a git commit invocation anywhere in a line. A completed
// commit means the working tree is clean and the index is stable — the safest
// place to snapshot and hand off, identical to the Claude adapter's heuristic.
var gitCommitRe = regexp.MustCompile(`(?i)git\s+commit`)

// TokenParser extracts (input, output) token counts from a single output line.
// Returns ok=true when the line carries usage info. nil = unsupported.
type TokenParser func(line string) (in, out int64, ok bool)

// LineParser converts one structured output line into an AgentEvent. Returns
// ok=false to fall through to plain-text handling. nil = provider emits no
// structured events.
type LineParser func(line string) (AgentEvent, bool)

// CLIAdapter is a generic adapter for CLI-based providers.
type CLIAdapter struct {
	name          ProviderName
	binary        string
	buildArgs     func(opts RunOptions) []string
	injectSem     InjectionSemantics
	tokenParser   TokenParser
	lineParser    LineParser
	supportsTools bool
	model         string // populated by SetModel before Run

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	safePauseCh chan SafePoint
}

func newCLIAdapter(name ProviderName, binary string, injectSem InjectionSemantics,
	buildArgs func(opts RunOptions) []string) *CLIAdapter {
	return &CLIAdapter{
		name:        name,
		binary:      binary,
		buildArgs:   buildArgs,
		injectSem:   injectSem,
		safePauseCh: make(chan SafePoint, 1),
	}
}

// SetModel sets the model identifier the adapter will pass to its CLI.
// Empty means the CLI default.
func (a *CLIAdapter) SetModel(m string) {
	a.mu.Lock()
	a.model = m
	a.mu.Unlock()
}

// Model returns the configured model name (empty = CLI default).
func (a *CLIAdapter) Model() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// withTokenParser wires a per-line usage extractor.
func (a *CLIAdapter) withTokenParser(p TokenParser) *CLIAdapter {
	a.tokenParser = p
	return a
}

// withLineParser wires a structured event parser (provider JSON schema).
func (a *CLIAdapter) withLineParser(p LineParser) *CLIAdapter {
	a.lineParser = p
	return a
}

// withTools marks the provider as tool-capable in its Capability().
func (a *CLIAdapter) withTools() *CLIAdapter {
	a.supportsTools = true
	return a
}

func (a *CLIAdapter) Capability() ProviderCapability {
	return ProviderCapability{
		Name:                a.name,
		InjectionSemantics:  a.injectSem,
		MaxTokensPerSession: 128_000,
		SupportsStreaming:   true,
		SupportsTools:       a.supportsTools,
	}
}

func (a *CLIAdapter) Run(ctx context.Context, opts RunOptions, ch chan<- AgentEvent) error {
	args := a.buildArgs(opts)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = append(os.Environ(), opts.Env...)

	if err := process.SetupChildProcess(cmd); err != nil {
		return fmt.Errorf("%s: SetupChildProcess: %w", a.name, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%s: stdout pipe: %w", a.name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%s: stderr pipe: %w", a.name, err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%s: stdin pipe: %w", a.name, err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start binary %q: %w", a.name, a.binary, err)
	}
	if err := process.AssignToJobObject(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] job object warn: %v\n", a.name, err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.stdin = stdin
	a.mu.Unlock()

	go io.Copy(io.Discard, stderr)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, cliLineBufferSize), cliLineBufferSize)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			ev := a.toEvent(line)
			a.checkSafePause(ev, line)
			if sig := retry.Detect(line, time.Now()); sig != nil {
				meta := map[string]string{"reason": string(sig.Reason)}
				if sig.ResetAt != nil {
					meta["reset_at"] = sig.ResetAt.Format(time.RFC3339)
				}
				select {
				case ch <- AgentEvent{Type: EventRetry, Content: line, Meta: meta}:
				case <-ctx.Done():
					return
				}
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		_ = cmd.Wait()
	}()

	return nil
}

// toEvent turns one output line into an AgentEvent, preferring a structured
// parser, then a token parser, then plain text.
func (a *CLIAdapter) toEvent(line string) AgentEvent {
	if a.lineParser != nil {
		if ev, ok := a.lineParser(line); ok {
			return ev
		}
	}
	if a.tokenParser != nil {
		if in, out, ok := a.tokenParser(line); ok {
			return AgentEvent{
				Type:    EventToolResult, // ToolResult so the orchestrator's addUsage picks it up
				Content: line,
				Meta: map[string]string{
					"tokens_in":  fmt.Sprintf("%d", in),
					"tokens_out": fmt.Sprintf("%d", out),
				},
			}
		}
	}
	return AgentEvent{Type: EventText, Content: line}
}

// checkSafePause synthesises a safe_point when a git commit is observed — the
// working tree is clean and the index stable. Scans both the structured event
// (tool input/content) and the raw line so it works regardless of the exact
// JSON schema a provider uses.
func (a *CLIAdapter) checkSafePause(ev AgentEvent, line string) {
	hit := gitCommitRe.MatchString(line)
	if !hit && ev.Meta != nil {
		hit = gitCommitRe.MatchString(ev.Meta["input"]) || gitCommitRe.MatchString(ev.Content)
	}
	if !hit {
		return
	}
	sp := SafePoint{
		Message:   string(a.name) + ": post-commit safe point",
		Timestamp: time.Now().UnixMilli(),
	}
	select {
	case a.safePauseCh <- sp:
	default:
	}
}

// AwaitSafePauseWindow waits for a buffered safe point (a git commit, or an
// orchestrator-signalled idle) or times out.
func (a *CLIAdapter) AwaitSafePauseWindow(ctx context.Context, _ string) (SafePoint, error) {
	t := time.NewTimer(cliSafePauseTimeout)
	defer t.Stop()
	select {
	case sp := <-a.safePauseCh:
		return sp, nil
	case <-t.C:
		return SafePoint{}, &ErrSafePauseTimeout{Provider: a.name, After: cliSafePauseTimeout.String()}
	case <-ctx.Done():
		return SafePoint{}, ctx.Err()
	}
}

// SignalSafePause is called by the Orchestrator when it detects the provider is
// idle (e.g. waiting for user input).
func (a *CLIAdapter) SignalSafePause() {
	sp := SafePoint{
		Message:   string(a.name) + ": orchestrator-detected idle",
		Timestamp: time.Now().UnixMilli(),
	}
	select {
	case a.safePauseCh <- sp:
	default:
	}
}

// SendStdin writes a reply to the running process's stdin (StdinReplier). This
// gives every CLI provider the reply capability the Claude adapter has.
func (a *CLIAdapter) SendStdin(reply string) error {
	a.mu.Lock()
	w := a.stdin
	a.mu.Unlock()
	if w == nil {
		return fmt.Errorf("%s: no active stdin (process not running?)", a.name)
	}
	if !strings.HasSuffix(reply, "\n") {
		reply += "\n"
	}
	_, err := io.WriteString(w, reply)
	return err
}

func (a *CLIAdapter) ForceStop() error {
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// ── Contract injection helper ────────────────────────────────────────────────

// prependContract returns the task with the continuation contract prepended, for
// CLIs that have no system-prompt-file flag. The contract leads so its primacy
// (DO NOT REDO) survives.
func prependContract(opts RunOptions) string {
	if opts.SystemPromptFile == "" {
		return opts.Task
	}
	data, err := os.ReadFile(opts.SystemPromptFile)
	if err != nil || len(data) == 0 {
		return opts.Task
	}
	return string(data) + "\n\n---\n\nTask: " + opts.Task
}

// ── Generic token parsers ───────────────────────────────────────────────────

// parseStreamJSONUsage finds a top-level `usage` object with token counts in
// either Anthropic (input_tokens/output_tokens) or OpenAI
// (prompt_tokens/completion_tokens) shape.
func parseStreamJSONUsage(line string) (int64, int64, bool) {
	if !strings.Contains(line, `"usage"`) && !strings.Contains(line, `"input_tokens"`) {
		return 0, 0, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return 0, 0, false
	}
	usageRaw, ok := raw["usage"]
	if !ok {
		return 0, 0, false
	}
	var u struct {
		InputTokens          int64 `json:"input_tokens"`
		OutputTokens         int64 `json:"output_tokens"`
		PromptTokens         int64 `json:"prompt_tokens"`     // OpenAI shape
		CompletionTokens     int64 `json:"completion_tokens"` // OpenAI shape
		CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
	}
	if err := json.Unmarshal(usageRaw, &u); err != nil {
		return 0, 0, false
	}
	in := u.InputTokens + u.CacheReadInputTokens + u.PromptTokens
	out := u.OutputTokens + u.CompletionTokens
	if in == 0 && out == 0 {
		return 0, 0, false
	}
	return in, out, true
}

// parseCodexLine parses one JSONL event from `codex exec --json`. Event schema
// (verified against developers.openai.com/codex/noninteractive):
//   - turn.completed → usage{input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens}
//   - item.completed → item{type, text, command, ...}; type "agent_message" carries text
//   - turn.failed / error → error event
func parseCodexLine(line string) (AgentEvent, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return AgentEvent{}, false
	}
	var typ string
	if t, ok := raw["type"]; ok {
		_ = json.Unmarshal(t, &typ)
	}

	switch typ {
	case "turn.completed":
		var t struct {
			Usage struct {
				InputTokens           int64 `json:"input_tokens"`
				CachedInputTokens     int64 `json:"cached_input_tokens"`
				OutputTokens          int64 `json:"output_tokens"`
				ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal([]byte(line), &t)
		ev := AgentEvent{Type: EventToolResult, Content: "turn.completed", Meta: map[string]string{}}
		if in := t.Usage.InputTokens + t.Usage.CachedInputTokens; in > 0 {
			ev.Meta["tokens_in"] = fmt.Sprintf("%d", in)
		}
		if out := t.Usage.OutputTokens + t.Usage.ReasoningOutputTokens; out > 0 {
			ev.Meta["tokens_out"] = fmt.Sprintf("%d", out)
		}
		return ev, true

	case "item.completed", "item.started":
		var it struct {
			Item struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Command string `json:"command"`
			} `json:"item"`
		}
		_ = json.Unmarshal([]byte(line), &it)
		switch it.Item.Type {
		case "agent_message", "assistant_message":
			if strings.TrimSpace(it.Item.Text) != "" {
				return AgentEvent{Type: EventText, Content: it.Item.Text}, true
			}
		case "command_execution", "local_shell_call", "command":
			cmd := it.Item.Command
			if cmd == "" {
				cmd = line
			}
			return AgentEvent{Type: EventToolUse, Content: "command", Meta: map[string]string{"input": cmd}}, true
		case "file_change", "patch", "apply_patch", "file_update":
			return AgentEvent{Type: EventToolUse, Content: "edit", Meta: map[string]string{"input": line}}, true
		case "mcp_tool_call":
			return AgentEvent{Type: EventToolUse, Content: "mcp", Meta: map[string]string{"input": line}}, true
		}
		return AgentEvent{Type: EventSystem, Content: typ}, true

	case "turn.failed", "error":
		return AgentEvent{Type: EventError, Content: line}, true

	case "thread.started", "turn.started":
		return AgentEvent{Type: EventSystem, Content: typ}, true
	}
	return AgentEvent{}, false
}

// ── Provider-specific constructors ──────────────────────────────────────────

// NewCodexAdapter creates an OpenAI Codex CLI adapter.
//
// Non-interactive invocation (verified against developers.openai.com/codex):
//
//	codex exec --json --sandbox workspace-write --skip-git-repo-check [--model m] "<task>"
//
// `exec` is the headless subcommand; `--json` emits a JSONL event stream;
// workspace-write lets it edit files in the working tree without prompting;
// skip-git-repo-check tolerates a non-repo working dir. Codex has no
// system-prompt flag, so the contract is prepended to the task.
func NewCodexAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderCodex, "codex", InjectionSystemLayer, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{"exec", "--json", "--sandbox", "workspace-write", "--skip-git-repo-check"}
		if m := a.Model(); m != "" {
			args = append(args, "--model", m)
		}
		args = append(args, prependContract(opts))
		return args
	}
	return a.withLineParser(parseCodexLine).withTools()
}

// NewAntigravityAdapter creates an Antigravity (agy) CLI adapter.
//
// agy v1.x flags (verified against `agy --help`):
//
//	-p / --print            single-prompt non-interactive mode
//	--add-dir <dir>         add a directory to the workspace
//	--dangerously-skip-permissions   auto-approve tool calls (required headless)
//	--print-timeout <dur>   how long to wait in print mode
//
// agy has no system-prompt-file flag, so the contract is prepended to the task.
func NewAntigravityAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderAntigravity, "agy", InjectionUserTurn, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{
			"-p", prependContract(opts),
			"--add-dir", opts.WorkDir,
			"--dangerously-skip-permissions",
			"--print-timeout", "10m",
		}
		if m := a.Model(); m != "" {
			args = append(args, "--model", m)
		}
		return args
	}
	return a.withTokenParser(parseStreamJSONUsage).withTools()
}

// NewOpenCodeAdapter creates an OpenCode CLI adapter.
func NewOpenCodeAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderOpenCode, "opencode", InjectionSystemLayer, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{"run", "--no-interactive"}
		if m := a.Model(); m != "" {
			args = append(args, "--model", m)
		}
		if opts.SystemPromptFile != "" {
			args = append(args, "--system-file", opts.SystemPromptFile)
		}
		args = append(args, opts.Task)
		return args
	}
	return a.withTokenParser(parseStreamJSONUsage)
}

// NewCopilotAdapter creates a GitHub Copilot CLI adapter.
func NewCopilotAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderCopilot, "gh", InjectionUserTurn, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{"copilot", "suggest", "--target", "shell"}
		if opts.SystemPromptFile != "" {
			args = append(args, "--context", opts.SystemPromptFile)
		}
		args = append(args, opts.Task)
		return args
	}
	return a
}

// NewContinueAdapter creates a Continue CLI adapter.
func NewContinueAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderContinue, "continue", InjectionSystemLayer, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{"task"}
		if m := a.Model(); m != "" {
			args = append(args, "--model", m)
		}
		if opts.SystemPromptFile != "" {
			args = append(args, "--system", opts.SystemPromptFile)
		}
		args = append(args, opts.Task)
		return args
	}
	return a.withTokenParser(parseStreamJSONUsage)
}

// NewClineAdapter creates a Cline CLI adapter.
func NewClineAdapter() *CLIAdapter {
	a := newCLIAdapter(ProviderCline, "cline", InjectionSystemLayer, nil)
	a.buildArgs = func(opts RunOptions) []string {
		args := []string{"run", "--auto-approve"}
		if m := a.Model(); m != "" {
			args = append(args, "--model", m)
		}
		if opts.SystemPromptFile != "" {
			args = append(args, "--system-prompt", opts.SystemPromptFile)
		}
		args = append(args, opts.Task)
		return args
	}
	return a.withTokenParser(parseStreamJSONUsage)
}
