#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-minicms}"
OUT="${OUT:-$ROOT/bin/$APP_NAME}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need go
need npm

if ! command -v gcc >/dev/null 2>&1 && ! command -v cc >/dev/null 2>&1; then
  echo "missing C compiler: install gcc or build-essential for SQLite CGO support" >&2
  exit 1
fi

cd "$ROOT/web"
if [ -f package-lock.json ]; then
  npm ci
else
  npm install
fi
npm run build

cd "$ROOT"
mkdir -p "$(dirname "$OUT")"
CGO_ENABLED="${CGO_ENABLED:-1}" go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/uvoo-minicms

echo "built $OUT"
echo "run from the repo root with: $OUT"
