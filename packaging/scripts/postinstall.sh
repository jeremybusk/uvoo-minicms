#!/usr/bin/env sh
set -eu

ENV_FILE=/etc/uvoo-minicms/uvoo-minicms.env
STATE_DIR=/run/uvoo-minicms
WAS_ACTIVE_FILE=$STATE_DIR/was-active
GENERATED_PASS=

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

ensure_password() {
  [ -f "$ENV_FILE" ] || return 0

  if grep -Eq '^CMS_ADMIN_PASS=(change-me|change-me-now)?$' "$ENV_FILE"; then
    pass="$(random_password)"
    tmp="${ENV_FILE}.tmp.$$"
    sed "s/^CMS_ADMIN_PASS=.*/CMS_ADMIN_PASS=$pass/" "$ENV_FILE" > "$tmp"
    install -m 0640 -o root -g uvoo-minicms "$tmp" "$ENV_FILE"
    rm -f "$tmp"
    GENERATED_PASS="$pass"
    return 0
  fi

  if ! grep -Eq '^CMS_ADMIN_PASS=' "$ENV_FILE"; then
    pass="$(random_password)"
    printf '\nCMS_ADMIN_PASS=%s\n' "$pass" >> "$ENV_FILE"
    GENERATED_PASS="$pass"
  fi
}

mkdir -p /etc/uvoo-minicms /var/lib/uvoo-minicms/uploads "$STATE_DIR"
chown -R uvoo-minicms:uvoo-minicms /var/lib/uvoo-minicms
chmod 0750 /var/lib/uvoo-minicms /var/lib/uvoo-minicms/uploads
chown root:uvoo-minicms /etc/uvoo-minicms || true
chmod 0750 /etc/uvoo-minicms || true

ensure_password
[ -f "$ENV_FILE" ] && chown root:uvoo-minicms "$ENV_FILE" && chmod 0640 "$ENV_FILE"

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  systemctl enable uvoo-minicms.service || true
  if [ -f "$WAS_ACTIVE_FILE" ]; then
    systemctl restart uvoo-minicms.service || true
    rm -f "$WAS_ACTIVE_FILE"
  else
    systemctl start uvoo-minicms.service || true
  fi
fi

if [ -n "$GENERATED_PASS" ]; then
  cat <<MSG
Uvoo-MiniCMS installed and started.

Generated a strong admin password in:
  $ENV_FILE

Admin login:
  user: admin
  password: $GENERATED_PASS

MSG
else
  cat <<MSG
Uvoo-MiniCMS installed and started.

Admin credentials are configured in:
  $ENV_FILE

MSG
fi
