// internal/quota/claude.go
//
// Claude quota adapter — 3-layer detection:
//
//   Layer 1 (primary):   proxy-header  anthropic-ratelimit-unified-tokens-remaining
//   Layer 2 (bootstrap): declared cap  (default 40_000 tokens; calibrated with /1.5 safety margin)
//   Layer 3 (backstop):  HTTP 429      detected via proxy; triggers immediate handoff
//
// The proxy runs on an OS-assigned port. The Relay daemon sets
// ANTHROPIC_BASE_URL=http://127.0.0.1:<port> in Claude Code's environment.
// All requests pass through; we extract quota headers without modifying traffic.

package quota

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	claudeDefaultDeclaredCap = int64(40_000) // tokens; overridden by first header
	claudeBreachFraction     = float64(0.85)
	claudeCalibrationDivisor = float64(1.5) // safety margin until first header observed
	claudeUpstreamBase       = "https://api.anthropic.com"
)

// ClaudeProxyServer is a local HTTP reverse proxy that intercepts Anthropic
// rate-limit headers. Run it before starting ClaudeAdapter.
type ClaudeProxyServer struct {
	listener net.Listener
	server   *http.Server

	mu             sync.RWMutex
	remaining      int64 // -1 = not yet observed
	total          int64 // -1 = not yet observed
	resetsAt       *time.Time
	headerObserved bool
	got429         atomic.Bool
}

// NewClaudeProxyServer creates and starts the proxy on an OS-assigned port.
func NewClaudeProxyServer() (*ClaudeProxyServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("claude proxy: listen: %w", err)
	}

	p := &ClaudeProxyServer{
		listener:  ln,
		remaining: -1,
		total:     -1,
	}

	upstream, _ := url.Parse(claudeUpstreamBase)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ModifyResponse = p.modifyResponse

	mux := http.NewServeMux()
	mux.Handle("/", proxy)

	p.server = &http.Server{Handler: mux}
	go p.server.Serve(ln)
	return p, nil
}

// Port returns the port the proxy is listening on.
func (p *ClaudeProxyServer) Port() int {
	return p.listener.Addr().(*net.TCPAddr).Port
}

// Close stops the proxy.
func (p *ClaudeProxyServer) Close() error {
	return p.server.Close()
}

// modifyResponse intercepts Anthropic rate-limit headers on responses.
func (p *ClaudeProxyServer) modifyResponse(resp *http.Response) error {
	// Backstop: 429 → immediate handoff flag
	if resp.StatusCode == http.StatusTooManyRequests {
		p.got429.Store(true)
		return nil
	}

	hRemaining := resp.Header.Get("anthropic-ratelimit-unified-tokens-remaining")
	hLimit := resp.Header.Get("anthropic-ratelimit-unified-tokens-limit")
	hReset := resp.Header.Get("anthropic-ratelimit-unified-tokens-reset")

	if hRemaining == "" {
		return nil
	}

	remaining, err := strconv.ParseInt(hRemaining, 10, 64)
	if err != nil {
		return nil
	}
	total := int64(-1)
	if hLimit != "" {
		total, _ = strconv.ParseInt(hLimit, 10, 64)
	}

	var resetsAt *time.Time
	if hReset != "" {
		if t, err := time.Parse(time.RFC3339, hReset); err == nil {
			resetsAt = &t
		}
	}

	p.mu.Lock()
	p.remaining = remaining
	if total > 0 {
		p.total = total
	}
	p.resetsAt = resetsAt
	p.headerObserved = true
	p.mu.Unlock()

	return nil
}

// Reset clears observed quota state. Called when the session switches to a
// different account behind the same proxy: the new login's first response
// repopulates the headers, and until then we fall back to the declared cap.
func (p *ClaudeProxyServer) Reset() {
	p.mu.Lock()
	p.remaining = -1
	p.total = -1
	p.resetsAt = nil
	p.headerObserved = false
	p.mu.Unlock()
	p.got429.Store(false)
}

// snapshot returns the current observed state.
func (p *ClaudeProxyServer) snapshot() (remaining, total int64, resetsAt *time.Time, observed bool, got429 bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.remaining, p.total, p.resetsAt, p.headerObserved, p.got429.Load()
}

// ─────────────────────────────────────────────────────────────────────────────

// ClaudeQuotaAdapter implements QuotaAdapter using the proxy's observed headers.
type ClaudeQuotaAdapter struct {
	proxy        *ClaudeProxyServer
	declaredCap  int64
	reqCount     atomic.Int64
	tokensLogged atomic.Int64
}

// NewClaudeQuotaAdapter creates the adapter. Takes ownership of the proxy.
func NewClaudeQuotaAdapter(proxy *ClaudeProxyServer, declaredCap int64) *ClaudeQuotaAdapter {
	if declaredCap <= 0 {
		declaredCap = claudeDefaultDeclaredCap
	}
	return &ClaudeQuotaAdapter{
		proxy:       proxy,
		declaredCap: declaredCap,
	}
}

func (a *ClaudeQuotaAdapter) Current() Snapshot {
	remaining, total, resetsAt, observed, got429 := a.proxy.snapshot()

	// Backstop: 429 → treat as fully exhausted
	if got429 {
		return Snapshot{
			Remaining:    0,
			Total:        total,
			ResetsAt:     resetsAt,
			Source:       "429_backstop",
			FractionUsed: 1.0,
		}
	}

	// Layer 1: proxy headers
	if observed && remaining >= 0 {
		cap := total
		if cap <= 0 {
			cap = a.declaredCap
		}
		frac := float64(0)
		if cap > 0 {
			frac = float64(cap-remaining) / float64(cap)
		}
		return Snapshot{
			Remaining:    remaining,
			Total:        cap,
			ResetsAt:     resetsAt,
			Source:       "proxy_header",
			FractionUsed: frac,
		}
	}

	// Layer 2: declared cap / calibration
	// Safety margin: treat declaredCap/1.5 as effective cap until first header
	effectiveCap := int64(float64(a.declaredCap) / claudeCalibrationDivisor)
	logged := a.tokensLogged.Load()
	frac := float64(0)
	if effectiveCap > 0 {
		frac = float64(logged) / float64(effectiveCap)
	}
	return Snapshot{
		Remaining:    max64(effectiveCap-logged, 0),
		Total:        effectiveCap,
		ResetsAt:     nil,
		Source:       "declared_cap",
		FractionUsed: frac,
	}
}

func (a *ClaudeQuotaAdapter) BreachFraction() float64 { return claudeBreachFraction }

func (a *ClaudeQuotaAdapter) ShouldHandoff() bool {
	snap := a.Current()
	return snap.FractionUsed >= claudeBreachFraction
}

func (a *ClaudeQuotaAdapter) RecordRequest(tokensUsed int64) {
	a.reqCount.Add(1)
	a.tokensLogged.Add(tokensUsed)
}

func (a *ClaudeQuotaAdapter) Close() error {
	return a.proxy.Close()
}

// Reset clears the proxy's observed headers and this adapter's local tallies so
// a freshly-switched account is not seen as already exhausted.
func (a *ClaudeQuotaAdapter) Reset() {
	a.proxy.Reset()
	a.reqCount.Store(0)
	a.tokensLogged.Store(0)
}

// ─────────────────────────────────────────────────────────────────────────────

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// Stub for /api/show context-window fetch (Ollama quota)
// ─────────────────────────────────────────────────────────────────────────────

// RequestCountAdapter is a simple request-count based quota adapter for
// providers where no token header is available (Antigravity, Codex, etc.).
type RequestCountAdapter struct {
	provider    string
	declaredCap int64
	reqCount    atomic.Int64
	breachFrac  float64
}

// NewRequestCountAdapter creates a quota adapter that tracks by request count.
func NewRequestCountAdapter(provider string, declaredCap int64, breachFrac float64) *RequestCountAdapter {
	if breachFrac <= 0 {
		breachFrac = 0.85
	}
	return &RequestCountAdapter{
		provider:    provider,
		declaredCap: declaredCap,
		breachFrac:  breachFrac,
	}
}

func (a *RequestCountAdapter) Current() Snapshot {
	used := a.reqCount.Load()
	frac := float64(0)
	if a.declaredCap > 0 {
		frac = float64(used) / float64(a.declaredCap)
	}
	return Snapshot{
		Remaining:    max64(a.declaredCap-used, 0),
		Total:        a.declaredCap,
		Source:       "request_count",
		FractionUsed: frac,
	}
}

func (a *RequestCountAdapter) BreachFraction() float64 { return a.breachFrac }
func (a *RequestCountAdapter) ShouldHandoff() bool {
	return a.Current().FractionUsed >= a.breachFrac
}
func (a *RequestCountAdapter) RecordRequest(tokensUsed int64) { a.reqCount.Add(1) }
func (a *RequestCountAdapter) Close() error                   { return nil }
func (a *RequestCountAdapter) Reset()                         { a.reqCount.Store(0) }

// NullQuotaAdapter never triggers a handoff (for providers without quota).
type NullQuotaAdapter struct{}

func (n *NullQuotaAdapter) Current() Snapshot {
	return Snapshot{Remaining: -1, Total: -1, Source: "unknown", FractionUsed: -1}
}
func (n *NullQuotaAdapter) BreachFraction() float64 { return 1.0 }
func (n *NullQuotaAdapter) ShouldHandoff() bool     { return false }
func (n *NullQuotaAdapter) RecordRequest(_ int64)   {}
func (n *NullQuotaAdapter) Close() error            { return nil }

// Ensure interfaces are satisfied at compile time.
var _ io.Closer = (*ClaudeQuotaAdapter)(nil)
var _ QuotaAdapter = (*ClaudeQuotaAdapter)(nil)
var _ QuotaAdapter = (*RequestCountAdapter)(nil)
var _ QuotaAdapter = (*NullQuotaAdapter)(nil)
var _ Resetter = (*ClaudeQuotaAdapter)(nil)
var _ Resetter = (*RequestCountAdapter)(nil)
