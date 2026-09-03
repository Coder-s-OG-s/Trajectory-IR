#!/usr/bin/env bash
# Smoke the live integration matrix against docker-compose.live.yml.
# Usage (from repo root):
#   ./scripts/run_live_matrix.sh
#   ./scripts/run_live_matrix.sh --with-temporal
#   ./scripts/run_live_matrix.sh --up-only
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

WITH_TEMPORAL=0
UP_ONLY=0
SKIP_UP=0
for arg in "$@"; do
  case "$arg" in
    --with-temporal) WITH_TEMPORAL=1 ;;
    --up-only) UP_ONLY=1 ;;
    --skip-up) SKIP_UP=1 ;;
    -h|--help)
      sed -n '1,8p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      exit 2
      ;;
  esac
done

if [[ -f .env ]]; then
  # shellcheck disable=SC1091
  set -a
  source .env
  set +a
fi

POSTGRES_USER="${POSTGRES_USER:-trajir}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trajir}"
POSTGRES_DB="${POSTGRES_DB:-trajir}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"

export TRAJIR_DATABASE_URL="${TRAJIR_DATABASE_URL:-postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:5432/${POSTGRES_DB}}"
export TRAJIR_S3_ENDPOINT_URL="${TRAJIR_S3_ENDPOINT_URL:-http://127.0.0.1:9000}"
export TRAJIR_S3_BUCKET="${TRAJIR_S3_BUCKET:-trajir}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-$MINIO_ROOT_USER}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-$MINIO_ROOT_PASSWORD}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export TEMPORAL_HOSTPORT="${TEMPORAL_HOSTPORT:-localhost:7233}"

is_loopback() {
  local target="$1"
  if [[ "$target" == *127.0.0.1* || "$target" == *localhost* || "$target" == *::1* ]]; then
    return 0
  fi
  return 1
}

if [[ "$POSTGRES_PASSWORD" == "trajir" ]] || [[ "$AWS_SECRET_ACCESS_KEY" == "minioadmin" ]]; then
  if ! is_loopback "$TRAJIR_DATABASE_URL" || ! is_loopback "$TRAJIR_S3_ENDPOINT_URL"; then
    echo "WARNING: default development credentials detected on non-loopback endpoint." >&2
    echo "Do not use default credentials on public or untrusted networks." >&2
  fi
fi

if [[ "$SKIP_UP" -eq 0 ]]; then
  echo "==> docker compose up (postgres + minio$( [[ $WITH_TEMPORAL -eq 1 ]] && echo ' + temporal' ))"
  if [[ "$WITH_TEMPORAL" -eq 1 ]]; then
    docker compose -f docker-compose.live.yml up -d
  else
    docker compose -f docker-compose.live.yml up -d postgres minio
  fi

  echo "==> wait postgres healthy"
  for i in $(seq 1 60); do
    if docker exec trajir-live-postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
      break
    fi
    sleep 2
    if [[ "$i" -eq 60 ]]; then
      echo "postgres not ready" >&2
      exit 1
    fi
  done

  echo "==> wait minio healthy"
  minio_live_url="${TRAJIR_S3_ENDPOINT_URL%/}/minio/health/live"
  for i in $(seq 1 60); do
    if curl -sf "$minio_live_url" >/dev/null; then
      break
    fi
    sleep 2
    if [[ "$i" -eq 60 ]]; then
      echo "minio not ready" >&2
      exit 1
    fi
  done
fi

if [[ "$UP_ONLY" -eq 1 ]]; then
  echo "stack is up; exiting (--up-only)"
  exit 0
fi

echo "==> ensure MinIO bucket exists"
python - <<'PY'
import os
from drivers.s3.cas import build_s3_client_from_env

bucket = os.environ["TRAJIR_S3_BUCKET"]
client = build_s3_client_from_env()
try:
    client.head_bucket(Bucket=bucket)
    print(f"bucket exists: {bucket}")
except Exception:
    client.create_bucket(Bucket=bucket)
    print(f"bucket created: {bucket}")
PY

echo "==> Python live Postgres"
python -m pytest test/integration/test_postgres_live.py -q

echo "==> Python live MinIO"
python -m pytest test/integration/test_s3_minio_live.py -q

echo "==> Go live Postgres"
( cd go && go test ./trajir/postgres/... -count=1 )

echo "==> Go live S3 CAS"
( cd go && go test ./trajir/cas -run TestLiveS3StoreFromEnvRoundTrip -count=1 )

if [[ "$WITH_TEMPORAL" -eq 1 ]]; then
  echo "==> Go Temporal integration"
  ( cd go && go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v )
else
  echo "==> skip Temporal (pass --with-temporal to enable)"
fi

echo "OK: live matrix passed"
