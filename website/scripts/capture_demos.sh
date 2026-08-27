#!/usr/bin/env bash
# Capture Go demo stdout into website/fixtures for docs embeds.
# Run from repo root: bash website/scripts/capture_demos.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GO_DIR="$ROOT/go"
FIX="$ROOT/website/fixtures"
mkdir -p "$FIX"

cd "$GO_DIR"

echo "Capturing adoption_host -with-package..."
go run ./examples/adoption_host -with-package 2>&1 \
  | sed -E 's|tir:[[:space:]]+.+$|tir:          /tmp/adoption-run.tir|' \
  > "$FIX/adoption_host_package.txt"

echo "Capturing adoption_host -sandbox..."
go run ./examples/adoption_host -sandbox 2>&1 > "$FIX/adoption_host_sandbox.txt" || true

echo "Capturing kill_mid_deploy (crash + resume)..."
WD="$GO_DIR/_demo_capture_site"
rm -rf "$WD"
mkdir -p "$WD"

go run ./examples/kill_mid_deploy -workdir "$WD" -crash-during=tool_call \
  >"$WD/first.txt" 2>"$WD/first.err" &
PID=$!

for _ in $(seq 1 225); do
  if [[ -f "$WD/tool_started.marker" ]]; then
    break
  fi
  sleep 0.4
done

if [[ ! -f "$WD/tool_started.marker" ]]; then
  kill -9 "$PID" 2>/dev/null || true
  echo "Timed out waiting for TOOL_CALL start marker" >&2
  exit 1
fi

kill -9 "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true
sleep 1

go run ./examples/kill_mid_deploy -workdir "$WD" -resume 2>&1 >"$WD/resume.txt"

sed "s|$WD|./kill_mid_deploy-data|g" "$WD/first.txt" > "$FIX/kill_mid_deploy_first.txt"
sed "s|$WD|./kill_mid_deploy-data|g" "$WD/resume.txt" > "$FIX/kill_mid_deploy_resume.txt"

echo "Fixtures written to $FIX"
