// internal/contract/golden_test.go
//
// Golden-fixture tests for the signed JSON sidecar format. The fixtures under
// testdata/ are the checked-in canonical serialisations of a v2 and a v1
// contract, signed with a fixed test-only key. They pin the wire format:
// round-tripping must be byte-stable and Verify must catch any tampering.
//
// Regenerate after an intentional schema change with:
//
//	go test ./internal/contract -run TestContractGoldenFixtures -update

package contract

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden fixtures under testdata/")

// goldenKey is a fixed HMAC-SHA256 key for fixtures only. Never use it
// outside tests; real keys come from .relay/.signing-key.
var goldenKey = []byte("relay-golden-test-key-0123456789")

func goldenV2() *ContinuationContract {
	return &ContinuationContract{
		ContractID:     "c-golden01-1750000000000",
		SessionID:      "golden01-sess",
		TaskID:         "task-golden",
		CreatedAt:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Version:        2,
		TaskGoal:       "Add refund flow to the billing service",
		NextAction:     "Wire RefundHandler into the router",
		DoNotRedo:      []string{"POST /refund endpoint scaffolding", "refunds DB migration"},
		InitialPrompt:  "Add a refund flow to the billing service",
		Plan:           []string{"Scaffold endpoint", "Add DB migration", "Wire handler", "Add tests"},
		TasksRemaining: []string{"Wire handler", "Add tests"},
		SkillsLoaded:   []string{"backend-patterns"},
		SkillsInUse:    []string{"go-concurrency-patterns"},
		SkillsToUse:    []string{"testing-patterns"},
		InFlightCode: []InFlightFile{
			{Path: "internal/billing/refund.go", Snippet: "func (h *RefundHandler) ServeHTTP("},
		},
		AcceptanceAssertions: []string{
			"go test ./internal/billing/... passes",
			"POST /refund returns 200 on the happy path",
		},
		FileManifest: []FileEntry{
			{Path: "internal/billing/refund.go", SHA256: "aa11bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff00112233", Modified: true},
			{Path: "internal/billing/router.go", SHA256: "bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff001122334455", Modified: false},
		},
		Decisions: []Decision{
			{Summary: "Use idempotency keys for refunds", Rationale: "protects against double-submit"},
		},
		Constraints: []Constraint{
			{Rule: "Never refund more than the original charge", Source: "PRD BIL-4"},
		},
		BaseCommitSHA:     "0123456789abcdef0123456789abcdef01234567",
		SnapshotCommitSHA: "89abcdef0123456789abcdef0123456789abcdef",
	}
}

func goldenV1() *ContinuationContract {
	return &ContinuationContract{
		ContractID:           "c-golden01-1700000000000",
		SessionID:            "golden01-sess",
		TaskID:               "task-golden-v1",
		CreatedAt:            time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Version:              1,
		TaskGoal:             "Add refund flow to the billing service",
		NextAction:           "Wire RefundHandler into the router",
		DoNotRedo:            []string{"POST /refund endpoint scaffolding"},
		AcceptanceAssertions: []string{"go test ./internal/billing/... passes"},
		FileManifest: []FileEntry{
			{Path: "internal/billing/refund.go", SHA256: "aa11bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff00112233", Modified: true},
		},
		BaseCommitSHA:     "0123456789abcdef0123456789abcdef01234567",
		SnapshotCommitSHA: "89abcdef0123456789abcdef0123456789abcdef",
	}
}

func goldenPath(name string) string { return filepath.Join("testdata", name) }

func signGolden(t *testing.T, c *ContinuationContract) *ContinuationContract {
	t.Helper()
	if err := NewBuilder(nil, goldenKey).Sign(c); err != nil {
		t.Fatalf("sign golden contract: %v", err)
	}
	return c
}

func TestContractGoldenFixtures(t *testing.T) {
	fixtures := []struct {
		file  string
		build func() *ContinuationContract
	}{
		{"contract_v2.json", goldenV2},
		{"contract_v1.json", goldenV1},
	}

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		for _, f := range fixtures {
			if err := WriteJSON(goldenPath(f.file), signGolden(t, f.build())); err != nil {
				t.Fatalf("write %s: %v", f.file, err)
			}
		}
	}

	for _, f := range fixtures {
		t.Run(f.file, func(t *testing.T) {
			c, err := ReadJSON(goldenPath(f.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			// (a) Round-trip stability: re-marshalling the parsed contract
			// must reproduce the checked-in bytes (modulo checkout EOLs).
			remarshalled, err := json.MarshalIndent(c, "", "  ")
			if err != nil {
				t.Fatalf("re-marshal: %v", err)
			}
			raw, err := os.ReadFile(goldenPath(f.file))
			if err != nil {
				t.Fatalf("read raw fixture: %v", err)
			}
			want := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
			want = bytes.TrimRight(want, "\n")
			if !bytes.Equal(remarshalled, want) {
				t.Errorf("round-trip drift:\n got: %s\nwant: %s", remarshalled, want)
			}

			// The fixture signature must match a fresh signing of the in-code
			// golden definition, so file and definition cannot drift apart.
			if fresh := signGolden(t, f.build()); fresh.Signature != c.Signature {
				t.Errorf("fixture signature %s does not match freshly signed definition %s",
					c.Signature, fresh.Signature)
			}

			// (b) Verify passes with the right key and restores the signature.
			b := NewBuilder(nil, goldenKey)
			sigBefore := c.Signature
			if err := b.Verify(c); err != nil {
				t.Fatalf("verify with correct key: %v", err)
			}
			if c.Signature != sigBefore {
				t.Error("Verify must restore the Signature field")
			}

			// Tampering any signed field must break verification.
			c.TaskGoal = "tampered"
			if err := b.Verify(c); err == nil {
				t.Error("verify should fail after tampering TaskGoal")
			}

			// The wrong key must fail verification too.
			c2, err := ReadJSON(goldenPath(f.file))
			if err != nil {
				t.Fatalf("re-read fixture: %v", err)
			}
			wrong := NewBuilder(nil, []byte("wrong-key-wrong-key-wrong-key-00"))
			if err := wrong.Verify(c2); err == nil {
				t.Error("verify should fail with the wrong key")
			}
		})
	}
}

// TestGoldenV1SerializesWithoutRichSections is the migration guard: a v1
// fixture must render without any v2-only section so old contracts inject
// byte-identically to how they always did.
func TestGoldenV1SerializesWithoutRichSections(t *testing.T) {
	c, err := ReadJSON(goldenPath("contract_v1.json"))
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}
	out := NewSerializer().Serialize(c, DefaultCapability)
	for _, banned := range []string{"## Original Prompt", "## Plan", "## Skills", "## In-Flight Code"} {
		if strings.Contains(out, banned) {
			t.Errorf("v1 contract must not render %q", banned)
		}
	}
	if !strings.Contains(out, "**Version:** 1") {
		t.Error("serialized v1 contract missing version footer")
	}
}
