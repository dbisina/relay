# scripts/dev.ps1 — dev loop with hot reload for Windows.

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

Write-Host "Starting daemon + UI with hot reload. Ctrl+C to stop both." -ForegroundColor Cyan

$daemonJob = Start-Job -ScriptBlock {
    param($root)
    Set-Location "$root/packages/daemon-go"
    if (Get-Command air -ErrorAction SilentlyContinue) {
        & air -c "$root/.air.toml" -- daemon 2>&1 | ForEach-Object { "[daemon] $_" }
    } else {
        "[daemon] 'air' not found. Hot-reloading disabled. Falling back to 'go run'."
        & go run ./cmd/relay daemon 2>&1 | ForEach-Object { "[daemon] $_" }
    }
} -ArgumentList $RepoRoot

$uiJob = Start-Job -ScriptBlock {
    param($root)
    Set-Location "$root/packages/ui"
    $cargoCmd = if (Get-Command cargo -ErrorAction SilentlyContinue) { "cargo" } elseif (Test-Path "$env:USERPROFILE\.cargo\bin\cargo.exe") { "$env:USERPROFILE\.cargo\bin\cargo.exe" } else { $null }
    
    if ($cargoCmd) {
        if (Get-Command cargo-watch -ErrorAction SilentlyContinue) {
            & $cargoCmd watch -x 'run --quiet' 2>&1 | ForEach-Object { "[ui]     $_" }
        } else {
            "[ui]     'cargo-watch' not found. Hot-reloading disabled. Falling back to 'cargo run'."
            & $cargoCmd run --quiet 2>&1 | ForEach-Object { "[ui]     $_" }
        }
    } else {
        "[ui]     'cargo' not found. Cannot start UI."
    }
} -ArgumentList $RepoRoot

try {
    while ($true) {
        Receive-Job $daemonJob, $uiJob
        Start-Sleep -Milliseconds 200
    }
} finally {
    Stop-Job  $daemonJob, $uiJob -ErrorAction SilentlyContinue
    Remove-Job $daemonJob, $uiJob -ErrorAction SilentlyContinue
    Write-Host "`nStopped." -ForegroundColor Cyan
}
