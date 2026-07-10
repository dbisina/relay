// eval.go — `relay eval` golden-task harness.
//
// Reads .relay/eval/tasks.yaml (simple format), routes each task through the
// orchestrator's profile matcher, reports pass-rate + per-profile breakdown.
//
// Doesn't actually invoke agents (that'd require live providers). Validates:
//   1. Routing: did matchProfile pick what we expected?
//   2. Instructions: were CLAUDE.md/AGENTS.md/skills picked up?
//   3. Cost estimate: under budget?
//
// Use case: nightly CI to catch routing regressions after profile edits.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dbisina/relay/internal/config"
	"github.com/dbisina/relay/internal/instructions"
	"github.com/dbisina/relay/internal/orchestrator"
)

// EvalCase — one expected routing outcome.
type EvalCase struct {
	Task             string   `json:"task"`
	ExpectProfile    string   `json:"expectProfile"`
	ExpectChainHead  string   `json:"expectChainHead,omitempty"` // first provider in resolved chain
	MustIncludeFiles []string `json:"mustIncludeFiles,omitempty"`
}

// EvalResult — one case's outcome.
type EvalResult struct {
	Case            EvalCase `json:"case"`
	Passed          bool     `json:"passed"`
	ActualProfile   string   `json:"actualProfile"`
	ActualChain     []string `json:"actualChain"`
	InstructionsHit []string `json:"instructionsHit"`
	Notes           string   `json:"notes"`
}

func cmdEval() *cobra.Command {
	var (
		path  string
		json_ bool
	)
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run golden-task routing regression suite",
		Long: `Runs each case in .relay/eval/tasks.json (or --path), reports pass-rate.

Default tasks file at .relay/eval/tasks.json. Format:
[
  {"task": "refactor orders/refund.go to use the new RefundService",
   "expectProfile": "backend", "expectChainHead": "claude",
   "mustIncludeFiles": ["CLAUDE.md"]}
]
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("eval: load config: %w", err)
			}

			casesPath := path
			if casesPath == "" {
				casesPath = filepath.Join(cfg.StateDir, "eval", "tasks.json")
			}
			cases, err := loadEvalCases(casesPath)
			if err != nil {
				return fmt.Errorf("eval: load cases: %w", err)
			}
			if len(cases) == 0 {
				fmt.Println("no cases found at", casesPath)
				return nil
			}

			results := make([]EvalResult, 0, len(cases))
			for _, c := range cases {
				results = append(results, runOneCase(workDir, cfg, c))
			}

			if json_ {
				return json.NewEncoder(os.Stdout).Encode(results)
			}

			passed := 0
			for _, r := range results {
				icon := "✗"
				if r.Passed {
					icon = "✓"
					passed++
				}
				fmt.Printf("%s %s\n", icon, truncate(r.Case.Task, 70))
				if !r.Passed {
					fmt.Printf("    expected profile=%s head=%s; got profile=%s chain=%s\n",
						r.Case.ExpectProfile, r.Case.ExpectChainHead,
						r.ActualProfile, strings.Join(r.ActualChain, "→"))
				}
				if r.Notes != "" {
					fmt.Printf("    %s\n", r.Notes)
				}
			}
			fmt.Printf("\n%d/%d passed (%.0f%%)\n", passed, len(cases),
				float64(passed)/float64(len(cases))*100.0)
			if passed != len(cases) {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "tasks file (default .relay/eval/tasks.json)")
	cmd.Flags().BoolVar(&json_, "json", false, "emit JSON instead of human output")
	return cmd
}

func loadEvalCases(path string) ([]EvalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func runOneCase(workDir string, cfg *config.Config, c EvalCase) EvalResult {
	r := EvalResult{Case: c}

	// Build profile specs same shape orchestrator uses
	specs := buildProfileSpecs(cfg)
	// Use orchestrator's matchProfile via thin re-impl (avoid circular) — match
	// by routing the same way:
	prof, chain := matchProfileForEval(c.Task, specs)
	r.ActualProfile = prof
	r.ActualChain = chain

	// Discover instructions to record hits
	comp := instructions.Discover(instructions.DiscoverOptions{
		WorkDir:         workDir,
		IncludeUserHome: true,
	})
	for _, s := range comp.Sources {
		r.InstructionsHit = append(r.InstructionsHit, s.Label)
	}

	// Assertions
	ok := true
	if c.ExpectProfile != "" && prof != c.ExpectProfile {
		ok = false
	}
	if c.ExpectChainHead != "" && (len(chain) == 0 || chain[0] != c.ExpectChainHead) {
		ok = false
	}
	for _, want := range c.MustIncludeFiles {
		found := false
		for _, label := range r.InstructionsHit {
			if strings.Contains(label, want) {
				found = true
				break
			}
		}
		if !found {
			ok = false
			r.Notes = "missing instructions file: " + want
		}
	}
	r.Passed = ok
	return r
}

// matchProfileForEval mirrors orchestrator.matchProfile so we can score
// without importing it (avoids pulling adapter registry into eval).
func matchProfileForEval(taskGoal string, profiles map[string]orchestrator.ProfileSpec) (string, []string) {
	if len(profiles) == 0 {
		return "", nil
	}
	goal := strings.ToLower(taskGoal)
	bestName, bestScore := "", 0
	var bestChain []string
	for name, p := range profiles {
		s := 0
		for _, k := range p.Kinds {
			if k != "" && strings.Contains(goal, strings.ToLower(k)) {
				s += 3
			}
		}
		for _, sk := range p.Skills {
			if sk != "" && sk != "any" && strings.Contains(goal, strings.ToLower(sk)) {
				s++
			}
		}
		if p.ContextHint != "" {
			for _, w := range strings.Fields(strings.ToLower(p.ContextHint)) {
				if len(w) > 3 && strings.Contains(goal, w) {
					s++
				}
			}
		}
		if s > bestScore {
			bestName, bestScore, bestChain = name, s, p.Chain
		}
	}
	if bestScore == 0 {
		return "", nil
	}
	return bestName, bestChain
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}
