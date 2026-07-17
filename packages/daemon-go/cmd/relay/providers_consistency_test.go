// cmd/relay/providers_consistency_test.go — cross-registry consistency.
//
// Adding a provider requires touching the adapter registry, quota registry,
// pricing table, and CLI metadata in sync. Nothing in the type system enforces
// that, so a partially added provider compiles fine and degrades (or used to
// panic) at runtime. This test turns the sync requirement into a build gate.

package main

import (
	"testing"

	"github.com/dbisina/relay/internal/adapter"
	"github.com/dbisina/relay/internal/pricing"
	"github.com/dbisina/relay/internal/quota"
)

func TestProviderRegistriesConsistent(t *testing.T) {
	adapters := adapter.BuildAdapterRegistry(0, "", "")
	if len(adapters) == 0 {
		t.Fatal("adapter registry is empty")
	}

	caps := make(map[adapter.ProviderName]int64, len(adapters))
	for name := range adapters {
		caps[name] = 1000
	}
	quotaReg, err := quota.BuildQuotaRegistry(caps)
	if err != nil {
		t.Fatalf("BuildQuotaRegistry: %v", err)
	}
	defer quotaReg.Close() //nolint:errcheck

	metaNames := make(map[string]bool, len(allProviderMeta))
	for _, m := range allProviderMeta {
		metaNames[m.Name] = true
	}

	for name := range adapters {
		n := string(name)
		if qa, ok := quotaReg.Adapters[name]; !ok || qa == nil {
			t.Errorf("provider %q: registered adapter but no quota adapter (internal/quota/registry.go)", n)
		}
		if _, ok := pricing.Table[n]; !ok {
			t.Errorf("provider %q: registered adapter but no pricing entry (internal/pricing/pricing.go)", n)
		}
		if !metaNames[n] {
			t.Errorf("provider %q: registered adapter but no allProviderMeta entry (cmd/relay/providers.go)", n)
		}
	}

	// The reverse direction: metadata for providers that have no adapter is
	// probably a leftover from a removed provider.
	for _, m := range allProviderMeta {
		if _, ok := adapters[adapter.ProviderName(m.Name)]; !ok {
			t.Errorf("metadata %q has no adapter in BuildAdapterRegistry (internal/adapter/registry.go)", m.Name)
		}
	}
}
