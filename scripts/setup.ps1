# scripts/setup.ps1 — one-shot contributor setup for Windows PowerShell.
# Re-run is safe; everything is idempotent.

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

function Bold($msg) { Write-Host "`n$msg" -ForegroundColor White -BackgroundColor DarkGray }
function Note($msg) { Write-Host "  $msg" }
function Ok  ($msg) { Write-Host "  + $msg" -ForegroundColor Green }
function Err ($msg) { Write-Host "  ! $msg" -ForegroundColor Red }

Bold "Relay setup"

# --- Required tools ----------------------------------------------------------

# Refresh PATH from the registry so tools installed in this run become usable
# without restarting the shell.
function Update-SessionPath {
    $m = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $u = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = (@($m, $u) | Where-Object { $_ }) -join ";"
}

# Self-healing dependency check: if a tool is missing, try to install it via
# winget, refresh PATH, and re-check. Only fail if auto-install is impossible.
function Ensure($name, $wingetId, [scriptblock]$post = $null) {
    if (Get-Command $name -ErrorAction SilentlyContinue) { Ok "$name found"; return }
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        Err "$name not found and winget is unavailable. Install $name and re-run."
        exit 1
    }
    Note "$name not found — installing $wingetId via winget..."
    try {
        winget install --id $wingetId -e --source winget `
            --accept-package-agreements --accept-source-agreements `
            --disable-interactivity | Out-Null
    } catch {
        Err "winget install of $wingetId failed: $_"
    }
    Update-SessionPath
    if ($post) { & $post }
    Update-SessionPath
    if (Get-Command $name -ErrorAction SilentlyContinue) {
        Ok "$name installed"
    } else {
        Err "Could not auto-install $name. Install it manually and re-run (a new terminal may be needed for PATH)."
        exit 1
    }
}

Bold "Checking toolchain"
Ensure "git" "Git.Git"
Ensure "go"  "GoLang.Go"
# cargo ships via rustup; install the toolchain manager then a stable toolchain.
Ensure "cargo" "Rustlang.Rustup" {
    if (Get-Command rustup -ErrorAction SilentlyContinue) {
        rustup default stable | Out-Null
        $cargoBin = Join-Path $env:USERPROFILE ".cargo\bin"
        if (Test-Path $cargoBin) { $env:Path = "$cargoBin;$env:Path" }
    }
}

if (Get-Command "node" -ErrorAction SilentlyContinue) {
    Ok "node $((node --version))"
} else {
    Note "node not found — needed later for Claude/Codex CLI installs."
}

# --- Go side -----------------------------------------------------------------

Bold "Pulling Go modules"
Push-Location packages/daemon-go
go mod download
Pop-Location
Ok "go.mod resolved"

Bold "Building Go daemon"
New-Item -Force -ItemType Directory bin | Out-Null
Push-Location packages/daemon-go
go build -o "$RepoRoot/bin/relay.exe" ./cmd/relay/...
Pop-Location
Ok "bin/relay.exe"

# --- Rust side ---------------------------------------------------------------

Bold "Fetching Rust crates"
Push-Location packages/ui
cargo fetch
Pop-Location
Ok "Cargo.lock resolved"

Bold "Building Rust UI (release)"
Push-Location packages/ui
cargo build --release
Pop-Location

$ui_src = Join-Path $RepoRoot "target/release/relay-ui.exe"
if (-not (Test-Path $ui_src)) {
    $ui_src = Join-Path $RepoRoot "packages/ui/target/release/relay-ui.exe"
}
Copy-Item -Force $ui_src "$RepoRoot/bin/relay-ui.exe"
Ok "bin/relay-ui.exe"

# --- Smoke test --------------------------------------------------------------

Bold "Smoke test"
$daemon = Start-Process -FilePath "$RepoRoot/bin/relay.exe" `
    -ArgumentList "daemon","--port","4748" `
    -WindowStyle Hidden -PassThru

Start-Sleep -Milliseconds 1500

try {
    $r = Invoke-WebRequest -Uri "http://127.0.0.1:4748/api/health" -TimeoutSec 3 -UseBasicParsing
    if ($r.StatusCode -eq 200) { Ok "/api/health reachable" } else { throw "bad status" }
} catch {
    Err "daemon failed to start: $_"
    Stop-Process -Id $daemon.Id -Force -ErrorAction SilentlyContinue
    exit 1
} finally {
    Stop-Process -Id $daemon.Id -Force -ErrorAction SilentlyContinue
}

# --- Optional dev tools ------------------------------------------------------

Bold "Optional dev tools"
if (-not (Get-Command "air" -ErrorAction SilentlyContinue)) {
    Note "Installing air (Go hot-reload)..."
    go install "github.com/air-verse/air@latest"
    Ok "air"
}
$cw = cargo install --list 2>$null | Select-String "^cargo-watch "
if (-not $cw) {
    Note "Installing cargo-watch (Rust hot-reload)..."
    cargo install cargo-watch
    Ok "cargo-watch"
}

Bold "Done."
Note "Next:"
Note "  ./scripts/dev.ps1            # dev loop with hot reload"
Note "  ./bin/relay.exe init         # initialise a project"
Note "  ./bin/relay-ui.exe           # launch the desktop app"
