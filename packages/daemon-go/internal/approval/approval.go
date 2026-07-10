// internal/approval/approval.go
//
// Approval gate. Used by orchestrator to pause before risky agent actions
// (large diffs, shell commands not on allowlist). Each request gets an ID;
// the UI POSTs an approve/deny to /api/approvals/<id>. Until then, the
// requesting goroutine blocks on its dedicated channel.

package approval

import (
	"sync"
	"sync/atomic"
	"time"
)

// Request — one waiting approval.
type Request struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`   // e.g. "write 250 lines to orders/refund.go"
	Reason    string    `json:"reason"`   // short rationale
	Severity  string    `json:"severity"` // "info" | "warn" | "danger"
	CreatedAt time.Time `json:"createdAt"`
	resp      chan Response
}

// Response — user's decision.
type Response struct {
	Approved bool   `json:"approved"`
	Note     string `json:"note,omitempty"`
}

// Gate — registry of pending approvals.
type Gate struct {
	// DefaultTimeout — if user doesn't respond within this window, deny.
	DefaultTimeout time.Duration
	// AutoApprove — when true, all requests pass through immediately
	// (used by background/CI sessions).
	AutoApprove atomic.Bool

	mu      sync.Mutex
	pending map[string]*Request
	nextID  uint64
}

func NewGate() *Gate {
	return &Gate{
		DefaultTimeout: 5 * time.Minute,
		pending:        map[string]*Request{},
	}
}

// Ask blocks until user approves/denies or timeout fires. Returns true if approved.
func (g *Gate) Ask(action, reason, severity string) bool {
	if g.AutoApprove.Load() {
		return true
	}
	id := g.allocID()
	req := &Request{
		ID:        id,
		Action:    action,
		Reason:    reason,
		Severity:  severity,
		CreatedAt: time.Now(),
		resp:      make(chan Response, 1),
	}
	g.mu.Lock()
	g.pending[id] = req
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.pending, id)
		g.mu.Unlock()
	}()

	select {
	case r := <-req.resp:
		return r.Approved
	case <-time.After(g.DefaultTimeout):
		return false
	}
}

// Pending — current waiting requests for /api/approvals.
func (g *Gate) Pending() []Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Request, 0, len(g.pending))
	for _, r := range g.pending {
		// Copy without the channel
		out = append(out, Request{
			ID:        r.ID,
			Action:    r.Action,
			Reason:    r.Reason,
			Severity:  r.Severity,
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// Resolve — deliver the user's response to the waiting goroutine.
// Returns true if the request was found and the response delivered.
func (g *Gate) Resolve(id string, approved bool, note string) bool {
	g.mu.Lock()
	req, ok := g.pending[id]
	g.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case req.resp <- Response{Approved: approved, Note: note}:
		return true
	default:
		return false
	}
}

func (g *Gate) allocID() string {
	n := atomic.AddUint64(&g.nextID, 1)
	return ulidLike(n)
}

func ulidLike(n uint64) string {
	const c = "0123456789abcdefghjkmnpqrstvwxyz"
	var buf [12]byte
	for i := len(buf) - 1; i >= 0; i-- {
		buf[i] = c[n%32]
		n /= 32
	}
	return string(buf[:])
}
