package contract

import (
	"strings"
	"testing"
)

func sampleV2() *ContinuationContract {
	return &ContinuationContract{
		ContractID:     "c-test",
		SessionID:      "sess",
		TaskID:         "task",
		Version:        2,
		TaskGoal:       "Build login",
		NextAction:     "Add validation",
		InitialPrompt:  "Build a login form",
		Plan:           []string{"Write HTML", "Add validation", "Wire submit"},
		TasksRemaining: []string{"Add validation", "Wire submit"},
		SkillsLoaded:   []string{"frontend-patterns"},
		SkillsInUse:    []string{"react-patterns"},
		InFlightCode:   []InFlightFile{{Path: "src/login.tsx", Snippet: "const x = 1"}},
	}
}

func TestSerializeV2RendersRichSections(t *testing.T) {
	out := NewSerializer().Serialize(sampleV2(), DefaultCapability)
	for _, want := range []string{
		"## Original Prompt", "Build a login form",
		"## Plan", "- [x] Write HTML", "- [ ] Add validation",
		"## Skills", "In use:", "react-patterns", "frontend-patterns",
		"## In-Flight Code", "src/login.tsx", "const x = 1",
		"**Version:** 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("serialized v2 contract missing %q", want)
		}
	}
}

// The migration guard: a v1 contract must serialise without any v2 section even
// if the rich fields happen to be populated, so old contracts render unchanged.
func TestSerializeV1OmitsRichSections(t *testing.T) {
	c := sampleV2()
	c.Version = 1
	out := NewSerializer().Serialize(c, DefaultCapability)
	if strings.Contains(out, "## Original Prompt") || strings.Contains(out, "## In-Flight Code") {
		t.Error("v1 contract must not render v2 sections")
	}
}

func TestSignVerifyV2RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	b := NewBuilder(nil, key)
	c := sampleV2()
	if err := b.Sign(c); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if c.Signature == "" {
		t.Fatal("signature not set")
	}
	if err := b.Verify(c); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Tampering a rich v2 field must break the signature.
	c.InitialPrompt = "tampered"
	if err := b.Verify(c); err == nil {
		t.Fatal("verify should fail after tampering a v2 field")
	}
}
