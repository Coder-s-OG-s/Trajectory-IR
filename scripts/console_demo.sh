#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
data="${TRAJIR_CONSOLE_DATA:-$root/.console-data}"
mkdir -p "$data/trajectories" "$data/packages/console-demo"
cp "$root/go/trajir/console/testdata/console_demo.ndjson" "$data/trajectories/console-demo.ndjson"
cp "$root/testdata/sample_thin.tir" "$data/packages/console-demo/demo.tir"
printf 'TRAJIR_CONSOLE_DATA=%s\n' "$data"
printf 'http://127.0.0.1:8787/?id=console-demo\n'
printf 'cd go && go run ./cmd/trajir-console -addr 127.0.0.1:8787\n'
