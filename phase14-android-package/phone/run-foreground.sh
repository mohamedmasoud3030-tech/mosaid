#!/data/data/com.termux/files/usr/bin/bash
# Run Mosaid once in the foreground (manual testing/debugging).
# The runit service is the normal supervision path; this script is for
# controlled runs where the supervisor should not restart the binary.
set -euo pipefail

M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG="$M_HOME/config.json"
BIN="$M_HOME/bin/mosaid"

bash "$M_HOME/scripts/verify-config.sh"
[[ -x "$BIN" ]] || { echo "binary not found: $BIN" >&2; exit 1; }

echo "Starting mosaid in the foreground. Press Ctrl+C to stop."

# On Android, a stock-Go binary reads /etc/resolv.conf (-> /system/etc) and
# falls back to localhost:53 DNS, which fails. proot binds Termux's
# resolv.conf over /etc/resolv.conf (documented fix; installed by the
# phone installer).
case "${PREFIX:-}" in
  /data/data/com.termux/files/usr|/data/user/0/com.termux/files/usr)
    if command -v proot >/dev/null 2>&1; then
      exec proot -b "$PREFIX/etc/resolv.conf:/etc/resolv.conf" "$BIN" --config "$CONFIG"
    fi
    echo "WARN: proot not installed; DNS may fail on this Android device" >&2
    ;;
esac
exec "$BIN" --config "$CONFIG"
