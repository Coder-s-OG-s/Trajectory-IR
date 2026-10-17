# One-step local attach for Claude Code in this worktree.
# Builds the verify CLI and trajir-mcp, writes project-local hooks, smoke-verifies one pack.
# Does not edit the user profile Claude config. Does not start a console. Does not git commit.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host "== One-step Claude attach =="
Write-Host "  root: $root"

$prev = $ErrorActionPreference
$ErrorActionPreference = "Continue"
Push-Location (Join-Path $root "go")
& go build -o trajir.exe ./cmd/trajir
if ($LASTEXITCODE -ne 0) { Pop-Location; $ErrorActionPreference = $prev; throw "go build trajir failed" }
& go build -o trajir-mcp.exe ./cmd/trajir-mcp
if ($LASTEXITCODE -ne 0) { Pop-Location; $ErrorActionPreference = $prev; throw "go build trajir-mcp failed" }
Pop-Location
$ErrorActionPreference = $prev

$exe = Join-Path $root "go\trajir.exe"
$mcp = Join-Path $root "go\trajir-mcp.exe"
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "missing $exe" }
if (-not (Test-Path -LiteralPath $mcp -PathType Leaf)) { throw "missing $mcp" }
Write-Host "  OK $exe"
Write-Host "  OK $mcp"

$py = $env:TRAJIR_PYTHON
if (-not $py) {
  $repoVenv = Join-Path $root ".venv\Scripts\python.exe"
  if (Test-Path -LiteralPath $repoVenv) { $py = $repoVenv }
}
if (-not $py) { $py = "python" }

& $py (Join-Path $root "scripts\write_claude_local_config.py")
if ($LASTEXITCODE -ne 0) { throw "config write failed" }

& $py (Join-Path $root "scripts\local_one_step_smoke.py")
if ($LASTEXITCODE -ne 0) { throw "smoke verify failed" }

Write-Host ""
Write-Host "Attached inside this worktree only."
Write-Host "  Hooks: .claude\settings.local.json"
Write-Host "  MCP:   .mcp.json"
Write-Host "  Log:   .local-evidence\claude-agent"
Write-Host "  Verify a pack with: go\trajir.exe verify <pack.tir>"
Write-Host "The user profile at $env:USERPROFILE\.claude was not changed."
Write-Host "Console stays on 127.0.0.1:8787. This script does not start it."
Write-Host "INSTALL OK"
