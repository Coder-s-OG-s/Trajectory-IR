# Smoke the live integration matrix against docker-compose.live.yml.
# Usage (from repo root):
#   .\scripts\run_live_matrix.ps1
#   .\scripts\run_live_matrix.ps1 -WithTemporal
#   .\scripts\run_live_matrix.ps1 -UpOnly
param(
    [switch]$WithTemporal,
    [switch]$UpOnly,
    [switch]$SkipUp
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$env:TRAJIR_DATABASE_URL = if ($env:TRAJIR_DATABASE_URL) { $env:TRAJIR_DATABASE_URL } else { "postgresql://trajir:trajir@127.0.0.1:5432/trajir" }
$env:TRAJIR_S3_ENDPOINT_URL = if ($env:TRAJIR_S3_ENDPOINT_URL) { $env:TRAJIR_S3_ENDPOINT_URL } else { "http://127.0.0.1:9000" }
$env:TRAJIR_S3_BUCKET = if ($env:TRAJIR_S3_BUCKET) { $env:TRAJIR_S3_BUCKET } else { "trajir" }
$env:AWS_ACCESS_KEY_ID = if ($env:AWS_ACCESS_KEY_ID) { $env:AWS_ACCESS_KEY_ID } else { "minioadmin" }
$env:AWS_SECRET_ACCESS_KEY = if ($env:AWS_SECRET_ACCESS_KEY) { $env:AWS_SECRET_ACCESS_KEY } else { "minioadmin" }
$env:AWS_DEFAULT_REGION = if ($env:AWS_DEFAULT_REGION) { $env:AWS_DEFAULT_REGION } else { "us-east-1" }
$env:TEMPORAL_HOSTPORT = if ($env:TEMPORAL_HOSTPORT) { $env:TEMPORAL_HOSTPORT } else { "localhost:7233" }

if (-not $SkipUp) {
    Write-Host "==> docker compose up"
    if ($WithTemporal) {
        docker compose -f docker-compose.live.yml up -d
    } else {
        docker compose -f docker-compose.live.yml up -d postgres minio
    }
    if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

    Write-Host "==> wait postgres healthy"
    $ok = $false
    for ($i = 1; $i -le 60; $i++) {
        docker exec trajir-live-postgres pg_isready -U trajir -d trajir 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ok = $true; break }
        Start-Sleep -Seconds 2
    }
    if (-not $ok) { throw "postgres not ready" }

    Write-Host "==> wait minio healthy"
    $ok = $false
    for ($i = 1; $i -le 60; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9000/minio/health/live" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ok = $true; break }
        } catch {}
        Start-Sleep -Seconds 2
    }
    if (-not $ok) { throw "minio not ready" }
}

if ($UpOnly) {
    Write-Host "stack is up; exiting (-UpOnly)"
    exit 0
}

Write-Host "==> ensure MinIO bucket exists"
python -c @"
import os
from drivers.s3.cas import build_s3_client_from_env
bucket = os.environ['TRAJIR_S3_BUCKET']
client = build_s3_client_from_env()
try:
    client.head_bucket(Bucket=bucket)
    print('bucket exists:', bucket)
except Exception:
    client.create_bucket(Bucket=bucket)
    print('bucket created:', bucket)
"@
if ($LASTEXITCODE -ne 0) { throw "bucket ensure failed (pip install -e '.[s3]' ?)" }

Write-Host "==> Python live Postgres"
python -m pytest test/integration/test_postgres_live.py -q
if ($LASTEXITCODE -ne 0) { throw "postgres live failed" }

Write-Host "==> Python live MinIO"
python -m pytest test/integration/test_s3_minio_live.py -q
if ($LASTEXITCODE -ne 0) { throw "minio live failed" }

Write-Host "==> Go live Postgres"
Push-Location go
go test ./trajir/postgres/... -count=1
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "go postgres live failed" }
Write-Host "==> Go live S3 CAS"
go test ./trajir/cas -run TestLiveS3StoreFromEnvRoundTrip -count=1
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "go s3 live failed" }
if ($WithTemporal) {
    Write-Host "==> Go Temporal integration"
    go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
    if ($LASTEXITCODE -ne 0) { Pop-Location; throw "temporal live failed" }
} else {
    Write-Host "==> skip Temporal (pass -WithTemporal to enable)"
}
Pop-Location

Write-Host "OK: live matrix passed"
