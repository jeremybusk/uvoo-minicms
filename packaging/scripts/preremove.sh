#!/usr/bin/env sh
set -eu

STATE_DIR=/run/uvoominicms
WAS_ACTIVE_FILE=$STATE_DIR/was-active
ACTION=${1:-}

if command -v systemctl >/dev/null 2>&1; then
  mkdir -p "$STATE_DIR"
  if systemctl is-active --quiet uvoominicms.service; then
    : > "$WAS_ACTIVE_FILE"
  fi
  systemctl stop uvoominicms.service >/dev/null 2>&1 || true

  case "$ACTION" in
    remove|purge|0)
      systemctl disable uvoominicms.service >/dev/null 2>&1 || true
      rm -f "$WAS_ACTIVE_FILE"
      ;;
  esac
fi
