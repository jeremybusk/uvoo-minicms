#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoominicms}"
RAW_VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || date -u +%Y%m%d%H%M%S)}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-}"
if [ -z "$GOARCH" ]; then
  case "$(uname -m)" in
    x86_64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    armv7l) GOARCH=arm ;;
    *) echo "set GOARCH for architecture: $(uname -m)" >&2; exit 1 ;;
  esac
fi
FORMATS="${FORMATS:-deb,rpm}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need nfpm

package_version() {
  local raw="$1"
  local count="${2:-}"
  raw="${raw#v}"
  raw="$(printf '%s' "$raw" | sed -E 's/[^A-Za-z0-9.+~]+/./g; s/^\.+//; s/\.+$//')"
  if [[ "$raw" =~ ^[0-9] ]]; then
    printf '%s\n' "$raw"
  elif [[ "$count" =~ ^[0-9]+$ ]]; then
    printf '0.0.%s+git.%s\n' "$count" "${raw:-unknown}"
  else
    printf '0.0.0+git.%s\n' "${raw:-unknown}"
  fi
}

GIT_COUNT="$(git -C "$ROOT" rev-list --count HEAD 2>/dev/null || true)"
VERSION="$(package_version "$RAW_VERSION" "$GIT_COUNT")"

if [ "$GOOS" != "linux" ]; then
  echo "deb/rpm packages must target linux; got GOOS=$GOOS" >&2
  exit 1
fi

cd "$ROOT"
echo "packaging $APP_NAME version $VERSION from source version $RAW_VERSION"
GOOS="$GOOS" GOARCH="$GOARCH" OUT="$ROOT/bin/$APP_NAME" bash "$ROOT/scripts/build.sh"

mkdir -p "$ROOT/dist"
IFS=',' read -r -a formats <<< "$FORMATS"
for format in "${formats[@]}"; do
  format="$(printf '%s' "$format" | tr -d '[:space:]')"
  [ -n "$format" ] || continue
  VERSION="$VERSION" GOARCH="$GOARCH" nfpm package \
    --config "$ROOT/.nfpm.yaml" \
    --packager "$format" \
    --target "$ROOT/dist/"
done

echo "created Linux packages in $ROOT/dist"
