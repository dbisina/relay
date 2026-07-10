// tui.go — Interactive relay TUI modelled after gemini-cli.
//
// Layout (bottom-anchored):
//   title bar (1 line)
//   log area  (variable, scrollable)
//   [popup overlay when / is typed]
//   status bar (1 line)
//   input line (1 line)
//   hint line  (1 line)

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// ─── palette ────────────────────────────────────────────────────────────────

var (
	pBase   = lipgloss.Color("#080808")
	pS1     = lipgloss.Color("#101010")
	pS2     = lipgloss.Color("#1c1c1c")
	pTX0    = lipgloss.Color("#ececec")
	pTX1    = lipgloss.Color("#909090")
	pTX2    = lipgloss.Color("#555555")
	pTX3    = lipgloss.Color("#333333")
	pAccent = lipgloss.Color("#e06a38")
	pGreen  = lipgloss.Color("#3aaa70")
	pYellow = lipgloss.Color("#cc9420")
	pRed    = lipgloss.Color("#d04040")
	pBlue   = lipgloss.Color("#4a92d8")
)

// ─── slash commands ──────────────────────────────────────────────────────────

type slashCmd struct {
	cmd  string
	alt  string
	args string
	desc string
}

var slashCommands = []slashCmd{
	{"/run", "/r", "<task>", "Start an agent task"},
	{"/init", "/i", "", "Initialize .relay/ in current directory"},
	{"/daemon", "/d", "", "Start the relay daemon"},
	{"/handoff", "/h", "", "Trigger immediate provider handoff"},
	{"/status", "/s", "", "Show current session status"},
	{"/providers", "/p", "", "Show provider setup status"},
	{"/enable", "", "<name>", "Enable a provider"},
	{"/disable", "", "<name>", "Disable a provider"},
	{"/audit", "", "", "Verify audit log hash chain"},
	{"/graph", "", "", "Show knowledge graph statistics"},
	{"/open", "/o", "", "Open relay-ui desktop app"},
	{"/banner", "", "", "Reprint Relay banner"},
	{"/clear", "/cls", "", "Clear the log"},
	{"/help", "/?", "", "Show available commands"},
	{"/exit", "/q", "", "Quit relay TUI"},
}

func filteredCmds(query string) []slashCmd {
	if query == "/" || query == "" {
		return slashCommands
	}
	var out []slashCmd
	q := strings.ToLower(query)
	for _, c := range slashCommands {
		if strings.HasPrefix(c.cmd, q) || (c.alt != "" && strings.HasPrefix(c.alt, q)) {
			out = append(out, c)
		}
	}
	return out
}

// ─── api types ───────────────────────────────────────────────────────────────

type apiStatus struct {
	SessionID      string  `json:"sessionId"`
	TaskID         string  `json:"taskId"`
	TaskGoal       string  `json:"taskGoal"`
	ActiveProvider string  `json:"activeProvider"`
	TokensUsed     int64   `json:"tokensUsed"`
	HfsScore       float64 `json:"hfsScore"`
	FsmState       string  `json:"fsmState"`
}

type apiProvider struct {
	Name         string  `json:"name"`
	State        string  `json:"state"`
	FractionUsed float64 `json:"fractionUsed"`
	IsNext       bool    `json:"isNext"`
}

type apiEvent struct {
	ID  int64  `json:"id"`
	Ts  string `json:"ts"`
	Tag string `json:"tag"`
	Msg string `json:"msg"`
}

type daemonState struct {
	connected bool
	session   *apiStatus
	providers []apiProvider
}

// ─── log line ─────────────────────────────────────────────────────────────────

type logLine struct {
	ts   string
	tag  string
	msg  string
	kind string // "event" | "info" | "error" | "cmd"
}

// ─── messages ────────────────────────────────────────────────────────────────

type tickMsg struct{}
type pollResultMsg struct {
	state       daemonState
	newLines    []logLine
	lastEventID int64
}
type cmdOutputMsg struct {
	line string
	tag  string
}
type bannerMsg struct{}

// ─── model ───────────────────────────────────────────────────────────────────

type tuiModel struct {
	// input
	input   []rune
	cursor  int
	history []string
	histPos int
	histBuf string

	// popup
	showPopup  bool
	popupItems []slashCmd
	popupSel   int

	// log
	lines     []logLine
	scrollOff int // lines from bottom, 0 = pinned to bottom

	// daemon
	daemon      daemonState
	lastEventID int64

	// terminal
	width  int
	height int
}

func newTUIModel() tuiModel {
	m := tuiModel{
		histPos: -1,
		width:   80,
		height:  24,
	}
	// Branded splash on launch — replaces the bare hint line.
	for _, l := range renderSplash(m.width) {
		m.lines = append(m.lines, l)
	}
	return m
}

func now() string { return time.Now().Format("15:04:05") }

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(scheduleTick(), tea.EnterAltScreen)
}

// ─── scheduler / poll ─────────────────────────────────────────────────────────

func scheduleTick() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg{} })
}

func doPoll(lastID int64) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 2 * time.Second}
		base := "http://127.0.0.1:4748"
		var r pollResultMsg

		resp, err := client.Get(base + "/api/health")
		if err != nil {
			return r
		}
		resp.Body.Close()
		r.state.connected = true

		if res, err := client.Get(base + "/api/status"); err == nil {
			var s apiStatus
			if json.NewDecoder(res.Body).Decode(&s) == nil {
				r.state.session = &s
			}
			res.Body.Close()
		}
		if res, err := client.Get(base + "/api/providers"); err == nil {
			var ps []apiProvider
			if json.NewDecoder(res.Body).Decode(&ps) == nil {
				r.state.providers = ps
			}
			res.Body.Close()
		}
		if res, err := client.Get(fmt.Sprintf("%s/api/events?since=%d", base, lastID)); err == nil {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			var evs []apiEvent
			if json.Unmarshal(body, &evs) == nil {
				for _, e := range evs {
					r.newLines = append(r.newLines, logLine{ts: e.Ts, tag: e.Tag, msg: e.Msg})
					if e.ID > r.lastEventID {
						r.lastEventID = e.ID
					}
				}
			}
		}
		if r.lastEventID == 0 {
			r.lastEventID = lastID
		}
		return r
	}
}

// ─── update ───────────────────────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		firstSize := m.width == 80 && m.height == 24
		m.width, m.height = msg.Width, msg.Height
		// On first real WindowSizeMsg, replace the placeholder-width splash
		// with one rendered for the actual terminal width.
		if firstSize {
			isSplash := func(l logLine) bool { return l.tag == "relay" && l.kind == "info" }
			trim := 0
			for _, l := range m.lines {
				if !isSplash(l) {
					break
				}
				trim++
			}
			if trim > 0 {
				m.lines = m.lines[trim:]
			}
			prepend := renderSplash(m.width)
			m.lines = append(prepend, m.lines...)
		}

	case tickMsg:
		return m, tea.Batch(scheduleTick(), doPoll(m.lastEventID))

	case pollResultMsg:
		m.daemon = msg.state
		if msg.lastEventID > 0 {
			m.lastEventID = msg.lastEventID
		}
		for _, l := range msg.newLines {
			m.pushLine(l)
		}

	case cmdOutputMsg:
		m.pushLine(logLine{ts: now(), tag: msg.tag, msg: msg.line, kind: "info"})

	case bannerMsg:
		for _, l := range renderSplash(m.width) {
			m.pushLine(l)
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *tuiModel) pushLine(l logLine) {
	m.lines = append(m.lines, l)
	if len(m.lines) > 2000 {
		m.lines = m.lines[len(m.lines)-2000:]
	}
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	k := msg.String()

	// Global shortcuts
	switch k {
	case "ctrl+c", "ctrl+d":
		return m, tea.Quit
	case "ctrl+l":
		m.lines = nil
		m.scrollOff = 0
		return m, nil
	}

	// Popup navigation
	if m.showPopup {
		switch k {
		case "esc":
			m.showPopup = false
			return m, nil
		case "up":
			if m.popupSel > 0 {
				m.popupSel--
			}
			return m, nil
		case "down":
			if m.popupSel < len(m.popupItems)-1 {
				m.popupSel++
			}
			return m, nil
		case "tab":
			m.popupSel = (m.popupSel + 1) % len(m.popupItems)
			return m, nil
		case "enter":
			if len(m.popupItems) > 0 {
				selected := m.popupItems[m.popupSel]
				if selected.args != "" {
					// Complete with command + space, keep popup open until args entered
					m.input = []rune(selected.cmd + " ")
					m.cursor = len(m.input)
					m.showPopup = false
				} else {
					// Execute immediately
					m.showPopup = false
					m.input = nil
					m.cursor = 0
					return m, m.executeCommand(selected.cmd)
				}
			}
			return m, nil
		}
	}

	switch k {
	case "enter":
		line := strings.TrimSpace(string(m.input))
		m.input = nil
		m.cursor = 0
		m.showPopup = false
		if line == "" {
			return m, nil
		}
		m.history = append(m.history, line)
		m.histPos = -1
		m.histBuf = ""
		return m, m.executeCommand(line)

	case "tab":
		if !m.showPopup {
			query := string(m.input)
			m.popupItems = filteredCmds(query)
			if len(m.popupItems) > 0 {
				m.popupSel = 0
				m.showPopup = true
			}
		}

	case "up":
		if m.showPopup {
			return m, nil
		}
		if m.scrollOff < len(m.lines)-m.logHeight() {
			m.scrollOff += m.logHeight() / 3
		}
	case "down":
		m.scrollOff -= m.logHeight() / 3
		if m.scrollOff < 0 {
			m.scrollOff = 0
		}

	case "pgup":
		m.scrollOff += m.logHeight()
		max := len(m.lines) - m.logHeight()
		if max < 0 {
			max = 0
		}
		if m.scrollOff > max {
			m.scrollOff = max
		}
	case "pgdown":
		m.scrollOff -= m.logHeight()
		if m.scrollOff < 0 {
			m.scrollOff = 0
		}

	case "ctrl+up", "alt+up":
		if len(m.history) == 0 {
			break
		}
		if m.histPos == -1 {
			m.histBuf = string(m.input)
			m.histPos = len(m.history) - 1
		} else if m.histPos > 0 {
			m.histPos--
		}
		m.input = []rune(m.history[m.histPos])
		m.cursor = len(m.input)
	case "ctrl+down", "alt+down":
		if m.histPos == -1 {
			break
		}
		if m.histPos < len(m.history)-1 {
			m.histPos++
			m.input = []rune(m.history[m.histPos])
		} else {
			m.histPos = -1
			m.input = []rune(m.histBuf)
		}
		m.cursor = len(m.input)

	case "backspace":
		if m.cursor > 0 {
			m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
			m.cursor--
			m.updatePopup()
		}
	case "delete":
		if m.cursor < len(m.input) {
			m.input = append(m.input[:m.cursor], m.input[m.cursor+1:]...)
			m.updatePopup()
		}

	case "left":
		if m.cursor > 0 {
			m.cursor--
		}
	case "right":
		if m.cursor < len(m.input) {
			m.cursor++
		}
	case "home", "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = len(m.input)
	case "ctrl+u":
		m.input = m.input[m.cursor:]
		m.cursor = 0
		m.updatePopup()
	case "ctrl+k":
		m.input = m.input[:m.cursor]
		m.updatePopup()
	case "esc":
		if m.showPopup {
			m.showPopup = false
		}

	default:
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				m.input = append(m.input[:m.cursor], append([]rune{r}, m.input[m.cursor:]...)...)
				m.cursor++
			}
		}
		m.updatePopup()
	}

	return m, nil
}

func (m *tuiModel) updatePopup() {
	q := string(m.input[:m.cursor])
	if strings.HasPrefix(q, "/") {
		m.popupItems = filteredCmds(q)
		m.showPopup = len(m.popupItems) > 0
		if m.popupSel >= len(m.popupItems) {
			m.popupSel = 0
		}
	} else {
		m.showPopup = false
	}
}

// ─── command execution ────────────────────────────────────────────────────────

func resolveAlias(cmd string) string {
	for _, sc := range slashCommands {
		if sc.alt != "" && cmd == sc.alt {
			return sc.cmd
		}
	}
	return cmd
}

func (m tuiModel) executeCommand(line string) tea.Cmd {
	parts := strings.SplitN(line, " ", 2)
	cmd := resolveAlias(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/exit", "/q", "exit", "quit", "q":
		return tea.Quit

	case "/banner":
		return func() tea.Msg { return bannerMsg{} }

	case "/clear", "/cls":
		return func() tea.Msg { return cmdOutputMsg{} } // model clears in handleKey

	case "/help", "/?":
		lines := []string{"", "  Available commands:", ""}
		for _, sc := range slashCommands {
			alt := ""
			if sc.alt != "" {
				alt = "  " + sc.alt
			}
			a := ""
			if sc.args != "" {
				a = " " + sc.args
			}
			lines = append(lines, fmt.Sprintf("    %-12s%s%-12s  %s", sc.cmd, alt, a, sc.desc))
		}
		lines = append(lines, "", "  Keyboard shortcuts:", "    Tab / ↑↓  navigate popup  ·  PgUp/PgDn  scroll  ·  Ctrl+L  clear  ·  Ctrl+C  quit", "")
		return func() tea.Msg { return cmdOutputMsg{line: strings.Join(lines, "\n"), tag: "system"} }

	case "/init", "/i":
		return func() tea.Msg {
			out, err := exec.Command("relay", "init").CombinedOutput()
			if err != nil {
				return cmdOutputMsg{line: strings.TrimSpace(string(out)), tag: "error"}
			}
			return cmdOutputMsg{line: strings.TrimSpace(string(out)), tag: "result"}
		}

	case "/daemon", "/d":
		return func() tea.Msg {
			var c *exec.Cmd
			if runtime.GOOS == "windows" {
				c = exec.Command("cmd", "/c", "start", "/b", "relay", "daemon")
			} else {
				c = exec.Command("relay", "daemon")
				c.SysProcAttr = daemonSysProcAttr()
			}
			if err := c.Start(); err != nil {
				return cmdOutputMsg{line: "failed to start daemon: " + err.Error(), tag: "error"}
			}
			return cmdOutputMsg{line: "daemon starting on :4748 …", tag: "system"}
		}

	case "/run", "/r":
		if arg == "" {
			return func() tea.Msg {
				return cmdOutputMsg{line: "usage: /run <task description>", tag: "error"}
			}
		}
		taskArg := arg
		return func() tea.Msg {
			client := &http.Client{Timeout: 2 * time.Second}
			payload := fmt.Sprintf(`{"task":%q,"threshold":0.85}`, taskArg)
			resp, err := client.Post("http://127.0.0.1:4748/api/run", "application/json", strings.NewReader(payload))
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return cmdOutputMsg{line: "task accepted by daemon: " + taskArg, tag: "system"}
			}
			if resp != nil {
				resp.Body.Close()
			}
			c := exec.Command("relay", "run", "--yes", taskArg)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Start(); err != nil {
				return cmdOutputMsg{line: "failed: " + err.Error(), tag: "error"}
			}
			return cmdOutputMsg{line: "spawned: relay run --yes \"" + taskArg + "\"", tag: "system"}
		}

	case "/handoff", "/h":
		return func() tea.Msg {
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Post("http://127.0.0.1:4748/api/handoff", "application/json", strings.NewReader("{}"))
			if err != nil {
				return cmdOutputMsg{line: "handoff: " + err.Error(), tag: "error"}
			}
			resp.Body.Close()
			return cmdOutputMsg{line: "handoff triggered", tag: "handoff"}
		}

	case "/status", "/s":
		return func() tea.Msg {
			client := &http.Client{Timeout: 2 * time.Second}
			r, err := client.Get("http://127.0.0.1:4748/api/status")
			if err != nil {
				return cmdOutputMsg{line: "daemon not reachable", tag: "error"}
			}
			defer r.Body.Close()
			var s apiStatus
			if json.NewDecoder(r.Body).Decode(&s) != nil || s.SessionID == "" {
				return cmdOutputMsg{line: "no active session — use /run to start one", tag: "system"}
			}
			return cmdOutputMsg{
				line: fmt.Sprintf("task:%s  provider:%s  tokens:%s  state:%s  hfs:%.2f",
					s.TaskID, s.ActiveProvider, tuiFmtTok(s.TokensUsed), s.FsmState, s.HfsScore),
				tag: "system",
			}
		}

	case "/providers", "/p":
		return func() tea.Msg {
			client := &http.Client{Timeout: 4 * time.Second}
			r, err := client.Get("http://127.0.0.1:4748/api/config/providers")
			if err != nil {
				return cmdOutputMsg{line: "daemon not running — start with /daemon first", tag: "error"}
			}
			defer r.Body.Close()
			var details []ApiProviderDetail
			if json.NewDecoder(r.Body).Decode(&details) != nil {
				return cmdOutputMsg{line: "could not parse provider details", tag: "error"}
			}
			return cmdOutputMsg{line: formatProvidersTable(details), tag: "system"}
		}

	case "/enable":
		if arg == "" {
			return func() tea.Msg { return cmdOutputMsg{line: "usage: /enable <name>", tag: "error"} }
		}
		return toggleProvider(arg, true)

	case "/disable":
		if arg == "" {
			return func() tea.Msg { return cmdOutputMsg{line: "usage: /disable <name>", tag: "error"} }
		}
		return toggleProvider(arg, false)

	case "/audit":
		return func() tea.Msg {
			out, err := exec.Command("relay", "audit", "verify").CombinedOutput()
			tag := "result"
			if err != nil {
				tag = "error"
			}
			return cmdOutputMsg{line: strings.TrimSpace(string(out)), tag: tag}
		}

	case "/graph":
		return func() tea.Msg {
			out, err := exec.Command("relay", "graph").CombinedOutput()
			if err != nil {
				return cmdOutputMsg{line: string(out), tag: "error"}
			}
			return cmdOutputMsg{line: strings.TrimSpace(string(out)), tag: "result"}
		}

	case "/open", "/o":
		return func() tea.Msg {
			var c *exec.Cmd
			switch runtime.GOOS {
			case "windows":
				c = exec.Command("cmd", "/c", "start", "relay-ui")
			case "darwin":
				c = exec.Command("open", "-a", "relay-ui")
			default:
				c = exec.Command("relay-ui")
			}
			if err := c.Start(); err != nil {
				return cmdOutputMsg{line: "could not open relay-ui: " + err.Error(), tag: "error"}
			}
			return cmdOutputMsg{line: "opening relay-ui…", tag: "system"}
		}

	default:
		if !strings.HasPrefix(cmd, "/") {
			return m.executeCommand("/run " + line)
		}
		return func() tea.Msg {
			return cmdOutputMsg{line: "unknown: " + cmd + "  (type /help)", tag: "error"}
		}
	}
}

func toggleProvider(name string, enable bool) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 3 * time.Second}
		payload := fmt.Sprintf(`{"name":%q,"enabled":%v}`, name, enable)
		resp, err := client.Post("http://127.0.0.1:4748/api/config/providers", "application/json", strings.NewReader(payload))
		if err != nil {
			return cmdOutputMsg{
				line: fmt.Sprintf("daemon not reachable — edit .relay/relay.toml to %s %s",
					map[bool]string{true: "enable", false: "disable"}[enable], name),
				tag: "error",
			}
		}
		resp.Body.Close()
		word := "enabled"
		if !enable {
			word = "disabled"
		}
		return cmdOutputMsg{line: word + " " + name, tag: "result"}
	}
}

// ─── view ─────────────────────────────────────────────────────────────────────

const (
	fixedLines = 4 // title + status + input + hint
	maxPopup   = 8 // max popup items shown
)

func (m tuiModel) logHeight() int {
	ph := 0
	if m.showPopup {
		n := len(m.popupItems)
		if n > maxPopup {
			n = maxPopup
		}
		ph = n + 2 // items + border top/bottom
	}
	h := m.height - fixedLines - ph
	if h < 3 {
		h = 3
	}
	return h
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return ""
	}

	var sections []string

	// Title bar
	sections = append(sections, m.renderTitle())

	// Log area
	sections = append(sections, m.renderLog(m.logHeight()))

	// Popup (above input)
	if m.showPopup && len(m.popupItems) > 0 {
		sections = append(sections, m.renderPopup())
	}

	// Status bar
	sections = append(sections, m.renderStatus())

	// Input
	sections = append(sections, m.renderInput())

	// Hint
	sections = append(sections, m.renderHint())

	return strings.Join(sections, "\n")
}

// ── title ─────────────────────────────────────────────────────────────────────

func (m tuiModel) renderTitle() string {
	left := lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render("relay")
	left += lipgloss.NewStyle().Foreground(pTX2).Render("  agent orchestrator")

	right := lipgloss.NewStyle().Foreground(pTX3).Render("v0.3.0")

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}

	line := left + strings.Repeat(" ", pad) + right
	return lipgloss.NewStyle().
		Background(pS1).
		Width(m.width).
		Padding(0, 1).
		Render(line)
}

// ── log ───────────────────────────────────────────────────────────────────────

func (m tuiModel) renderLog(h int) string {
	total := len(m.lines)
	end := total - m.scrollOff
	if end <= 0 {
		return strings.Repeat("\n", h-1)
	}
	start := end - h
	if start < 0 {
		start = 0
	}
	visible := m.lines[start:end]

	// Pad top if fewer lines than available
	var rows []string
	for i := len(visible); i < h; i++ {
		rows = append(rows, "")
	}
	for _, l := range visible {
		rows = append(rows, m.renderLogLine(l))
	}

	// Scroll indicator
	if m.scrollOff > 0 {
		rows[0] = lipgloss.NewStyle().
			Foreground(pTX3).
			Render(fmt.Sprintf("  ↓ %d more lines below — PgDn to scroll", m.scrollOff))
	}

	return strings.Join(rows, "\n")
}

func (m tuiModel) renderLogLine(l logLine) string {
	w := m.width - 2 // 2 for left padding

	if l.kind == "info" && l.tag == "relay" {
		// Welcome / info line
		return "  " + lipgloss.NewStyle().Foreground(pTX2).Render(l.msg)
	}

	// Multi-line messages (e.g. /help output)
	if strings.Contains(l.msg, "\n") {
		var sb strings.Builder
		for i, line := range strings.Split(l.msg, "\n") {
			if i == 0 && line == "" {
				continue
			}
			sb.WriteString("  ")
			sb.WriteString(lipgloss.NewStyle().Foreground(pTX1).Render(line))
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	// Timestamp
	ts := lipgloss.NewStyle().Foreground(pTX3).Width(10).Render(l.ts)

	// Tag badge
	tagC, tagBg, tagLabel := tuiTagStyle(l.tag)
	badge := lipgloss.NewStyle().
		Foreground(tagC).
		Background(tagBg).
		Bold(true).
		PaddingLeft(1).PaddingRight(1).
		Render(padRight(tagLabel, 7))

	// Message
	msgC := tuiMsgColor(l.tag)
	maxMsg := w - 10 - lipgloss.Width(badge) - 3
	msg := l.msg
	if len(msg) > maxMsg && maxMsg > 3 {
		msg = msg[:maxMsg-1] + "…"
	}
	msgStr := lipgloss.NewStyle().Foreground(msgC).Render(msg)

	return "  " + ts + badge + "  " + msgStr
}

// ── popup ─────────────────────────────────────────────────────────────────────

func (m tuiModel) renderPopup() string {
	items := m.popupItems
	if len(items) > maxPopup {
		items = items[:maxPopup]
	}

	// Measure content width
	maxW := 0
	for _, c := range items {
		w := 2 + len(c.cmd) + 2 + len(c.args) + 2 + len(c.desc) + 2
		if w > maxW {
			maxW = w
		}
	}
	if maxW < 36 {
		maxW = 36
	}
	if maxW > m.width-4 {
		maxW = m.width - 4
	}

	var rows []string
	for i, c := range items {
		cmdS := lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render(c.cmd)
		args := ""
		if c.args != "" {
			args = lipgloss.NewStyle().Foreground(pTX2).Italic(true).Render(" " + c.args)
		}
		desc := lipgloss.NewStyle().Foreground(pTX1).Render(c.desc)

		left := cmdS + args
		leftW := lipgloss.Width(left)
		gap := maxW - leftW - lipgloss.Width(desc) - 4
		if gap < 2 {
			gap = 2
		}
		row := "  " + left + strings.Repeat(" ", gap) + desc + "  "

		if i == m.popupSel {
			row = lipgloss.NewStyle().
				Background(lipgloss.Color("#1c1c1c")).
				Foreground(pTX0).
				Width(maxW).
				Render("  " + left + strings.Repeat(" ", gap) + desc + "  ")
		}
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")

	hint := lipgloss.NewStyle().Foreground(pTX3).
		Render("  ↑↓ navigate  ·  Enter select  ·  Esc close")

	popupContent := content + "\n" + hint

	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pAccent).
		Width(maxW).
		Render(popupContent)

	// Left-indent by 2 (aligns with input prompt)
	var sb strings.Builder
	for _, line := range strings.Split(popup, "\n") {
		sb.WriteString("  " + line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── status bar ────────────────────────────────────────────────────────────────

func (m tuiModel) renderStatus() string {
	var left string

	if m.daemon.connected {
		dot := lipgloss.NewStyle().Foreground(pGreen).Render("●")
		left = dot + " "

		if m.daemon.session != nil {
			s := m.daemon.session
			left += lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render(s.ActiveProvider)

			for _, p := range m.daemon.providers {
				if p.State == "active" {
					col := pGreen
					if p.FractionUsed > 0.75 {
						col = pYellow
					}
					left += lipgloss.NewStyle().Foreground(col).
						Render(fmt.Sprintf(" %.0f%%", p.FractionUsed*100))
					break
				}
			}
			left += lipgloss.NewStyle().Foreground(pTX2).Render("  ")
			left += lipgloss.NewStyle().Foreground(pGreen).
				Render(fmt.Sprintf("HFS %.2f", s.HfsScore))
			left += lipgloss.NewStyle().Foreground(pTX2).Render("  ")
			left += lipgloss.NewStyle().Foreground(pTX1).Render(s.TaskID)
			if s.TaskGoal != "" {
				g := s.TaskGoal
				if len(g) > 42 {
					g = g[:39] + "…"
				}
				left += lipgloss.NewStyle().Foreground(pTX2).Render("  " + g)
			}
		} else {
			left += lipgloss.NewStyle().Foreground(pTX1).Render("daemon ready")
			left += lipgloss.NewStyle().Foreground(pTX2).Render("  no session")
		}
	} else {
		dot := lipgloss.NewStyle().Foreground(pTX2).Render("○")
		left = dot + " "
		left += lipgloss.NewStyle().Foreground(pTX2).Render("disconnected")
	}

	right := lipgloss.NewStyle().Foreground(pTX3).Render("relay")

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 0 {
		pad = 0
	}

	line := " " + left + strings.Repeat(" ", pad) + right + " "
	return lipgloss.NewStyle().
		Background(pS1).
		Width(m.width).
		Render(line)
}

// ── input ─────────────────────────────────────────────────────────────────────

func (m tuiModel) renderInput() string {
	prompt := lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render(">")

	inputStr := string(m.input)
	var displayed string
	cursorStyle := lipgloss.NewStyle().Background(pTX1).Foreground(pBase)

	if m.cursor >= len(m.input) {
		displayed = lipgloss.NewStyle().Foreground(pTX0).Render(inputStr) +
			cursorStyle.Render(" ")
	} else {
		before := inputStr[:m.cursor]
		at := string(m.input[m.cursor])
		after := inputStr[m.cursor+1:]
		displayed = lipgloss.NewStyle().Foreground(pTX0).Render(before) +
			cursorStyle.Foreground(pBase).Render(at) +
			lipgloss.NewStyle().Foreground(pTX0).Render(after)
	}

	return lipgloss.NewStyle().
		Background(pS1).
		Width(m.width).
		Padding(0, 1).
		Render(prompt + "  " + displayed)
}

// ── hint ──────────────────────────────────────────────────────────────────────

func (m tuiModel) renderHint() string {
	var hint string
	if m.showPopup {
		hint = "Tab / ↑↓ navigate  ·  Enter select  ·  Esc close"
	} else if strings.HasPrefix(string(m.input), "/") {
		hint = "Tab to autocomplete  ·  ↑↓ in popup"
	} else if len(m.lines) > m.logHeight() {
		hint = "PgUp/PgDn scroll  ·  Ctrl+L clear  ·  / for commands"
	} else {
		hint = "Type / for commands  ·  bare text = /run shortcut  ·  Ctrl+C quit"
	}
	return "  " + lipgloss.NewStyle().Foreground(pTX3).Render(hint)
}

// ─── style helpers ────────────────────────────────────────────────────────────

func tuiTagStyle(tag string) (fg, bg lipgloss.Color, label string) {
	switch tag {
	case "tool", "tool_use":
		return pTX1, lipgloss.Color("#1c1c1c"), "tool   "
	case "result":
		return pGreen, lipgloss.Color("#0d1a12"), "result "
	case "quota":
		return pYellow, lipgloss.Color("#1a1608"), "quota  "
	case "handoff":
		return pAccent, lipgloss.Color("#1a1209"), "handoff"
	case "text":
		return pBlue, lipgloss.Color("#0d1420"), "text   "
	case "wait", "waiting":
		return pTX2, pBase, "wait   "
	case "error":
		return pRed, lipgloss.Color("#1a0808"), "error  "
	default:
		return pTX2, lipgloss.Color("#1c1c1c"), "system "
	}
}

func tuiMsgColor(tag string) lipgloss.Color {
	switch tag {
	case "result":
		return pTX0
	case "quota":
		return pYellow
	case "handoff":
		return pAccent
	case "error":
		return pRed
	default:
		return pTX1
	}
}

func tuiFmtTok(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// ─── cobra command ────────────────────────────────────────────────────────────

func cmdTUI() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Aliases: []string{"interactive", "shell"},
		Short:   "Interactive relay TUI with slash commands and autocomplete",
		Long: `Opens an interactive terminal UI modelled after gemini-cli.

Type / to see commands with popup.  Tab to autocomplete.  ↑↓ in popup.
Enter to select or execute.  Esc to close popup.  Ctrl+C to quit.

Slash commands: /run  /init  /daemon  /handoff  /status  /providers
                /enable  /disable  /audit  /graph  /open  /help`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
	}
}

func runTUI() error {
	p := tea.NewProgram(
		newTUIModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
