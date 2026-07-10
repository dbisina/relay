# scripts/demo.tape — canonical Relay demo. Renders docs/assets/demo.gif.
#
# Requires VHS (https://github.com/charmbracelet/vhs) with ttyd + ffmpeg on PATH:
#   macOS:   brew install vhs
#   Linux:   see the VHS README (apt: ttyd ffmpeg; vhs via go install or release)
#   Windows: needs ttyd, which is awkward — record under WSL, or use ScreenToGif.
#
# Then, from the repo root, with `relay` on PATH and your agents authed:
#   vhs scripts/demo.tape
#
# Story: detect every AI agent already running, then adopt one onto another
# provider through a signed continuation contract (the handoff protocol).
# Edit the --adopt id below to a real one from your own `relay detect` output.

Output docs/assets/demo.gif

Require relay

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set Theme "Dracula"
Set TypingSpeed 45ms

Hide
Type "clear" Enter
Show

Type "relay --version"
Sleep 600ms
Enter
Sleep 1500ms

Type "# 1. Detect every AI coding agent already running on this machine"
Enter
Type "relay detect --since-hours 48"
Sleep 600ms
Enter
Sleep 4s

Type "# 2. Lift one and continue it on another provider — a signed handoff"
Enter
Type "relay detect --adopt claude:REPLACE_WITH_A_REAL_ID --target codex"
Sleep 600ms
Enter
Sleep 4s

Type "# The continuation contract carries intent, plan, tasks, and in-flight code."
Enter
Sleep 2500ms
