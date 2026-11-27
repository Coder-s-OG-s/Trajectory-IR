#!/usr/bin/env bash
# Build the trajir binary for every release platform, plus SHA256SUMS.
#
# usage: scripts/release_binaries.sh <version tag> [out dir]
#
# The SQLite driver is pure Go (modernc.org/sqlite), so CGO_ENABLED=0
# cross-compiles every target from one runner. Archives hold trajir,
# LICENSE and README.md. Unix targets are .tar.gz, Windows is .zip.
set -euo pipefail

TAG="${1:?usage: release_binaries.sh <version tag> [out dir]}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${2:-$ROOT/dist-go}"
PY="$(command -v python3 || command -v python)"

TARGETS=(
  linux/amd64
  linux/arm64
  darwin/amd64
  darwin/arm64
  windows/amd64
  windows/arm64
)

rm -rf "$OUT"
mkdir -p "$OUT"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

for target in "${TARGETS[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="trajir_${TAG}_${goos}_${goarch}"
  stage="$WORK/$name"
  mkdir -p "$stage"
  exe="trajir"
  if [[ "$goos" == "windows" ]]; then
    exe="trajir.exe"
  fi
  (
    cd "$ROOT/go"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${TAG}" \
      -o "$stage/$exe" ./cmd/trajir
  )
  cp "$ROOT/LICENSE" "$ROOT/README.md" "$stage/"
  if [[ "$goos" == "windows" ]]; then
    (cd "$WORK" && "$PY" -m zipfile -c "$OUT/$name.zip" "$name")
  else
    tar -C "$WORK" -czf "$OUT/$name.tar.gz" "$name"
  fi
  echo "built $name"
done

(
  cd "$OUT"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum trajir_* > SHA256SUMS
  else
    shasum -a 256 trajir_* > SHA256SUMS
  fi
)
cat "$OUT/SHA256SUMS"
