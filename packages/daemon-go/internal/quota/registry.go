// internal/quota/registry.go

package quota

import (
	"fmt"

	"github.com/dbisina/relay/internal/adapter"
)

// Registry maps ProviderName → QuotaAdapter.
type Registry struct {
	Adapters    map[adapter.ProviderName]QuotaAdapter
	ClaudeProxy *ClaudeProxyServer // exposed so CLI can read proxy port
}

// BuildQuotaRegistry creates all 8 quota adapters and starts the Claude proxy.
func BuildQuotaRegistry(declaredCaps map[adapter.ProviderName]int64) (*Registry, error) {
	proxy, err := NewClaudeProxyServer()
	if err != nil {
		return nil, fmt.Errorf("quota registry: %w", err)
	}
	claudeDeclaredCap := capFor(declaredCaps, adapter.ProviderClaude, 40_000)

	return &Registry{
		ClaudeProxy: proxy,
		Adapters: map[adapter.ProviderName]QuotaAdapter{
			// Layer-1 proxy headers
			adapter.ProviderClaude: NewClaudeQuotaAdapter(proxy, claudeDeclaredCap),

			// Request-count adapters (declared caps, configurable)
			adapter.ProviderCodex:       NewRequestCountAdapter("codex", capFor(declaredCaps, adapter.ProviderCodex, 500), 0.85),
			adapter.ProviderAntigravity: NewRequestCountAdapter("antigravity", capFor(declaredCaps, adapter.ProviderAntigravity, 1000), 0.85),
			adapter.ProviderOpenCode:    NewRequestCountAdapter("opencode", capFor(declaredCaps, adapter.ProviderOpenCode, 500), 0.85),
			adapter.ProviderCopilot:     NewRequestCountAdapter("copilot", capFor(declaredCaps, adapter.ProviderCopilot, 300), 0.85),
			adapter.ProviderContinue:    NewRequestCountAdapter("continue", capFor(declaredCaps, adapter.ProviderContinue, 500), 0.85),
			adapter.ProviderCline:       NewRequestCountAdapter("cline", capFor(declaredCaps, adapter.ProviderCline, 500), 0.85),

			// Ollama: local, no real quota (context window tracked separately)
			adapter.ProviderOllama: &NullQuotaAdapter{},
		},
	}, nil
}

func capFor(caps map[adapter.ProviderName]int64, provider adapter.ProviderName, fallback int64) int64 {
	if caps == nil {
		return fallback
	}
	if cap, ok := caps[provider]; ok && cap > 0 {
		return cap
	}
	return fallback
}

// Close shuts down all quota adapters.
func (r *Registry) Close() error {
	for _, qa := range r.Adapters {
		qa.Close() //nolint:errcheck
	}
	return nil
}
