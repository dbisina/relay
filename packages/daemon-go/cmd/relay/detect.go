// detect.go — `relay detect` CLI + the daemon-side detection/adoption helpers.
//
// Detection finds AI coding agents already running on this machine (ones Relay
// did not spawn) and lifts their session intent. Adoption renders that intent
// into a portable continuation brief another provider can resume from.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dbisina/relay/internal/config"
	"github.com/dbisina/relay/internal/contract"
	"github.com/dbisina/relay/internal/detect"
	"github.com/dbisina/relay/internal/redact"
)

// safeID makes a detected-agent id safe for use as a filename.
func safeID(id string) string {
	return strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(id)
}

// contractFromDetected builds a v2 continuation contract from a detected agent's
// lifted session intent. This is pillar 2's core: the detector feeds Relay's
// signed, unified handoff format rather than a one-off Markdown brief.
func contractFromDetected(a detect.DetectedAgent) *contract.ContinuationContract {
	c := &contract.ContinuationContract{
		ContractID: "adopt-" + safeID(a.ID),
		SessionID:  a.ID,
		TaskID:     a.ID,
		CreatedAt:  time.Now().UTC(),
		Version:    2,
	}
	s := a.Session
	if s == nil {
		c.TaskGoal = "Continue " + a.DisplayName + " work (no transcript was captured)"
		c.NextAction = "Re-run from the original prompt"
		return c
	}

	if s.SessionID != "" {
		c.SessionID, c.TaskID = s.SessionID, s.SessionID
	}
	c.InitialPrompt = s.InitialPrompt
	c.TaskGoal = firstNonEmpty(s.InitialPrompt, s.LastPrompt, "(goal unknown)")
	c.NextAction = "Resume the adopted session"
	if len(s.TasksRemaining) > 0 {
		c.NextAction = s.TasksRemaining[0]
	}
	c.Plan = s.Plan
	c.TasksRemaining = s.TasksRemaining
	c.SkillsLoaded = s.Skills

	// Completed plan items become primacy DO-NOT-REDO entries.
	remaining := make(map[string]bool, len(s.TasksRemaining))
	for _, t := range s.TasksRemaining {
		remaining[t] = true
	}
	for _, p := range s.Plan {
		if !remaining[p] {
			c.DoNotRedo = append(c.DoNotRedo, p)
		}
	}
	for _, f := range s.FilesTouched {
		c.FileManifest = append(c.FileManifest, contract.FileEntry{Path: f, Modified: true})
	}
	if s.Model != "" {
		c.Decisions = append(c.Decisions, contract.Decision{
			Summary:   "Source agent ran on " + s.Model,
			Rationale: "preserve model continuity where the target supports it",
		})
	}
	return c
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// scanDetected runs a redacted detection pass. Every string that crosses the
// API/CLI boundary is scrubbed first (CLAUDE.md: nothing bypasses redact).
func scanDetected() ([]detect.DetectedAgent, error) {
	return scanDetectedSince(0)
}

// scanDetectedSince is scanDetected with an explicit recency window in hours
// (0 = use the default). Lets the CLI surface sessions paused more than a day
// ago, which the 24h default hides.
func scanDetectedSince(maxAgeHours int) ([]detect.DetectedAgent, error) {
	red := redact.NewDefault()
	opts := detect.DefaultOptions()
	if maxAgeHours > 0 {
		opts.MaxAgeHours = maxAgeHours
	}
	opts.Redact = red.Scrub
	return detect.Scan(opts)
}

// adoptDetected renders a handoff brief for the agent with id, persists it to
// .relay/adopted/<id>.md, and returns the markdown + path.
func adoptDetected(workDir, id, target string) (map[string]interface{}, error) {
	return adoptDetectedSince(workDir, id, target, 0)
}

func adoptDetectedSince(workDir, id, target string, maxAgeHours int) (map[string]interface{}, error) {
	agents, err := scanDetectedSince(maxAgeHours)
	if err != nil {
		return nil, err
	}
	var found *detect.DetectedAgent
	for i := range agents {
		if agents[i].ID == id {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no detected agent with id %q (it may have exited; re-run detect)", id)
	}
	md := detect.RenderHandoff(*found, target)

	dir := filepath.Join(workDir, ".relay", "adopted")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	safe := safeID(id)
	path := filepath.Join(dir, safe+".md")
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		return nil, err
	}

	// v2: also emit a signed continuation contract built from the lifted intent,
	// so an adopted session enters Relay's unified, verifiable handoff format.
	contractPath := filepath.Join(dir, safe+".contract.json")
	c := contractFromDetected(*found)
	if key, kerr := contract.LoadSigningKey(filepath.Join(workDir, ".relay", ".signing-key")); kerr == nil {
		_ = contract.NewBuilder(nil, key).Sign(c)
	}
	if werr := contract.WriteJSON(contractPath, c); werr != nil {
		return nil, werr
	}

	return map[string]interface{}{
		"id":           id,
		"target":       target,
		"path":         path,
		"markdown":     md,
		"contractPath": contractPath,
	}, nil
}

// adoptedTask wraps a rendered handoff brief in a resume instruction so the
// receiving agent treats it as work to continue rather than a document to read.
// The full brief is inlined (not just a file path) so the task survives across
// providers regardless of whether the agent reads .relay/adopted/ itself.
func adoptedTask(brief string) string {
	return "Continue this adopted coding session. Resume the remaining work exactly " +
		"where it left off; do not redo completed tasks.\n\n" + brief
}

// targetLabel renders a provider name for logs, defaulting to "any provider".
func targetLabel(target string) string {
	if target == "" {
		return "any provider"
	}
	return target
}

func cmdDetect() *cobra.Command {
	var asJSON, start bool
	var target, adoptID string
	var sinceHours int

	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect AI coding agents already running on this machine",
		Long: "Scan running processes and on-disk session transcripts for known\n" +
			"coding agents (Claude Code, Codex, Copilot, Ollama, OpenCode, …),\n" +
			"extract what each one was doing, and optionally render a handoff\n" +
			"brief to continue that work in another agent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()

			if adoptID != "" {
				res, err := adoptDetectedSince(workDir, adoptID, target, sinceHours)
				if err != nil {
					return err
				}
				brief := res["markdown"].(string)
				fmt.Println(brief)
				fmt.Fprintf(os.Stderr, "\n↳ brief written to %s\n", res["path"])
				if start {
					cfg, cerr := config.Load(workDir)
					if cerr != nil {
						return fmt.Errorf("start: load config: %w", cerr)
					}
					var pin []string
					if target != "" {
						pin = []string{target}
					}
					priority := buildProviderPriority(cfg, pin)
					fmt.Fprintf(os.Stderr, "↳ starting session on %s…\n", formatProviders(priority))
					return runSession(workDir, cfg, adoptedTask(brief), priority, 0.85, 0, nil, true, len(pin) > 0)
				}
				return nil
			}

			agents, err := scanDetectedSince(sinceHours)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(agents)
			}
			printDetected(agents)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().StringVar(&adoptID, "adopt", "", "render a handoff brief for the agent with this id")
	cmd.Flags().StringVar(&target, "target", "", "target provider for the adopted brief")
	cmd.Flags().BoolVar(&start, "start", false, "after adopting, start a Relay session to continue the work")
	cmd.Flags().IntVar(&sinceHours, "since-hours", 24, "only show sessions active within the last N hours")
	return cmd
}

func printDetected(agents []detect.DetectedAgent) {
	if len(agents) == 0 {
		fmt.Println("No agents detected (no matching process or recent transcript).")
		return
	}
	for _, a := range agents {
		var status string
		switch {
		case a.Running && a.PID > 0:
			status = fmt.Sprintf("running pid %d", a.PID)
		case a.Running:
			status = "active recently"
		case a.Session != nil:
			status = "idle session"
		default:
			status = "process only"
		}
		fmt.Printf("● %-16s %-26s [%s]\n", a.DisplayName, a.ID, status)
		if a.WorkDir != "" {
			fmt.Printf("    dir:    %s\n", a.WorkDir)
		}
		if s := a.Session; s != nil {
			if s.InitialPrompt != "" {
				fmt.Printf("    goal:   %s\n", oneLine(s.InitialPrompt, 80))
			}
			if len(s.TasksRemaining) > 0 {
				fmt.Printf("    todo:   %d left · %s\n", len(s.TasksRemaining), oneLine(strings.Join(s.TasksRemaining, "; "), 68))
			}
			if len(s.Skills) > 0 {
				fmt.Printf("    skills: %s\n", strings.Join(s.Skills, ", "))
			}
			if len(s.Mcps) > 0 {
				fmt.Printf("    mcps:   %s\n", strings.Join(s.Mcps, ", "))
			}
			fmt.Printf("    usage:  %d in / %d out tokens · %d msgs\n", s.TokensIn, s.TokensOut, s.MessageCount)
		}
		fmt.Printf("    adopt:  relay detect --adopt %s --target <provider>\n\n", a.ID)
	}
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
