// internal/quota/interface.go

package quota

import "time"

// Snapshot holds a point-in-time quota reading.
type Snapshot struct {
	Remaining    int64      // tokens or requests remaining; -1 = unknown
	Total        int64      // declared cap; -1 = unknown
	ResetsAt     *time.Time // nil = unknown
	Source       string     // "proxy_header" | "declared_cap" | "request_count" | "unknown"
	FractionUsed float64    // 0.0–1.0; -1 = unknown
}

// QuotaAdapter observes and reports quota state for one provider.
// Each adapter is isolated; no cross-provider contamination.
type QuotaAdapter interface {
	// Current returns the latest quota snapshot without blocking.
	Current() Snapshot

	// BreachFraction returns the threshold at which a handoff should be
	// triggered. Providers set this to 0.8–0.9 typically.
	BreachFraction() float64

	// ShouldHandoff returns true when Current().FractionUsed >= BreachFraction().
	ShouldHandoff() bool

	// RecordRequest is called after each agent request to update internal
	// request-count tracking (used when header-based quota is unavailable).
	RecordRequest(tokensUsed int64)

	// Close releases resources (e.g. the local proxy server for Claude).
	Close() error
}

// Resetter is implemented by quota adapters whose observed state can be cleared.
// The orchestrator calls Reset() after switching to a fresh account of the same
// provider, so the prior account's usage (and any 429 latch) does not instantly
// re-trigger a handoff on the new, unexhausted login.
type Resetter interface {
	Reset()
}
