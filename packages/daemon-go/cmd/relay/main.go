// cmd/relay/main.go
//
// Relay CLI — single binary, ~10 MB static.
//
// Commands:
//   relay init              — initialise .relay/ in current directory
//   relay run <task>        — run a task (starts daemon + session)
//   relay status            — show current session state
//   relay handoff           — trigger manual handoff via HTTP
//   relay resume            — write receiver heartbeat to unblock DISPATCHED state
//   relay audit verify      — verify audit log hash chain integrity
//   relay graph             — print graph node/edge counts

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dbisina/relay/internal/adapter"
	"github.com/dbisina/relay/internal/audit"
	"github.com/dbisina/relay/internal/config"
	"github.com/dbisina/relay/internal/contract"
	"github.com/dbisina/relay/internal/fsm"
	"github.com/dbisina/relay/internal/graph"
	"github.com/dbisina/relay/internal/orchestrator"
	"github.com/dbisina/relay/internal/quota"
	"github.com/dbisina/relay/internal/server"
	"github.com/dbisina/relay/internal/verify"
)

const banner = `
  ██████╗ ███████╗██╗      █████╗ ██╗   ██╗
  ██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝
  ██████╔╝█████╗  ██║     ███████║ ╚████╔╝
  ██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝
  ██║  ██║███████╗███████╗██║  ██║   ██║
  ╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝

  vendor-neutral AI coding agent orchestrator
  claude · codex · antigravity · opencode · ollama · copilot · continue · cline
`

func main() {
	root := &cobra.Command{
		Use:   "relay",
		Short: "Relay — vendor-neutral AI coding agent orchestrator",
		Long:  banner,
	}

	root.AddCommand(
		cmdInit(),
		cmdRun(),
		cmdDaemon(),
		cmdStatus(),
		cmdHandoff(),
		cmdResume(),
		cmdAudit(),
		cmdGraph(),
		cmdTUI(),
		cmdEval(),
		cmdMCP(),
		cmdDetect(),
	)

	// Open TUI when invoked with no args in an interactive terminal
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if isatty() {
			return runTUI()
		}
		return cmd.Help()
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ─── relay init ───────────────────────────────────────────────────────────────

func cmdDaemon() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the dashboard API without starting an agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("daemon: load config: %w", err)
			}
			if port != 0 {
				cfg.ServerPort = port
			}
			// Apply the persisted active-account selection so listings + handoffs
			// agree on which login is current (pillar 3).
			if sel, serr := config.LoadActiveAccounts(cfg.StateDir); serr == nil {
				cfg.ApplyActiveAccounts(sel)
			}
			if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
				return fmt.Errorf("daemon: create state dir: %w", err)
			}

			// Prepend user-local install dirs so adapters can spawn agy, claude, etc.
			augmentPATH()
			// Source .relay/.env so spawned agents inherit provider API keys
			loadEnvFileIntoEnv(workDir)

			httpServer := server.New(cfg.ServerPort)
			if err := httpServer.Start(); err != nil {
				return err
			}
			defer httpServer.Stop()

			httpServer.PushProviders(configuredProviderStatuses(cfg))
			pushStoredGraphSummary(httpServer, cfg.DBPath)

			// Retrieval over graph chunks (FTS5). Used by MCP server,
			// agents, and the IDE for context-economical lookups.
			httpServer.SetRetrievalHandler(func(query string, limit int) (interface{}, error) {
				gs, err := graph.Open(cfg.DBPath)
				if err != nil {
					return nil, err
				}
				defer gs.Close()
				chunks, err := gs.SearchChunks(context.Background(), query, limit)
				if err != nil {
					return nil, err
				}
				return chunks, nil
			})

			// Graph fetch scoped by project
			httpServer.SetGraphProjectHandler(func(project string) (interface{}, error) {
				gs, err := graph.Open(cfg.DBPath)
				if err != nil {
					return nil, err
				}
				defer gs.Close()
				recent, err := gs.RecentForProject(context.Background(), project, 500)
				if err != nil {
					return nil, err
				}

				apiNodes := make([]server.ApiGraphNode, 0, len(recent.Nodes))
				for _, n := range recent.Nodes {
					apiNodes = append(apiNodes, server.ApiGraphNode{
						ID:       n.ID,
						NodeType: n.NodeType,
						Weight:   n.Weight,
						Label:    graphNodeLabel(n),
					})
				}
				apiEdges := make([]server.ApiGraphEdge, 0, len(recent.Edges))
				for _, e := range recent.Edges {
					apiEdges = append(apiEdges, server.ApiGraphEdge{
						FromID:   e.FromID,
						ToID:     e.ToID,
						EdgeType: e.EdgeType,
					})
				}
				return map[string]interface{}{"nodes": apiNodes, "edges": apiEdges}, nil
			})
			httpServer.PushEvent("system", "daemon ready; run relay run \"your task\" or use the desktop app to start a session")

			// Provider config/probe handlers
			tomlPath := filepath.Join(cfg.StateDir, "relay.toml")
			emit := func(tag, msg string) {
				httpServer.PushEvent(tag, msg)
			}
			httpServer.SetProviderHandlers(
				// GET /api/config/providers — probe all providers
				func() []interface{} {
					brief := make(map[string]providerCfgBrief)
					for name, pc := range cfg.Providers {
						brief[name] = providerCfgBrief{
							enabled: pc.Enabled,
							cap:     pc.DeclaredCap,
							model:   pc.Model,
							baseURL: pc.BaseURL,
						}
					}
					details := decorateAccounts(cfg, probeAll(brief))
					out := make([]interface{}, len(details))
					for i, d := range details {
						out[i] = d
					}
					return out
				},
				// POST /api/config/providers — update one provider
				func(body []byte) error {
					var req UpdateProviderRequest
					if err := json.Unmarshal(body, &req); err != nil {
						return err
					}
					if err := writeProviderConfig(tomlPath, req); err != nil {
						return err
					}
					newCfg, err := config.Load(workDir)
					if err == nil {
						if sel, serr := config.LoadActiveAccounts(newCfg.StateDir); serr == nil {
							newCfg.ApplyActiveAccounts(sel)
						}
						cfg = newCfg
						httpServer.PushProviders(configuredProviderStatuses(cfg))
					}
					return nil
				},
				// POST /api/providers/install — auto-install provider
				func(name string) error {
					return runInstall(name, emit)
				},
				// POST /api/providers/oauth — start OAuth browser flow
				func(name string) error {
					return runOAuth(name, emit)
				},
				// POST /api/providers/api-key — save API key to .relay/.env
				func(name, value string) error {
					if err := setAPIKey(workDir, name, value); err != nil {
						emit("error", fmt.Sprintf("api-key save failed: %v", err))
						return err
					}
					// Reload .env into daemon process env so probes/agents see it
					loadEnvFileIntoEnv(workDir)
					emit("result", fmt.Sprintf("✓ API key saved for %s (.relay/.env)", name))
					return nil
				},
			)

			// ── Profiles ───────────────────────────────────────────────
			httpServer.SetProfileHandlers(
				func() []interface{} {
					ps := profilesToAPI(cfg)
					out := make([]interface{}, len(ps))
					for i, p := range ps {
						out[i] = p
					}
					return out
				},
				func(body []byte) error {
					var req UpdateProfileRequest
					if err := json.Unmarshal(body, &req); err != nil {
						return err
					}
					if err := writeProfileConfig(tomlPath, req); err != nil {
						return err
					}
					if newCfg, err := config.Load(workDir); err == nil {
						cfg = newCfg
					}
					return nil
				},
			)

			// ── Ollama bridge: list installed models + pull new ones ───
			httpServer.SetOllamaHandlers(
				func(baseURL string) (interface{}, error) {
					if baseURL == "" {
						if pc := cfg.Providers["ollama"]; pc != nil && pc.BaseURL != "" {
							baseURL = pc.BaseURL
						}
					}
					models, err := listOllamaModels(baseURL)
					if err != nil {
						return nil, err
					}
					return map[string]interface{}{
						"installed": models,
						"curated":   CuratedVisionModels,
					}, nil
				},
				func(baseURL, tag string) error {
					if baseURL == "" {
						if pc := cfg.Providers["ollama"]; pc != nil && pc.BaseURL != "" {
							baseURL = pc.BaseURL
						}
					}
					return pullOllamaModel(baseURL, tag, emit)
				},
				func(provider, model string) error {
					return runOllamaLaunch(provider, model, emit)
				},
			)

			// ── Vision ─────────────────────────────────────────────────
			httpServer.SetVisionHandlers(
				func() interface{} {
					return visionConfigToAPI(cfg)
				},
				func(body []byte) error {
					var req ApiVisionConfig
					if err := json.Unmarshal(body, &req); err != nil {
						return err
					}
					if err := writeVisionConfig(tomlPath, req); err != nil {
						return err
					}
					if newCfg, err := config.Load(workDir); err == nil {
						cfg = newCfg
					}
					return nil
				},
				func() (interface{}, error) {
					return probeVision(visionConfigToAPI(cfg))
				},
			)

			// Stub reply handler — overridden by orchestrator during a session,
			// restored when the session ends so the endpoint never 500s.
			stubReplyHandler := func(reply string) error {
				emit("system", fmt.Sprintf("no active session — reply dropped: %s", reply))
				return fmt.Errorf("no active session")
			}
			httpServer.SetSessionReplyHandler(stubReplyHandler)

			// External-agent detection + adoption (agents Relay did not spawn).
			// Adopt renders + persists a continuation brief; when start is true it
			// also launches a Relay session, pinned to the target provider, to
			// continue the lifted work — closing the detect→lift→continue loop.
			httpServer.SetDetectHandlers(
				func(sinceHours int) (interface{}, error) { a, err := scanDetectedSince(sinceHours); return a, err },
				func(id, target string, start bool) (interface{}, error) {
					res, err := adoptDetected(workDir, id, target)
					if err != nil {
						return nil, err
					}
					if start {
						brief, _ := res["markdown"].(string)
						var pin []string
						if target != "" {
							pin = []string{target}
						}
						startErr := httpServer.TryStartSession(server.RunRequest{
							Task:      adoptedTask(brief),
							Providers: pin,
						})
						if startErr != nil {
							res["started"] = false
							res["startError"] = startErr.Error()
						} else {
							res["started"] = true
							httpServer.PushEvent("system",
								fmt.Sprintf("adopted %s → starting session on %s", id, targetLabel(target)))
						}
					}
					return res, nil
				},
			)

			// Account-aware handoff: switch the active login for a provider and
			// persist it. Takes effect on the next handoff/run (pillar 3).
			httpServer.SetAccountHandler(func(provider, label string) error {
				pc := cfg.Providers[provider]
				if pc == nil {
					return fmt.Errorf("unknown provider %q", provider)
				}
				found := false
				for _, a := range pc.Accounts {
					if a.Label == label {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("provider %q has no account %q", provider, label)
				}
				sel, _ := config.LoadActiveAccounts(cfg.StateDir)
				if sel == nil {
					sel = map[string]string{}
				}
				sel[provider] = label
				if err := config.SaveActiveAccounts(cfg.StateDir, sel); err != nil {
					return err
				}
				cfg.ApplyActiveAccounts(sel)
				httpServer.PushEvent("system", fmt.Sprintf("switched %s account → %s", provider, label))
				return nil
			})

			// Pipeline designer: list / save / run multi-agent DAGs (pillar 4).
			httpServer.SetPipelineHandlers(
				func() []interface{} {
					ps, _ := config.LoadPipelines(cfg.StateDir)
					out := make([]interface{}, len(ps))
					for i, p := range ps {
						out[i] = p
					}
					return out
				},
				func(body []byte) error {
					var ps []config.Pipeline
					if err := json.Unmarshal(body, &ps); err != nil {
						return err
					}
					return config.SavePipelines(cfg.StateDir, ps)
				},
				func(name string) {
					if err := runPipeline(workDir, cfg, httpServer, name); err != nil {
						httpServer.PushEvent("system", fmt.Sprintf("pipeline error: %v", err))
					}
					httpServer.SetSessionReplyHandler(stubReplyHandler)
				},
			)

			// Quota wallet: per provider+account remaining/reset/forecast.
			httpServer.SetWalletHandler(func() interface{} { return buildWallet(cfg) })

			// Time machine: handoff timeline + git commit trail + diff + rewind.
			httpServer.SetHistoryHandlers(
				func() interface{} { return buildHistory(cfg.AuditPath) },
				func() interface{} { return gitCommits(workDir) },
				func(sha string) (interface{}, error) { return gitDiff(workDir, sha) },
				func(sha string) (interface{}, error) { return gitRewind(workDir, sha) },
			)

			// Accept run requests from the desktop app via POST /api/run
			httpServer.OnRun(func(req server.RunRequest) {
				threshold := req.Threshold
				if threshold <= 0 {
					threshold = 0.85
				}
				priority := buildProviderPriority(cfg, req.Providers)
				httpServer.PushEvent("system", fmt.Sprintf("starting task: %s", req.Task))
				if err := runSession(workDir, cfg, req.Task, priority, threshold, req.MaxHandoffs, httpServer, false, len(req.Providers) > 0); err != nil {
					httpServer.PushEvent("system", fmt.Sprintf("run error: %v", err))
				}
				// Restore stub so /api/session/reply doesn't dangle on a dead adapter
				httpServer.SetSessionReplyHandler(stubReplyHandler)
			})

			fmt.Printf("Dashboard API: http://127.0.0.1:%d\n", cfg.ServerPort)
			fmt.Println("No active session. Use the desktop app or run: relay run \"your task\"")

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			// Ambient detection: announce new active agent sessions in the background.
			go startAmbientDetection(httpServer, ctx.Done())
			<-ctx.Done()
			fmt.Println("\nRelay daemon stopped.")
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "dashboard API port (defaults to relay.toml or 4748)")
	return cmd
}

// defaultEvalTasks seeds `.relay/eval/tasks.json` so the golden-routing suite
// runs immediately after `relay init`. Edit it to match your provider chain.
var defaultEvalTasks = []byte(`[
  {"task":"refactor orders service to add a refund flow","expectProfile":"backend","expectChainHead":"claude"},
  {"task":"build a React component for the dashboard","expectProfile":"frontend","expectChainHead":"claude"},
  {"task":"write a sql migration to add email_verified column","expectProfile":"database","expectChainHead":"codex"},
  {"task":"review this pull request","expectProfile":"reviewer","expectChainHead":"claude"}
]
`)

func cmdInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise .relay/ in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			if err := config.WriteDefault(workDir); err != nil {
				return fmt.Errorf("init: write config: %w", err)
			}

			stateDir := filepath.Join(workDir, ".relay")
			keyPath := filepath.Join(stateDir, ".signing-key")
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				key := make([]byte, 32)
				if _, err2 := rand.Read(key); err2 != nil {
					return fmt.Errorf("init: generate signing key: %w", err2)
				}
				if err2 := os.WriteFile(keyPath, key, 0600); err2 != nil {
					return fmt.Errorf("init: write signing key: %w", err2)
				}
				fmt.Println("✓ Generated signing key (.relay/.signing-key)")
			}

			// Scaffold a starter eval suite so `relay eval` works out of the box.
			evalPath := filepath.Join(stateDir, "eval", "tasks.json")
			if _, err := os.Stat(evalPath); os.IsNotExist(err) {
				if err2 := os.MkdirAll(filepath.Dir(evalPath), 0755); err2 != nil {
					return fmt.Errorf("init: create eval dir: %w", err2)
				}
				if err2 := os.WriteFile(evalPath, defaultEvalTasks, 0644); err2 != nil {
					return fmt.Errorf("init: write eval tasks: %w", err2)
				}
				fmt.Println("✓ Wrote starter eval suite (.relay/eval/tasks.json)")
			}

			appendToGitignore(workDir)
			fmt.Printf("✓ Relay initialised in %s/.relay/\n", workDir)
			fmt.Println()
			fmt.Println("  Next steps:")
			fmt.Println("  1. Edit .relay/relay.toml to configure providers")
			fmt.Println("  2. relay run \"your task here\"")
			fmt.Println("  3. Open relay-ui.exe for the live dashboard")
			fmt.Println()
			fmt.Println("  Dashboard: http://127.0.0.1:4748 (when daemon is running)")
			return nil
		},
	}
}

// ─── relay run ────────────────────────────────────────────────────────────────

func cmdRun() *cobra.Command {
	var (
		forceHandoffAt float64
		maxHandoffs    int
		providers      []string
		noUI           bool
		autoYes        bool
	)

	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a task with the configured provider chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskGoal := args[0]
			workDir, _ := os.Getwd()

			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("run: load config: %w", err)
			}

			priority := buildProviderPriority(cfg, providers)

			fmt.Printf("\n  Task:      %s\n", taskGoal)
			fmt.Printf("  Providers: %s\n", formatProviders(priority))
			fmt.Printf("  Threshold: %.0f%%\n\n", forceHandoffAt*100)

			if !autoYes {
				fmt.Print("Proceed? [y/N] ")
				var answer string
				fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			return runSession(workDir, cfg, taskGoal, priority, forceHandoffAt, maxHandoffs, nil, !noUI, len(providers) > 0)
		},
	}

	cmd.Flags().Float64VarP(&forceHandoffAt, "force-handoff", "f", 0.85, "handoff threshold (0.0–1.0)")
	cmd.Flags().IntVarP(&maxHandoffs, "max-handoffs", "n", 0, "max handoffs (0=unlimited)")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "ordered provider list (overrides config)")
	cmd.Flags().BoolVar(&noUI, "no-ui", false, "disable HTTP server (no egui dashboard)")
	cmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "skip confirmation prompt (for app-launched tasks)")
	return cmd
}

// runPipeline executes a pipeline's nodes in dependency (topological) order.
// Each node runs as its own Relay session with priority = its primary provider
// then its fallbacks, so per-node failover reuses the quota-breach handoff chain.
// Sequential by design: nodes share one working tree and the single-session guard.
func runPipeline(workDir string, cfg *config.Config, httpServer *server.Server, name string) error {
	pipelines, err := config.LoadPipelines(cfg.StateDir)
	if err != nil {
		return err
	}
	p, ok := config.FindPipeline(pipelines, name)
	if !ok {
		return fmt.Errorf("no pipeline %q", name)
	}
	order, err := p.TopoOrder()
	if err != nil {
		return err
	}
	byID := make(map[string]config.PipelineNode, len(p.Nodes))
	for _, n := range p.Nodes {
		byID[n.ID] = n
	}
	httpServer.PushEvent("system", fmt.Sprintf("pipeline %q: running %d node(s)", name, len(order)))
	for _, id := range order {
		n := byID[id]
		priority := buildProviderPriority(cfg, n.Priority())
		httpServer.PushEvent("system", fmt.Sprintf("pipeline %q · node %q → %s", name, id, formatProviders(priority)))
		if err := runSession(workDir, cfg, n.Task, priority, 0.85, 0, httpServer, false, true); err != nil {
			return fmt.Errorf("node %q: %w", id, err)
		}
		// Verifier gate: the node's acceptance checks must pass. On failure, retry
		// once on the next fallback provider; if it still fails, abort the pipeline
		// rather than build dependents on a broken foundation.
		if res := verify.Run(context.Background(), workDir, n.Verify); !res.AllPassed {
			httpServer.PushEvent("system", fmt.Sprintf("pipeline %q · node %q FAILED checks: %s",
				name, id, strings.Join(res.Failed(), ", ")))
			if len(priority) <= 1 {
				return fmt.Errorf("node %q failed verification: %s", id, strings.Join(res.Failed(), ", "))
			}
			retry := priority[1:]
			httpServer.PushEvent("system", fmt.Sprintf("pipeline %q · node %q retry on %s", name, id, formatProviders(retry)))
			if err := runSession(workDir, cfg, n.Task, retry, 0.85, 0, httpServer, false, true); err != nil {
				return fmt.Errorf("node %q retry: %w", id, err)
			}
			if res2 := verify.Run(context.Background(), workDir, n.Verify); !res2.AllPassed {
				return fmt.Errorf("node %q failed verification after retry: %s", id, strings.Join(res2.Failed(), ", "))
			}
		} else if len(n.Verify) > 0 {
			httpServer.PushEvent("system", fmt.Sprintf("pipeline %q · node %q passed %d check(s)", name, id, len(n.Verify)))
		}
	}
	httpServer.PushEvent("system", fmt.Sprintf("pipeline %q complete", name))
	return nil
}

// runSession starts an agent session.
// If existingServer is non-nil it is reused (daemon mode) and no new HTTP server is started.
// If nil and startServer is true a fresh HTTP server is started on cfg.ServerPort.
func runSession(
	workDir string,
	cfg *config.Config,
	taskGoal string,
	priority []adapter.ProviderName,
	handoffAt float64,
	maxHandoffs int,
	existingServer *server.Server,
	startServer bool,
	pinned bool, // ProviderPriority was set explicitly → skip profile auto-routing
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Session ID
	sessionBytes := make([]byte, 8)
	if _, err := rand.Read(sessionBytes); err != nil {
		return fmt.Errorf("session id: %w", err)
	}
	sessionID := hex.EncodeToString(sessionBytes)

	// Signing key
	keyPath := filepath.Join(cfg.StateDir, ".signing-key")
	signingKey, err := contract.LoadSigningKey(keyPath)
	if err != nil {
		return fmt.Errorf("signing key: %w", err)
	}

	// Quota registry (starts Claude proxy)
	quotaReg, err := quota.BuildQuotaRegistry(declaredCaps(cfg))
	if err != nil {
		return fmt.Errorf("quota registry: %w", err)
	}
	defer quotaReg.Close()

	// Adapter registry
	proxyPort := quotaReg.ClaudeProxy.Port()
	ollamaBaseURL, ollamaModel := ollamaSettings(cfg)
	adapterReg := adapter.BuildAdapterRegistry(proxyPort, ollamaBaseURL, ollamaModel)

	// Graph store
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return err
	}
	graphStore, err := graph.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	defer graphStore.Close()

	// Audit log
	auditLog, err := audit.Open(cfg.AuditPath)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	defer auditLog.Close()

	// HTTP server — reuse existing (daemon mode) or start fresh
	httpServer := existingServer
	if httpServer == nil && startServer {
		httpServer = server.New(cfg.ServerPort)
		if serr := httpServer.Start(); serr != nil {
			fmt.Fprintf(os.Stderr, "warning: HTTP server: %v\n", serr)
			httpServer = nil
		} else {
			fmt.Printf("  Dashboard: http://127.0.0.1:%d\n", cfg.ServerPort)
			fmt.Printf("  Claude proxy: http://127.0.0.1:%d\n\n", proxyPort)
			defer httpServer.Stop()
		}
	}

	// Account state (pillar 3): resolveAccountEnv applies the persisted active
	// selection to cfg, then buildAccountSpecs reads it back as the failover set.
	accountEnv := resolveAccountEnv(cfg)
	accountSpecs, activeAccountLabels := buildAccountSpecs(cfg)

	// Orchestrator
	orch, err := orchestrator.New(
		orchestrator.Options{
			WorkDir:            workDir,
			StateDir:           cfg.StateDir,
			SessionID:          sessionID,
			TaskGoal:           taskGoal,
			ProviderPriority:   priority,
			PinnedPriority:     pinned,
			ForceHandoffAt:     handoffAt,
			MaxHandoffs:        maxHandoffs,
			ProviderModels:     buildProviderModels(cfg),
			ProviderAccountEnv: accountEnv,
			ProviderAccounts:   accountSpecs,
			ActiveAccountLabel: activeAccountLabels,
			OnAccountSwitch: func(provider, label string) {
				persistActiveAccount(cfg.StateDir, provider, label)
				if httpServer != nil {
					httpServer.PushEvent("handoff", fmt.Sprintf("active account for %s → %s", provider, label))
				}
			},
			Profiles: buildProfileSpecs(cfg),
			Vision: orchestrator.VisionSpec{
				Enabled:  cfg.Vision.Enabled,
				PollMs:   cfg.Vision.PollMs,
				Provider: cfg.Vision.Provider,
			},
			VisionProbe: func() (orchestrator.VisionResult, error) {
				obs, err := probeVision(visionConfigToAPI(cfg))
				if err != nil {
					return orchestrator.VisionResult{}, err
				}
				return orchestrator.VisionResult{
					NeedsInput: obs.NeedsInput,
					Question:   obs.Question,
					Choices:    obs.Choices,
					Summary:    obs.Summary,
				}, nil
			},
		},
		adapterReg,
		quotaReg,
		graphStore,
		auditLog,
		httpServer,
		signingKey,
	)
	if err != nil {
		return fmt.Errorf("orchestrator: %w", err)
	}

	return orch.Run(ctx)
}

// ─── relay status ─────────────────────────────────────────────────────────────

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current session state",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			stateFile := filepath.Join(workDir, ".relay", "session.json")
			data, err := os.ReadFile(stateFile)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No active session. Run: relay run \"task\"")
					return nil
				}
				return err
			}
			// Pretty-print JSON
			var pretty bytes.Buffer
			if jerr := json.Indent(&pretty, data, "", "  "); jerr == nil {
				fmt.Println(pretty.String())
			} else {
				fmt.Println(string(data))
			}
			return nil
		},
	}
}

// ─── relay handoff ────────────────────────────────────────────────────────────

func cmdHandoff() *cobra.Command {
	return &cobra.Command{
		Use:   "handoff",
		Short: "Trigger immediate handoff on the running session",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := http.Post("http://127.0.0.1:4748/api/handoff", "application/json", nil)
			if err != nil {
				return fmt.Errorf("handoff: is relay running? (%w)", err)
			}
			defer resp.Body.Close()
			fmt.Printf("Handoff triggered: HTTP %d\n", resp.StatusCode)
			return nil
		},
	}
}

// ─── relay resume ─────────────────────────────────────────────────────────────

func cmdResume() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Write receiver heartbeat (unblocks DISPATCHED state)",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			cfg, err := config.Load(workDir)
			if err != nil {
				return err
			}
			stateFile := filepath.Join(cfg.StateDir, "session.json")
			data, err := os.ReadFile(stateFile)
			if err != nil {
				return fmt.Errorf("resume: no session: %w", err)
			}
			var rec struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(data, &rec); err != nil {
				return fmt.Errorf("resume: parse session: %w", err)
			}
			if err := fsm.WriteReceiverHeartbeat(cfg.StateDir, rec.SessionID); err != nil {
				return fmt.Errorf("resume: heartbeat: %w", err)
			}
			fmt.Printf("✓ Receiver heartbeat written for session %s\n", rec.SessionID)
			return nil
		},
	}
}

// ─── relay audit ─────────────────────────────────────────────────────────────

func cmdAudit() *cobra.Command {
	auditCmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit log commands",
	}
	auditCmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify audit log hash chain integrity",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			cfg, err := config.Load(workDir)
			if err != nil {
				return err
			}
			if err := audit.Verify(cfg.AuditPath); err != nil {
				return fmt.Errorf("INTEGRITY FAILURE: %w", err)
			}
			fmt.Println("✓ Audit log hash chain verified — no tampering detected")
			return nil
		},
	})
	return auditCmd
}

// ─── relay graph ─────────────────────────────────────────────────────────────

func cmdGraph() *cobra.Command {
	return &cobra.Command{
		Use:   "graph",
		Short: "Print knowledge graph statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, _ := os.Getwd()
			cfg, err := config.Load(workDir)
			if err != nil {
				return err
			}
			g, err := graph.Open(cfg.DBPath)
			if err != nil {
				return err
			}
			defer g.Close()
			nodes, edges, err := g.Stats(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Nodes: %d\nEdges: %d\n", nodes, edges)
			return nil
		},
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildProviderPriority(cfg *config.Config, override []string) []adapter.ProviderName {
	names := override
	if len(names) == 0 {
		// Default order from config
		names = []string{"claude", "codex", "antigravity", "opencode", "ollama", "copilot", "continue", "cline"}
	}
	result := make([]adapter.ProviderName, 0, len(names))
	for _, n := range names {
		if pc, ok := cfg.Providers[n]; ok && !pc.Enabled {
			continue
		}
		result = append(result, adapter.ProviderName(n))
	}
	if len(result) == 0 {
		result = []adapter.ProviderName{adapter.ProviderClaude}
	}
	return result
}

func configuredProviderStatuses(cfg *config.Config) []server.ApiProviderStatus {
	order := configuredProviderOrder(cfg)
	statuses := make([]server.ApiProviderStatus, 0, len(order))
	firstEnabled := ""
	for _, name := range order {
		if pc := cfg.Providers[name]; pc != nil && pc.Enabled {
			firstEnabled = name
			break
		}
	}

	for _, name := range order {
		pc := cfg.Providers[name]
		if pc == nil {
			continue
		}

		state := "standby"
		source := "relay.toml"
		var remaining *int64
		if !pc.Enabled {
			state = "unknown"
			source = "disabled in relay.toml"
		} else if pc.DeclaredCap > 0 {
			value := pc.DeclaredCap
			remaining = &value
			source = "relay.toml declared cap"
		} else if name == string(adapter.ProviderOllama) {
			source = "local ollama"
		}

		statuses = append(statuses, server.ApiProviderStatus{
			Name:         name,
			State:        state,
			FractionUsed: 0,
			Remaining:    remaining,
			IsNext:       name == firstEnabled,
			QuotaSource:  source,
		})
	}
	return statuses
}

func configuredProviderOrder(cfg *config.Config) []string {
	defaultOrder := []string{"claude", "codex", "antigravity", "opencode", "ollama", "copilot", "continue", "cline"}
	seen := map[string]bool{}
	order := make([]string, 0, len(cfg.Providers))
	for _, name := range defaultOrder {
		if _, ok := cfg.Providers[name]; ok {
			order = append(order, name)
			seen[name] = true
		}
	}
	var extras []string
	for name := range cfg.Providers {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	return append(order, extras...)
}

func pushStoredGraphSummary(httpServer *server.Server, dbPath string) {
	graphStore, err := graph.Open(dbPath)
	if err != nil {
		httpServer.PushEvent("system", fmt.Sprintf("graph unavailable: %v", err))
		return
	}
	defer graphStore.Close()

	ctx := context.Background()
	nodes, edges, err := graphStore.Stats(ctx)
	if err == nil {
		httpServer.PushGraphStats(nodes, edges)
	}

	recent, err := graphStore.Recent(ctx, 200)
	if err != nil {
		return
	}
	apiNodes := make([]server.ApiGraphNode, 0, len(recent.Nodes))
	for _, n := range recent.Nodes {
		apiNodes = append(apiNodes, server.ApiGraphNode{
			ID:       n.ID,
			NodeType: n.NodeType,
			Weight:   n.Weight,
			Label:    graphNodeLabel(n),
		})
	}
	apiEdges := make([]server.ApiGraphEdge, 0, len(recent.Edges))
	for _, e := range recent.Edges {
		apiEdges = append(apiEdges, server.ApiGraphEdge{
			FromID:   e.FromID,
			ToID:     e.ToID,
			EdgeType: e.EdgeType,
		})
	}
	httpServer.PushGraphDetail(apiNodes, apiEdges)
}

func graphNodeLabel(n graph.Node) string {
	for _, key := range []string{"label", "goal", "path", "summary", "rule", "text"} {
		if value, ok := n.Payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return n.ID
}

// buildProviderModels — extract Model field from each ProviderConfig.
func buildProviderModels(cfg *config.Config) map[string]string {
	out := map[string]string{}
	for name, pc := range cfg.Providers {
		if pc == nil || pc.Model == "" {
			continue
		}
		out[name] = pc.Model
	}
	return out
}

// buildWallet merges the persisted quota ledger with the configured providers +
// accounts so the wallet shows every login — even ones not yet observed (with
// their declared cap as a placeholder).
func buildWallet(cfg *config.Config) []quota.LedgerEntry {
	led, _ := quota.LoadLedger(cfg.StateDir)
	var out []quota.LedgerEntry
	for _, name := range configuredProviderOrder(cfg) {
		pc := cfg.Providers[name]
		if pc == nil || !pc.Enabled {
			continue
		}
		labels := []string{""}
		if len(pc.Accounts) > 0 {
			labels = labels[:0]
			for _, a := range pc.Accounts {
				labels = append(labels, a.Label)
			}
		}
		for _, label := range labels {
			if e, ok := led[quota.LedgerKey(name, label)]; ok {
				out = append(out, e)
				continue
			}
			out = append(out, quota.LedgerEntry{
				Provider:     name,
				Account:      label,
				Remaining:    -1,
				Total:        pc.DeclaredCap,
				FractionUsed: -1,
				Source:       "declared_cap",
				EtaMinutes:   -1,
			})
		}
	}
	return out
}

// resolveAccountEnv applies the persisted active-account selection to cfg and
// returns provider→env overrides for the active accounts (pillar 3). Mutating
// cfg here means provider listings reflect the live selection too.
func resolveAccountEnv(cfg *config.Config) map[string][]string {
	if sel, err := config.LoadActiveAccounts(cfg.StateDir); err == nil {
		cfg.ApplyActiveAccounts(sel)
	}
	return cfg.AllAccountEnv()
}

// buildAccountSpecs returns every provider's accounts (label + resolved env) plus
// the active label per provider, for the orchestrator's auto-failover (pillar 3).
func buildAccountSpecs(cfg *config.Config) (map[string][]orchestrator.AccountSpec, map[string]string) {
	specs := map[string][]orchestrator.AccountSpec{}
	active := map[string]string{}
	for name, pc := range cfg.Providers {
		if len(pc.Accounts) == 0 {
			continue
		}
		list := make([]orchestrator.AccountSpec, 0, len(pc.Accounts))
		for i := range pc.Accounts {
			label := pc.Accounts[i].Label
			list = append(list, orchestrator.AccountSpec{Label: label, Env: cfg.AccountEnvFor(name, label)})
		}
		specs[name] = list
		if a := cfg.ActiveAccount(name); a != nil {
			active[name] = a.Label
		}
	}
	return specs, active
}

// persistActiveAccount records an auto-failover account switch so it survives a
// restart and the desktop UI's switcher reflects the live selection.
func persistActiveAccount(stateDir, provider, label string) {
	sel, _ := config.LoadActiveAccounts(stateDir)
	if sel == nil {
		sel = map[string]string{}
	}
	sel[provider] = label
	_ = config.SaveActiveAccounts(stateDir, sel)
}

// decorateAccounts attaches each provider's accounts + active label to its
// API detail so the desktop UI can render a switcher.
func decorateAccounts(cfg *config.Config, details []ApiProviderDetail) []ApiProviderDetail {
	for i := range details {
		pc := cfg.Providers[details[i].Name]
		if pc == nil || len(pc.Accounts) == 0 {
			continue
		}
		active := ""
		accts := make([]ApiAccount, 0, len(pc.Accounts))
		for _, a := range pc.Accounts {
			accts = append(accts, ApiAccount{Label: a.Label, Active: a.Active, ConfigDir: a.ConfigDir})
			if a.Active {
				active = a.Label
			}
		}
		if active == "" && len(accts) > 0 {
			active = accts[0].Label // first is the implicit default
		}
		details[i].Accounts = accts
		details[i].ActiveAccount = active
	}
	return details
}

// buildProfileSpecs converts config.ProfileConfig map → orchestrator.ProfileSpec map.
func buildProfileSpecs(cfg *config.Config) map[string]orchestrator.ProfileSpec {
	out := make(map[string]orchestrator.ProfileSpec, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		if p == nil {
			continue
		}
		out[name] = orchestrator.ProfileSpec{
			Chain:       append([]string{}, p.Chain...),
			Kinds:       append([]string{}, p.Kinds...),
			Skills:      append([]string{}, p.Skills...),
			ContextHint: p.ContextHint,
		}
	}
	return out
}

func declaredCaps(cfg *config.Config) map[adapter.ProviderName]int64 {
	caps := map[adapter.ProviderName]int64{}
	for name, pc := range cfg.Providers {
		if pc == nil || pc.DeclaredCap <= 0 {
			continue
		}
		caps[adapter.ProviderName(name)] = pc.DeclaredCap
	}
	return caps
}

func ollamaSettings(cfg *config.Config) (baseURL, model string) {
	if pc, ok := cfg.Providers[string(adapter.ProviderOllama)]; ok && pc != nil {
		return pc.BaseURL, pc.Model
	}
	return "", ""
}

func formatProviders(priority []adapter.ProviderName) string {
	names := make([]string, len(priority))
	for i, p := range priority {
		names[i] = string(p)
	}
	return strings.Join(names, " → ")
}

func appendToGitignore(workDir string) {
	gitignorePath := filepath.Join(workDir, ".gitignore")
	entries := []string{
		".relay/memory/",
		".relay/graph.db*",
		".relay/audit.jsonl*",
		".relay/**/*.snap",
		".relay/relay.lock",
		".relay/receiver-heartbeat",
		".relay/.signing-key",
	}

	existing, _ := os.ReadFile(gitignorePath)
	existingStr := string(existing)

	var toAdd []string
	for _, e := range entries {
		if !strings.Contains(existingStr, e) {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString("\n# Relay runtime (SEC-5)\n")
	for _, e := range toAdd {
		_, _ = f.WriteString(e + "\n")
	}
}
