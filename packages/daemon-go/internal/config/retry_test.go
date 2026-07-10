package config

import (
	"testing"
	"time"
)

func TestRetryDefaults(t *testing.T) {
	c := Default("/tmp/x")
	if !c.Retry.Enabled || c.Retry.Prefer != "wait-then-handoff" {
		t.Fatalf("unexpected retry defaults: %+v", c.Retry)
	}
	eng := c.Retry.ToEngine()
	if eng.MaxWait != 360*time.Minute || eng.BackoffBase != 5*time.Second || eng.Margin != 30*time.Second {
		t.Errorf("ToEngine conversion wrong: %+v", eng)
	}
	if eng.RetryMessage != "continue" {
		t.Errorf("retry message = %q", eng.RetryMessage)
	}
}

func TestRetryParse(t *testing.T) {
	toml := `
[retry]
enabled          = false
prefer           = "wait"
max_wait_minutes = 120
backoff_seconds  = 10
jitter_pct       = 5
max_retries      = 3
margin_seconds   = 15
retry_message    = "keep going"
`
	cfg := Default("/tmp/x")
	cfg, err := parseTOML(cfg, toml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := cfg.Retry
	if r.Enabled {
		t.Error("enabled should be false")
	}
	if r.Prefer != "wait" || r.MaxWaitMinutes != 120 || r.BackoffSeconds != 10 ||
		r.JitterPct != 5 || r.MaxRetries != 3 || r.MarginSeconds != 15 || r.RetryMessage != "keep going" {
		t.Errorf("parsed retry mismatch: %+v", r)
	}
}
