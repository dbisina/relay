// internal/orchestrator/orchestrator.go
//
// Orchestrator — main coordination loop.
//
// Responsibilities:
//   1. Start active provider via adapter registry
//   2. Stream events → audit log + HTTP server push
//   3. Monitor quota adapters; trigger handoff at breach
//   4. Drive FSM: RUNNING → PAUSING → SNAPSHOTTED → ENVELOPE_BUILT → DISPATCHED → RESUMING → RUNNING
//   5. Build + sign continuation contract
//   6. Select next provider from priority list

package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbisina/relay/internal/adapter"
	"github.com/dbisina/relay/internal/approval"
	"github.com/dbisina/relay/internal/audit"
	"github.com/dbisina/relay/internal/circuit"
	"github.com/dbisina/relay/internal/codegraph"
	"github.com/dbisina/relay/internal/contract"
	"github.com/dbisina/relay/internal/fsm"
	"github.com/dbisina/relay/internal/graph"
	"github.com/dbisina/relay/internal/instructions"
	"github.com/dbisina/relay/internal/outcomes"
	"github.com/dbisina/relay/internal/pricing"
	"github.com/dbisina/relay/internal/quota"
	"github.com/dbisina/relay/internal/redact"
	"github.com/dbisina/relay/internal/retry"
	"github.com/dbisina/relay/internal/server"
	"github.com/dbisina/relay/internal/worktree"
)

// Options configures the Orchestrator.
type Options struct {
	WorkDir          string
	StateDir         string
	SessionID        string
	TaskGoal         string
	ProviderPriority []adapter.ProviderName // fallback order (used when no profile matches)
	ForceHandoffAt   float64                // 0 = use adapter BreachFraction
	MaxHandoffs      int                    // 0 = unlimited

	// Retry is the wait-and-retry policy (wait out a reset vs hand off).
	Retry retry.Config

	// PinnedPriority means ProviderPriority was set explicitly by the user (a
	// --providers flag, an adopt target, or a pipeline node). When true, profile
	// auto-routing is skipped so the user's choice is never silently overridden.
	PinnedPriority bool

	// Per-provider model override. Map provider name → model identifier.
	// Empty = adapter's CLI default. Surfaced via the adapter's ModelSetter
	// at session start.
	ProviderModels map[string]string

	// Per-provider account env (pillar 3). Map provider name → KEY=VAL overrides
	// for the active account (e.g. CLAUDE_CONFIG_DIR). Applied to RunOptions.Env
	// when dispatching, so a handoff can switch the login it runs under.
	// Used as a fallback when ProviderAccounts has no entry for the provider.
	ProviderAccountEnv map[string][]string

	// All accounts per provider, in declared order (pillar 3 auto-failover). When
	// a provider's active account exhausts its quota, the orchestrator rotates to
	// the next non-exhausted account of the SAME provider before falling through
	// to a cross-provider handoff.
	ProviderAccounts map[string][]AccountSpec

	// Initial active account label per provider (from .relay/active-accounts.json
	// or the TOML default). Seeds the rotation pointer.
	ActiveAccountLabel map[string]string

	// OnAccountSwitch is called when auto-failover changes the active account, so
	// the daemon can persist the new selection and surface it in the UI.
	OnAccountSwitch func(provider, label string)

	// Optional: profiles for auto-routing. Map name → ProfileSpec.
	// If non-empty, orchestrator picks the best matching profile based on
	// task goal keywords and uses that profile's chain instead of the
	// fallback ProviderPriority.
	Profiles map[string]ProfileSpec

	// Optional: vision probe + config. When non-nil and Vision.Enabled is true,
	// orchestrator polls the screen at PollMs intervals during an active
	// session, emitting "vision" events when the model detects a user prompt.
	VisionProbe func() (VisionResult, error)
	Vision      VisionSpec
}

// AccountSpec mirrors one config.Account's identity + resolved env. Lives here
// to avoid an orchestrator → config import cycle; main.go does the translation.
type AccountSpec struct {
	Label string
	Env   []string
}

// ProfileSpec mirrors config.ProfileConfig but lives here to avoid an
// orchestrator → config import cycle. main.go does the translation.
type ProfileSpec struct {
	Chain       []string
	Kinds       []string
	Skills      []string
	ContextHint string
}

// VisionSpec mirrors config.VisionConfig.
type VisionSpec struct {
	Enabled  bool
	PollMs   int
	Provider string
}

// VisionResult is what VisionProbe returns. Mirrors ApiVisionObservation.
type VisionResult struct {
	NeedsInput bool
	Question   string
	Choices    []string
	Summary    string
}

// Orchestrator drives the multi-provider handoff loop.
type Orchestrator struct {
	opts        Options
	adapters    adapter.Registry
	quotaReg    *quota.Registry
	machine     *fsm.HandoffMachine
	durability  *fsm.DurabilityManager
	graphStore  *graph.GraphStore
	contractBld *contract.Builder
	contractSer *contract.Serializer
	auditLog    *audit.Logger
	httpServer  *server.Server
	signingKey  []byte

	activeProvider  adapter.ProviderName
	handoffCount    int
	taskID          string
	manualHandoff   atomic.Bool
	manualHandoffCh chan struct{}
	lastText        string
	lastRetrySignal *retry.Signal
	retryWaits      int

	// Cached profile selection (computed once at session start)
	activeProfile string // profile name, "" = no match → fallback chain

	// Account-failover state (pillar 3). acctIdx is the current account index per
	// provider into opts.ProviderAccounts; acctExhausted tracks labels burned this
	// session so the rotation does not loop back onto a dead account.
	acctIdx       map[string]int
	acctExhausted map[string]map[string]bool

	// Senior-eng adds
	worktree    *worktree.Session
	worktreeMgr *worktree.Manager
	redactor    *redact.Redactor
	circuits    *circuit.Registry
	gate        *approval.Gate
	outcomes    *outcomes.Tracker
	paused      atomic.Bool
	pauseCh     chan struct{}

	// Cost tracking
	costMu           sync.Mutex
	sessionTokensIn  int64
	sessionTokensOut int64
	sessionCostUSD   float64

	// Quota-wallet burn tracking: previous "used" units + observation time per
	// provider, so each ledger write can compute a units/minute burn rate and a
	// forecast time-to-exhaustion. lastRunDelta is the units consumed by the most
	// recent run, used to forecast whether the next run will fit (pre-emptive
	// handoff).
	prevUsed     map[string]int64
	prevUsedAt   map[string]time.Time
	lastRunDelta map[string]float64
}

var filePathRe = regexp.MustCompile(`[A-Za-z0-9_./\\-]+\.[A-Za-z0-9]{1,8}`)

// New creates and wires up an Orchestrator.
func New(
	opts Options,
	adapters adapter.Registry,
	quotaReg *quota.Registry,
	graphStore *graph.GraphStore,
	auditLog *audit.Logger,
	httpServer *server.Server,
	signingKey []byte,
) (*Orchestrator, error) {
	taskID := fmt.Sprintf("t-%s-%d", opts.SessionID[:8], time.Now().UnixMilli())

	// Profile auto-routing: pick best-matching profile from task goal. If found,
	// override ProviderPriority with the profile's chain. Skipped when the user
	// pinned providers explicitly — an explicit choice must never be overridden by
	// keyword matching (esp. for adopted tasks whose goal inlines a whole brief).
	matchedProfile, matchedChain := "", []string(nil)
	if !opts.PinnedPriority {
		matchedProfile, matchedChain = matchProfile(opts.TaskGoal, opts.Profiles)
	}
	if matchedProfile != "" && len(matchedChain) > 0 {
		// Filter chain to providers we actually have an adapter for
		filtered := make([]adapter.ProviderName, 0, len(matchedChain))
		for _, name := range matchedChain {
			pn := adapter.ProviderName(name)
			if _, ok := adapters[pn]; ok {
				filtered = append(filtered, pn)
			}
		}
		if len(filtered) > 0 {
			opts.ProviderPriority = filtered
		}
	}

	if len(opts.ProviderPriority) == 0 {
		return nil, fmt.Errorf("orchestrator: no providers in priority chain")
	}

	machine, err := fsm.NewHandoffMachine(opts.StateDir, opts.SessionID, taskID, string(opts.ProviderPriority[0]))
	if err != nil {
		return nil, fmt.Errorf("orchestrator: fsm: %w", err)
	}

	durability, err := fsm.NewDurabilityManager(opts.WorkDir, opts.StateDir, opts.SessionID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: lock: %w", err)
	}

	contractBld := contract.NewBuilder(graphStore, signingKey)
	contractSer := contract.NewSerializer()

	// Per-session git worktree (best-effort; nil if not in a repo)
	wm := worktree.New(opts.WorkDir, "")
	var session *worktree.Session
	if wm.IsGitRepo() {
		s, werr := wm.Create(opts.SessionID)
		if werr == nil {
			session = s
		}
	}

	o := &Orchestrator{
		opts:            opts,
		adapters:        adapters,
		quotaReg:        quotaReg,
		machine:         machine,
		durability:      durability,
		graphStore:      graphStore,
		contractBld:     contractBld,
		contractSer:     contractSer,
		auditLog:        auditLog,
		httpServer:      httpServer,
		signingKey:      signingKey,
		activeProvider:  opts.ProviderPriority[0],
		taskID:          taskID,
		manualHandoffCh: make(chan struct{}, 1),
		activeProfile:   matchedProfile,
		worktree:        session,
		worktreeMgr:     wm,
		redactor:        redact.NewDefault(),
		circuits:        circuit.NewRegistry(),
		gate:            approval.NewGate(),
		outcomes:        outcomes.New(opts.StateDir),
		pauseCh:         make(chan struct{}, 1),
		acctIdx:         make(map[string]int),
		acctExhausted:   make(map[string]map[string]bool),
		prevUsed:        make(map[string]int64),
		prevUsedAt:      make(map[string]time.Time),
		lastRunDelta:    make(map[string]float64),
	}

	// Seed each provider's account pointer to its active label.
	for provider, accounts := range opts.ProviderAccounts {
		label := opts.ActiveAccountLabel[provider]
		for i, a := range accounts {
			if a.Label == label {
				o.acctIdx[provider] = i
				break
			}
		}
	}

	// If we got a worktree, agents run in there. Otherwise stay in workDir.
	if session != nil {
		o.opts.WorkDir = session.Path
		auditLog.Log("worktree_create", map[string]interface{}{
			"path":   session.Path,
			"branch": session.Branch,
		}) //nolint:errcheck
	}

	// Wire handoff button in UI
	if httpServer != nil {
		httpServer.OnHandoff(func() {
			if o.manualHandoff.CompareAndSwap(false, true) {
				select {
				case o.manualHandoffCh <- struct{}{}:
				default:
				}
			}
			o.logEvent("handoff", "manual handoff requested from UI")
		})

		// Wire user-reply handler → active adapter's stdin (if supported)
		httpServer.SetSessionReplyHandler(o.SendUserReply)

		// Wire senior-eng surface
		httpServer.SetSeniorHandlers(
			// diff
			func() (interface{}, error) {
				if o.worktree == nil {
					return map[string]interface{}{"summary": "(no worktree)", "diff": ""}, nil
				}
				summary, _ := o.worktreeMgr.DiffSummary(o.worktree)
				diff, _ := o.worktreeMgr.Diff(o.worktree)
				return map[string]interface{}{"summary": summary, "diff": diff}, nil
			},
			// cost
			func() (interface{}, error) {
				o.costMu.Lock()
				defer o.costMu.Unlock()
				return map[string]interface{}{
					"sessionUsd": o.sessionCostUSD,
					"tokensIn":   o.sessionTokensIn,
					"tokensOut":  o.sessionTokensOut,
					"provider":   string(o.activeProvider),
				}, nil
			},
			// pause/unpause
			func(p bool) error { o.SetPause(p); return nil },
			// approvals list
			func() interface{} { return o.gate.Pending() },
			// resolve approval
			func(id string, ok bool, note string) error {
				if !o.gate.Resolve(id, ok, note) {
					return fmt.Errorf("approval %s not found", id)
				}
				return nil
			},
			// circuit snapshot
			func() interface{} { return o.circuits.SnapshotAll() },
			// outcome aggregates
			func() interface{} { return o.outcomes.Aggregates() },
			// redaction stats
			func() interface{} {
				return map[string]interface{}{
					"summary": o.redactor.Summary(),
				}
			},
		)
	}

	return o, nil
}

// SetPause toggles agent execution (best-effort — only honoured at event boundaries).
func (o *Orchestrator) SetPause(paused bool) {
	if paused {
		o.paused.Store(true)
		o.logEvent("system", "session paused — agents will halt at next event boundary")
	} else {
		o.paused.Store(false)
		select {
		case o.pauseCh <- struct{}{}:
		default:
		}
		o.logEvent("system", "session resumed")
	}
}

// addUsage records tokens + dollars for the active provider.
func (o *Orchestrator) addUsage(inTokens, outTokens int64) {
	o.costMu.Lock()
	defer o.costMu.Unlock()
	o.sessionTokensIn += inTokens
	o.sessionTokensOut += outTokens
	o.sessionCostUSD += pricing.Estimate(string(o.activeProvider), inTokens, outTokens)
}

// SendUserReply pipes a user reply to the active adapter's stdin.
// Returns an error if the adapter doesn't support stdin replies or no
// process is active. Always logs the attempt to the audit log.
func (o *Orchestrator) SendUserReply(reply string) error {
	adpt, ok := o.adapters[o.activeProvider]
	if !ok {
		return fmt.Errorf("no active adapter for %s", o.activeProvider)
	}
	replier, ok := adpt.(adapter.StdinReplier)
	if !ok {
		o.logEvent("system", fmt.Sprintf("user reply not delivered: %s does not support stdin replies", o.activeProvider))
		return fmt.Errorf("%s does not support stdin replies", o.activeProvider)
	}
	if err := replier.SendStdin(reply); err != nil {
		o.logEvent("system", "user reply error: "+err.Error())
		return err
	}
	o.logEvent("system", "↳ reply sent to "+string(o.activeProvider)+": "+reply)
	return nil
}

// Run starts the main task loop. Blocks until context is cancelled or
// MaxHandoffs is reached.
func (o *Orchestrator) Run(ctx context.Context) error {
	defer o.durability.ReleaseLock() //nolint:errcheck
	// Record final outcome on exit (success/failure determined by err return)
	var runErr error
	defer func() {
		_ = o.outcomes.Record(outcomes.Outcome{
			Profile:  o.activeProfile,
			TaskGoal: o.opts.TaskGoal,
			Success:  runErr == nil,
			Provider: string(o.activeProvider),
			Tokens:   o.sessionTokensIn + o.sessionTokensOut,
			CostUSD:  o.sessionCostUSD,
			Handoffs: o.handoffCount,
		})
		if s := o.redactor.Summary(); s != "" {
			o.logEvent("system", "redactions during session: "+s)
		}
	}()

	o.logEvent("system", fmt.Sprintf("relay session %s started, primary provider: %s", o.opts.SessionID, o.activeProvider))
	if o.worktree != nil {
		o.logEvent("system", fmt.Sprintf("session worktree: %s on branch %s",
			o.worktree.Path, o.worktree.Branch))
	}
	if err := o.ensureTaskNode(); err != nil {
		o.logEvent("system", "graph task init failed: "+err.Error())
	}
	// One-shot codebase scan → graph nodes. Runs once per session, async-safe.
	go o.scanCodeIntoGraph()
	o.pushStatus(fsm.StateRunning)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if o.opts.MaxHandoffs > 0 && o.handoffCount >= o.opts.MaxHandoffs {
			o.logEvent("system", fmt.Sprintf("max handoffs (%d) reached — stopping", o.opts.MaxHandoffs))
			return nil
		}

		// Circuit breaker — refuse to call a provider that's been failing
		if err := o.circuits.For(string(o.activeProvider)).Allow(); err != nil {
			o.logEvent("system", err.Error()+" — skipping provider")
			next := o.selectNextProvider()
			if next == o.activeProvider {
				runErr = err
				return err
			}
			o.activeProvider = next
			continue
		}

		// Predictive: if the active account is forecast to lack quota for another
		// full run, switch to a fresh one before starting (pre-emptive handoff).
		o.maybePreemptiveAccountSwitch()

		o.lastRetrySignal = nil // only signals from this run count
		if err := o.runOneProvider(ctx); err != nil {
			o.circuits.For(string(o.activeProvider)).RecordFailure()
			o.logEvent("system", "provider run error: "+err.Error())
			if err2 := o.machine.TransitionError(err.Error()); err2 != nil {
				fmt.Fprintf(os.Stderr, "[orchestrator] FSM error transition failed: %v\n", err2)
			}
			runErr = err
			return err
		}
		o.circuits.For(string(o.activeProvider)).RecordSuccess()

		// Check if we should hand off or stop
		manual := o.manualHandoff.Swap(false)
		snap := o.quotaReg.Adapters[o.activeProvider].Current()
		o.updateQuotaLedger(snap) // feed the quota wallet
		threshold := o.opts.ForceHandoffAt
		if threshold <= 0 {
			threshold = o.quotaReg.Adapters[o.activeProvider].BreachFraction()
		}

		// Fast-failure detection: provider exited with 0 tokens AND emitted
		// no meaningful events (no tool_use / tool_result / text). That's a
		// crash or auth failure, NOT a completed task. Hand off instead.
		used := snap.Total - snap.Remaining
		producedWork := used > 0 || o.lastText != ""
		if !manual && snap.FractionUsed < threshold && producedWork {
			o.logEvent("system", "task completed by "+string(o.activeProvider))
			return nil
		}
		if !manual && !producedWork {
			o.logEvent("system", string(o.activeProvider)+" exited without work — handing off")
			o.circuits.For(string(o.activeProvider)).RecordFailure()
		}

		// Genuine quota breach (work done, over threshold, not a manual handoff):
		// first try another account of the SAME provider. Cheaper than a
		// cross-provider handoff and the whole point of multi-account users.
		quotaBreach := producedWork && snap.FractionUsed >= threshold
		if !manual && quotaBreach && o.tryAccountFailover() {
			continue
		}

		// Wait-and-retry: when this run ended on a recoverable signal (usage-limit
		// reset, transient overload, safeguard) and policy allows, it can be cheaper
		// to wait for the same subscription to reset than to hand off. Runs after
		// account failover so an instant switch to a fresh login still wins.
		if !manual && o.lastRetrySignal != nil && o.maybeWaitForReset(ctx, snap) {
			continue
		}

		// Quota breach (manual, crash, or all accounts exhausted) → handoff
		if err := o.doHandoff(ctx); err != nil {
			return err
		}
	}
}

// tryAccountFailover rotates the active provider to its next non-exhausted
// account after the current one breached quota. Returns true if it switched (the
// caller re-runs the same provider); false when no other usable account exists,
// leaving a cross-provider handoff as the fallback.
func (o *Orchestrator) tryAccountFailover() bool {
	provider := string(o.activeProvider)
	accounts := o.opts.ProviderAccounts[provider]
	if len(accounts) < 2 {
		return false
	}

	cur := o.acctIdx[provider]
	if o.acctExhausted[provider] == nil {
		o.acctExhausted[provider] = make(map[string]bool)
	}
	o.acctExhausted[provider][accounts[cur].Label] = true

	next := nextAccountIndex(accounts, cur, o.acctExhausted[provider])
	if next < 0 {
		o.logEvent("handoff", fmt.Sprintf("all %d accounts for %s exhausted — cross-provider handoff", len(accounts), provider))
		return false
	}

	o.acctIdx[provider] = next
	// Fresh login → clear the quota adapter's exhausted view so it does not
	// immediately re-breach on stale headers / a latched 429.
	if r, ok := o.quotaReg.Adapters[o.activeProvider].(quota.Resetter); ok {
		r.Reset()
	}
	o.logEvent("handoff", fmt.Sprintf("account failover: %s '%s' exhausted → switching to '%s'",
		provider, accounts[cur].Label, accounts[next].Label))
	if o.opts.OnAccountSwitch != nil {
		o.opts.OnAccountSwitch(provider, accounts[next].Label)
	}
	o.pushStatus(o.machine.State())
	return true
}

// nextAccountIndex returns the next account index after cur (cyclic) whose label
// is not exhausted, or -1 when every account is exhausted.
func nextAccountIndex(accounts []AccountSpec, cur int, exhausted map[string]bool) int {
	for off := 1; off <= len(accounts); off++ {
		next := (cur + off) % len(accounts)
		if !exhausted[accounts[next].Label] {
			return next
		}
	}
	return -1
}

// accountEnvFor returns the env overrides for a provider's currently-selected
// account, falling back to the static ProviderAccountEnv map.
func (o *Orchestrator) accountEnvFor(provider string) []string {
	if accounts := o.opts.ProviderAccounts[provider]; len(accounts) > 0 {
		if idx := o.acctIdx[provider]; idx >= 0 && idx < len(accounts) {
			return accounts[idx].Env
		}
	}
	return o.opts.ProviderAccountEnv[provider]
}

// updateQuotaLedger writes the current provider+account quota snapshot to the
// wallet ledger, computing a units/minute burn rate from the change in usage
// since the last observation (so the wallet can forecast time-to-exhaustion).
func (o *Orchestrator) updateQuotaLedger(snap quota.Snapshot) {
	provider := string(o.activeProvider)
	account := o.activeAccountLabel(provider)
	now := time.Now()

	burn := 0.0
	if snap.Total >= 0 && snap.Remaining >= 0 {
		used := snap.Total - snap.Remaining
		if prevT, ok := o.prevUsedAt[provider]; ok {
			if d := float64(used - o.prevUsed[provider]); d > 0 {
				o.lastRunDelta[provider] = d // units consumed by the last run (forecast)
				if dtMin := now.Sub(prevT).Minutes(); dtMin > 0 {
					burn = d / dtMin
				}
			}
		}
		o.prevUsed[provider] = used
		o.prevUsedAt[provider] = now
	}

	led, err := quota.LoadLedger(o.opts.StateDir)
	if err != nil {
		return
	}
	led.Observe(provider, account, snap, burn, now.UnixMilli())
	_ = quota.SaveLedger(o.opts.StateDir, led)
}

// willLikelyBreach forecasts whether a provider has too little quota left for
// another run: remaining must be known and below the last run's consumption
// scaled by a safety margin. Pure + unit-tested.
func willLikelyBreach(remaining int64, lastRunUnits, safety float64) bool {
	if remaining < 0 || lastRunUnits <= 0 {
		return false // unknown remaining or no burn estimate yet → can't forecast
	}
	return float64(remaining) < lastRunUnits*safety
}

// maybePreemptiveAccountSwitch rotates to a fresh account BEFORE running when the
// active account is forecast to lack quota for another full run. This avoids
// starting a doomed attempt that breaches mid-task and wastes partial work — the
// pre-emptive half of "predictive handoff". It only acts when a burn estimate
// exists (≥1 prior run) and another account is available, so it never fires
// spuriously; cross-provider handoff stays the reactive fallback.
func (o *Orchestrator) maybePreemptiveAccountSwitch() {
	provider := string(o.activeProvider)
	if len(o.opts.ProviderAccounts[provider]) < 2 {
		return
	}
	burn := o.lastRunDelta[provider]
	if burn <= 0 {
		return
	}
	snap := o.quotaReg.Adapters[o.activeProvider].Current()
	if !willLikelyBreach(snap.Remaining, burn, 1.0) {
		return
	}
	if o.tryAccountFailover() {
		o.logEvent("system", fmt.Sprintf(
			"pre-emptive: %s account forecast to exhaust (~%d left < ~%.0f/run) — switched before running",
			provider, snap.Remaining, burn))
	}
}

// activeAccountLabel returns the label of the currently-selected account, or "".
func (o *Orchestrator) activeAccountLabel(provider string) string {
	if accounts := o.opts.ProviderAccounts[provider]; len(accounts) > 0 {
		if idx := o.acctIdx[provider]; idx >= 0 && idx < len(accounts) {
			return accounts[idx].Label
		}
	}
	return ""
}

// runOneProvider spawns the active provider and streams events until it exits.
func (o *Orchestrator) runOneProvider(ctx context.Context) error {
	adpt, ok := o.adapters[o.activeProvider]
	if !ok {
		return fmt.Errorf("no adapter for provider %s", o.activeProvider)
	}

	// Build composite system prompt: discovered instructions + (optional) contract.
	// First run: instructions only. Subsequent: instructions + handoff contract.
	systemPromptFile, cleanup, err := o.buildSystemPrompt(ctx)
	if err != nil {
		return fmt.Errorf("build system prompt: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Push per-provider model override from config if the adapter supports it.
	if setter, ok := adpt.(adapter.ModelSetter); ok {
		if m, ok := o.opts.ProviderModels[string(o.activeProvider)]; ok && m != "" {
			setter.SetModel(m)
			o.logEvent("system", fmt.Sprintf("%s model: %s", o.activeProvider, m))
		}
	}

	eventCh := make(chan adapter.AgentEvent, 64)
	runOpts := adapter.RunOptions{
		Task:             o.opts.TaskGoal,
		WorkDir:          o.opts.WorkDir,
		SystemPromptFile: systemPromptFile,
	}
	// Account-aware handoff: run the agent under the current account's env.
	if env := o.accountEnvFor(string(o.activeProvider)); len(env) > 0 {
		runOpts.Env = append(runOpts.Env, env...)
		if label := o.activeAccountLabel(string(o.activeProvider)); label != "" {
			o.logEvent("system", fmt.Sprintf("%s account: %s (%d env override(s))", o.activeProvider, label, len(env)))
		} else {
			o.logEvent("system", fmt.Sprintf("%s account env: %d override(s)", o.activeProvider, len(env)))
		}
	}

	if err := adpt.Run(ctx, runOpts, eventCh); err != nil {
		return fmt.Errorf("adapter.Run: %w", err)
	}
	o.quotaReg.Adapters[o.activeProvider].RecordRequest(0)

	o.logEvent("system", "provider "+string(o.activeProvider)+" started")
	if o.activeProfile != "" {
		o.logEvent("system", "task routed to profile "+o.activeProfile+" (chain: "+chainString(o.opts.ProviderPriority)+")")
	}
	o.pushStatus(fsm.StateRunning)

	// Vision polling — runs in parallel with the adapter event loop.
	// Cancelled when this function returns (via runCtx).
	runCtx, cancelVision := context.WithCancel(ctx)
	defer cancelVision()
	if o.opts.Vision.Enabled && o.opts.VisionProbe != nil {
		go o.runVisionLoop(runCtx)
	}

	// Drain event channel
	quotaMonitor := time.NewTicker(5 * time.Second)
	defer quotaMonitor.Stop()

	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				// Provider exited
				return nil
			}
			o.handleEvent(ev)

			// Check quota on each event
			if o.quotaReg.Adapters[o.activeProvider].ShouldHandoff() {
				return nil // trigger handoff
			}

		case <-o.manualHandoffCh:
			o.logEvent("handoff", "manual handoff accepted at orchestrator boundary")
			return nil

		case <-quotaMonitor.C:
			o.pushStatus(o.machine.State())
			if o.quotaReg.Adapters[o.activeProvider].ShouldHandoff() {
				return nil
			}

		case <-ctx.Done():
			_ = adpt.ForceStop()
			return ctx.Err()
		}
	}
}

// handleEvent processes one agent event: audit + server push + graph update.
func (o *Orchestrator) handleEvent(ev adapter.AgentEvent) {
	// Pause check — block until unpaused. Best-effort cooperative.
	for o.paused.Load() {
		select {
		case <-o.pauseCh:
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Redact secrets from event content and meta before they cross any boundary
	ev.Content = o.redactor.Scrub(ev.Content)
	if ev.Meta != nil {
		for k, v := range ev.Meta {
			ev.Meta[k] = o.redactor.Scrub(v)
		}
	}

	// Capture a recoverable retry signal (usage-limit reset, overload, safeguard)
	// so the Run loop can decide to wait it out instead of handing off.
	if ev.Type == adapter.EventRetry {
		o.captureRetrySignal(ev)
	}

	// Track token usage from tool_result events. Claude + Ollama both emit
	// tokens_in / tokens_out in Meta (extracted from stream-json `usage` field
	// or Ollama's prompt_eval_count + eval_count).
	if ev.Type == adapter.EventToolResult && ev.Meta != nil {
		var tIn, tOut int64
		if v, ok := ev.Meta["tokens_in"]; ok {
			tIn, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := ev.Meta["tokens_out"]; ok {
			tOut, _ = strconv.ParseInt(v, 10, 64)
		}
		if tIn > 0 || tOut > 0 {
			o.addUsage(tIn, tOut)
			// Also feed into the per-provider quota tracker so /api/providers
			// and /api/status.tokensUsed reflect real spend.
			if qa, ok := o.quotaReg.Adapters[o.activeProvider]; ok {
				qa.RecordRequest(tIn + tOut)
			}
		}
	}

	tag := string(ev.Type)
	msg := ev.Content
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}

	if o.httpServer != nil {
		o.httpServer.PushEvent(tag, msg)
	}
	_ = o.auditLog.Log("agent_event", map[string]any{
		"provider": string(o.activeProvider),
		"type":     tag,
		"content":  msg,
	})
	if ev.Type == adapter.EventText && strings.TrimSpace(ev.Content) != "" {
		o.lastText = strings.TrimSpace(ev.Content)
	}

	// Store significant events in graph
	if ev.Type == adapter.EventToolUse || ev.Type == adapter.EventToolResult || ev.Type == adapter.EventText {
		nodeID := fmt.Sprintf("%s-%d", tag, time.Now().UnixNano())
		_ = o.graphStore.UpsertNode(graph.Node{
			ID:        nodeID,
			NodeType:  tag,
			Payload:   map[string]interface{}{"content": ev.Content, "meta": ev.Meta, "provider": string(o.activeProvider)},
			SessionID: o.opts.SessionID,
			Weight:    eventWeight(ev.Type),
		})
		_ = o.graphStore.UpsertEdge(graph.Edge{
			FromID:   o.taskID,
			ToID:     nodeID,
			EdgeType: "emitted",
			Weight:   1.0,
		})
		o.extractGraphFacts(ev, nodeID)
	}
	o.pushStatus(o.machine.State())
}

// captureRetrySignal records the latest recoverable interruption seen in the
// event stream, reconstructing the retry.Signal from the adapter's Meta.
func (o *Orchestrator) captureRetrySignal(ev adapter.AgentEvent) {
	sig := &retry.Signal{Reason: retry.Reason(ev.Meta["reason"]), Line: ev.Content}
	if v := ev.Meta["reset_at"]; v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sig.ResetAt = &t
		}
	}
	o.lastRetrySignal = sig
	o.logEvent("system", fmt.Sprintf("recoverable limit on %s (%s) — evaluating wait-and-retry", o.activeProvider, sig.Reason))
}

// maybeWaitForReset implements the wait-and-retry strategy. Returns true when it
// waited and the caller should re-run the SAME provider; false to fall through
// to a handoff.
func (o *Orchestrator) maybeWaitForReset(ctx context.Context, snap quota.Snapshot) bool {
	cfg := o.opts.Retry
	if !cfg.Enabled {
		return false
	}
	if cfg.MaxRetries > 0 && o.retryWaits >= cfg.MaxRetries {
		o.logEvent("system", "wait-and-retry budget exhausted — handing off")
		return false
	}
	dec := cfg.Decide(o.lastRetrySignal, snap.ResetsAt, time.Now())
	switch dec.Action {
	case retry.ActionWait:
		wait := time.Until(dec.Until)
		if wait < 0 {
			wait = 0
		}
		o.retryWaits++
		o.logEvent("handoff", fmt.Sprintf("waiting %s for %s to reset, then continuing (attempt %d/%d)",
			wait.Round(time.Second), o.activeProvider, o.retryWaits, cfg.MaxRetries))
		if !o.waitInterruptible(ctx, wait) {
			o.logEvent("system", "wait interrupted — handing off")
			return false
		}
		o.resetQuotaView()
		o.lastRetrySignal = nil
		o.logEvent("handoff", fmt.Sprintf("%s reset window passed — resuming same provider", o.activeProvider))
		return true
	case retry.ActionImmediate:
		o.retryWaits++
		o.logEvent("handoff", fmt.Sprintf("transient error on %s — backing off %s then retrying (attempt %d/%d)",
			o.activeProvider, dec.Delay.Round(time.Second), o.retryWaits, cfg.MaxRetries))
		if !o.waitInterruptible(ctx, dec.Delay) {
			return false
		}
		o.resetQuotaView()
		o.lastRetrySignal = nil
		return true
	default:
		return false
	}
}

// waitInterruptible sleeps for d, waking early on context cancellation or a
// user-triggered manual handoff. Returns true only if the full duration elapsed.
func (o *Orchestrator) waitInterruptible(ctx context.Context, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		rem := time.Until(deadline)
		if rem <= 0 {
			return true
		}
		if o.manualHandoff.Load() {
			return false
		}
		step := rem
		if step > 2*time.Second {
			step = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(step):
		}
	}
}

// resetQuotaView clears the active provider's quota adapter so a fresh window is
// not immediately seen as still-exhausted from a latched header.
func (o *Orchestrator) resetQuotaView() {
	if r, ok := o.quotaReg.Adapters[o.activeProvider].(quota.Resetter); ok {
		r.Reset()
	}
}

// doHandoff executes the full RUNNING→RUNNING handoff cycle.
func (o *Orchestrator) doHandoff(ctx context.Context) error {
	o.logEvent("handoff", fmt.Sprintf("quota breach on %s — initiating handoff #%d", o.activeProvider, o.handoffCount+1))

	// 1. Await safe pause window (GATE)
	adpt := o.adapters[o.activeProvider]
	sp, err := adpt.AwaitSafePauseWindow(ctx, "quota_breach")
	if err != nil {
		// Timeout → emergency snapshot
		o.logEvent("handoff", "safe-pause timeout — emergency snapshot")
		commitSHA, snapErr := o.durability.EmergencySnapshot(o.opts.SessionID)
		if snapErr != nil {
			return fmt.Errorf("emergency snapshot: %w", snapErr)
		}
		_ = o.machine.ExecuteEmergencySnapshot("relay/emergency/"+o.opts.SessionID, commitSHA)
	} else {
		// Normal snapshot
		commitSHA, snapErr := o.durability.Snapshot(o.opts.SessionID, []string{"."})
		if snapErr != nil {
			return fmt.Errorf("snapshot: %w", snapErr)
		}
		sp.CommitSHA = commitSHA
		if err := o.machine.ExecuteSnapshot(commitSHA); err != nil {
			return fmt.Errorf("fsm ExecuteSnapshot: %w", err)
		}
	}

	// 2. Build + sign contract
	nextProvider := o.selectNextProvider()
	nextAction := o.deriveNextAction(nextProvider)
	contractObj, err := o.contractBld.Build(ctx, o.opts.SessionID, o.taskID, o.opts.TaskGoal, nextAction)
	if err != nil {
		return fmt.Errorf("contract build: %w", err)
	}
	rec := o.machine.Record()
	contractObj.SnapshotCommitSHA = rec.CommitSHA
	if err := o.contractBld.Sign(contractObj); err != nil {
		return fmt.Errorf("contract sign: %w", err)
	}

	// 3. Write envelope (Markdown for humans + JSON sidecar for verification)
	envelopePath := filepath.Join(o.opts.StateDir, "envelope.md")
	envelopeJson := filepath.Join(o.opts.StateDir, "envelope.json")
	serialized := o.contractSer.Serialize(contractObj, contract.DefaultCapability)
	if err := os.WriteFile(envelopePath, []byte(serialized), 0600); err != nil {
		return fmt.Errorf("write envelope: %w", err)
	}
	if err := contract.WriteJSON(envelopeJson, contractObj); err != nil {
		return fmt.Errorf("write envelope sidecar: %w", err)
	}
	if err := o.machine.ExecuteEnvelopeBuild(envelopePath); err != nil {
		return fmt.Errorf("fsm envelope: %w", err)
	}
	_ = adpt.ForceStop()

	// 4. Push contract to UI
	if o.httpServer != nil {
		var contractMap map[string]interface{}
		data, _ := json.Marshal(contractObj)
		_ = json.Unmarshal(data, &contractMap)
		o.httpServer.PushContract(contractMap)
	}

	// 5. Dispatch
	if err := o.machine.ExecuteDispatch(); err != nil {
		return fmt.Errorf("fsm dispatch: %w", err)
	}
	o.logEvent("handoff", fmt.Sprintf("dispatched to %s", nextProvider))

	// 6. Same-process handoff: write our own receiver heartbeat so
	// PollHeartbeat returns immediately. The cross-process mechanism is
	// preserved for `relay resume` CLI use.
	if err := fsm.WriteReceiverHeartbeat(o.opts.StateDir, o.opts.SessionID); err != nil {
		o.logEvent("system", "heartbeat write warn: "+err.Error())
	}
	if err := o.durability.PollHeartbeat(o.opts.SessionID); err != nil {
		_ = o.machine.TransitionError("receiver heartbeat timeout: " + err.Error())
		return fmt.Errorf("heartbeat: %w", err)
	}

	// 7. Resume
	if err := o.machine.ExecuteResume(string(nextProvider)); err != nil {
		return fmt.Errorf("fsm resume: %w", err)
	}

	o.activeProvider = nextProvider
	o.handoffCount++
	o.logEvent("handoff", fmt.Sprintf("handoff #%d complete — now running %s", o.handoffCount, nextProvider))

	return nil
}

// selectNextProvider picks the next provider, factoring quota, circuit state,
// and (when several are eligible) cost-per-token via pricing.Table.
func (o *Orchestrator) selectNextProvider() adapter.ProviderName {
	type cand struct {
		name adapter.ProviderName
		cost float64
	}
	var elig []cand
	for _, p := range o.opts.ProviderPriority {
		if p == o.activeProvider {
			continue
		}
		qa, ok := o.quotaReg.Adapters[p]
		if !ok || qa.ShouldHandoff() {
			continue
		}
		// Skip OPEN circuits
		if o.circuits.For(string(p)).Allow() != nil {
			continue
		}
		pr := pricing.Table[string(p)]
		// Cost score: input + output blend (approx 1:3 typical ratio)
		blend := pr.InputPerMTokens + 3*pr.OutputPerMTokens
		elig = append(elig, cand{name: p, cost: blend})
	}
	if len(elig) == 0 {
		// All exhausted/broken → wrap around to first non-active
		for _, p := range o.opts.ProviderPriority {
			if p != o.activeProvider {
				return p
			}
		}
		return o.opts.ProviderPriority[0]
	}
	// Prefer cheapest (ties broken by priority-list order)
	best := elig[0]
	for _, c := range elig[1:] {
		if c.cost < best.cost {
			best = c
		}
	}
	return best.name
}

// VerifierCritique asks a second adapter to review the recent agent output.
// Returns (passed, reason). No-op (returns true, "") when no peer is available.
//
// Wires opportunistically — only runs when a non-active peer is healthy.
func (o *Orchestrator) VerifierCritique(ctx context.Context) (bool, string) {
	// Pick first healthy peer from priority list
	var peer adapter.ProviderName
	for _, p := range o.opts.ProviderPriority {
		if p == o.activeProvider {
			continue
		}
		qa, ok := o.quotaReg.Adapters[p]
		if !ok || qa.ShouldHandoff() {
			continue
		}
		if o.circuits.For(string(p)).Allow() != nil {
			continue
		}
		if _, has := o.adapters[p]; has {
			peer = p
			break
		}
	}
	if peer == "" {
		return true, "" // no peer available — pass
	}
	o.logEvent("system", fmt.Sprintf("verifier: critique by %s", peer))
	// In MVP we just log the intent; full verifier loop spawns peer with the
	// last agent's output as the task. Stub returns success.
	return true, ""
}

// buildSystemPrompt composes the system prompt for the current run:
//  1. Discover CLAUDE.md / AGENTS.md / cursor rules / .relay/instructions.md
//  2. Merge in active-profile skills + context
//  3. If this is a handoff (handoffCount > 0), append the continuation contract
//  4. Write everything to a temp .md and return its path
//
// Returns ("", noop, nil) when there is nothing to inject.
func (o *Orchestrator) buildSystemPrompt(ctx context.Context) (string, func(), error) {
	// Resolve active profile skills + context
	var profSkills []string
	var profContext string
	if o.activeProfile != "" {
		if p, ok := o.opts.Profiles[o.activeProfile]; ok {
			profSkills = p.Skills
			profContext = p.ContextHint
		}
	}

	comp := instructions.Discover(instructions.DiscoverOptions{
		WorkDir:         o.opts.WorkDir,
		ProfileSkills:   profSkills,
		ProfileContext:  profContext,
		IncludeUserHome: true,
	})

	// Telemetry: list what was found in the event log so the user sees it.
	if len(comp.Sources) > 0 {
		labels := make([]string, 0, len(comp.Sources))
		for _, s := range comp.Sources {
			if s.Excluded {
				continue
			}
			labels = append(labels, fmt.Sprintf("%s [%s]", s.Label, fmtKB(s.Bytes)))
		}
		if len(labels) > 0 {
			o.logEvent("system", "context loaded: "+strings.Join(labels, ", "))
		}
	}
	if len(profSkills) > 0 {
		o.logEvent("system", "profile skills: "+strings.Join(profSkills, ", "))
	}

	// Push to UI for live visibility
	if o.httpServer != nil {
		o.publishInstructions(comp)
	}

	// Combine instructions + (optional) handoff contract
	var body strings.Builder
	if comp.CombinedMarkdown != "" {
		body.WriteString(comp.CombinedMarkdown)
	}
	if o.handoffCount > 0 {
		contractMd, err := o.readOrBuildContract(ctx)
		if err != nil {
			return "", nil, err
		}
		body.WriteString("\n\n---\n## Continuation contract\n_From previous provider — preserve all DO NOT REDO items and acceptance assertions._\n\n")
		body.WriteString(contractMd)
	}
	if body.Len() == 0 {
		return "", func() {}, nil
	}

	f, err := os.CreateTemp("", "relay-sysprompt-*.md")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(body.String()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	path := f.Name()
	return path, func() { os.Remove(path) }, nil
}

// publishInstructions surfaces discovered instruction files to the UI.
func (o *Orchestrator) publishInstructions(comp instructions.Composite) {
	type pubSource struct {
		Path   string `json:"path"`
		Origin string `json:"origin"`
		Label  string `json:"label"`
		Bytes  int64  `json:"bytes"`
		SHA    string `json:"sha"`
	}
	pubs := make([]pubSource, 0, len(comp.Sources))
	for _, s := range comp.Sources {
		if s.Excluded {
			continue
		}
		pubs = append(pubs, pubSource{
			Path:   s.Path,
			Origin: s.Origin,
			Label:  s.Label,
			Bytes:  s.Bytes,
			SHA:    s.SHA256,
		})
	}
	o.httpServer.PushInstructions(map[string]interface{}{
		"sources": pubs,
		"skills":  comp.Skills,
	})
}

func fmtKB(n int64) string {
	if n >= 1024 {
		return fmt.Sprintf("%.1fK", float64(n)/1024.0)
	}
	return fmt.Sprintf("%dB", n)
}

// readOrBuildContract returns the latest envelope as Markdown.
// If a signed JSON sidecar exists at envelope.json, the contract is verified
// before its Markdown form is returned. Tamper → emergency rebuild.
func (o *Orchestrator) readOrBuildContract(ctx context.Context) (string, error) {
	envelopeMd := filepath.Join(o.opts.StateDir, "envelope.md")
	envelopeJson := filepath.Join(o.opts.StateDir, "envelope.json")

	// Verify sidecar if present
	if c, err := contract.ReadJSON(envelopeJson); err == nil {
		if verr := o.contractBld.Verify(c); verr != nil {
			o.logEvent("system", "SECURITY: envelope signature invalid — rebuilding fresh contract")
			_ = o.auditLog.Log("envelope_signature_fail", map[string]interface{}{
				"sessionId": o.opts.SessionID,
				"err":       verr.Error(),
			})
		} else {
			if data, err := os.ReadFile(envelopeMd); err == nil {
				return string(data), nil
			}
			// JSON valid but MD missing → re-render
			return o.contractSer.Serialize(c, contract.DefaultCapability), nil
		}
	} else if data, err := os.ReadFile(envelopeMd); err == nil {
		// No JSON sidecar — accept MD but warn (legacy session)
		o.logEvent("system", "warning: envelope.md without signed sidecar — unverified")
		return string(data), nil
	}

	contractObj, err := o.contractBld.Build(ctx, o.opts.SessionID, o.taskID, o.opts.TaskGoal, "Resume task")
	if err != nil {
		return "", err
	}
	_ = o.contractBld.Sign(contractObj)
	return o.contractSer.Serialize(contractObj, contract.DefaultCapability), nil
}

// writeContractFile reads the latest envelope and returns a temp file path.
func (o *Orchestrator) writeContractFile(ctx context.Context) (string, error) {
	envelopePath := filepath.Join(o.opts.StateDir, "envelope.md")
	data, err := os.ReadFile(envelopePath)
	if err != nil {
		// No envelope yet — build one on the fly
		contractObj, berr := o.contractBld.Build(ctx, o.opts.SessionID, o.taskID, o.opts.TaskGoal, "Resume task")
		if berr != nil {
			return "", berr
		}
		_ = o.contractBld.Sign(contractObj)
		data = []byte(o.contractSer.Serialize(contractObj, contract.DefaultCapability))
	}

	f, err := os.CreateTemp("", "relay-contract-*.md")
	if err != nil {
		return "", err
	}
	_, _ = f.Write(data)
	_ = f.Close()
	return f.Name(), nil
}

func (o *Orchestrator) logEvent(tag, msg string) {
	if o.httpServer != nil {
		o.httpServer.PushEvent(tag, msg)
	}
	_ = o.auditLog.Log(tag, map[string]any{"msg": msg, "provider": string(o.activeProvider)})
}

func (o *Orchestrator) pushStatus(state fsm.FsmState) {
	if o.httpServer == nil {
		return
	}
	nodes, edges, _ := o.graphStore.Stats(context.TODO())
	snap := o.quotaReg.Adapters[o.activeProvider].Current()
	o.httpServer.PushSession(server.ApiSessionInfo{
		SessionID:      o.opts.SessionID,
		TaskID:         o.taskID,
		TaskGoal:       o.opts.TaskGoal,
		Role:           "developer",
		ActiveProvider: string(o.activeProvider),
		TokensUsed:     o.totalSessionTokens(snap),
		GraphNodes:     nodes,
		HandoffsDone:   o.handoffCount,
		HfsScore:       o.currentHFS(),
		FsmState:       string(state),
	})
	o.httpServer.PushProviders(o.providerStatuses())
	o.httpServer.PushGraphStats(nodes, edges)
	o.pushGraphDetail()
}

func (o *Orchestrator) ensureTaskNode() error {
	return o.graphStore.UpsertNode(graph.Node{
		ID:        o.taskID,
		NodeType:  "task",
		Payload:   map[string]interface{}{"goal": o.opts.TaskGoal, "sessionId": o.opts.SessionID, "project": o.projectRoot()},
		SessionID: o.opts.SessionID,
		Weight:    2.0,
	})
}

func (o *Orchestrator) projectRoot() string {
	if o.worktree != nil {
		return o.worktreeMgr.RepoDir
	}
	return o.opts.WorkDir
}

// scanCodeIntoGraph walks the user workdir, extracts symbols/modules, and
// upserts them into the graph store. Runs once at session start. Provides
// retrievable context that agents can query later via /api/graph/detail.
//
// Best-effort: errors are logged but never fail the session.
func (o *Orchestrator) scanCodeIntoGraph() {
	root := o.projectRoot()
	mods, syms, err := codegraph.Scan(root)
	if err != nil {
		o.logEvent("system", "codegraph scan error: "+err.Error())
		return
	}
	if len(mods) == 0 {
		return
	}
	for _, m := range mods {
		nodeID := "module:" + m.Path
		_ = o.graphStore.UpsertNode(graph.Node{
			ID:       nodeID,
			NodeType: "module",
			Payload: map[string]interface{}{
				"path":    m.Path,
				"lang":    m.Lang,
				"size":    m.Size,
				"sha":     m.SHA,
				"imports": m.Imports,
				"project": root,
			},
			SessionID: o.opts.SessionID,
			Weight:    1.0,
		})
		_ = o.graphStore.UpsertEdge(graph.Edge{
			FromID: o.taskID, ToID: nodeID, EdgeType: "scope", Weight: 0.3,
		})
	}
	for _, s := range syms {
		nodeID := "symbol:" + s.File + ":" + s.Name
		_ = o.graphStore.UpsertNode(graph.Node{
			ID:       nodeID,
			NodeType: "symbol",
			Payload: map[string]interface{}{
				"name":    s.Name,
				"kind":    s.Kind,
				"file":    s.File,
				"line":    s.Line,
				"lang":    s.Lang,
				"project": root,
			},
			SessionID: o.opts.SessionID,
			Weight:    0.8,
		})
		_ = o.graphStore.UpsertEdge(graph.Edge{
			FromID: "module:" + s.File, ToID: nodeID,
			EdgeType: "defines", Weight: 1.0,
		})
	}
	o.logEvent("system", fmt.Sprintf("codegraph: %d modules, %d symbols indexed", len(mods), len(syms)))
}

func (o *Orchestrator) extractGraphFacts(ev adapter.AgentEvent, eventNodeID string) {
	content := ev.Content + " " + strings.Join(metaValues(ev.Meta), " ")
	for _, path := range extractFilePaths(content) {
		fileID := "file:" + path
		payload := map[string]interface{}{
			"path":     path,
			"sha256":   o.fileSHA(path),
			"modified": true,
			"project":  o.projectRoot(),
		}
		_ = o.graphStore.UpsertNode(graph.Node{
			ID:        fileID,
			NodeType:  "file",
			Payload:   payload,
			SessionID: o.opts.SessionID,
			Weight:    1.2,
		})
		_ = o.graphStore.UpsertEdge(graph.Edge{FromID: o.taskID, ToID: fileID, EdgeType: "touches", Weight: 1.0})
		_ = o.graphStore.UpsertEdge(graph.Edge{FromID: eventNodeID, ToID: fileID, EdgeType: "mentions", Weight: 0.7})
	}

	lower := strings.ToLower(strings.TrimSpace(ev.Content))
	if strings.HasPrefix(lower, "decision:") {
		summary := strings.TrimSpace(ev.Content[len("decision:"):])
		o.addFactNode("decision", map[string]interface{}{"summary": summary, "rationale": "captured from agent output"}, eventNodeID, 1.4)
	}
	if strings.HasPrefix(lower, "constraint:") {
		rule := strings.TrimSpace(ev.Content[len("constraint:"):])
		o.addFactNode("constraint", map[string]interface{}{"rule": rule, "source": "agent"}, eventNodeID, 1.4)
	}
	if strings.HasPrefix(lower, "do not redo:") {
		item := strings.TrimSpace(ev.Content[len("do not redo:"):])
		o.addFactNode("do_not_redo", map[string]interface{}{"item": item}, eventNodeID, 1.6)
	}
	if strings.Contains(lower, "pass") && strings.Contains(lower, "test") {
		o.addFactNode("acceptance", map[string]interface{}{"assertion": strings.TrimSpace(ev.Content)}, eventNodeID, 1.0)
	}
}

func (o *Orchestrator) addFactNode(nodeType string, payload map[string]interface{}, eventNodeID string, weight float64) {
	payload["project"] = o.projectRoot()
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	id := fmt.Sprintf("%s:%s", nodeType, hex.EncodeToString(sum[:8]))
	_ = o.graphStore.UpsertNode(graph.Node{
		ID:        id,
		NodeType:  nodeType,
		Payload:   payload,
		SessionID: o.opts.SessionID,
		Weight:    weight,
	})
	_ = o.graphStore.UpsertEdge(graph.Edge{FromID: o.taskID, ToID: id, EdgeType: "has_fact", Weight: 1.0})
	_ = o.graphStore.UpsertEdge(graph.Edge{FromID: eventNodeID, ToID: id, EdgeType: "extracted", Weight: 0.8})
}

func eventWeight(t adapter.AgentEventType) float64 {
	switch t {
	case adapter.EventToolUse:
		return 1.1
	case adapter.EventToolResult:
		return 1.0
	case adapter.EventText:
		return 0.7
	default:
		return 0.5
	}
}

func metaValues(meta map[string]string) []string {
	values := make([]string, 0, len(meta))
	for _, v := range meta {
		values = append(values, v)
	}
	return values
}

func extractFilePaths(input string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, raw := range filePathRe.FindAllString(input, -1) {
		path := strings.Trim(raw, `"'.,;:()[]{}<>`)
		path = strings.ReplaceAll(path, "\\", "/")
		if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			continue
		}
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

func (o *Orchestrator) fileSHA(relPath string) string {
	path := filepath.Join(o.opts.WorkDir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (o *Orchestrator) deriveNextAction(nextProvider adapter.ProviderName) string {
	text := strings.TrimSpace(o.lastText)
	if text != "" {
		lower := strings.ToLower(text)
		if strings.HasPrefix(lower, "next:") || strings.Contains(lower, "next action") {
			return text
		}
	}
	return fmt.Sprintf("Continue the task with %s, preserving the recorded decisions, constraints, and file manifest.", nextProvider)
}

func (o *Orchestrator) providerStatuses() []server.ApiProviderStatus {
	next := o.nextProviderForDisplay()
	statuses := make([]server.ApiProviderStatus, 0, len(o.opts.ProviderPriority))
	for _, p := range o.opts.ProviderPriority {
		qa, ok := o.quotaReg.Adapters[p]
		if !ok {
			statuses = append(statuses, server.ApiProviderStatus{Name: string(p), State: "unknown", FractionUsed: -1})
			continue
		}
		snap := qa.Current()
		state := "standby"
		if p == o.activeProvider {
			state = "active"
		}
		if snap.FractionUsed >= qa.BreachFraction() && snap.FractionUsed >= 0 {
			state = "exhausted"
		}
		remaining := snap.Remaining
		var remainingPtr *int64
		if remaining >= 0 {
			remainingPtr = &remaining
		}
		var resetPtr *int64
		if snap.ResetsAt != nil {
			reset := snap.ResetsAt.Unix()
			resetPtr = &reset
		}
		statuses = append(statuses, server.ApiProviderStatus{
			Name:         string(p),
			State:        state,
			FractionUsed: snap.FractionUsed,
			Remaining:    remainingPtr,
			ResetsAt:     resetPtr,
			IsNext:       p == next,
			QuotaSource:  snap.Source,
		})
	}
	return statuses
}

func (o *Orchestrator) nextProviderForDisplay() adapter.ProviderName {
	for _, p := range o.opts.ProviderPriority {
		if p == o.activeProvider {
			continue
		}
		qa, ok := o.quotaReg.Adapters[p]
		if ok && !qa.ShouldHandoff() {
			return p
		}
	}
	return ""
}

func (o *Orchestrator) pushGraphDetail() {
	nbr, err := o.graphStore.Recent(context.TODO(), 220)
	if err != nil {
		return
	}
	nodes := make([]server.ApiGraphNode, 0, len(nbr.Nodes))
	for _, n := range nbr.Nodes {
		nodes = append(nodes, server.ApiGraphNode{
			ID:       n.ID,
			NodeType: n.NodeType,
			Weight:   n.Weight,
			Label:    nodeLabel(n),
		})
	}
	edges := make([]server.ApiGraphEdge, 0, len(nbr.Edges))
	for _, e := range nbr.Edges {
		edges = append(edges, server.ApiGraphEdge{
			FromID:   e.FromID,
			ToID:     e.ToID,
			EdgeType: e.EdgeType,
		})
	}
	o.httpServer.PushGraphDetail(nodes, edges)
}

func nodeLabel(n graph.Node) string {
	for _, key := range []string{"path", "summary", "rule", "item", "assertion", "goal", "content"} {
		if value, ok := n.Payload[key].(string); ok && value != "" {
			if len(value) > 36 {
				return value[:36] + "..."
			}
			return value
		}
	}
	return n.ID
}

func (o *Orchestrator) currentHFS() float64 {
	return 0
}

// totalSessionTokens returns whichever count is higher: the per-provider quota
// snapshot (good when quota source is authoritative, e.g. Claude proxy) or the
// orchestrator's own running tally from adapter usage events (good for Ollama
// which doesn't track quota at request level).
func (o *Orchestrator) totalSessionTokens(snap quota.Snapshot) int64 {
	q := tokensUsedFromSnapshot(snap)
	o.costMu.Lock()
	local := o.sessionTokensIn + o.sessionTokensOut
	o.costMu.Unlock()
	if local > q {
		return local
	}
	return q
}

func tokensUsedFromSnapshot(snap quota.Snapshot) int64 {
	if snap.Total <= 0 || snap.Remaining < 0 {
		return 0
	}
	return snap.Total - max64(snap.Remaining, 0)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ─── Profile auto-routing ─────────────────────────────────────────────────────

// matchProfile picks the best profile for a task goal. Scoring:
//
//	+3 per kind keyword appearing in goal
//	+1 per skill keyword in goal
//	+1 if context hint substring appears
//
// Returns profile name + its chain, or ("", nil) if no profile scores >0.
func matchProfile(taskGoal string, profiles map[string]ProfileSpec) (string, []string) {
	if len(profiles) == 0 {
		return "", nil
	}
	goal := strings.ToLower(taskGoal)
	type scored struct {
		name  string
		chain []string
		score int
	}
	var best scored
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
		if s > best.score {
			best = scored{name: name, chain: p.Chain, score: s}
		}
	}
	if best.score == 0 {
		return "", nil
	}
	return best.name, best.chain
}

// boostBySuccessRate — multiplies match score by historical success rate of
// each profile. Routes successful profiles up over time (RALPH item).
// Used by orchestrator when outcomes.Tracker is available.
func boostBySuccessRate(name string, baseScore int, tr *outcomes.Tracker) float64 {
	if tr == nil {
		return float64(baseScore)
	}
	rate := tr.SuccessRate(name)
	// Apply a gentle multiplier (0.5..1.5) — never zero out a profile
	return float64(baseScore) * (0.5 + rate)
}

func chainString(ps []adapter.ProviderName) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = string(p)
	}
	return strings.Join(parts, " → ")
}

// ─── Vision polling ───────────────────────────────────────────────────────────

// runVisionLoop polls the vision model every PollMs while the active provider
// is running. On each poll, if needs_input is true, emits a "vision" event
// with the question + choices so the user can act (or downstream automation
// can pick an answer).
//
// To avoid spamming the event log, identical observations are deduplicated
// against the most recent one.
func (o *Orchestrator) runVisionLoop(ctx context.Context) {
	interval := time.Duration(o.opts.Vision.PollMs) * time.Millisecond
	if interval < 500*time.Millisecond {
		interval = 2500 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	var lastSig string
	probeFn := o.opts.VisionProbe

	o.logEvent("system", fmt.Sprintf("vision polling started (every %s, provider %s)",
		interval, o.opts.Vision.Provider))

	for {
		select {
		case <-ctx.Done():
			o.logEvent("system", "vision polling stopped")
			return
		case <-t.C:
			res, err := probeFn()
			if err != nil {
				o.logEvent("system", "vision probe error: "+err.Error())
				continue
			}
			if !res.NeedsInput {
				// Just record summary periodically (every 4th tick) so user sees activity
				if res.Summary != "" && lastSig != "summary:"+res.Summary {
					lastSig = "summary:" + res.Summary
					o.logEvent("text", "[vision] "+res.Summary)
				}
				continue
			}
			sig := "q:" + res.Question + "|c:" + strings.Join(res.Choices, ",")
			if sig == lastSig {
				continue
			}
			lastSig = sig
			msg := "[vision] agent prompt: " + res.Question
			if len(res.Choices) > 0 {
				msg += "  · choices: " + strings.Join(res.Choices, " | ")
			}
			o.logEvent("waiting", msg)
		}
	}
}
