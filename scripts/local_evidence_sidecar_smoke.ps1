# Local-only smoke for TURNING_POINT evidence sidecar.
# Never pushes. Run from worktree root: H:\Trajectory-IR-wt-evidence
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$py = if (Test-Path "H:\Trajectory-IR\.venv\Scripts\python.exe") {
    "H:\Trajectory-IR\.venv\Scripts\python.exe"
} else {
    "python"
}
$env:PYTHONPATH = "$root;$root\pkg"

Write-Host "== build trajir verify =="
Push-Location go
go build -o trajir.exe ./cmd/trajir
Pop-Location

Write-Host "== unit: audit =="
Push-Location go
go test ./trajir/audit/... -count=1
Pop-Location

Write-Host "== unit: sidecar + otel =="
& $py -m pytest integrations/langgraph/test_sidecar.py integrations/otel/test_correlate.py -q --tb=line

Write-Host "== integration: evidence sidecar =="
& $py -m pytest test/integration/test_evidence_sidecar.py -q --tb=line
if ($LASTEXITCODE -ne 0) { throw "evidence integration tests failed" }

Write-Host "== stub demo =="
& $py integrations/langgraph/demo_stub.py
if ($LASTEXITCODE -ne 0) { throw "demo_stub.py failed with exit $LASTEXITCODE" }
$pack = Join-Path $root ".local-evidence\langgraph-demo.tir"
if (-not (Test-Path $pack)) { throw "missing $pack" }

Write-Host "== trajir verify (expect OK) =="
& .\go\trajir.exe verify $pack
if ($LASTEXITCODE -ne 0) { throw "verify failed on good pack" }

Write-Host "== trajir verify flipped byte (expect FAIL) =="
$bad = Join-Path $root ".local-evidence\langgraph-demo-flipped.tir"
$bytes = [IO.File]::ReadAllBytes($pack)
$bytes[$bytes.Length / 2] = $bytes[$bytes.Length / 2] -bxor 0xff
[IO.File]::WriteAllBytes($bad, $bytes)
& .\go\trajir.exe verify $bad
if ($LASTEXITCODE -eq 0) { throw "verify should fail on flipped pack" }

Write-Host "== trajir verify --require-signature on unsigned (expect FAIL) =="
& .\go\trajir.exe verify --require-signature $pack
if ($LASTEXITCODE -eq 0) { throw "verify should fail without signature" }

Write-Host ""
Write-Host "SMOKE OK"
Write-Host "  pack: $pack"
Write-Host "  CLI:  $root\go\trajir.exe verify <pack.tir>"
