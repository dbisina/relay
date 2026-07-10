// internal/circuit/circuit.go
//
// Per-provider circuit breaker. Trips after N failures within a window;
// stays open for a cooldown; half-open after that allows a single probe.
//
// Wrapped at the adapter-call boundary so a flapping provider can't stall
// the orchestrator forever.

package circuit

import (
	"fmt"
	"sync"
	"time"
)

// State of the breaker.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	return []string{"closed", "open", "half-open"}[s]
}

// Breaker — a single provider's breaker.
type Breaker struct {
	Threshold int           // failures within Window to trip
	Window    time.Duration // failure-counting window
	Cooldown  time.Duration // how long to stay Open before HalfOpen
	onTrip    func(name string)

	mu       sync.Mutex
	name     string
	state    State
	failures []time.Time
	openedAt time.Time
}

// Registry — one Breaker per provider name, lazily created with defaults.
type Registry struct {
	mu       sync.Mutex
	breakers map[string]*Breaker
	OnTrip   func(name string)
}

func NewRegistry() *Registry {
	return &Registry{breakers: map[string]*Breaker{}}
}

// For returns (creating if needed) the breaker for a provider.
func (r *Registry) For(name string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.breakers[name]; ok {
		return b
	}
	b := &Breaker{
		Threshold: 3,
		Window:    60 * time.Second,
		Cooldown:  5 * time.Minute,
		name:      name,
		onTrip:    r.OnTrip,
	}
	r.breakers[name] = b
	return b
}

// Allow returns nil if a call is permitted, or an error if the breaker is open.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case Closed:
		return nil
	case Open:
		if time.Since(b.openedAt) >= b.Cooldown {
			b.state = HalfOpen
			return nil
		}
		retryIn := b.Cooldown - time.Since(b.openedAt)
		return fmt.Errorf("circuit OPEN for %s — retry in %s", b.name, retryIn.Round(time.Second))
	case HalfOpen:
		return nil
	}
	return nil
}

// RecordSuccess closes a half-open breaker and clears failure history.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == HalfOpen {
		b.state = Closed
	}
	b.failures = nil
}

// RecordFailure increments failure count and trips if threshold met.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == HalfOpen {
		b.state = Open
		b.openedAt = time.Now()
		if b.onTrip != nil {
			b.onTrip(b.name)
		}
		return
	}
	// Trim failures outside window
	now := time.Now()
	cutoff := now.Add(-b.Window)
	fresh := b.failures[:0]
	for _, t := range b.failures {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	b.failures = append(fresh, now)
	if len(b.failures) >= b.Threshold {
		b.state = Open
		b.openedAt = now
		if b.onTrip != nil {
			b.onTrip(b.name)
		}
	}
}

// Snapshot — observable state for /api.
type Snapshot struct {
	Name     string    `json:"name"`
	State    string    `json:"state"`
	Failures int       `json:"failures"`
	OpenedAt time.Time `json:"openedAt,omitempty"`
}

func (r *Registry) SnapshotAll() []Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Snapshot, 0, len(r.breakers))
	for name, b := range r.breakers {
		b.mu.Lock()
		out = append(out, Snapshot{
			Name:     name,
			State:    b.state.String(),
			Failures: len(b.failures),
			OpenedAt: b.openedAt,
		})
		b.mu.Unlock()
	}
	return out
}
