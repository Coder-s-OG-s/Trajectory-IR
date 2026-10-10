# Install the local Evidence Sidecar slice (TURNING_POINT wrap + verify).
# No GitHub push. Run from anywhere; resolves worktree root via this script.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host "== Evidence Sidecar install =="
Write-Host "  root: $root"

Write-Host "== build trajir verify CLI =="
Push-Location (Join-Path $root "go")
go build -o trajir.exe ./cmd/trajir
Pop-Location
$exe = Join-Path $root "go\trajir.exe"
if (-not (Test-Path $exe)) { throw "missing $exe" }
Write-Host "  OK $exe"
& $exe verify -h 2>&1 | Select-Object -First 5

Write-Host ""
Write-Host "Next:"
Write-Host "  1. python integrations/langgraph/demo_stub.py"
Write-Host "  2. go\trajir.exe verify .local-evidence\langgraph-demo.tir"
Write-Host "  3. Docs: docs\EVIDENCE_SIDECAR.md"
Write-Host "  4. Optional console on 8787 only:"
Write-Host "       `$env:TRAJIR_CONSOLE_DATA = `"$root\.console-data`""
Write-Host "       cd go; go run ./cmd/trajir-console -addr 127.0.0.1:8787"
Write-Host "  5. Optional LangGraph extra: pip install -e `".[langgraph]`""
Write-Host ""
Write-Host "INSTALL OK"
