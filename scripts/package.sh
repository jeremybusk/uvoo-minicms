#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-minicms}"
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

random_password() {
  if [ -r /dev/urandom ] && command -v od >/dev/null 2>&1; then
    od -An -N 32 -tx1 /dev/urandom | tr -d ' \n'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  echo 'cannot generate a secure random CMS_ADMIN_PASS: install coreutils or openssl' >&2
  exit 1
}

: "${CMS_ADDR:=:8080}"
: "${CMS_SITE_NAME:=Uvoo-MiniCMS}"
: "${CMS_ADMIN_USER:=admin}"
: "${CMS_ADMIN_PASS:=change-me-now}"
: "${CMS_ADMIN_RATE_LIMIT:=0}"
: "${CMS_DATA_DIR:=./data}"
: "${CMS_DB:=$CMS_DATA_DIR/cms.db}"
: "${CMS_UPLOAD_DIR:=$CMS_DATA_DIR/uploads}"
: "${CMS_WEB_ROOT:=./web/dist}"
: "${CMS_CSP_MODE:=enforce}"

case "$CMS_ADMIN_PASS" in
  ""|change-me|change-me-now)
    CMS_ADMIN_PASS="$(random_password)"
    if [ -f .env ]; then
      if grep -Eq '^CMS_ADMIN_PASS=' .env; then
        tmp=".env.tmp.$$"
        sed "s/^CMS_ADMIN_PASS=.*/CMS_ADMIN_PASS=$CMS_ADMIN_PASS/" .env > "$tmp"
        mv "$tmp" .env
      else
        printf '\nCMS_ADMIN_PASS=%s\n' "$CMS_ADMIN_PASS" >> .env
      fi
    else
      cat > .env <<EOF
CMS_ADDR=$CMS_ADDR
CMS_SITE_NAME=$CMS_SITE_NAME
CMS_ADMIN_USER=$CMS_ADMIN_USER
CMS_ADMIN_PASS=$CMS_ADMIN_PASS
CMS_ADMIN_RATE_LIMIT=$CMS_ADMIN_RATE_LIMIT
CMS_DATA_DIR=$CMS_DATA_DIR
CMS_DB=$CMS_DB
CMS_UPLOAD_DIR=$CMS_UPLOAD_DIR
CMS_WEB_ROOT=$CMS_WEB_ROOT
CMS_CSP_MODE=$CMS_CSP_MODE
EOF
    fi
    printf 'Generated admin password in .env: %s\n' "$CMS_ADMIN_PASS" >&2
    ;;
esac

export CMS_ADDR CMS_SITE_NAME CMS_ADMIN_USER CMS_ADMIN_PASS CMS_ADMIN_RATE_LIMIT CMS_DATA_DIR CMS_DB CMS_UPLOAD_DIR CMS_WEB_ROOT CMS_CSP_MODE
mkdir -p "$CMS_UPLOAD_DIR"

exec ./uvoo-minicms "$@"
RUNSH

chmod 0755 "$PKG_DIR/$APP_NAME" "$PKG_DIR/run.sh"
tar -C "$ROOT/dist" -czf "$TARBALL" "$PKG_NAME"

echo "created $TARBALL"
echo "install:"
echo "  tar -xzf $TARBALL"
echo "  cd $PKG_NAME"
echo "  cp .env.example .env"
echo "  ./run.sh"
