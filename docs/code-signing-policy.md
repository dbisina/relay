# Code signing policy

Relay's release binaries and installers are code signed so that Windows and
macOS can verify they came from this project and were not tampered with after
they were built.

Free code signing provided by [SignPath.io](https://signpath.io), certificate by
[SignPath Foundation](https://signpath.org).

## Team roles

Relay is maintained in the open at
[github.com/dbisina/relay](https://github.com/dbisina/relay).

- **Committers and reviewers:** [repository maintainers](https://github.com/dbisina/relay/graphs/contributors).
  Every change that does not come from a maintainer arrives as a pull request
  and is reviewed before it is merged.
- **Approvers:** [repository owner](https://github.com/dbisina). Each signing
  request for a release is approved manually. No release is signed
  automatically without that approval.

## What gets signed

Only artefacts built by this project's own release pipeline
([`.github/workflows/release.yml`](../.github/workflows/release.yml)) from
source in this repository:

- the `relay` daemon and CLI binary,
- the Electron desktop installers (`.exe`, `.dmg`, `.AppImage`),
- the legacy egui desktop binary (`relay-ui`).

Relay bundles no proprietary components. Third party open source libraries are
listed in [OPEN_SOURCE.md](../OPEN_SOURCE.md).

## Privacy policy

Relay runs locally. The daemon binds only to `127.0.0.1` and the desktop app
talks to it over that loopback interface. Relay itself will not transfer any
information to other networked systems unless specifically requested by the
user or the person installing or operating it.

Relay's purpose is to orchestrate AI coding agents that you have already
installed and signed into, for example Claude Code, OpenAI Codex, GitHub
Copilot, or a local Ollama model. When you run a task, those agents send your
prompts and code to their own providers under their own privacy policies and
your own accounts. Relay does not add a destination of its own, and it does not
collect telemetry or usage analytics.

Before any text crosses a boundary it passes through a secret redactor
(`internal/redact`), which scrubs common credential patterns such as API keys,
tokens, JWTs, and PEM blocks.

## Verifying a download

Every release publishes a `SHA256SUMS` file listing the checksum of each
artefact. On any platform:

```bash
sha256sum --check SHA256SUMS
```

On Windows you can compare a single file with:

```powershell
Get-FileHash .\Relay-<version>-windows-x64-setup.exe -Algorithm SHA256
```

## Reporting a problem

If you believe a signed Relay artefact violates the SignPath Foundation code of
conduct, please open an issue at
[github.com/dbisina/relay/issues](https://github.com/dbisina/relay/issues) and
contact `support@signpath.io` with details.
