// internal/outcomes/outcomes.go
//
// Profile outcome tracking — every completed session records (profile, success)
// to .relay/outcomes.jsonl. matchProfile() in the orchestrator factors in
// historical success rate so over time the routing self-tunes.

package outcomes

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Outcome — one session result.
type Outcome struct {
	Profile   string    `json:"profile"`
	TaskGoal  string    `json:"taskGoal"`
	Success   bool      `json:"success"`
	Provider  string    `json:"provider"` // final provider when task ended
	Tokens    int64     `json:"tokens"`
	CostUSD   float64   `json:"costUsd"`
	Handoffs  int       `json:"handoffs"`
	Timestamp time.Time `json:"timestamp"`
}

// Tracker — append-only JSONL log + in-memory aggregate.
type Tracker struct {
	Path string

	mu  sync.Mutex
	agg map[string]*Aggregate // profile → stats
}

// Aggregate — rolling stats.
type Aggregate struct {
	Profile     string    `json:"profile"`
	Runs        int       `json:"runs"`
	Successes   int       `json:"successes"`
	SuccessRate float64   `json:"successRate"`
	AvgTokens   int64     `json:"avgTokens"`
	AvgCostUSD  float64   `json:"avgCostUsd"`
	LastRun     time.Time `json:"lastRun"`
}

func New(stateDir string) *Tracker {
	t := &Tracker{
		Path: filepath.Join(stateDir, "outcomes.jsonl"),
		agg:  map[string]*Aggregate{},
	}
	t.load()
	return t
}

// Record appends an outcome and updates aggregates.
func (t *Tracker) Record(o Outcome) error {
	if o.Profile == "" {
		o.Profile = "_unrouted"
	}
	o.Timestamp = time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Update aggregate
	a, ok := t.agg[o.Profile]
	if !ok {
		a = &Aggregate{Profile: o.Profile}
		t.agg[o.Profile] = a
	}
	a.Runs++
	if o.Success {
		a.Successes++
	}
	a.SuccessRate = float64(a.Successes) / float64(a.Runs)
	a.AvgTokens = (a.AvgTokens*int64(a.Runs-1) + o.Tokens) / int64(a.Runs)
	a.AvgCostUSD = (a.AvgCostUSD*float64(a.Runs-1) + o.CostUSD) / float64(a.Runs)
	a.LastRun = o.Timestamp

	// Append to file
	f, err := os.OpenFile(t.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(o)
}

// Aggregates returns a copy of the current per-profile stats.
func (t *Tracker) Aggregates() []Aggregate {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Aggregate, 0, len(t.agg))
	for _, a := range t.agg {
		out = append(out, *a)
	}
	return out
}

// SuccessRate returns the historical pass rate for a profile (0..1).
// Returns 0.5 (neutral) when there's no data, so unknown profiles aren't penalised.
func (t *Tracker) SuccessRate(profile string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.agg[profile]
	if !ok || a.Runs < 2 {
		return 0.5
	}
	return a.SuccessRate
}

func (t *Tracker) load() {
	f, err := os.Open(t.Path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256*1024), 1024*1024)
	for sc.Scan() {
		var o Outcome
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			continue
		}
		a, ok := t.agg[o.Profile]
		if !ok {
			a = &Aggregate{Profile: o.Profile}
			t.agg[o.Profile] = a
		}
		a.Runs++
		if o.Success {
			a.Successes++
		}
		if a.Runs > 0 {
			a.SuccessRate = float64(a.Successes) / float64(a.Runs)
		}
		if o.Tokens > 0 {
			a.AvgTokens = (a.AvgTokens*int64(a.Runs-1) + o.Tokens) / int64(a.Runs)
		}
		if o.CostUSD > 0 {
			a.AvgCostUSD = (a.AvgCostUSD*float64(a.Runs-1) + o.CostUSD) / float64(a.Runs)
		}
		a.LastRun = o.Timestamp
	}
}
