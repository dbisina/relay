// internal/adapter/interface.go
//
// AdapterContract — the interface every provider adapter must implement.
// Designed around Windows correctness: AwaitSafePauseWindow is a GATE
// (blocks until safe), not a predicate (closes the buffered-breach race).

package adapter

import "context"

// ProviderName identifies an AI coding agent provider.
type ProviderName string

const (
	ProviderClaude      ProviderName = "claude"
	ProviderCodex       ProviderName = "codex"
	ProviderAntigravity ProviderName = "antigravity"
	ProviderOpenCode    ProviderName = "opencode"
	ProviderOllama      ProviderName = "ollama"
	ProviderCopilot     ProviderName = "copilot"
	ProviderContinue    ProviderName = "continue"
	ProviderCline       ProviderName = "cline"
)

// InjectionSemantics controls how the continuation contract is injected.
type InjectionSemantics string

const (
	InjectionSystemLayer InjectionSemantics = "SYSTEM_LAYER"
	InjectionUserTurn    InjectionSemantics = "USER_TURN"
)

// ProviderCapability describes static metadata for a provider.
type ProviderCapability struct {
	Name                ProviderName
	InjectionSemantics  InjectionSemantics
	MaxTokensPerSession int
	SupportsStreaming   bool
	SupportsTools       bool
}

// AgentEventType enumerates structured event types.
type AgentEventType string

const (
	EventToolUse    AgentEventType = "tool_use"
	EventToolResult AgentEventType = "tool_result"
	EventText       AgentEventType = "text"
	EventSystem     AgentEventType = "system"
	EventSafePoint  AgentEventType = "safe_point"
	EventQuota      AgentEventType = "quota"
	EventError      AgentEventType = "error"
)

// AgentEvent is a parsed event from the agent's stdout stream.
type AgentEvent struct {
	Type    AgentEventType
	Content string
	Meta    map[string]string // tool name, input JSON, etc.
}

// SafePoint marks a confirmed safe-pause location in the agent's execution.
// CommitSHA is populated by DurabilityManager after snapshot.
type SafePoint struct {
	CommitSHA string
	Message   string
	Timestamp int64 // Unix milliseconds
}

// RunOptions configures how the agent is invoked.
type RunOptions struct {
	Task             string
	WorkDir          string
	SystemPromptFile string // path to temp file containing continuation contract
	Env              []string
	MaxTokens        int
}

// AdapterContract is the interface every provider adapter must implement.
// All methods are goroutine-safe after Run() returns nil.
type AdapterContract interface {
	// Capability returns static metadata about this provider.
	Capability() ProviderCapability

	// Run starts the agent process and streams events to ch.
	// ch is closed when the process exits (success or error).
	// Returns an error only if the process cannot be started.
	// The caller must drain ch until it is closed.
	Run(ctx context.Context, opts RunOptions, ch chan<- AgentEvent) error

	// AwaitSafePauseWindow BLOCKS until a safe pause window is confirmed
	// or ctx is cancelled. This is a GATE — it guarantees atomicity between
	// quota-breach detection and snapshot acquisition.
	//
	// Returns ErrSafePauseTimeout if no window appears within the
	// adapter-specific timeout; the caller must then call ForceEscape().
	AwaitSafePauseWindow(ctx context.Context, breachReason string) (SafePoint, error)

	// ForceStop sends SIGKILL (Unix) or TerminateProcess (Windows) to the
	// agent immediately, without waiting for a safe point.
	ForceStop() error
}

// StdinReplier is implemented by adapters that can accept a user reply
// while the agent is waiting for input (e.g. answering a prompt).
// Adapters that don't implement this can't receive replies.
type StdinReplier interface {
	SendStdin(reply string) error
}

// ModelSetter is implemented by adapters that accept a runtime model override.
// The orchestrator calls SetModel before Run() based on relay.toml config.
type ModelSetter interface {
	SetModel(model string)
}

// ErrSafePauseTimeout is returned when AwaitSafePauseWindow times out.
type ErrSafePauseTimeout struct {
	Provider ProviderName
	After    string
}

func (e *ErrSafePauseTimeout) Error() string {
	return string(e.Provider) + ": safe-pause timeout after " + e.After + " — ForceEscape required"
}
