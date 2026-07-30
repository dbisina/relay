// internal/server/server.go
//
// RelayHttpServer — HTTP API on port 4748.
// Feeds the egui desktop UI (packages/ui).
//
// Endpoints:
//   GET  /api/health
//   GET  /api/status      → ApiSessionInfo
//   GET  /api/providers   → []ApiProviderStatus
//   GET  /api/events?since=<id> → []ApiEventLine
//   GET  /api/contract    → contract JSON
//   GET  /api/graph       → { nodes, edges }
//   POST /api/handoff     → trigger immediate handoff
//   WS   /ws              → live push (RFC 6455 minimal)

package server

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ─── API types (shape the egui api.rs expects) ───────────────────────────────

type ApiSessionInfo struct {
	SessionID      string    `json:"sessionId"`
	TaskID         string    `json:"taskId"`
	TaskGoal       string    `json:"taskGoal"`
	Role           string    `json:"role"`
	ActiveProvider string    `json:"activeProvider"`
	TokensUsed     int64     `json:"tokensUsed"`
	GraphNodes     int       `json:"graphNodes"`
	HandoffsDone   int       `json:"handoffsDone"`
	HfsScore       float64   `json:"hfsScore"`
	HfsHistory     []float64 `json:"hfsHistory"`
	FsmState       string    `json:"fsmState"`
}

type ApiProviderStatus struct {
	Name         string  `json:"name"`
	State        string  `json:"state"` // "active" | "standby" | "exhausted" | "error" | "unknown"
	FractionUsed float64 `json:"fractionUsed"`
	Remaining    *int64  `json:"remaining"`
	ResetsAt     *int64  `json:"resetsAt"` // Unix seconds
	IsNext       bool    `json:"isNext"`
	QuotaSource  string  `json:"quotaSource,omitempty"`
}

type ApiGraphNode struct {
	ID       string  `json:"id"`
	NodeType string  `json:"nodeType"`
	Weight   float64 `json:"weight"`
	Label    string  `json:"label,omitempty"`
}

type ApiGraphEdge struct {
	FromID   string `json:"fromId"`
	ToID     string `json:"toId"`
	EdgeType string `json:"edgeType"`
}

type ApiEventLine struct {
	ID  int64  `json:"id"`
	Ts  string `json:"ts"`
	Tag string `json:"tag"` // "tool_use" | "result" | "quota" | "handoff" | "system" | "text" | "waiting"
	Msg string `json:"msg"`
}

// ─── Server ───────────────────────────────────────────────────────────────────

// RunRequest is the payload for POST /api/run.
type RunRequest struct {
	Task        string   `json:"task"`
	Threshold   float64  `json:"threshold"`
	MaxHandoffs int      `json:"maxHandoffs"`
	Providers   []string `json:"providers,omitempty"` // ordered provider override; empty → config default
	// WorkDir is the project directory the session runs in. Empty means the
	// daemon's own working directory (the default for a task typed into the
	// app). The detect→adopt flow sets it to the detected agent's directory so
	// an adopted session continues against the real code rather than the
	// daemon's scratch folder.
	WorkDir string `json:"workDir,omitempty"`
}

// Server is the Relay HTTP + WebSocket server.
type Server struct {
	port int
	ln   net.Listener
	srv  *http.Server

	mu           sync.RWMutex
	session      *ApiSessionInfo
	providers    []ApiProviderStatus
	events       []ApiEventLine
	contract     map[string]interface{}
	instructions map[string]interface{}
	graphStats   struct{ Nodes, Edges int }
	graphNodes   []ApiGraphNode
	graphEdges   []ApiGraphEdge
	hfsHistory   []float64

	// Provider config/probe (set via SetProviderHandlers)
	providerDetailsCB func() []interface{} // returns []ApiProviderDetail (any to avoid import cycle)
	updateProviderCB  func(body []byte) error
	installProviderCB func(name string) error
	oauthProviderCB   func(name string) error
	apiKeyProviderCB  func(name, value string) error

	// Profiles
	listProfilesCB  func() []interface{}
	updateProfileCB func(body []byte) error

	// Vision
	visionConfigGetCB    func() interface{}
	visionConfigUpdateCB func(body []byte) error
	visionProbeCB        func() (interface{}, error)

	// Ollama helpers
	ollamaListCB   func(baseURL string) (interface{}, error)
	ollamaPullCB   func(baseURL, tag string) error
	ollamaLaunchCB func(provider, model string) error

	// Retrieval over chunks
	retrievalCB    func(query string, limit int) (interface{}, error)
	graphProjectCB func(project string) (interface{}, error)

	// Senior-eng additions
	diffCB       func() (interface{}, error)
	costCB       func() (interface{}, error)
	pauseCB      func(pause bool) error
	approvalsCB  func() interface{}
	approveCB    func(id string, ok bool, note string) error
	circuitCB    func() interface{}
	outcomesCB   func() interface{}
	redactionsCB func() interface{}

	// Stdin reply
	stdinReplyCB func(reply string) error

	// Agent detection (external agents Relay did not spawn)
	detectCB func(sinceHours int) (interface{}, error)
	adoptCB  func(id, target string, start bool) (interface{}, error)

	// Account-aware handoff: switch the active account for a provider.
	switchAccountCB func(provider, label string) error
	addAccountCB    func(provider, label, configDir string) error
	removeAccountCB func(provider, label string) error
	loginAccountCB  func(provider, label string) error

	// Pipeline designer (multi-agent DAGs).
	pipelineListCB func() []interface{}
	pipelineSaveCB func(body []byte) error
	pipelineRunCB  func(name string)

	// Quota wallet (per provider+account remaining/reset/forecast).
	walletCB func() interface{}

	// Time machine (handoff history + git commit trail + diff + rewind).
	historyCB       func() interface{}
	commitsCB       func() interface{}
	historyDiffCB   func(sha string) (interface{}, error)
	historyRewindCB func(sha string) (interface{}, error)

	sessionActive atomic.Bool

	eventIDCounter atomic.Int64
	handoffCB      func()
	runCB          func(RunRequest)

	wsClients   map[net.Conn]struct{}
	wsClientsMu sync.Mutex
}

// SetProviderHandlers wires provider detail listing and update callbacks.
func (s *Server) SetProviderHandlers(
	details func() []interface{},
	update func(body []byte) error,
	install func(name string) error,
	oauth func(name string) error,
	apiKey func(name, value string) error,
) {
	s.providerDetailsCB = details
	s.updateProviderCB = update
	s.installProviderCB = install
	s.oauthProviderCB = oauth
	s.apiKeyProviderCB = apiKey
}

// SetProfileHandlers wires profile listing + update callbacks.
func (s *Server) SetProfileHandlers(
	list func() []interface{},
	update func(body []byte) error,
) {
	s.listProfilesCB = list
	s.updateProfileCB = update
}

// SetVisionHandlers wires vision config + probe.
func (s *Server) SetVisionHandlers(
	getCfg func() interface{},
	updateCfg func(body []byte) error,
	probe func() (interface{}, error),
) {
	s.visionConfigGetCB = getCfg
	s.visionConfigUpdateCB = updateCfg
	s.visionProbeCB = probe
}

// SetOllamaHandlers wires Ollama list/pull/launch callbacks.
func (s *Server) SetOllamaHandlers(
	list func(baseURL string) (interface{}, error),
	pull func(baseURL, tag string) error,
	launch func(provider, model string) error,
) {
	s.ollamaListCB = list
	s.ollamaPullCB = pull
	s.ollamaLaunchCB = launch
}

// SetSeniorHandlers wires diff/cost/pause/approval/circuit/outcomes/redactions.
func (s *Server) SetSeniorHandlers(
	diff func() (interface{}, error),
	cost func() (interface{}, error),
	pause func(bool) error,
	approvals func() interface{},
	approve func(id string, ok bool, note string) error,
	circuit func() interface{},
	outcomes func() interface{},
	redactions func() interface{},
) {
	s.diffCB = diff
	s.costCB = cost
	s.pauseCB = pause
	s.approvalsCB = approvals
	s.approveCB = approve
	s.circuitCB = circuit
	s.outcomesCB = outcomes
	s.redactionsCB = redactions
}

// SetSessionReplyHandler wires the user-reply (stdin) callback.
func (s *Server) SetSessionReplyHandler(cb func(reply string) error) {
	s.stdinReplyCB = cb
}

// SetDetectHandlers wires external-agent detection (GET /api/detect) and
// adoption (POST /api/detect/adopt) callbacks.
func (s *Server) SetDetectHandlers(
	scan func(sinceHours int) (interface{}, error),
	adopt func(id, target string, start bool) (interface{}, error),
) {
	s.detectCB = scan
	s.adoptCB = adopt
}

// SetAccountHandler wires the account switcher (POST /api/providers/account).
func (s *Server) SetAccountHandler(switchAccount func(provider, label string) error) {
	s.switchAccountCB = switchAccount
}

// SetAccountManageHandlers wires add / remove / login for provider accounts.
func (s *Server) SetAccountManageHandlers(
	add func(provider, label, configDir string) error,
	remove func(provider, label string) error,
	login func(provider, label string) error,
) {
	s.addAccountCB = add
	s.removeAccountCB = remove
	s.loginAccountCB = login
}

// SetPipelineHandlers wires the pipeline designer: list (GET /api/pipelines),
// save (POST /api/pipelines), and run (POST /api/pipelines/run).
func (s *Server) SetPipelineHandlers(
	list func() []interface{},
	save func(body []byte) error,
	run func(name string),
) {
	s.pipelineListCB = list
	s.pipelineSaveCB = save
	s.pipelineRunCB = run
}

// SetWalletHandler wires the quota wallet (GET /api/quota/wallet).
func (s *Server) SetWalletHandler(wallet func() interface{}) {
	s.walletCB = wallet
}

// SetHistoryHandlers wires the time machine: handoff timeline (GET /api/history),
// git commit trail (GET /api/history/commits), per-commit diff (GET
// /api/history/diff?sha=), and a non-destructive rewind (POST /api/history/rewind).
func (s *Server) SetHistoryHandlers(
	history func() interface{},
	commits func() interface{},
	diff func(sha string) (interface{}, error),
	rewind func(sha string) (interface{}, error),
) {
	s.historyCB = history
	s.commitsCB = commits
	s.historyDiffCB = diff
	s.historyRewindCB = rewind
}

// New creates a server. Call Start() to begin listening.
func New(port int) *Server {
	if port == 0 {
		port = 4748
	}
	return &Server{
		port:      port,
		wsClients: make(map[net.Conn]struct{}),
	}
}

// Start begins listening on 127.0.0.1:<port>.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("server: listen :%d: %w", s.port, err)
	}
	s.ln = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/contract", s.handleContract)
	mux.HandleFunc("/api/instructions", s.handleInstructions)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/graph/detail", s.handleGraphDetail)
	mux.HandleFunc("/api/graph/project", s.handleGraphProject)
	mux.HandleFunc("/api/retrieval", s.handleRetrieval)
	mux.HandleFunc("/api/handoff", s.handleHandoff)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/config/providers", s.handleConfigProviders)
	mux.HandleFunc("/api/providers/install", s.handleProviderInstall)
	mux.HandleFunc("/api/providers/oauth", s.handleProviderOAuth)
	mux.HandleFunc("/api/providers/api-key", s.handleProviderAPIKey)
	mux.HandleFunc("/api/profiles", s.handleProfiles)
	mux.HandleFunc("/api/vision/config", s.handleVisionConfig)
	mux.HandleFunc("/api/vision/probe", s.handleVisionProbe)
	mux.HandleFunc("/api/session/reply", s.handleSessionReply)
	mux.HandleFunc("/api/detect", s.handleDetect)
	mux.HandleFunc("/api/detect/adopt", s.handleDetectAdopt)
	mux.HandleFunc("/api/providers/account", s.handleSwitchAccount)
	mux.HandleFunc("/api/providers/account/add", s.handleAddAccount)
	mux.HandleFunc("/api/providers/account/remove", s.handleRemoveAccount)
	mux.HandleFunc("/api/providers/account/login", s.handleLoginAccount)
	mux.HandleFunc("/api/pipelines", s.handlePipelines)
	mux.HandleFunc("/api/pipelines/run", s.handlePipelineRun)
	mux.HandleFunc("/api/quota/wallet", s.handleWallet)
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/history/commits", s.handleHistoryCommits)
	mux.HandleFunc("/api/history/diff", s.handleHistoryDiff)
	mux.HandleFunc("/api/history/rewind", s.handleHistoryRewind)
	mux.HandleFunc("/api/ollama/models", s.handleOllamaModels)
	mux.HandleFunc("/api/ollama/pull", s.handleOllamaPull)
	mux.HandleFunc("/api/ollama/launch", s.handleOllamaLaunch)
	mux.HandleFunc("/api/session/diff", s.handleDiff)
	mux.HandleFunc("/api/session/cost", s.handleCost)
	mux.HandleFunc("/api/session/pause", s.handlePause)
	mux.HandleFunc("/api/approvals", s.handleApprovals)
	mux.HandleFunc("/api/approvals/", s.handleApprovalResolve)
	mux.HandleFunc("/api/circuit", s.handleCircuit)
	mux.HandleFunc("/api/outcomes", s.handleOutcomes)
	mux.HandleFunc("/api/redactions", s.handleRedactions)
	mux.HandleFunc("/ws", s.handleWS)

	s.srv = &http.Server{Handler: mux}
	go s.srv.Serve(ln)
	return nil
}

// Stop shuts down the server.
func (s *Server) Stop() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

// OnHandoff registers a callback triggered by POST /api/handoff.
func (s *Server) OnHandoff(cb func()) { s.handoffCB = cb }

// OnRun registers a callback triggered by POST /api/run.
// The callback is called in a new goroutine; only one session runs at a time.
func (s *Server) OnRun(cb func(RunRequest)) { s.runCB = cb }

// ─── State push (called by Orchestrator) ─────────────────────────────────────

func (s *Server) PushSession(info ApiSessionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = &info
	s.hfsHistory = append(s.hfsHistory, info.HfsScore)
	if len(s.hfsHistory) > 20 {
		s.hfsHistory = s.hfsHistory[1:]
	}
	s.session.HfsHistory = append([]float64{}, s.hfsHistory...)
}

func (s *Server) PushProviders(providers []ApiProviderStatus) {
	s.mu.Lock()
	s.providers = providers
	s.mu.Unlock()
}

func (s *Server) PushEvent(tag, msg string) {
	now := time.Now()
	ts := fmt.Sprintf("%02d:%02d:%02d", now.Hour(), now.Minute(), now.Second())
	ev := ApiEventLine{
		ID:  s.eventIDCounter.Add(1),
		Ts:  ts,
		Tag: tag,
		Msg: msg,
	}
	s.mu.Lock()
	s.events = append(s.events, ev)
	if len(s.events) > 500 {
		s.events = s.events[1:]
	}
	s.mu.Unlock()

	s.broadcastWS(map[string]interface{}{"type": "event", "data": ev})
}

func (s *Server) PushContract(c map[string]interface{}) {
	s.mu.Lock()
	s.contract = c
	s.mu.Unlock()
}

func (s *Server) PushInstructions(i map[string]interface{}) {
	s.mu.Lock()
	s.instructions = i
	s.mu.Unlock()
}

func (s *Server) PushGraphStats(nodes, edges int) {
	s.mu.Lock()
	s.graphStats.Nodes = nodes
	s.graphStats.Edges = edges
	s.mu.Unlock()
}

func (s *Server) PushGraphDetail(nodes []ApiGraphNode, edges []ApiGraphEdge) {
	s.mu.Lock()
	s.graphNodes = nodes
	s.graphEdges = edges
	s.mu.Unlock()
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.json(w, map[string]interface{}{"ok": true, "port": s.port})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sess := s.session
	s.mu.RUnlock()
	if sess == nil {
		s.json(w, map[string]string{"error": "no active session"})
		return
	}
	s.json(w, sess)
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	p := s.providers
	s.mu.RUnlock()
	s.json(w, p)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	since, _ := strconv.ParseInt(sinceStr, 10, 64)

	s.mu.RLock()
	var result []ApiEventLine
	for _, ev := range s.events {
		if ev.ID > since {
			result = append(result, ev)
		}
	}
	s.mu.RUnlock()
	s.json(w, result)
}

func (s *Server) handleContract(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	c := s.contract
	s.mu.RUnlock()
	if c == nil {
		s.json(w, map[string]string{"error": "no contract built yet"})
		return
	}
	s.json(w, c)
}

func (s *Server) handleInstructions(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	i := s.instructions
	s.mu.RUnlock()
	if i == nil {
		s.json(w, map[string]interface{}{"sources": []interface{}{}, "skills": []interface{}{}})
		return
	}
	s.json(w, i)
}

// SetRetrievalHandler wires the chunks/FTS5 retrieval callback.
func (s *Server) SetRetrievalHandler(cb func(query string, limit int) (interface{}, error)) {
	s.retrievalCB = cb
}

// SetGraphProjectHandler wires the project-specific graph fetch callback.
func (s *Server) SetGraphProjectHandler(cb func(project string) (interface{}, error)) {
	s.graphProjectCB = cb
}

// handleRetrieval: GET /api/retrieval?q=<text>&limit=20 → top-K chunks
func (s *Server) handleRetrieval(w http.ResponseWriter, r *http.Request) {
	if s.retrievalCB == nil {
		s.json(w, []interface{}{})
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing q"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	res, err := s.retrievalCB(q, limit)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	stats := s.graphStats
	s.mu.RUnlock()
	s.json(w, map[string]int{"nodes": stats.Nodes, "edges": stats.Edges})
}

func (s *Server) handleGraphDetail(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	nodes := s.graphNodes
	edges := s.graphEdges
	s.mu.RUnlock()
	s.json(w, map[string]interface{}{"nodes": nodes, "edges": edges})
}

func (s *Server) handleGraphProject(w http.ResponseWriter, r *http.Request) {
	if s.graphProjectCB == nil {
		s.json(w, map[string]interface{}{"nodes": []interface{}{}, "edges": []interface{}{}})
		return
	}
	project := r.URL.Query().Get("path")
	if project == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing path"})
		return
	}
	res, err := s.graphProjectCB(project)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

func (s *Server) handleHandoff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.handoffCB != nil {
		go s.handoffCB()
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleConfigProviders serves GET /api/config/providers and POST /api/config/providers.
func (s *Server) handleConfigProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.providerDetailsCB == nil {
			s.json(w, map[string]string{"error": "provider config not available"})
			return
		}
		s.json(w, s.providerDetailsCB())

	case http.MethodPost:
		if s.updateProviderCB == nil {
			s.json(w, map[string]string{"error": "provider update not available"})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		if err := s.updateProviderCB(body); err != nil {
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		s.json(w, map[string]bool{"ok": true})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleProfiles: GET → list; POST → upsert/delete; body has full ApiProfile + delete bool.
func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.listProfilesCB == nil {
			s.json(w, []interface{}{})
			return
		}
		s.json(w, s.listProfilesCB())
	case http.MethodPost:
		if s.updateProfileCB == nil {
			s.json(w, map[string]string{"error": "profiles not available"})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		if err := s.updateProfileCB(body); err != nil {
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		s.json(w, map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleVisionConfig: GET → current config + status; POST → update.
func (s *Server) handleVisionConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.visionConfigGetCB == nil {
			s.json(w, map[string]string{"error": "vision unavailable"})
			return
		}
		s.json(w, s.visionConfigGetCB())
	case http.MethodPost:
		if s.visionConfigUpdateCB == nil {
			s.json(w, map[string]string{"error": "vision unavailable"})
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := s.visionConfigUpdateCB(body); err != nil {
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		s.json(w, map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleVisionProbe: POST → captures screen, calls vision LLM, returns observation.
func (s *Server) handleVisionProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.visionProbeCB == nil {
		s.json(w, map[string]string{"error": "vision unavailable"})
		return
	}
	obs, err := s.visionProbeCB()
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, obs)
}

// handleSessionReply: POST → writes a user reply to the active adapter's stdin.
func (s *Server) handleSessionReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.stdinReplyCB == nil {
		s.json(w, map[string]string{"error": "no active session"})
		return
	}
	var req struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reply == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing reply"})
		return
	}
	if err := s.stdinReplyCB(req.Reply); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleDetect: GET → list AI coding agents already running on this machine.
func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	if s.detectCB == nil {
		s.json(w, []interface{}{})
		return
	}
	sinceHours := 0 // 0 → daemon default window
	if v := r.URL.Query().Get("sinceHours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sinceHours = n
		}
	}
	res, err := s.detectCB(sinceHours)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

// handleDetectAdopt: POST {id,target} → render + persist a continuation brief
// for a detected agent so its work can be ported to another provider.
func (s *Server) handleDetectAdopt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.adoptCB == nil {
		s.json(w, map[string]string{"error": "adopt unavailable"})
		return
	}
	var req struct {
		ID     string `json:"id"`
		Target string `json:"target"`
		Start  bool   `json:"start"` // also launch a Relay session to continue the work
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.ID == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "id required"})
		return
	}
	res, err := s.adoptCB(req.ID, req.Target, req.Start)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

// handleSwitchAccount: POST {provider,label} → set the active account for a
// provider so the next handoff runs under that login.
func (s *Server) handleSwitchAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.switchAccountCB == nil {
		s.json(w, map[string]string{"error": "account switching unavailable"})
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Label    string `json:"label"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Provider == "" || req.Label == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "provider and label required"})
		return
	}
	if err := s.switchAccountCB(req.Provider, req.Label); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleAddAccount: POST {provider,label,configDir} → add a login for a provider.
func (s *Server) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.addAccountCB == nil {
		s.json(w, map[string]string{"error": "account management unavailable"})
		return
	}
	var req struct {
		Provider  string `json:"provider"`
		Label     string `json:"label"`
		ConfigDir string `json:"configDir"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Provider == "" || req.Label == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "provider and label required"})
		return
	}
	if err := s.addAccountCB(req.Provider, req.Label, req.ConfigDir); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleRemoveAccount: POST {provider,label} → remove a login.
func (s *Server) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.removeAccountCB == nil {
		s.json(w, map[string]string{"error": "account management unavailable"})
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Label    string `json:"label"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Provider == "" || req.Label == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "provider and label required"})
		return
	}
	if err := s.removeAccountCB(req.Provider, req.Label); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleLoginAccount: POST {provider,label} → open a terminal to sign in to that
// account's config dir.
func (s *Server) handleLoginAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.loginAccountCB == nil {
		s.json(w, map[string]string{"error": "account login unavailable"})
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Label    string `json:"label"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Provider == "" || req.Label == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "provider and label required"})
		return
	}
	if err := s.loginAccountCB(req.Provider, req.Label); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handlePipelines: GET → list pipelines; POST → save the full pipeline list.
func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.pipelineListCB == nil {
			s.json(w, []interface{}{})
			return
		}
		s.json(w, s.pipelineListCB())
	case http.MethodPost:
		if s.pipelineSaveCB == nil {
			s.json(w, map[string]string{"error": "pipelines unavailable"})
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := s.pipelineSaveCB(body); err != nil {
			s.json(w, map[string]string{"error": err.Error()})
			return
		}
		s.json(w, map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleWallet: GET → the quota wallet (per provider+account remaining, reset,
// and burn-rate forecast).
func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	if s.walletCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.walletCB())
}

// handleHistory: GET → the handoff timeline (from the audit log).
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.historyCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.historyCB())
}

// handleHistoryCommits: GET → the session's git commit trail (snapshot points).
func (s *Server) handleHistoryCommits(w http.ResponseWriter, r *http.Request) {
	if s.commitsCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.commitsCB())
}

// handleHistoryDiff: GET ?sha= → the diff for one commit.
func (s *Server) handleHistoryDiff(w http.ResponseWriter, r *http.Request) {
	if s.historyDiffCB == nil {
		s.json(w, map[string]string{"error": "history unavailable"})
		return
	}
	sha := r.URL.Query().Get("sha")
	if sha == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "sha required"})
		return
	}
	res, err := s.historyDiffCB(sha)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

// handleHistoryRewind: POST {sha} → create a non-destructive rewind branch at a
// snapshot so the user can recover that point without losing current work.
func (s *Server) handleHistoryRewind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.historyRewindCB == nil {
		s.json(w, map[string]string{"error": "rewind unavailable"})
		return
	}
	var req struct {
		SHA string `json:"sha"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.SHA == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "sha required"})
		return
	}
	res, err := s.historyRewindCB(req.SHA)
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, res)
}

// handlePipelineRun: POST {name} → execute a pipeline's nodes in dependency
// order. Honours the single-session guard so a run can't overlap another.
func (s *Server) handlePipelineRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.pipelineRunCB == nil {
		s.json(w, map[string]string{"error": "pipelines unavailable"})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "name required"})
		return
	}
	if !s.sessionActive.CompareAndSwap(false, true) {
		s.json(w, map[string]string{"error": "session already running"})
		return
	}
	name := req.Name
	go func() {
		defer s.sessionActive.Store(false)
		s.pipelineRunCB(name)
	}()
	s.json(w, map[string]bool{"ok": true})
}

// ── Senior-eng endpoints ─────────────────────────────────────────────────────

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if s.diffCB == nil {
		s.json(w, map[string]string{"error": "diff unavailable"})
		return
	}
	d, err := s.diffCB()
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, d)
}

func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	if s.costCB == nil {
		s.json(w, map[string]interface{}{"sessionUsd": 0, "todayUsd": 0, "monthUsd": 0})
		return
	}
	c, err := s.costCB()
	if err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, c)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.pauseCB == nil {
		s.json(w, map[string]string{"error": "pause unavailable"})
		return
	}
	var req struct {
		Pause bool `json:"pause"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if err := s.pauseCB(req.Pause); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if s.approvalsCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.approvalsCB())
}

// /api/approvals/<id>  POST {approved, note}
func (s *Server) handleApprovalResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/approvals/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if s.approveCB == nil {
		s.json(w, map[string]string{"error": "approvals unavailable"})
		return
	}
	var req struct {
		Approved bool   `json:"approved"`
		Note     string `json:"note"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if err := s.approveCB(id, req.Approved, req.Note); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

func (s *Server) handleCircuit(w http.ResponseWriter, r *http.Request) {
	if s.circuitCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.circuitCB())
}

func (s *Server) handleOutcomes(w http.ResponseWriter, r *http.Request) {
	if s.outcomesCB == nil {
		s.json(w, []interface{}{})
		return
	}
	s.json(w, s.outcomesCB())
}

func (s *Server) handleRedactions(w http.ResponseWriter, r *http.Request) {
	if s.redactionsCB == nil {
		s.json(w, map[string]interface{}{"count": 0, "summary": ""})
		return
	}
	s.json(w, s.redactionsCB())
}

// handleOllamaModels: GET /api/ollama/models?baseUrl=...  → installed models + curated list
func (s *Server) handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.ollamaListCB == nil {
		s.json(w, map[string]string{"error": "ollama unavailable"})
		return
	}
	baseURL := r.URL.Query().Get("baseUrl")
	models, err := s.ollamaListCB(baseURL)
	if err != nil {
		s.json(w, map[string]interface{}{"error": err.Error()})
		return
	}
	s.json(w, models)
}

// handleOllamaLaunch: POST /api/ollama/launch  body={provider, model}
//
//	→ spawns `ollama launch <tool> --model <model>` in a new terminal.
func (s *Server) handleOllamaLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.ollamaLaunchCB == nil {
		s.json(w, map[string]string{"error": "ollama launch unavailable"})
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing provider"})
		return
	}
	if err := s.ollamaLaunchCB(req.Provider, req.Model); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// handleOllamaPull: POST /api/ollama/pull  body={baseUrl, tag}  → spawns pull
func (s *Server) handleOllamaPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.ollamaPullCB == nil {
		s.json(w, map[string]string{"error": "ollama unavailable"})
		return
	}
	var req struct {
		BaseURL string `json:"baseUrl"`
		Tag     string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Tag == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing tag"})
		return
	}
	go s.ollamaPullCB(req.BaseURL, req.Tag) //nolint:errcheck
	s.json(w, map[string]bool{"ok": true})
}

// handleProviderInstall: POST /api/providers/install  body={name}
func (s *Server) handleProviderInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.installProviderCB == nil {
		s.json(w, map[string]string{"error": "install not available"})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing name"})
		return
	}
	go s.installProviderCB(req.Name) //nolint:errcheck
	s.json(w, map[string]bool{"ok": true})
}

// handleProviderOAuth: POST /api/providers/oauth  body={name}
func (s *Server) handleProviderOAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.oauthProviderCB == nil {
		s.json(w, map[string]string{"error": "OAuth not available"})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing name"})
		return
	}
	go s.oauthProviderCB(req.Name) //nolint:errcheck
	s.json(w, map[string]bool{"ok": true})
}

// handleProviderAPIKey: POST /api/providers/api-key  body={name, value}
func (s *Server) handleProviderAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.apiKeyProviderCB == nil {
		s.json(w, map[string]string{"error": "API key auth not available"})
		return
	}
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Value == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.json(w, map[string]string{"error": "missing name or value"})
		return
	}
	if err := s.apiKeyProviderCB(req.Name, req.Value); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req RunRequest
	req.Threshold = 0.85
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if err := s.TryStartSession(req); err != nil {
		s.json(w, map[string]string{"error": err.Error()})
		return
	}
	s.json(w, map[string]bool{"ok": true})
}

// TryStartSession launches a task in the background unless a session is already
// running. It is the single guarded entry point for starting work — used by both
// POST /api/run and the detect→adopt flow — so the single-session invariant holds
// no matter who initiates. Returns an error (and starts nothing) when the task is
// empty, the daemon has no run callback, or a session is already active.
func (s *Server) TryStartSession(req RunRequest) error {
	if req.Task == "" {
		return fmt.Errorf("task required")
	}
	if s.runCB == nil {
		return fmt.Errorf("daemon not ready for run requests")
	}
	if !s.sessionActive.CompareAndSwap(false, true) {
		return fmt.Errorf("session already running")
	}
	go func() {
		defer s.sessionActive.Store(false)
		s.runCB(req)
	}()
	return nil
}

func (s *Server) json(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := json.NewEncoder(w)
	_ = enc.Encode(data)
}

// ─── WebSocket (minimal RFC 6455 — text frames only) ─────────────────────────

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		http.Error(w, "not a websocket upgrade", http.StatusBadRequest)
		return
	}

	// Compute accept key
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return
	}

	// Send 101 Switching Protocols
	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	buf.WriteString(handshake)
	buf.Flush()

	s.wsClientsMu.Lock()
	s.wsClients[conn] = struct{}{}
	s.wsClientsMu.Unlock()

	// Read loop (discard client frames, detect close)
	go func() {
		buf2 := make([]byte, 128)
		for {
			if _, err := conn.Read(buf2); err != nil {
				break
			}
		}
		s.wsClientsMu.Lock()
		delete(s.wsClients, conn)
		s.wsClientsMu.Unlock()
		conn.Close()
	}()
}

func (s *Server) broadcastWS(data interface{}) {
	msg, err := json.Marshal(data)
	if err != nil {
		return
	}
	// Build text frame
	payload := msg
	frameLen := len(payload)
	var frame []byte
	if frameLen <= 125 {
		frame = make([]byte, 2+frameLen)
		frame[0] = 0x81 // FIN + text opcode
		frame[1] = byte(frameLen)
		copy(frame[2:], payload)
	} else {
		frame = make([]byte, 4+frameLen)
		frame[0] = 0x81
		frame[1] = 126
		frame[2] = byte(frameLen >> 8)
		frame[3] = byte(frameLen)
		copy(frame[4:], payload)
	}

	s.wsClientsMu.Lock()
	defer s.wsClientsMu.Unlock()
	for conn := range s.wsClients {
		conn.Write(frame) //nolint:errcheck
	}
}
