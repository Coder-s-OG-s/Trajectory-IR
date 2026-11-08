# One-step local attach for Claude Code.
# Builds the verify CLI and trajir-mcp, writes project-local hooks, smoke-verifies one pack.
# Does not edit the user profile Claude config. Does not git commit.
# Starts trajir-console on 127.0.0.1:8787 only when that port is free.
param(
    [string]$Target = "",
    [switch]$SkipConsole
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
# Resolve -Target before Set-Location. After the cd, -Target . is this checkout.
if ($Target) {
    if (-not (Test-Path -LiteralPath $Target -PathType Container)) {
        throw "target directory not found: $Target"
    }
    $Target = (Resolve-Path -LiteralPath $Target).Path
}
Set-Location $root

Write-Host "== One-step Claude attach =="
Write-Host "  root: $root"
if ($Target) { Write-Host "  target: $Target" }

$prev = $ErrorActionPreference
$ErrorActionPreference = "Continue"
Push-Location (Join-Path $root "go")
& go build -o trajir.exe ./cmd/trajir
if ($LASTEXITCODE -ne 0) { Pop-Location; $ErrorActionPreference = $prev; throw "go build trajir failed" }
& go build -o trajir-mcp.exe ./cmd/trajir-mcp
if ($LASTEXITCODE -ne 0) { Pop-Location; $ErrorActionPreference = $prev; throw "go build trajir-mcp failed" }
& go build -o trajir-console.exe ./cmd/trajir-console
if ($LASTEXITCODE -ne 0) { Pop-Location; $ErrorActionPreference = $prev; throw "go build trajir-console failed" }
Pop-Location
$ErrorActionPreference = $prev

$exe = Join-Path $root "go\trajir.exe"
$mcp = Join-Path $root "go\trajir-mcp.exe"
$console = Join-Path $root "go\trajir-console.exe"
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "missing $exe" }
if (-not (Test-Path -LiteralPath $mcp -PathType Leaf)) { throw "missing $mcp" }
if (-not (Test-Path -LiteralPath $console -PathType Leaf)) { throw "missing $console" }
Write-Host "  OK $exe"
Write-Host "  OK $mcp"
Write-Host "  OK $console"

$py = $env:TRAJIR_PYTHON
if (-not $py) {
  $repoVenv = Join-Path $root ".venv\Scripts\python.exe"
  if (Test-Path -LiteralPath $repoVenv) { $py = $repoVenv }
}
if (-not $py) { $py = "python" }

$configArgs = @()
if ($Target) { $configArgs += @("--target", $Target) }
if ($SkipConsole) { $configArgs += "--skip-console" }
& $py (Join-Path $root "scripts\write_claude_local_config.py") @configArgs
if ($LASTEXITCODE -ne 0) { throw "config write failed" }

& $py (Join-Path $root "scripts\local_one_step_smoke.py")
if ($LASTEXITCODE -ne 0) { throw "smoke verify failed" }

Write-Host ""
if ($Target) {
  Write-Host "Attached $Target. Hook scripts stay in this install."
} else {
  Write-Host "Attached inside this worktree only."
}
Write-Host "  Hooks: .claude\settings.local.json"
Write-Host "  MCP:   .mcp.json .cursor\mcp.json .vscode\mcp.json .gemini\settings.json .codex\config.toml"
Write-Host "  Log:   .local-evidence\claude-agent"
Write-Host "  Verify a pack with: go\trajir.exe verify <pack.tir>"
Write-Host ".local-evidence/ is listed in the target .gitignore."
Write-Host "MCP command paths are absolute on this machine."
Write-Host "The user profile at $env:USERPROFILE\.claude was not changed."
Write-Host "Console is 127.0.0.1:8787 only. A listener already on that port is left alone."
Write-Host "INSTALL OK"
