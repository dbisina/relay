// internal/adapter/claude.go
//
// Claude Code adapter.
//
// Invocation:
//   claude -p "<task>" --output-format stream-json --verbose
//           [--append-system-prompt-file <contract.md>]
//
// Quota Layer-1: local proxy via ANTHROPIC_BASE_URL env var.
// Streams NDJSON on stdout; each line is a claude stream-json event.
// Safe-pause: synthesised on tool_result where input matches git commit.

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
	"sync"
	"time"

	"github.com/dbisina/relay/internal/process"
)

const (
	claudeSafePauseTimeout = 60 * time.Second
	claudeLineBufferSize   = 1 * 1024 * 1024 // 1 MB
)

var claudeGitCommitRe = regexp.MustCompile(`(?i)git\s+commit`)

// ClaudeAdapter implements AdapterContract for Claude Code CLI.
type ClaudeAdapter struct {
	proxyPort int

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser // populated after Run; used by SendStdin
	safePauseCh chan SafePoint // buffered(1)
	model       string         // optional --model override (empty = CLI default)
}

// SetModel selects the model passed to `claude --model <name>`.
// Empty string leaves the CLI default in place.
func (a *ClaudeAdapter) SetModel(m string) {
	a.mu.Lock()
	a.model = m
	a.mu.Unlock()
}

// Model returns the configured model name (empty = CLI default).
func (a *ClaudeAdapter) Model() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// NewClaudeAdapter creates a ClaudeAdapter.
// proxyPort: the port of the local Claude proxy server (0 = no proxy).
func NewClaudeAdapter(proxyPort int) *ClaudeAdapter {
	return &ClaudeAdapter{
		proxyPort:   proxyPort,
		safePauseCh: make(chan SafePoint, 1),
	}
}

func (a *ClaudeAdapter) Capability() ProviderCapability {
	return ProviderCapability{
		Name:                ProviderClaude,
		InjectionSemantics:  InjectionSystemLayer,
		MaxTokensPerSession: 200_000,
		SupportsStreaming:   true,
		SupportsTools:       true,
	}
}

func (a *ClaudeAdapter) Run(ctx context.Context, opts RunOptions, ch chan<- AgentEvent) error {
	// Claude Code v2.x flags:
	//   -p <task>                  prompt mode (non-interactive)
	//   --output-format stream-json  NDJSON per event on stdout
	//   --append-system-prompt-file  appends our contract to the system prompt
	// --verbose was removed in some 2.x builds and breaks stream-json; omit it.
	args := []string{
		"-p", opts.Task,
		"--output-format", "stream-json",
	}
	if m := a.Model(); m != "" {
		args = append(args, "--model", m)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = opts.WorkDir

	// Environment: inherit + proxy override
	env := os.Environ()
	if a.proxyPort > 0 {
		env = append(env, fmt.Sprintf("ANTHROPIC_BASE_URL=http://127.0.0.1:%d", a.proxyPort))
	}
	cmd.Env = append(env, opts.Env...)

	// Windows Job Object / Unix process group setup
	if err := process.SetupChildProcess(cmd); err != nil {
		return fmt.Errorf("claude: SetupChildProcess: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claude: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("claude: stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("claude: stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claude: start: %w", err)
	}

	a.mu.Lock()
	a.stdin = stdin
	a.mu.Unlock()

	// Windows: assign to Job Object after start (best-effort)
	if err := process.AssignToJobObject(cmd); err != nil {
		// Non-fatal — log to stderr
		fmt.Fprintf(os.Stderr, "[ClaudeAdapter] job object warn: %v\n", err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()

	// Drain stderr silently (errors come via stream-json)
	go io.Copy(io.Discard, stderr)

	// Stream stdout NDJSON → ch
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, claudeLineBufferSize), claudeLineBufferSize)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			ev := a.parseLine(line)
			a.checkSafePause(ev)
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

// parseLine converts one NDJSON line from claude --output-format stream-json.
func (a *ClaudeAdapter) parseLine(line string) AgentEvent {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return AgentEvent{Type: EventText, Content: line}
	}

	var evType string
	if t, ok := raw["type"]; ok {
		_ = json.Unmarshal(t, &evType)
	}

	switch evType {
	case "tool_use":
		var name string
		if n, ok := raw["name"]; ok {
			_ = json.Unmarshal(n, &name)
		}
		input := "{}"
		if inp, ok := raw["input"]; ok {
			input = string(inp)
		}
		return AgentEvent{
			Type:    EventToolUse,
			Content: name,
			Meta:    map[string]string{"input": input},
		}

	case "tool_result":
		var content string
		if c, ok := raw["content"]; ok {
			// content may be string or array
			if err := json.Unmarshal(c, &content); err != nil {
				content = string(c)
			}
		}
		input := "{}"
		if inp, ok := raw["input"]; ok {
			input = string(inp)
		}
		return AgentEvent{
			Type:    EventToolResult,
			Content: content,
			Meta:    map[string]string{"input": input},
		}

	case "assistant":
		// Extract text from content array
		var msg struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if msgRaw, ok := raw["message"]; ok {
			_ = json.Unmarshal(msgRaw, &msg)
		}
		for _, c := range msg.Content {
			if c.Type == "text" && c.Text != "" {
				return AgentEvent{Type: EventText, Content: c.Text}
			}
		}
		return AgentEvent{Type: EventSystem, Content: line}

	case "result":
		// Final result event from `claude -p`. Per Anthropic stream-json spec,
		// includes a `usage` object with input_tokens + output_tokens that we
		// surface in Meta so the orchestrator's addUsage() can record spend.
		var result struct {
			Subtype string `json:"subtype"`
			Result  string `json:"result"`
			Usage   struct {
				InputTokens          int64 `json:"input_tokens"`
				OutputTokens         int64 `json:"output_tokens"`
				CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		_ = json.Unmarshal([]byte(line), &result)
		ev := AgentEvent{
			Type:    EventToolResult, // ToolResult so orchestrator.addUsage picks it up
			Content: "result:" + result.Subtype + " " + result.Result,
			Meta:    map[string]string{},
		}
		if result.Usage.InputTokens > 0 {
			ev.Meta["tokens_in"] = fmt.Sprintf("%d", result.Usage.InputTokens+result.Usage.CacheReadInputTokens)
		}
		if result.Usage.OutputTokens > 0 {
			ev.Meta["tokens_out"] = fmt.Sprintf("%d", result.Usage.OutputTokens)
		}
		if result.TotalCostUSD > 0 {
			ev.Meta["cost_usd"] = fmt.Sprintf("%.6f", result.TotalCostUSD)
		}
		return ev

	case "system":
		var sys struct {
			Subtype string `json:"subtype"`
		}
		_ = json.Unmarshal([]byte(line), &sys)
		return AgentEvent{Type: EventSystem, Content: "system:" + sys.Subtype}

	default:
		return AgentEvent{Type: EventSystem, Content: line}
	}
}

// checkSafePause synthesises a safe_point event when a tool_result is
// observed for a git commit operation. This is the canonical safe pause
// location: commit is done, working tree is clean, index is stable.
func (a *ClaudeAdapter) checkSafePause(ev AgentEvent) {
	if ev.Type != EventToolResult {
		return
	}
	if !claudeGitCommitRe.MatchString(ev.Meta["input"]) {
		return
	}
	sp := SafePoint{
		Message:   "post-commit tool_result (claude)",
		Timestamp: time.Now().UnixMilli(),
	}
	select {
	case a.safePauseCh <- sp:
	default:
		// Channel already has a pending safe point — fine.
	}
}

func (a *ClaudeAdapter) AwaitSafePauseWindow(ctx context.Context, breachReason string) (SafePoint, error) {
	t := time.NewTimer(claudeSafePauseTimeout)
	defer t.Stop()

	select {
	case sp := <-a.safePauseCh:
		return sp, nil
	case <-t.C:
		return SafePoint{}, &ErrSafePauseTimeout{
			Provider: ProviderClaude,
			After:    claudeSafePauseTimeout.String(),
		}
	case <-ctx.Done():
		return SafePoint{}, ctx.Err()
	}
}

func (a *ClaudeAdapter) ForceStop() error {
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// SendStdin writes a reply to the running claude process's stdin.
// Appends newline if not present so the agent's readline picks it up.
func (a *ClaudeAdapter) SendStdin(reply string) error {
	a.mu.Lock()
	w := a.stdin
	a.mu.Unlock()
	if w == nil {
		return fmt.Errorf("claude: no active stdin (process not running?)")
	}
	if !strings.HasSuffix(reply, "\n") {
		reply += "\n"
	}
	_, err := io.WriteString(w, reply)
	return err
}
