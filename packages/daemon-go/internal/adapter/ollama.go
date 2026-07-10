// internal/adapter/ollama.go
//
// Ollama adapter — HTTP to localhost:11434.
// POST /api/chat with stream:true → NDJSON response.
// "Quota" = context window tokens; fetched from /api/show.
// No CLI subprocess; runs entirely via HTTP.

package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	ollamaDefaultBase      = "http://localhost:11434"
	ollamaSafePauseTimeout = 30 * time.Second
)

// OllamaAdapter implements AdapterContract for local Ollama models.
type OllamaAdapter struct {
	baseURL string
	model   string

	mu          sync.Mutex
	cancelFn    context.CancelFunc
	safePauseCh chan SafePoint
}

// NewOllamaAdapter creates an OllamaAdapter.
// baseURL defaults to http://localhost:11434 if empty.
// model defaults to "qwen2.5-coder:32b" if empty.
func NewOllamaAdapter(baseURL, model string) *OllamaAdapter {
	if baseURL == "" {
		baseURL = ollamaDefaultBase
	}
	if model == "" {
		model = "qwen2.5-coder:32b"
	}
	return &OllamaAdapter{
		baseURL:     baseURL,
		model:       model,
		safePauseCh: make(chan SafePoint, 1),
	}
}

// SetModel overrides the model used for subsequent /api/chat requests.
// Empty leaves the constructor default in place.
func (a *OllamaAdapter) SetModel(m string) {
	a.mu.Lock()
	if m != "" {
		a.model = m
	}
	a.mu.Unlock()
}

// Model returns the configured model name.
func (a *OllamaAdapter) Model() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

func (a *OllamaAdapter) Capability() ProviderCapability {
	return ProviderCapability{
		Name:                ProviderOllama,
		InjectionSemantics:  InjectionSystemLayer,
		MaxTokensPerSession: 0, // determined at runtime from /api/show
		SupportsStreaming:   true,
		SupportsTools:       false,
	}
}

func (a *OllamaAdapter) Run(ctx context.Context, opts RunOptions, ch chan<- AgentEvent) error {
	ctx2, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancelFn = cancel
	a.mu.Unlock()

	messages := []map[string]string{}
	if opts.SystemPromptFile != "" {
		data, err := os.ReadFile(opts.SystemPromptFile)
		if err == nil {
			messages = append(messages, map[string]string{
				"role":    "system",
				"content": string(data),
			})
		}
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": opts.Task,
	})

	body, err := json.Marshal(map[string]interface{}{
		"model":    a.model,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx2, http.MethodPost, a.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		cancel()
		return fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("ollama: POST /api/chat: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("ollama: HTTP %d from /api/chat", resp.StatusCode)
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()
		defer cancel()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			ev := a.parseLine(line)
			select {
			case ch <- ev:
			case <-ctx2.Done():
				return
			}
		}
	}()

	return nil
}

func (a *OllamaAdapter) parseLine(line string) AgentEvent {
	var raw struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done            bool  `json:"done"`
		PromptEvalCount int64 `json:"prompt_eval_count"`
		EvalCount       int64 `json:"eval_count"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return AgentEvent{Type: EventText, Content: line}
	}
	if raw.Done {
		// Final frame has usage counts. Emit as ToolResult so
		// orchestrator.addUsage picks tokens_in/tokens_out from Meta.
		ev := AgentEvent{
			Type:    EventToolResult,
			Content: "done",
			Meta:    map[string]string{},
		}
		if raw.PromptEvalCount > 0 {
			ev.Meta["tokens_in"] = fmt.Sprintf("%d", raw.PromptEvalCount)
		}
		if raw.EvalCount > 0 {
			ev.Meta["tokens_out"] = fmt.Sprintf("%d", raw.EvalCount)
		}
		return ev
	}
	return AgentEvent{Type: EventText, Content: raw.Message.Content}
}

func (a *OllamaAdapter) AwaitSafePauseWindow(ctx context.Context, _ string) (SafePoint, error) {
	// Ollama has no tool-use hooks; safe-pause is immediate (HTTP, stateless).
	return SafePoint{
		Message:   "ollama: stateless http — safe point immediate",
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (a *OllamaAdapter) ForceStop() error {
	a.mu.Lock()
	fn := a.cancelFn
	a.mu.Unlock()
	if fn != nil {
		fn()
	}
	return nil
}
