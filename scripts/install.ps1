# scripts/install.ps1 — install Relay from GitHub Releases (Windows).
# Usage: irm https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.ps1 | iex
#
# Installs both relay.exe (daemon + CLI + TUI) and relay-ui.exe (desktop app).

$ErrorActionPreference = "Stop"

$Repo = "dbisina/relay"
$InstallDir = Join-Path $env:LOCALAPPDATA "relay"

function Bold($msg) { Write-Host "`n$msg" -ForegroundColor White }
function Green($msg) { Write-Host "  $msg" -ForegroundColor Green }
function Red($msg) { Write-Host "  $msg" -ForegroundColor Red }

# ── Find latest release ───────────────────────────────────────────────────────

function NoReleaseHelp {
    Red "No release found for $Repo."
    Red "Either no releases have been published yet, or the GitHub API request failed (rate limit / network)."
    Bold "Build from source instead:"
    Bold "  git clone https://github.com/dbisina/relay; cd relay; ./scripts/setup.ps1"
    Bold "Or pin a specific version and re-run:"
    Bold "  `$env:RELAY_VERSION = 'v0.1.0'"
}

# RELAY_VERSION pins a version and skips the GitHub API query entirely.
if ($env:RELAY_VERSION) {
    $Version = $env:RELAY_VERSION
} else {
    try {
        $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $release.tag_name
    } catch {
        NoReleaseHelp
        exit 1
    }
}

if (-not $Version) {
    NoReleaseHelp
    exit 1
}

$Target = "windows-amd64"

Bold ""
Bold "  Relay installer"
Bold "  Version:  $Version"
Bold "  Platform: $Target"
Bold ""

$url = "https://github.com/$Repo/releases/download/$Version/relay-$Version-$Target.zip"
$checksumsUrl = "https://github.com/$Repo/releases/download/$Version/SHA256SUMS"

$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "relay-install-$(Get-Random)"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

try {
    Bold "Downloading..."
    Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmpDir "relay.zip") -UseBasicParsing
    Invoke-WebRequest -Uri $checksumsUrl -OutFile (Join-Path $tmpDir "SHA256SUMS") -UseBasicParsing

    Bold "Verifying checksum..."
    $expectedLine = (Get-Content (Join-Path $tmpDir "SHA256SUMS")) | Where-Object { $_ -match "relay-$Version-$Target.zip" }
    $expectedHash = ($expectedLine -split "\s+")[0]
    $actualHash = (Get-FileHash (Join-Path $tmpDir "relay.zip") -Algorithm SHA256).Hash.ToLower()

    if ($expectedHash -ne $actualHash) {
        Red "Checksum mismatch! Expected: $expectedHash Got: $actualHash"
        exit 1
    }
    Green "Checksum verified"

    Bold "Extracting..."
    Expand-Archive -Path (Join-Path $tmpDir "relay.zip") -DestinationPath $tmpDir -Force

    Bold "Installing to $InstallDir..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item (Join-Path $tmpDir "relay.exe") $InstallDir -Force
    Copy-Item (Join-Path $tmpDir "relay-ui.exe") $InstallDir -Force

    Green "relay.exe installed to $InstallDir"
    Green "relay-ui.exe installed to $InstallDir"

    # Add to PATH if not already there
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        Bold "Adding $InstallDir to PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        $env:Path = "$env:Path;$InstallDir"
        Green "Added to user PATH (restart your terminal for effect)"
    }

    Bold ""
    Bold "  Get started:"
    Bold "    cd your-project"
    Bold "    relay init"
    Bold '    relay run "your task here"'
    Bold ""
    Bold "  Or launch the desktop app:  relay-ui"
    Bold "  Docs: https://github.com/$Repo#readme"
    Bold ""
} finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
