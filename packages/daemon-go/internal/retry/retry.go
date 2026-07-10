// internal/retry/retry.go — provider-neutral wait-and-retry engine.
//
// Relay's default response to a provider limit is to hand off to another
// account or provider. Sometimes the cheaper move is to WAIT for the same
// subscription to reset and continue there. This package detects the three
// classes of recoverable interruption an agent CLI prints, decides whether to
// wait or hand off, and computes how long to wait. It is a Go generalization of
// the single-provider claude-auto-retry tool, applied across every adapter.
package retry

import (
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Reason classifies why an agent stopped in a recoverable way.
type Reason string

const (
	// SessionLimit: a subscription/session usage limit with a printed reset time.
	SessionLimit Reason = "session_limit"
	// Overload: sustained server overload (HTTP 5xx / overloaded_error). Transient.
	Overload Reason = "overload"
	// Safeguard: a false-positive content safeguard flag. Bounded immediate re-send.
	Safeguard Reason = "safeguard"
)

// Signal is a detected recoverable interruption.
type Signal struct {
	Reason  Reason
	ResetAt *time.Time // set for SessionLimit when a reset time was parseable
	Line    string     // the raw line that matched (already redacted upstream)
}

// Action is what the orchestrator should do in response to a signal.
type Action string

const (
	ActionWait      Action = "wait"      // sleep until Until, then re-run the same provider
	ActionHandoff   Action = "handoff"   // fall through to account/provider handoff
	ActionImmediate Action = "immediate" // short bounded delay then re-run (overload/safeguard)
)

// Decision is the resolved plan for a signal under a policy.
type Decision struct {
	Action Action
	Until  time.Time     // for ActionWait: absolute wake time (reset + margin)
	Delay  time.Duration // for ActionImmediate: how long to pause first
}

// Config controls the strategy. Mirrored by config.RetryConfig in the TOML layer.
type Config struct {
	Enabled      bool
	Prefer       string // "wait" | "handoff" | "wait-then-handoff"
	MaxWait      time.Duration
	BackoffBase  time.Duration
	JitterPct    int
	MaxRetries   int
	Margin       time.Duration // added after a reset time before resuming
	RetryMessage string        // e.g. "continue"
}

// DefaultConfig is a safe, conservative default.
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		Prefer:       "wait-then-handoff",
		MaxWait:      6 * time.Hour,
		BackoffBase:  5 * time.Second,
		JitterPct:    20,
		MaxRetries:   8,
		Margin:       30 * time.Second,
		RetryMessage: "continue",
	}
}

var (
	reSessionLimit = regexp.MustCompile(`(?i)(\d+-hour limit reached|hit your session limit|usage limit reached|out of extra usage|limit reached)`)
	reOverload     = regexp.MustCompile(`(?i)(overloaded_error|api error:\s*5(29|00|02|03|04)\b|temporarily limiting requests|server is overloaded)`)
	reSafeguard    = regexp.MustCompile(`(?i)safeguards? flagged this message`)
	// "resets 3pm", "resets 2am (Europe/Zurich)", "resets 14:30"
	reReset = regexp.MustCompile(`(?i)resets?\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?(?:\s*\(([A-Za-z_]+/[A-Za-z_]+)\))?`)
)

// Detect scans a single line of agent output for a recoverable interruption.
// Returns nil when the line is not a retry signal. Order matters: a safeguard
// or overload render is not a usage limit.
func Detect(line string, now time.Time) *Signal {
	switch {
	case reSafeguard.MatchString(line):
		return &Signal{Reason: Safeguard, Line: line}
	case reOverload.MatchString(line):
		return &Signal{Reason: Overload, Line: line}
	case reSessionLimit.MatchString(line):
		return &Signal{Reason: SessionLimit, ResetAt: ParseResetTime(line, now), Line: line}
	}
	return nil
}

// ParseResetTime turns a "resets 3pm" style clock time into the next absolute
// instant that clock time occurs at or after now. Timezone in parentheses is
// honored when loadable; otherwise now's location is used. Returns nil when no
// reset clause is present or it cannot be parsed.
func ParseResetTime(line string, now time.Time) *time.Time {
	m := reReset.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	hour, err := strconv.Atoi(m[1])
	if err != nil || hour < 0 || hour > 23 {
		return nil
	}
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	if minute < 0 || minute > 59 {
		minute = 0
	}
	switch strings.ToLower(m[3]) {
	case "pm":
		if hour < 12 {
			hour += 12
		}
	case "am":
		if hour == 12 {
			hour = 0
		}
	}
	loc := now.Location()
	if m[4] != "" {
		if l, err := time.LoadLocation(m[4]); err == nil {
			loc = l
		}
	}
	nowLoc := now.In(loc)
	reset := time.Date(nowLoc.Year(), nowLoc.Month(), nowLoc.Day(), hour, minute, 0, 0, loc)
	if !reset.After(nowLoc) {
		reset = reset.Add(24 * time.Hour) // next occurrence
	}
	return &reset
}

// Backoff returns the base delay for a given attempt (0-indexed) with jitter.
// Exponential: base * 2^attempt, capped at MaxWait, then +/- JitterPct. A nil
// rand source falls back to the deterministic (un-jittered) value so callers in
// tests get a stable result.
func (c Config) Backoff(attempt int, r *rand.Rand) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := time.Duration(float64(c.BackoffBase) * math.Pow(2, float64(attempt)))
	if c.MaxWait > 0 && d > c.MaxWait {
		d = c.MaxWait
	}
	if r != nil && c.JitterPct > 0 {
		span := float64(d) * float64(c.JitterPct) / 100.0
		d += time.Duration((r.Float64()*2 - 1) * span)
	}
	if d < 0 {
		d = 0
	}
	return d
}

// Decide resolves a signal into an action under the configured policy. snapReset
// is an optional reset time from the quota subsystem, used when the signal did
// not carry one. now is injected for testability.
func (c Config) Decide(sig *Signal, snapReset *time.Time, now time.Time) Decision {
	if sig == nil || !c.Enabled || c.Prefer == "handoff" {
		return Decision{Action: ActionHandoff}
	}

	switch sig.Reason {
	case Overload, Safeguard:
		// Transient: a bounded immediate retry is almost always right, regardless
		// of wait/handoff preference.
		return Decision{Action: ActionImmediate, Delay: c.Backoff(0, nil)}
	case SessionLimit:
		reset := sig.ResetAt
		if reset == nil {
			reset = snapReset
		}
		if reset == nil {
			// No known reset time: waiting is unbounded, so hand off.
			return Decision{Action: ActionHandoff}
		}
		until := reset.Add(c.Margin)
		wait := until.Sub(now)
		if wait <= 0 {
			// Already reset — resume immediately.
			return Decision{Action: ActionWait, Until: now}
		}
		if c.MaxWait > 0 && wait > c.MaxWait {
			// Reset is too far away to wait for; hand off instead.
			return Decision{Action: ActionHandoff}
		}
		return Decision{Action: ActionWait, Until: until}
	}
	return Decision{Action: ActionHandoff}
}
