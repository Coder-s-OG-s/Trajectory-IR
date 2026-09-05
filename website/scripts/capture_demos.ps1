# Capture Go demo stdout into website/fixtures for docs embeds.
# Run from repo root: pwsh website/scripts/capture_demos.ps1

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$Go = Join-Path $Root "go"
$Fix = Join-Path $Root "website\fixtures"
New-Item -ItemType Directory -Force -Path $Fix | Out-Null

Push-Location $Go
try {
  Write-Host "Capturing adoption_host -with-package..."
  $pkg = & go run ./examples/adoption_host -with-package 2>&1 | Out-String
  # Normalize temp paths for stable fixtures
  $pkg = $pkg -replace "(?m)tir:\s+.+$", "tir:          /tmp/adoption-run.tir"
  Set-Content -Encoding utf8 (Join-Path $Fix "adoption_host_package.txt") $pkg.TrimEnd()

  Write-Host "Capturing adoption_host -sandbox..."
  $sb = & go run ./examples/adoption_host -sandbox 2>&1 | Out-String
  Set-Content -Encoding utf8 (Join-Path $Fix "adoption_host_sandbox.txt") $sb.TrimEnd()

  Write-Host "Capturing kill_mid_deploy (crash + resume)..."
  $wd = Join-Path $Go "_demo_capture_site"
  if (Test-Path $wd) { Remove-Item -Recurse -Force $wd }
  New-Item -ItemType Directory -Force -Path $wd | Out-Null

  $firstOut = Join-Path $wd "first.txt"
  $firstErr = Join-Path $wd "first.err"
  $p = Start-Process -FilePath "go" `
    -ArgumentList @("run", "./examples/kill_mid_deploy", "-workdir", $wd, "-crash-during=tool_call") `
    -WorkingDirectory $Go -PassThru -NoNewWindow `
    -RedirectStandardOutput $firstOut -RedirectStandardError $firstErr

  $deadline = (Get-Date).AddSeconds(90)
  while ((Get-Date) -lt $deadline) {
    if (Test-Path (Join-Path $wd "tool_started.marker")) { break }
    Start-Sleep -Milliseconds 400
  }
  if (-not (Test-Path (Join-Path $wd "tool_started.marker"))) {
    throw "Timed out waiting for TOOL_CALL start marker"
  }

  Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 1

  $resume = & go run ./examples/kill_mid_deploy -workdir $wd -resume 2>&1 | Out-String
  $first = (Get-Content $firstOut -Raw)

  $first = $first -replace [regex]::Escape($wd), "./kill_mid_deploy-data"
  $resume = $resume -replace [regex]::Escape($wd), "./kill_mid_deploy-data"

  Set-Content -Encoding utf8 (Join-Path $Fix "kill_mid_deploy_first.txt") $first.TrimEnd()
  Set-Content -Encoding utf8 (Join-Path $Fix "kill_mid_deploy_resume.txt") $resume.TrimEnd()

  Write-Host "Fixtures written to $Fix"
}
finally {
  Pop-Location
}
