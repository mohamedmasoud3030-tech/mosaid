#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
exec "$P0_HOME/scripts/supervisor.sh" --once
