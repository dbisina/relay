// internal/pricing/pricing.go
//
// Per-provider token pricing for live cost meter. Approximate, updated by hand.
// Local providers (ollama) cost 0.

package pricing

import "fmt"

// Pricing — per million tokens, USD.
type Pricing struct {
	InputPerMTokens  float64
	OutputPerMTokens float64
}

// Table maps provider name → pricing. Conservative latest-flagship-tier defaults.
var Table = map[string]Pricing{
	"claude":      {InputPerMTokens: 3.00, OutputPerMTokens: 15.00}, // Sonnet
	"codex":       {InputPerMTokens: 5.00, OutputPerMTokens: 15.00}, // GPT-4o-ish
	"antigravity": {InputPerMTokens: 3.00, OutputPerMTokens: 15.00},
	"opencode":    {InputPerMTokens: 3.00, OutputPerMTokens: 15.00},
	"ollama":      {InputPerMTokens: 0.0, OutputPerMTokens: 0.0},
	"copilot":     {InputPerMTokens: 0.0, OutputPerMTokens: 0.0}, // flat subscription
	"continue":    {InputPerMTokens: 0.0, OutputPerMTokens: 0.0}, // depends on backend
	"cline":       {InputPerMTokens: 0.0, OutputPerMTokens: 0.0},
}

// Estimate returns the USD cost for a provider given input/output token counts.
func Estimate(provider string, inputTokens, outputTokens int64) float64 {
	p, ok := Table[provider]
	if !ok {
		return 0
	}
	return float64(inputTokens)/1_000_000*p.InputPerMTokens +
		float64(outputTokens)/1_000_000*p.OutputPerMTokens
}

// Format4 returns a $X.XXXX string (4dp for sub-cent visibility).
func Format4(usd float64) string {
	return fmt.Sprintf("$%.4f", usd)
}

// Format2 returns $X.XX.
func Format2(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
