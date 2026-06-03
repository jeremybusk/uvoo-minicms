#!/usr/bin/env sh
set -eu

if ! getent group uvoo-minicms >/dev/null 2>&1; then
  groupadd --system uvoo-minicms
fi

if ! id -u uvoo-minicms >/dev/null 2>&1; then
  nologin=/usr/sbin/nologin
  if [ ! -x "$nologin" ]; then
    nologin=/sbin/nologin
  fi

  useradd --system \
    --gid uvoo-minicms \
    --home-dir /var/lib/uvoo-minicms \
    --shell "$nologin" \
    uvoo-minicms
fi
