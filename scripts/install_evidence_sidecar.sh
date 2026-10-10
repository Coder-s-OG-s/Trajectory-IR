#!/usr/bin/env bash
# Install the local Evidence Sidecar slice (TURNING_POINT wrap + verify).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== Evidence Sidecar install =="
echo "  root: $ROOT"

echo "== build trajir verify CLI =="
( cd go && go build -o trajir ./cmd/trajir )
EXE="$ROOT/go/trajir"
if [[ ! -x "$EXE" && ! -f "$EXE" ]]; then
  echo "missing $EXE" >&2
  exit 1
fi
echo "  OK $EXE"
"$EXE" verify -h 2>&1 | head -n 5 || true

cat <<EOF

Next:
  1. python integrations/langgraph/demo_stub.py
  2. ./go/trajir verify .local-evidence/langgraph-demo.tir
  3. Docs: docs/EVIDENCE_SIDECAR.md
  4. Optional console on 8787 only:
       export TRAJIR_CONSOLE_DATA="\$ROOT/.console-data"
       (cd go && go run ./cmd/trajir-console -addr 127.0.0.1:8787)
  5. Optional LangGraph extra: pip install -e '.[langgraph]'

INSTALL OK
EOF
