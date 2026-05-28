#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoominicms}"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || date -u +%Y%m%d%H%M%S)}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
PKG_NAME="$APP_NAME-$VERSION-$GOOS-$GOARCH"
PKG_DIR="$ROOT/dist/$PKG_NAME"
TARBALL="$ROOT/dist/$PKG_NAME.tar.gz"

if [ "$GOOS" != "linux" ]; then
  echo "warning: packaging for GOOS=$GOOS; set GOOS=linux for a Linux release tarball" >&2
fi

OUT="$ROOT/bin/$APP_NAME" bash "$ROOT/scripts/build.sh"

rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/web" "$PKG_DIR/data/uploads"
cp "$ROOT/bin/$APP_NAME" "$PKG_DIR/$APP_NAME"
cp -a "$ROOT/web/dist" "$PKG_DIR/web/dist"
cp "$ROOT/README.md" "$PKG_DIR/README.md"
cp "$ROOT/.env.example" "$PKG_DIR/.env.example"

cat > "$PKG_DIR/run.sh" <<'RUNSH'
#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi

: "${CMS_ADDR:=:8080}"
: "${CMS_SITE_NAME:=UvooMiniCMS}"
: "${CMS_ADMIN_USER:=admin}"
: "${CMS_ADMIN_PASS:=change-me-now}"
: "${CMS_DATA_DIR:=./data}"
: "${CMS_DB:=$CMS_DATA_DIR/cms.db}"
: "${CMS_UPLOAD_DIR:=$CMS_DATA_DIR/uploads}"
: "${CMS_WEB_ROOT:=./web/dist}"

export CMS_ADDR CMS_SITE_NAME CMS_ADMIN_USER CMS_ADMIN_PASS CMS_DATA_DIR CMS_DB CMS_UPLOAD_DIR CMS_WEB_ROOT
mkdir -p "$CMS_UPLOAD_DIR"

exec ./uvoominicms "$@"
RUNSH

chmod 0755 "$PKG_DIR/$APP_NAME" "$PKG_DIR/run.sh"
tar -C "$ROOT/dist" -czf "$TARBALL" "$PKG_NAME"

echo "created $TARBALL"
echo "install:"
echo "  tar -xzf $TARBALL"
echo "  cd $PKG_NAME"
echo "  cp .env.example .env"
echo "  ./run.sh"
