#!/data/data/com.termux/files/usr/bin/bash
# Uninstall the Mosaid product service and boot hook from Termux.
# Data and secrets are kept by default. Use --purge-data to remove
# $MOSAID_HOME and $MOSAID_CONFIG_HOME as well.
set -euo pipefail
umask 077

MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"
PURGE=0
[[ "${1:-}" == "--purge-data" ]] && PURGE=1
[[ "${1:-}" == "" || "${1:-}" == "--purge-data" ]] || { echo "usage: uninstall-product.sh [--purge-data]" >&2; exit 2; }

SERVICE="$PREFIX/var/service/mosaid"
BOOT="$HOME/.termux/boot/10-mosaid"

if [[ -d "$SERVICE" ]]; then
  command -v sv >/dev/null 2>&1 && sv -w 15 down mosaid 2>/dev/null || true
  rm -rf "$SERVICE"
  echo "removed runit service: $SERVICE"
else
  echo "runit service not present"
fi

if [[ -f "$BOOT" ]]; then
  rm -f "$BOOT"
  echo "removed boot hook: $BOOT"
else
  echo "boot hook not present"
fi

if [[ -f "$MOSAID_HOME/runtime/supervisor.lock/pid" ]]; then
  old_pid="$(cat "$MOSAID_HOME/runtime/supervisor.lock/pid" 2>/dev/null || true)"
  [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -TERM "$old_pid" 2>/dev/null || true
fi
command -v termux-wake-unlock >/dev/null 2>&1 && termux-wake-unlock >/dev/null 2>&1 || true

if (( PURGE )); then
  echo "WARNING: removing all Mosaid product data and secrets"
  printf 'Type DELETE to confirm: '
  IFS= read -r confirm
  [[ "$confirm" == "DELETE" ]] || { echo "aborted, nothing was deleted" >&2; exit 3; }
  rm -rf "$MOSAID_HOME" "$CONFIG_HOME"
  echo "removed: $MOSAID_HOME $CONFIG_HOME"
else
  echo "kept data: $MOSAID_HOME"
  echo "kept secrets: $CONFIG_HOME"
  echo "reinstall later by running the phone-kit installer again"
fi

echo "Mosaid product uninstalled"
