// internal/config/accounts.go — account-aware handoff (pillar 3).
//
// Accounts are DEFINED in relay.toml (static: label, config dir, env). Which one
// is ACTIVE is runtime state, persisted to .relay/active-accounts.json so a
// switch survives restarts without rewriting the TOML. The resolver turns the
// active account into the env overrides an adapter spawns the agent with.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// accountConfigEnvVar maps a provider to the env var its CLI reads for an
// alternate config/credential directory. Providers absent here can still switch
// accounts via the account's explicit Env entries.
var accountConfigEnvVar = map[string]string{
	"claude": "CLAUDE_CONFIG_DIR",
	"codex":  "CODEX_HOME",
}

// accountPtr returns a pointer to the named account on pc, creating it if absent.
// Callers must re-fetch per use: append may reallocate the backing slice.
func accountPtr(pc *ProviderConfig, label string) *Account {
	for i := range pc.Accounts {
		if pc.Accounts[i].Label == label {
			return &pc.Accounts[i]
		}
	}
	pc.Accounts = append(pc.Accounts, Account{Label: label})
	return &pc.Accounts[len(pc.Accounts)-1]
}

// ActiveAccount returns the active account for a provider: the one flagged
// Active, else the first declared, else nil when the provider has no accounts.
func (c *Config) ActiveAccount(provider string) *Account {
	pc := c.Providers[provider]
	if pc == nil || len(pc.Accounts) == 0 {
		return nil
	}
	for i := range pc.Accounts {
		if pc.Accounts[i].Active {
			return &pc.Accounts[i]
		}
	}
	return &pc.Accounts[0]
}

// accountEnvOverrides turns one account into KEY=VAL env overrides: the
// provider's config-dir var (if mapped) plus the account's explicit Env.
func accountEnvOverrides(provider string, acc *Account) []string {
	if acc == nil {
		return nil
	}
	var env []string
	if acc.ConfigDir != "" {
		if v, ok := accountConfigEnvVar[provider]; ok {
			env = append(env, v+"="+acc.ConfigDir)
		}
	}
	return append(env, acc.Env...)
}

// AccountEnv returns the env overrides for a provider's active account, ready to
// append to adapter.RunOptions.Env. Empty when the provider has no accounts.
func (c *Config) AccountEnv(provider string) []string {
	return accountEnvOverrides(provider, c.ActiveAccount(provider))
}

// AccountEnvFor returns the env overrides for a specific account label. Used by
// account-failover, which needs each account's env, not just the active one.
func (c *Config) AccountEnvFor(provider, label string) []string {
	pc := c.Providers[provider]
	if pc == nil {
		return nil
	}
	for i := range pc.Accounts {
		if pc.Accounts[i].Label == label {
			return accountEnvOverrides(provider, &pc.Accounts[i])
		}
	}
	return nil
}

// AllAccountEnv builds provider→env for every provider that has accounts, for
// passing to the orchestrator in one shot.
func (c *Config) AllAccountEnv() map[string][]string {
	out := map[string][]string{}
	for name, pc := range c.Providers {
		if len(pc.Accounts) > 0 {
			if env := c.AccountEnv(name); len(env) > 0 {
				out[name] = env
			}
		}
	}
	return out
}

// ApplyActiveAccounts overlays a runtime {provider: label} selection onto the
// declared accounts, flipping the Active flag. Unknown providers/labels are
// ignored so a stale state file never breaks load.
func (c *Config) ApplyActiveAccounts(sel map[string]string) {
	for provider, label := range sel {
		pc := c.Providers[provider]
		if pc == nil {
			continue
		}
		for i := range pc.Accounts {
			pc.Accounts[i].Active = pc.Accounts[i].Label == label
		}
	}
}

const activeAccountsFile = "active-accounts.json"

// LoadActiveAccounts reads the runtime active-account selection. A missing file
// is not an error — it just means "use the TOML defaults".
func LoadActiveAccounts(stateDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, activeAccountsFile))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sel map[string]string
	if err := json.Unmarshal(data, &sel); err != nil {
		return nil, err
	}
	if sel == nil {
		sel = map[string]string{}
	}
	return sel, nil
}

// SaveActiveAccounts persists the runtime active-account selection.
func SaveActiveAccounts(stateDir string, sel map[string]string) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, activeAccountsFile), data, 0o600)
}
