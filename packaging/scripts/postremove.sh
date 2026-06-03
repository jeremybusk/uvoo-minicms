#!/usr/bin/env sh
set -eu

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi

cat <<'MSG'

Uvoo-MiniCMS package removed.

The service user, configuration, and data were left in place:
  /etc/uvoo-minicms
  /var/lib/uvoo-minicms

MSG
