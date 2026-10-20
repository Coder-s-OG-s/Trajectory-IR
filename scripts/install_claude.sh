#!/usr/bin/env bash
# One-step local attach for Claude Code in this worktree.
# Does not edit the user profile. Does not start a console. Does not git commit.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== One-step Claude attach =="
echo "  root: $ROOT"

# go/trajir is the package directory. The binary is go/trajir.exe so the
# build does not write go/trajir/trajir and then treat the directory as OK.
# trajir-mcp.exe uses the same suffix on every OS.
( cd "$ROOT/go" && go build -o trajir.exe ./cmd/trajir && go build -o trajir-mcp.exe ./cmd/trajir-mcp )
test -f "$ROOT/go/trajir.exe"
test -x "$ROOT/go/trajir.exe" || chmod +x "$ROOT/go/trajir.exe"
test -f "$ROOT/go/trajir-mcp.exe"

python "$ROOT/scripts/write_claude_local_config.py"
python "$ROOT/scripts/local_one_step_smoke.py"

echo ""
echo "Attached inside this worktree only."
echo "  Hooks: .claude/settings.local.json"
echo "  MCP:   .mcp.json"
echo "  Verify a pack with: ./go/trajir.exe verify <pack.tir>"
echo "INSTALL OK"
