// profiles.go — task-routing profile CRUD.
//
// Each profile maps task kinds to an ordered provider chain.
// Stored in .relay/relay.toml under [profiles.NAME]; round-tripped via API.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dbisina/relay/internal/config"
)

// ApiProfile is the JSON shape exposed by /api/profiles.
type ApiProfile struct {
	Name        string   `json:"name"`
	Chain       []string `json:"chain"`
	Kinds       []string `json:"kinds"`
	Skills      []string `json:"skills"`
	ContextHint string   `json:"contextHint"`
}

func profilesToAPI(cfg *config.Config) []ApiProfile {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ApiProfile, 0, len(names))
	for _, n := range names {
		p := cfg.Profiles[n]
		out = append(out, ApiProfile{
			Name:        n,
			Chain:       append([]string{}, p.Chain...),
			Kinds:       append([]string{}, p.Kinds...),
			Skills:      append([]string{}, p.Skills...),
			ContextHint: p.ContextHint,
		})
	}
	return out
}

// UpdateProfileRequest is the body for POST /api/profiles.
type UpdateProfileRequest struct {
	Name        string   `json:"name"`
	Chain       []string `json:"chain"`
	Kinds       []string `json:"kinds"`
	Skills      []string `json:"skills"`
	ContextHint string   `json:"contextHint"`
	Delete      bool     `json:"delete"`
}

// writeProfileConfig patches a single [profiles.NAME] block in relay.toml.
// Brutally simple: read all lines, drop existing [profiles.NAME] block (and
// trailing keys until next section), append fresh block at end. If Delete
// flag set, just drop block.
func writeProfileConfig(tomlPath string, req UpdateProfileRequest) error {
	data, err := os.ReadFile(tomlPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	target := "[profiles." + req.Name + "]"
	var out []string
	skip := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if t == target {
				skip = true
				continue
			}
			if skip {
				skip = false
			}
		}
		if skip {
			continue
		}
		out = append(out, line)
	}

	if !req.Delete {
		out = append(out, "", target,
			"chain        = "+tomlStringArray(req.Chain),
			"kinds        = "+tomlStringArray(req.Kinds),
			"skills       = "+tomlStringArray(req.Skills),
			"context_hint = "+tomlQuote(req.ContextHint),
		)
	}

	return os.WriteFile(tomlPath, []byte(strings.Join(out, "\n")), 0600)
}

func tomlStringArray(items []string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = tomlQuote(it)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlQuote(s string) string {
	b, _ := json.Marshal(s) // gives proper quoted/escaped string
	return string(b)
}
