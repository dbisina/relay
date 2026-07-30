// internal/quota/claude_test.go — regression for a live infinite-handoff loop:
// with a single account and no real proxy header ever observed, Layer 2's
// fallback (tokensLogged / effectiveCap) accumulates for the orchestrator
// session's entire lifetime. Once cumulative usage crosses the fallback
// threshold, every future run reports an instant breach regardless of the
// account's real remaining quota, and there is nothing to fail over to.
// orchestrator.doHandoff now calls Reset() on every dispatch; this proves
// Reset() actually clears the stale accumulated total back to a fresh
// baseline rather than leaving it poisoned for the rest of the session.

package quota

import "testing"

func TestDeclaredCapBreachesAfterCumulativeUsage(t *testing.T) {
	proxy, err := NewClaudeProxyServer()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	a := NewClaudeQuotaAdapter(proxy, claudeDefaultDeclaredCap) // 40_000, effective cap 26_666

	// No proxy header ever observed (the exact live scenario: auth or network
	// trouble means Layer 1 never populates). Simulate several runs' worth of
	// logged usage without ever seeing a real anthropic-ratelimit-* header.
	a.RecordRequest(10_000)
	a.RecordRequest(10_000)
	a.RecordRequest(10_000) // cumulative 30_000 > 26_666 effective cap

	snap := a.Current()
	if snap.Source != "declared_cap" {
		t.Fatalf("expected declared_cap fallback with no header observed, got %q", snap.Source)
	}
	if snap.FractionUsed < a.BreachFraction() {
		t.Fatalf("expected cumulative usage to breach (fraction=%.2f, threshold=%.2f): test setup invalid",
			snap.FractionUsed, a.BreachFraction())
	}
	// This is the bug as it manifested live: nothing about a fresh dispatch
	// clears this on its own, so every subsequent Current() call keeps
	// reporting breach forever, even for a brand new attempt.
	stillBreached := a.Current()
	if stillBreached.FractionUsed < a.BreachFraction() {
		t.Fatal("fallback usage should remain accumulated without an explicit Reset")
	}

	// What orchestrator.doHandoff now does on every dispatch: resetQuotaView()
	// calls Reset() on the active provider's adapter. A fresh dispatch must
	// get a fresh baseline, not inherit the poisoned cumulative total.
	a.Reset()

	fresh := a.Current()
	if fresh.FractionUsed >= a.BreachFraction() {
		t.Fatalf("after Reset, a fresh dispatch should not be pre-breached: fraction=%.2f", fresh.FractionUsed)
	}
	if fresh.Source != "declared_cap" {
		t.Fatalf("still expected declared_cap fallback post-reset (no header yet), got %q", fresh.Source)
	}
}

func TestResetClearsObservedProxyHeader(t *testing.T) {
	proxy, err := NewClaudeProxyServer()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	a := NewClaudeQuotaAdapter(proxy, claudeDefaultDeclaredCap)

	// Simulate a real observed header showing the account nearly exhausted.
	proxy.mu.Lock()
	proxy.remaining = 100
	proxy.total = 10_000
	proxy.headerObserved = true
	proxy.mu.Unlock()

	before := a.Current()
	if before.Source != "proxy_header" {
		t.Fatalf("expected proxy_header source, got %q", before.Source)
	}
	if before.FractionUsed < a.BreachFraction() {
		t.Fatalf("expected near-exhausted header to breach, got fraction=%.2f", before.FractionUsed)
	}

	a.Reset()

	after := a.Current()
	if after.Source != "declared_cap" {
		t.Fatalf("after Reset, a stale header must not still be reported: got source %q", after.Source)
	}
	if after.FractionUsed >= a.BreachFraction() {
		t.Fatalf("fresh state after Reset should not be pre-breached: fraction=%.2f", after.FractionUsed)
	}
}
