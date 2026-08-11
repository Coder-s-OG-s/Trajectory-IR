#!/usr/bin/env bash
# Fail if total statement coverage for go/trajir/... is below GO_COV_FAIL_UNDER (percent).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/go"

MIN="${GO_COV_FAIL_UNDER:-50}"
PROFILE="${GO_COVERPROFILE:-coverage.out}"

# Default unit floor excludes durable/temporal: that package needs a live
# Temporal cluster for meaningful coverage (see temporal_integration tests).
# Other trajir packages (including postgres offline mocks and S3) stay in scope.
mapfile -t COVER_PKGS < <(go list ./trajir/... | grep -v '/durable/temporal$')
if [[ ${#COVER_PKGS[@]} -eq 0 ]]; then
  echo "error: no packages to cover under trajir/" >&2
  exit 1
fi

echo "Running go test (excluding durable/temporal) with coverage (floor ${MIN}%)"
go test "${COVER_PKGS[@]}" -count=1 -coverprofile="$PROFILE"

if [[ ! -f "$PROFILE" ]]; then
  echo "error: coverprofile $PROFILE was not produced" >&2
  exit 1
fi

# last line: total: (statements) XX.X%
TOTAL_LINE="$(go tool cover -func="$PROFILE" | tail -n 1)"
echo "$TOTAL_LINE"

PCT="$(echo "$TOTAL_LINE" | awk '{print $3}' | tr -d '%')"
if [[ -z "$PCT" ]]; then
  echo "error: could not parse coverage percent from: $TOTAL_LINE" >&2
  exit 1
fi

# awk numeric compare (avoids bash float issues)
awk -v pct="$PCT" -v min="$MIN" 'BEGIN {
  if (pct + 0 < min + 0) {
    printf "error: Go trajir coverage %.1f%% is below floor %s%%\n", pct, min > "/dev/stderr"
    exit 1
  }
  printf "Go trajir coverage %.1f%% meets floor %s%%\n", pct, min
}'
