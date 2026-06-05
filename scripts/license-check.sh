#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWED_GO="${ALLOWED_GO:-Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC,BlueOak-1.0.0}"
ALLOWED_NPM="${ALLOWED_NPM:-Apache-2.0;MIT;BSD-2-Clause;BSD-3-Clause;ISC;BlueOak-1.0.0;0BSD;Python-2.0;CC-BY-4.0;W3C-20150513}"
ALLOWED_LOCK_PATTERN="${ALLOWED_LOCK_PATTERN:-^(Apache-2.0|MIT|BSD-2-Clause|BSD-3-Clause|ISC|BlueOak-1.0.0|0BSD|Python-2.0|CC-BY-4.0|W3C-20150513)$}"
INCLUDE_DEV_LICENSES="${INCLUDE_DEV_LICENSES:-0}"
status=0

echo "checking Go module licenses"
if command -v go-licenses >/dev/null 2>&1; then
  if ! go-licenses check "$ROOT/..." --allowed_licenses="$ALLOWED_GO"; then
    status=1
  fi
else
  echo "skipping Go license scan: install with 'go install github.com/google/go-licenses@latest'" >&2
fi

echo "checking npm package licenses"
if command -v jq >/dev/null 2>&1 && [ -f "$ROOT/web/package-lock.json" ]; then
  lock_issues="$(
    jq -r --arg pattern "$ALLOWED_LOCK_PATTERN" '
      .packages
      | to_entries[]
      | select(env.INCLUDE_DEV_LICENSES == "1" or (.value.dev != true))
      | select(.value.license and (.value.license | test($pattern) | not))
      | "\(.key)\t\(.value.version // "")\t\(.value.license)"
    ' "$ROOT/web/package-lock.json"
  )"
  if [ -n "$lock_issues" ]; then
    echo "package-lock.json contains licenses outside the allow-list:" >&2
    printf '%s\n' "$lock_issues" >&2
    status=1
  fi
else
  echo "skipping package-lock license check: install jq" >&2
fi

if [ -x "$ROOT/web/node_modules/.bin/license-checker-rseidelsohn" ]; then
  (
    cd "$ROOT/web"
    ./node_modules/.bin/license-checker-rseidelsohn --production --excludePrivatePackages --summary --onlyAllow "$ALLOWED_NPM"
  ) || status=1
elif command -v license-checker-rseidelsohn >/dev/null 2>&1; then
  (
    cd "$ROOT/web"
    license-checker-rseidelsohn --production --excludePrivatePackages --summary --onlyAllow "$ALLOWED_NPM"
  ) || status=1
elif [ -x "$ROOT/web/node_modules/.bin/license-checker" ]; then
  (
    cd "$ROOT/web"
    ./node_modules/.bin/license-checker --production --excludePrivatePackages --summary --onlyAllow "$ALLOWED_NPM"
  ) || status=1
elif command -v license-checker >/dev/null 2>&1; then
  (
    cd "$ROOT/web"
    license-checker --production --excludePrivatePackages --summary --onlyAllow "$ALLOWED_NPM"
  ) || status=1
else
  echo "skipping npm license scan: install with 'cd web && npm install --save-dev license-checker-rseidelsohn'" >&2
fi

exit "$status"
