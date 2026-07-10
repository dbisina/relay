// internal/contract/serializer.go
//
// ContractSerializer — Markdown-first serialisation.
//
// Rules (from PRD §TOON):
//   - DO NOT REDO in primacy position (first section)
//   - Sections with ≥8 rows AND eligibility ≥0.4 → TOON table
//   - Otherwise → Markdown table
//   - Compact JSON fallback for unstructured data
//
// Receiver capability governs whether TOON or Markdown is emitted.

package contract

import (
	"fmt"
	"strings"
)

const (
	toonRowThreshold         = 8
	toonEligibilityThreshold = 0.4
)

// ReceiverCapability describes what the receiving agent can parse.
type ReceiverCapability struct {
	SupportsTOON     bool
	SupportsMarkdown bool
}

// DefaultCapability is Markdown-only (conservative default).
var DefaultCapability = ReceiverCapability{
	SupportsTOON:     false,
	SupportsMarkdown: true,
}

// Serializer serialises ContinuationContracts to string.
type Serializer struct{}

// NewSerializer creates a Serializer.
func NewSerializer() *Serializer { return &Serializer{} }

// Serialize renders a contract to a string for injection into the next agent.
// DO NOT REDO section always appears first.
func (s *Serializer) Serialize(c *ContinuationContract, cap ReceiverCapability) string {
	var b strings.Builder

	b.WriteString("# Relay Continuation Contract\n\n")
	b.WriteString(fmt.Sprintf("**Session:** `%s`  \n", c.SessionID))
	b.WriteString(fmt.Sprintf("**Task:** `%s`  \n", c.TaskID))
	b.WriteString(fmt.Sprintf("**Snapshot:** `%s`  \n\n", c.SnapshotCommitSHA))

	// ── PRIMACY: DO NOT REDO ────────────────────────────────────────────────
	b.WriteString("## ⛔ DO NOT REDO (PRIMACY)\n\n")
	b.WriteString("The following work is COMPLETE. Do not repeat, refactor, or undo it.\n\n")
	if len(c.DoNotRedo) == 0 {
		b.WriteString("_(none recorded this session)_\n\n")
	} else {
		for _, item := range c.DoNotRedo {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
		b.WriteString("\n")
	}

	// ── GOAL & NEXT ACTION ──────────────────────────────────────────────────
	b.WriteString("## Task Goal\n\n")
	b.WriteString(c.TaskGoal + "\n\n")

	b.WriteString("## Next Action\n\n")
	b.WriteString(c.NextAction + "\n\n")

	// ── RICH SESSION INTENT (v2) ────────────────────────────────────────────
	// Migration: v1 contracts carry none of these fields, so the whole block is
	// skipped and their serialised form is byte-identical to before.
	if c.Version >= 2 {
		if c.InitialPrompt != "" {
			b.WriteString("## Original Prompt\n\n")
			b.WriteString(c.InitialPrompt + "\n\n")
		}
		if len(c.Plan) > 0 {
			b.WriteString("## Plan\n\n")
			remaining := make(map[string]bool, len(c.TasksRemaining))
			for _, t := range c.TasksRemaining {
				remaining[t] = true
			}
			for _, p := range c.Plan {
				box := "x"
				if remaining[p] {
					box = " "
				}
				b.WriteString(fmt.Sprintf("- [%s] %s\n", box, p))
			}
			b.WriteString("\n")
		} else if len(c.TasksRemaining) > 0 {
			b.WriteString("## Tasks Remaining\n\n")
			for _, t := range c.TasksRemaining {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", t))
			}
			b.WriteString("\n")
		}
		if len(c.SkillsLoaded)+len(c.SkillsInUse)+len(c.SkillsToUse) > 0 {
			b.WriteString("## Skills\n\n")
			if len(c.SkillsInUse) > 0 {
				b.WriteString(fmt.Sprintf("- **In use:** %s\n", strings.Join(c.SkillsInUse, ", ")))
			}
			if len(c.SkillsToUse) > 0 {
				b.WriteString(fmt.Sprintf("- **To use:** %s\n", strings.Join(c.SkillsToUse, ", ")))
			}
			if len(c.SkillsLoaded) > 0 {
				b.WriteString(fmt.Sprintf("- **Loaded:** %s\n", strings.Join(c.SkillsLoaded, ", ")))
			}
			b.WriteString("\n")
		}
		if len(c.InFlightCode) > 0 {
			b.WriteString("## In-Flight Code\n\n")
			for _, f := range c.InFlightCode {
				b.WriteString(fmt.Sprintf("**`%s`**\n", f.Path))
				if f.Snippet != "" {
					b.WriteString("```\n" + f.Snippet + "\n```\n")
				}
				b.WriteString("\n")
			}
		}
	}

	// ── ACCEPTANCE ASSERTIONS ───────────────────────────────────────────────
	b.WriteString("## Acceptance Assertions\n\n")
	b.WriteString("Task is complete ONLY when ALL of the following pass:\n\n")
	for i, a := range c.AcceptanceAssertions {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, a))
	}
	b.WriteString("\n")

	// ── DECISIONS ───────────────────────────────────────────────────────────
	b.WriteString("## Decisions\n\n")
	if len(c.Decisions) == 0 {
		b.WriteString("_(none)_\n\n")
	} else if s.eligibleForTOON(c.Decisions) && cap.SupportsTOON {
		b.WriteString(s.decisionsAsTOON(c.Decisions))
	} else {
		b.WriteString(s.decisionsAsMarkdown(c.Decisions))
	}

	// ── CONSTRAINTS ─────────────────────────────────────────────────────────
	b.WriteString("## Constraints\n\n")
	if len(c.Constraints) == 0 {
		b.WriteString("_(none)_\n\n")
	} else {
		for _, con := range c.Constraints {
			if con.Source != "" {
				b.WriteString(fmt.Sprintf("- **[%s]** %s\n", con.Source, con.Rule))
			} else {
				b.WriteString(fmt.Sprintf("- %s\n", con.Rule))
			}
		}
		b.WriteString("\n")
	}

	// ── FILE MANIFEST ────────────────────────────────────────────────────────
	b.WriteString("## File Manifest\n\n")
	if len(c.FileManifest) == 0 {
		b.WriteString("_(empty)_\n\n")
	} else {
		b.WriteString("| Path | Modified | SHA-256 |\n")
		b.WriteString("|------|----------|---------|\n")
		for _, f := range c.FileManifest {
			mod := "·"
			if f.Modified {
				mod = "✓"
			}
			sha := f.SHA256
			if len(sha) > 12 {
				sha = sha[:12] + "…"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | `%s` |\n", f.Path, mod, sha))
		}
		b.WriteString("\n")
	}

	// ── SIGNATURE ────────────────────────────────────────────────────────────
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("**Signature:** `%s`  \n", c.Signature))
	b.WriteString(fmt.Sprintf("**Version:** %d\n", c.Version))

	return b.String()
}

// eligibleForTOON checks if a slice of decisions meets the TOON eligibility threshold.
func (s *Serializer) eligibleForTOON(decisions []Decision) bool {
	if len(decisions) < toonRowThreshold {
		return false
	}
	// Eligibility = fraction of non-empty fields
	total := len(decisions) * 2 // Summary + Rationale per row
	nonEmpty := 0
	for _, d := range decisions {
		if d.Summary != "" {
			nonEmpty++
		}
		if d.Rationale != "" {
			nonEmpty++
		}
	}
	eligibility := float64(nonEmpty) / float64(total)
	return eligibility >= toonEligibilityThreshold
}

func (s *Serializer) decisionsAsMarkdown(decisions []Decision) string {
	var b strings.Builder
	b.WriteString("| Decision | Rationale |\n")
	b.WriteString("|----------|----------|\n")
	for _, d := range decisions {
		b.WriteString(fmt.Sprintf("| %s | %s |\n", d.Summary, d.Rationale))
	}
	b.WriteString("\n")
	return b.String()
}

func (s *Serializer) decisionsAsTOON(decisions []Decision) string {
	// TOON v3.3 tabular format
	var b strings.Builder
	b.WriteString("```toon\n")
	b.WriteString("@table decisions\n")
	b.WriteString("cols: Summary | Rationale\n")
	for _, d := range decisions {
		b.WriteString(fmt.Sprintf("| %s | %s |\n", d.Summary, d.Rationale))
	}
	b.WriteString("@end\n")
	b.WriteString("```\n\n")
	return b.String()
}
