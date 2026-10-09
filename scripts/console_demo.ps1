$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($env:TRAJIR_CONSOLE_DATA)) {
    $env:TRAJIR_CONSOLE_DATA = Join-Path $root ".console-data"
}
$dir = Join-Path $env:TRAJIR_CONSOLE_DATA "trajectories"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$src = Join-Path $root "go/trajir/console/testdata/console_demo.ndjson"
Copy-Item $src (Join-Path $dir "console-demo.ndjson") -Force
$pkg = Join-Path $env:TRAJIR_CONSOLE_DATA "packages/console-demo"
New-Item -ItemType Directory -Force -Path $pkg | Out-Null
Copy-Item (Join-Path $root "testdata/sample_thin.tir") (Join-Path $pkg "demo.tir") -Force
Write-Output "TRAJIR_CONSOLE_DATA=$env:TRAJIR_CONSOLE_DATA"
Write-Output "http://127.0.0.1:8787/?id=console-demo"
Write-Output "cd go; go run ./cmd/trajir-console -addr 127.0.0.1:8787"
