package quota

import (
	"testing"
	"time"
)

func TestForecastEtaMinutes(t *testing.T) {
	if got := ForecastEtaMinutes(1000, 100); got != 10 {
		t.Errorf("eta = %v, want 10 (1000/100)", got)
	}
	if got := ForecastEtaMinutes(-1, 100); got != -1 {
		t.Errorf("unknown remaining → -1, got %v", got)
	}
	if got := ForecastEtaMinutes(1000, 0); got != -1 {
		t.Errorf("zero burn → -1 (cannot forecast), got %v", got)
	}
}

func TestLedgerObserveAndKey(t *testing.T) {
	l := Ledger{}
	reset := time.Unix(1_000_000, 0)
	l.Observe("claude", "work", Snapshot{Remaining: 600, Total: 1000, FractionUsed: 0.4, ResetsAt: &reset, Source: "proxy_header"}, 60, 12345)
	e, ok := l[LedgerKey("claude", "work")]
	if !ok {
		t.Fatal("entry not stored under provider/account key")
	}
	if e.Remaining != 600 || e.BurnPerMin != 60 {
		t.Fatalf("entry = %+v", e)
	}
	if e.EtaMinutes != 10 { // 600 / 60
		t.Errorf("eta = %v, want 10", e.EtaMinutes)
	}
	if LedgerKey("ollama", "") != "ollama" {
		t.Error("empty account should key by provider alone")
	}
}

func TestLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := Ledger{}
	l.Observe("codex", "personal", Snapshot{Remaining: 50, Total: 100, FractionUsed: 0.5, Source: "request_count"}, 5, 1)
	if err := SaveLedger(dir, l); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[LedgerKey("codex", "personal")]; !ok {
		t.Fatalf("round trip lost the entry: %+v", got)
	}
	// Missing file → empty, no error.
	empty, err := LoadLedger(t.TempDir())
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing ledger should be empty/no-error, got %v err %v", empty, err)
	}
}
