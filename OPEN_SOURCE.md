# Open source credits

Relay is built on the work of many. This file enumerates every direct dependency, what it does for us, and its licence. Indirect deps (transitive) are tracked in `go.sum` and `Cargo.lock`.

## Bundled provider tooling

We integrate with these but do not redistribute them. Users install separately.

| Tool | What | Licence | Repo |
|---|---|---|---|
| Claude Code | Anthropic's agentic CLI | Anthropic terms | [anthropics/claude-code](https://github.com/anthropics/claude-code) |
| Codex CLI | OpenAI's coding agent CLI | OpenAI terms | [openai/codex](https://github.com/openai/codex) |
| Ollama | Local LLM serving | MIT | [ollama/ollama](https://github.com/ollama/ollama) |
| GitHub CLI | `gh` + Copilot extension | MIT | [cli/cli](https://github.com/cli/cli) |
| OpenCode | Open-source AI coding assistant | Apache-2.0 | [opencode-ai/opencode](https://github.com/opencode-ai/opencode) |
| Continue | VS Code extension | Apache-2.0 | [continuedev/continue](https://github.com/continuedev/continue) |
| Cline | Autonomous coding agent (VS Code) | MIT | [cline/cline](https://github.com/cline/cline) |

## Go direct dependencies

`packages/daemon-go/go.mod`:

| Module | Use | Licence |
|---|---|---|
| [github.com/spf13/cobra](https://github.com/spf13/cobra) | CLI framework | Apache-2.0 |
| [github.com/spf13/pflag](https://github.com/spf13/pflag) | Cobra flag parsing | BSD-3-Clause |
| [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework | MIT |
| [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | TUI styling | MIT |
| [github.com/charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | TUI components | MIT |
| [github.com/mattn/go-isatty](https://github.com/mattn/go-isatty) | Terminal detection | MIT |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | CGo-free SQLite for the graph store | BSD-3-Clause |
| [github.com/google/uuid](https://github.com/google/uuid) | UUID generation | BSD-3-Clause |
| [github.com/hashicorp/golang-lru/v2](https://github.com/hashicorp/golang-lru) | LRU cache | MPL-2.0 |
| [github.com/kbinani/screenshot](https://github.com/kbinani/screenshot) | Cross-platform screen capture for vision | MIT |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | Platform-specific syscalls | BSD-3-Clause |

Standard library: `net/http`, `encoding/json`, `crypto/{sha256,hmac,rand}`, `os/exec`, `regexp`, etc.

## Rust direct dependencies

`packages/ui/Cargo.toml`:

| Crate | Use | Licence |
|---|---|---|
| [eframe](https://crates.io/crates/eframe) | egui framework (window + render loop) | Apache-2.0 OR MIT |
| [egui](https://crates.io/crates/egui) | Immediate-mode GUI | Apache-2.0 OR MIT |
| [egui_extras](https://crates.io/crates/egui_extras) | Extras (loaders) | Apache-2.0 OR MIT |
| [serde](https://crates.io/crates/serde) | Serialization | Apache-2.0 OR MIT |
| [serde_json](https://crates.io/crates/serde_json) | JSON | Apache-2.0 OR MIT |
| [ureq](https://crates.io/crates/ureq) | HTTP client | Apache-2.0 OR MIT |
| [image](https://crates.io/crates/image) | Logo PNG decode | Apache-2.0 OR MIT |
| [rfd](https://crates.io/crates/rfd) | Native file/folder dialogs | MIT |

## Design assets

| Asset | Source | Licence |
|---|---|---|
| Inter font | [rsms/inter](https://github.com/rsms/inter) | OFL-1.1 |
| JetBrains Mono font | [JetBrains/JetBrainsMono](https://github.com/JetBrains/JetBrainsMono) | OFL-1.1 |
| Provider logos in docs | respective vendors | their terms |

## Reference docs we read

- [Ollama API docs](https://github.com/ollama/ollama/tree/main/docs)
- [Anthropic Claude API docs](https://docs.anthropic.com)
- [OpenAI Codex docs](https://github.com/openai/codex)
- [VS Code extension API](https://code.visualstudio.com/api)

## License compatibility

All direct dependencies are permissively licensed (MIT, Apache-2.0, BSD, MPL-2.0, OFL-1.1). Compatible with our Apache-2.0 release.

Full licence texts are vendored on first release into `vendor/licences/` for offline reference.
