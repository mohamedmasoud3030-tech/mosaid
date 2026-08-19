#!/data/data/com.termux/files/usr/bin/bash
# Mosaid Phase 14 Termux installer (owner/private-chat only, fail-closed).
set -euo pipefail
umask 077

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG="$M_HOME/config.json"
SERVICE="$PREFIX/var/service/mosaid"

fail() { echo "INSTALL FAIL: $*" >&2; exit 1; }

case "${PREFIX:-}" in
  /data/data/com.termux/files/usr|/data/user/0/com.termux/files/usr) ;;
  *) fail "run this only inside the official Termux application" ;;
esac
[[ "$(uname -m)" == "aarch64" ]] || fail "the Mosaid phone kit supports aarch64 only"
[[ "$(getprop ro.product.cpu.abi 2>/dev/null)" == "arm64-v8a" ]] || fail "Android ABI must be arm64-v8a"

command -v pkg >/dev/null 2>&1 || fail "pkg command is missing"
pkg update -y
pkg install -y ca-certificates curl jq coreutils procps findutils gawk grep sed termux-services termux-api

for f in "$KIT_DIR/bin/mosaid" "$KIT_DIR/BINARY.sha256" "$KIT_DIR/config/config.phone.template.json"; do
  [[ -f "$f" ]] || fail "kit is incomplete: $f"
done
( cd "$KIT_DIR" && sha256sum -c BINARY.sha256 ) || fail "phone-kit binary checksum failed"

printf 'Numeric Telegram owner user ID: '
IFS= read -r owner
[[ "$owner" =~ ^[1-9][0-9]*$ ]] || fail "owner ID must be a positive integer"

printf 'Telegram bot token (hidden): '
IFS= read -rs bot_token; echo
[[ "$bot_token" =~ ^[0-9]{7,12}:[A-Za-z0-9_-]{30,}$ ]] || fail "Telegram token format is invalid"

printf 'Model API base URL [https://generativelanguage.googleapis.com/v1beta/openai]: '
IFS= read -r api_base
api_base="${api_base:-https://generativelanguage.googleapis.com/v1beta/openai}"
[[ "$api_base" == https://* ]] || fail "model endpoint must be HTTPS"

printf 'Model ID [gemini-2.5-flash]: '
IFS= read -r model_id
model_id="${model_id:-gemini-2.5-flash}"
[[ -n "$model_id" && "$model_id" != *$'\n'* ]] || fail "model ID is required"

printf 'Model API key (hidden): '
IFS= read -rs api_key; echo
[[ -n "$api_key" && "$api_key" != *$'\n'* && "${#api_key}" -ge 16 ]] || fail "API key must be one line of at least 16 characters"

mkdir -p "$M_HOME"/{bin,scripts,secrets,state,logs,runtime,tmp,reports,manifests}
chmod 700 "$M_HOME" "$M_HOME/secrets"
install -m 0700 "$KIT_DIR/bin/mosaid" "$M_HOME/bin/mosaid"
install -m 0600 "$KIT_DIR/BINARY.sha256" "$M_HOME/BINARY.sha256"
for script in verify-config.sh preflight.sh supervisor.sh run-foreground.sh health-sampler.sh collect-diagnostics.sh; do
  install -m 0700 "$KIT_DIR/phone/$script" "$M_HOME/scripts/$script"
done

printf '%s\n' "$bot_token" > "$M_HOME/secrets/telegram.token"
printf '%s\n' "$api_key" > "$M_HOME/secrets/model.key"
chmod 600 "$M_HOME/secrets/telegram.token" "$M_HOME/secrets/model.key"
unset bot_token api_key

jq \
  --arg data_dir "$M_HOME" \
  --argjson owner "$owner" \
  --arg token_file "$M_HOME/secrets/telegram.token" \
  --arg key_file "$M_HOME/secrets/model.key" \
  --arg base "${api_base%/}" \
  --arg model "$model_id" \
  '.data_dir=$data_dir
   | .owner_telegram_id=$owner
   | .telegram.token_file=$token_file
   | .model.api_key_file=$key_file
   | .model.base_url=$base
   | .model.name=$model' \
  "$KIT_DIR/config/config.phone.template.json" > "$CONFIG"
chmod 600 "$CONFIG"

# runit service: the supervisor supplies backoff, wake lock, health sampling
# and clean shutdown; the binary itself holds the singleton lock and redacts logs.
mkdir -p "$SERVICE/log"
cat > "$SERVICE/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec "$M_HOME/scripts/supervisor.sh" 2>&1
EOF
cat > "$SERVICE/log/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$M_HOME/logs"
EOF
chmod 0700 "$SERVICE/run" "$SERVICE/log/run"
cat > "$M_HOME/logs/config" <<'EOF'
s1000000
n10
t
EOF
rm -f "$SERVICE/down"

mkdir -p "$HOME/.termux/boot"
install -m 0700 "$KIT_DIR/phone/10-mosaid.boot" "$HOME/.termux/boot/10-mosaid"

# DNS guard: Termux builds may point $PREFIX/etc/resolv.conf at an inactive
# local resolver (nameserver ::1) which breaks pure-Go DNS. Normalize it now;
# the supervisor re-checks on every start.
if ! grep -qE '^nameserver[[:space:]]+[0-9]+(\.[0-9]+){3}[[:space:]]*$' "$PREFIX/etc/resolv.conf" 2>/dev/null; then
  printf 'nameserver 1.1.1.1\nnameserver 8.8.8.8\n' > "$PREFIX/etc/resolv.conf" 2>/dev/null || true
fi

"$M_HOME/scripts/verify-config.sh"
"$M_HOME/scripts/preflight.sh"

# shellcheck disable=SC1091
. "$PREFIX/etc/profile.d/start-services.sh"
sv up mosaid || true

cat <<EOF

Mosaid installed in: $M_HOME
No shared-storage permission was requested.

Manual Android actions still required:
1. Install Termux:Boot from the same source/signing family as Termux and open it once.
2. Set Android battery usage for Termux and Termux:Boot to Unrestricted.
3. Verify the persistent Termux notification shows a wake lock.
4. Run: $M_HOME/scripts/preflight.sh --network
5. In Telegram, send /status to your bot and expect a Mosaid reply.
6. Keep this session open for the first 30-minute run: $M_HOME/scripts/run-foreground.sh
7. When finished testing: $M_HOME/scripts/collect-diagnostics.sh
EOF
