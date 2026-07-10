// internal/detect/signatures.go — the registry of known agent fingerprints.
//
// Each signature knows how to recognise its agent in the OS process list and
// (where the format is implemented) where that agent stores session logs. The
// process regex is matched against the full command line, because most of
// these CLIs run as `node <path>/cli.js …` — the binary name alone is `node`.

package detect

import (
	"regexp"
	"strings"
	"time"
)

// signature fingerprints one provider.
type signature struct {
	Provider        string
	DisplayName     string
	Surface         string         // default surface when found via process
	cmdRe           *regexp.Regexp // matched against the full lowercased command line
	scanTranscripts func(home string, maxAge time.Duration) []SessionIntel
}

func (s signature) matchProcess(p procInfo) bool {
	if s.cmdRe == nil {
		return false
	}
	hay := strings.ToLower(p.Cmdline)
	if hay == "" {
		hay = strings.ToLower(p.Name)
	}
	if !s.cmdRe.MatchString(hay) {
		return false
	}
	// Never flag Relay itself or an editor host masquerading via a matched path.
	if strings.Contains(hay, "relay daemon") || strings.Contains(hay, "relay run") || strings.Contains(hay, "relay detect") {
		return false
	}
	return true
}

// signatures is the compiled registry. Order is irrelevant; first match wins
// per process. Patterns target the install markers (npm package paths, known
// binary names) to keep false positives low.
var signatures = []signature{
	{
		Provider: "claude", DisplayName: "Claude Code", Surface: "terminal",
		cmdRe:           regexp.MustCompile(`(?i)claude[-_ ]?code|@anthropic-ai[\\/]claude|[\\/]claude(\.cmd|\.exe|\.js)?(\s|"|$)`),
		scanTranscripts: scanClaudeTranscripts,
	},
	{
		Provider: "codex", DisplayName: "OpenAI Codex", Surface: "terminal",
		cmdRe:           regexp.MustCompile(`(?i)@openai[\\/]codex|[\\/]codex(\.cmd|\.exe)?(\s|"|$)`),
		scanTranscripts: scanCodexTranscripts,
	},
	{
		Provider: "antigravity", DisplayName: "Antigravity", Surface: "ide",
		cmdRe:           regexp.MustCompile(`(?i)antigravity|[\\/]agy(\.exe)?(\s|"|$)`),
		scanTranscripts: scanAntigravityTrajectories,
	},
	{
		Provider: "opencode", DisplayName: "OpenCode", Surface: "terminal",
		cmdRe: regexp.MustCompile(`(?i)opencode(-ai)?`),
	},
	{
		Provider: "ollama", DisplayName: "Ollama", Surface: "desktop",
		cmdRe: regexp.MustCompile(`(?i)[\\/ ]ollama(\.exe)?(\s|"|$|serve|run)`),
	},
	{
		Provider: "copilot", DisplayName: "GitHub Copilot", Surface: "ide",
		cmdRe:           regexp.MustCompile(`(?i)gh-copilot|copilot-language-server|[\\/]copilot(\.exe)?(\s|"|$)`),
		scanTranscripts: scanCopilot,
	},
	{
		Provider: "gemini", DisplayName: "Gemini CLI", Surface: "terminal",
		cmdRe: regexp.MustCompile(`(?i)@google[\\/]gemini-cli|[\\/]gemini(\.cmd|\.exe)?(\s|"|$)`),
	},
	{
		Provider: "aider", DisplayName: "Aider", Surface: "terminal",
		cmdRe: regexp.MustCompile(`(?i)[\\/ ]aider(\.exe)?(\s|"|$)`),
	},
	{
		Provider: "cursor", DisplayName: "Cursor Agent", Surface: "ide",
		cmdRe:           regexp.MustCompile(`(?i)cursor-agent|cursor[\\/]agent`),
		scanTranscripts: scanCursorTranscripts,
	},
	// IDE extensions: no own process, detected purely from their on-disk stores.
	{
		Provider: "cline", DisplayName: "Cline", Surface: "ide",
		scanTranscripts: scanClineTasks,
	},
	{
		Provider: "continue", DisplayName: "Continue", Surface: "ide",
		scanTranscripts: scanContinueSessions,
	},
}
