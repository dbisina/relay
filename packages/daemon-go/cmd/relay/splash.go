// splash.go — branded TUI banner.
//
// Shown on TUI start + via /banner slash command. Block-letter RELAY wordmark
// with the orange "bridge" stripe matching the desktop app logomark.

package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Block letter art — one row per line. The middle row is the accent stripe.
var relayBlock = []string{
	`  ██████╗ ███████╗██╗      █████╗ ██╗   ██╗`,
	`  ██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝`,
	`  ██████╔╝█████╗  ██║     ███████║ ╚████╔╝ `,
	`  ██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝  `,
	`  ██║  ██║███████╗███████╗██║  ██║   ██║   `,
	`  ╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝   `,
}

// renderSplash returns the styled banner — block letters, then the >—< logomark,
// then a 1-line tagline. Stitched together for emission as TUI log lines.
func renderSplash(width int) []logLine {
	if width < 50 {
		// Compact form for narrow terminals
		return renderCompactSplash()
	}

	// Block letter coloring: white at top, accent across the middle two rows,
	// dim at the bottom. Reads as the wordmark with an orange brand stripe.
	top := lipgloss.NewStyle().Foreground(pTX0)
	mid := lipgloss.NewStyle().Foreground(pAccent).Bold(true)
	bot := lipgloss.NewStyle().Foreground(pTX2)

	rows := []string{
		top.Render(relayBlock[0]),
		top.Render(relayBlock[1]),
		mid.Render(relayBlock[2]),
		mid.Render(relayBlock[3]),
		bot.Render(relayBlock[4]),
		bot.Render(relayBlock[5]),
	}

	// Logomark line: > ──── < with orange bridge in the middle
	logoLeft := lipgloss.NewStyle().Foreground(pTX0).Bold(true).Render(">═")
	logoBr := lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render("══════")
	logoRight := lipgloss.NewStyle().Foreground(pTX1).Bold(true).Render("═<")
	logoLine := "  " + logoLeft + logoBr + logoRight + "  " +
		lipgloss.NewStyle().Foreground(pTX3).Italic(true).Render(
			"vendor-neutral agent orchestrator",
		)

	hint := "  " + lipgloss.NewStyle().Foreground(pTX2).Render(
		"v0.3.0  ·  type ",
	) + lipgloss.NewStyle().Foreground(pAccent).Render("/") +
		lipgloss.NewStyle().Foreground(pTX2).Render(
			" for commands  ·  ",
		) + lipgloss.NewStyle().Foreground(pAccent).Render("Tab") +
		lipgloss.NewStyle().Foreground(pTX2).Render(
			" autocomplete  ·  ",
		) + lipgloss.NewStyle().Foreground(pAccent).Render("Ctrl+C") +
		lipgloss.NewStyle().Foreground(pTX2).Render(" quit")

	lines := []logLine{}
	lines = append(lines, logLine{kind: "info", tag: "relay", msg: ""}) // top blank
	for _, r := range rows {
		lines = append(lines, logLine{kind: "info", tag: "relay", msg: r})
	}
	lines = append(lines, logLine{kind: "info", tag: "relay", msg: ""})
	lines = append(lines, logLine{kind: "info", tag: "relay", msg: logoLine})
	lines = append(lines, logLine{kind: "info", tag: "relay", msg: hint})
	lines = append(lines, logLine{kind: "info", tag: "relay", msg: ""})
	return lines
}

// renderCompactSplash — single-line variant for narrow terminals.
func renderCompactSplash() []logLine {
	left := lipgloss.NewStyle().Foreground(pTX0).Bold(true).Render(">═")
	br := lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render("════")
	right := lipgloss.NewStyle().Foreground(pTX1).Bold(true).Render("═<")
	logo := "  " + left + br + right + " " +
		lipgloss.NewStyle().Foreground(pAccent).Bold(true).Render("relay") + " " +
		lipgloss.NewStyle().Foreground(pTX2).Render("· agent orchestrator")
	hint := "  " + lipgloss.NewStyle().Foreground(pTX3).Render(
		"v0.3.0  /  for commands  ·  Ctrl+C quit",
	)
	return []logLine{
		{kind: "info", tag: "relay", msg: logo},
		{kind: "info", tag: "relay", msg: hint},
		{kind: "info", tag: "relay", msg: ""},
	}
}

// splashString — non-styled banner for `relay --help` / non-TTY output.
func splashString() string {
	return strings.Join(relayBlock, "\n") + "\n\n" +
		"  vendor-neutral AI coding agent orchestrator\n" +
		"  claude · codex · antigravity · opencode · ollama · copilot · continue · cline\n"
}
