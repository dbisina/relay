// internal/quota/ledger.go — the quota wallet's persistent store.
//
// Relay's headline value is rotating work across the subscriptions you already
// pay for. To do that well the user needs one screen showing, per provider and
// per account, how much quota is left and when it resets — plus a forecast of
// how long the current burn rate will last. The orchestrator writes observations
// here as sessions run; the daemon serves them to the wallet UI even when no
// session is active.

package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LedgerEntry is the last-known quota state for one provider+account login.
type LedgerEntry struct {
	Provider     string     `json:"provider"`
	Account      string     `json:"account"` // "" when the provider has no named accounts
	Remaining    int64      `json:"remaining"`
	Total        int64      `json:"total"`
	FractionUsed float64    `json:"fractionUsed"`
	ResetsAt     *time.Time `json:"resetsAt,omitempty"`
	Source       string     `json:"source"`
	UpdatedMs    int64      `json:"updatedMs"`
	BurnPerMin   float64    `json:"burnPerMin"` // observed units/minute; 0 = unknown
	EtaMinutes   float64    `json:"etaMinutes"` // forecast minutes to exhaustion; -1 = unknown
}

// Ledger maps "provider/account" → entry.
type Ledger map[string]LedgerEntry

// LedgerKey is the stable map key for a provider+account pair.
func LedgerKey(provider, account string) string {
	if account == "" {
		return provider
	}
	return provider + "/" + account
}

const ledgerFile = "quota-ledger.json"

// LoadLedger reads the persisted wallet. A missing file is an empty ledger.
func LoadLedger(stateDir string) (Ledger, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, ledgerFile))
	if os.IsNotExist(err) {
		return Ledger{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l == nil {
		l = Ledger{}
	}
	return l, nil
}

// SaveLedger persists the wallet.
func SaveLedger(stateDir string, l Ledger) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, ledgerFile), data, 0o600)
}

// Observe records a snapshot for provider+account, computing the forecast ETA
// from the supplied burn rate. nowMs/burnPerMin are passed in so the function is
// pure and testable (no wall-clock dependency).
func (l Ledger) Observe(provider, account string, snap Snapshot, burnPerMin float64, nowMs int64) {
	e := LedgerEntry{
		Provider:     provider,
		Account:      account,
		Remaining:    snap.Remaining,
		Total:        snap.Total,
		FractionUsed: snap.FractionUsed,
		ResetsAt:     snap.ResetsAt,
		Source:       snap.Source,
		UpdatedMs:    nowMs,
		BurnPerMin:   burnPerMin,
		EtaMinutes:   ForecastEtaMinutes(snap.Remaining, burnPerMin),
	}
	l[LedgerKey(provider, account)] = e
}

// ForecastEtaMinutes predicts minutes until exhaustion at the given burn rate.
// Returns -1 when it cannot be forecast (unknown remaining or non-positive burn).
func ForecastEtaMinutes(remaining int64, burnPerMin float64) float64 {
	if remaining < 0 || burnPerMin <= 0 {
		return -1
	}
	return float64(remaining) / burnPerMin
}
