// internal/adapter/registry.go

package adapter

// Registry maps ProviderName → AdapterContract.
type Registry map[ProviderName]AdapterContract

// BuildAdapterRegistry constructs all 8 provider adapters.
// proxyPort: port of the Claude local proxy (0 = no proxy, Layer-1 disabled).
func BuildAdapterRegistry(proxyPort int, ollamaBaseURL, ollamaModel string) Registry {
	return Registry{
		ProviderClaude:      NewClaudeAdapter(proxyPort),
		ProviderCodex:       NewCodexAdapter(),
		ProviderAntigravity: NewAntigravityAdapter(),
		ProviderOpenCode:    NewOpenCodeAdapter(),
		ProviderOllama:      NewOllamaAdapter(ollamaBaseURL, ollamaModel),
		ProviderCopilot:     NewCopilotAdapter(),
		ProviderContinue:    NewContinueAdapter(),
		ProviderCline:       NewClineAdapter(),
	}
}
