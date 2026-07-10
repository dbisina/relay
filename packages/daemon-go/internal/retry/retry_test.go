package retry

import (
	"math/rand"
	"testing"
	"time"
)

func TestDetect(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		line string
		want Reason
	}{
		{"You've hit your session limit · resets 2am (Europe/Zurich)", SessionLimit},
		{"5-hour limit reached - resets 3pm", SessionLimit},
		{"Claude usage limit reached", SessionLimit},
		{`API Error: 529 {"type":"overloaded_error"}`, Overload},
		{"Server is temporarily limiting requests (not your usage limit)", Overload},
		{"claude-opus's safeguards flagged this message", Safeguard},
	}
	for _, c := range cases {
		sig := Detect(c.line, now)
		if sig == nil {
			t.Fatalf("Detect(%q) = nil, want %s", c.line, c.want)
		}
		if sig.Reason != c.want {
			t.Errorf("Detect(%q).Reason = %s, want %s", c.line, sig.Reason, c.want)
		}
	}
	if got := Detect("just a normal line of output", now); got != nil {
		t.Errorf("Detect(normal) = %+v, want nil", got)
	}
}

func TestDetectPrecedence(t *testing.T) {
	now := time.Now()
	// A safeguard render must not be misread as a usage limit even if the word
	// "limit" appears nearby.
	if s := Detect("safeguards flagged this message (rate limit unrelated)", now); s == nil || s.Reason != Safeguard {
		t.Fatalf("expected Safeguard precedence, got %+v", s)
	}
}

func TestParseResetTime(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC) // 10:00 UTC

	got := ParseResetTime("resets 3pm", now)
	if got == nil || got.Hour() != 15 {
		t.Fatalf("resets 3pm -> %v, want 15:00 today", got)
	}
	// A time earlier than now rolls to tomorrow.
	got = ParseResetTime("resets 2am", now)
	if got == nil || got.Hour() != 2 || !got.After(now) {
		t.Fatalf("resets 2am -> %v, want next 02:00", got)
	}
	if !got.After(now) || got.Day() != 11 {
		t.Errorf("resets 2am should be tomorrow, got %v", got)
	}
	// 24-hour clock.
	got = ParseResetTime("resets at 14:30", now)
	if got == nil || got.Hour() != 14 || got.Minute() != 30 {
		t.Fatalf("resets 14:30 -> %v", got)
	}
	if ParseResetTime("no reset here", now) != nil {
		t.Errorf("expected nil for line without a reset clause")
	}
}

func TestBackoffExponential(t *testing.T) {
	c := DefaultConfig()
	c.BackoffBase = time.Second
	c.MaxWait = time.Hour
	// Deterministic (nil rand): base * 2^attempt.
	for attempt, want := range map[int]time.Duration{0: time.Second, 1: 2 * time.Second, 3: 8 * time.Second} {
		if got := c.Backoff(attempt, nil); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
	// Jitter stays within the configured band.
	c.JitterPct = 20
	r := rand.New(rand.NewSource(1))
	base := 8 * time.Second
	for i := 0; i < 100; i++ {
		got := c.Backoff(3, r)
		if got < base-base/5 || got > base+base/5 {
			t.Fatalf("jittered Backoff out of band: %v", got)
		}
	}
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	c := DefaultConfig()
	c.MaxWait = 6 * time.Hour
	c.Margin = 0

	// Overload -> immediate bounded retry regardless of preference.
	d := c.Decide(&Signal{Reason: Overload}, nil, now)
	if d.Action != ActionImmediate {
		t.Errorf("overload -> %s, want immediate", d.Action)
	}

	// Session limit resetting in 2h, within MaxWait -> wait.
	reset := now.Add(2 * time.Hour)
	d = c.Decide(&Signal{Reason: SessionLimit, ResetAt: &reset}, nil, now)
	if d.Action != ActionWait || !d.Until.Equal(reset) {
		t.Errorf("session limit -> %+v, want wait until %v", d, reset)
	}

	// Reset 8h away, beyond MaxWait -> handoff.
	far := now.Add(8 * time.Hour)
	d = c.Decide(&Signal{Reason: SessionLimit, ResetAt: &far}, nil, now)
	if d.Action != ActionHandoff {
		t.Errorf("far reset -> %s, want handoff", d.Action)
	}

	// No reset time anywhere -> handoff (waiting would be unbounded).
	d = c.Decide(&Signal{Reason: SessionLimit}, nil, now)
	if d.Action != ActionHandoff {
		t.Errorf("no reset -> %s, want handoff", d.Action)
	}

	// Falls back to the quota snapshot's reset time when the signal lacks one.
	d = c.Decide(&Signal{Reason: SessionLimit}, &reset, now)
	if d.Action != ActionWait {
		t.Errorf("snap reset -> %s, want wait", d.Action)
	}

	// prefer=handoff disables waiting.
	c.Prefer = "handoff"
	d = c.Decide(&Signal{Reason: SessionLimit, ResetAt: &reset}, nil, now)
	if d.Action != ActionHandoff {
		t.Errorf("prefer=handoff -> %s, want handoff", d.Action)
	}
}
