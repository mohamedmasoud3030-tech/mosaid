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
exec "$BIN" --config "$CONFIG"
