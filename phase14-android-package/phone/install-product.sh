#!/data/data/com.termux/files/usr/bin/bash
# Mosaid product installer for Termux (Android arm64-v8a).
# Run inside the official Termux application after extracting the phone kit:
#   bash phone/install-product.sh
set -euo pipefail
umask 077

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"

fail() { echo "INSTALL FAIL: $*" >&2; exit 1; }

case "${PREFIX:-}" in
  /data/data/com.termux/files/usr|/data/user/0/com.termux/files/usr) ;;
  *) fail "run this only inside the official Termux application" ;;
esac
[[ "$(uname -m)" == "aarch64" ]] || fail "the product binary supports aarch64 only"
[[ "$(getprop ro.product.cpu.abi 2>/dev/null)" == "arm64-v8a" ]] || fail "Android ABI must be arm64-v8a"
[[ -n "${HOME:-}" && "$HOME" == /data/* ]] || fail "HOME must be inside the Termux app directory"

command -v pkg >/dev/null 2>&1 || fail "pkg command is missing"
pkg update -y
pkg install -y ca-certificates curl jq coreutils procps findutils gawk grep sed termux-services termux-api

for f in \
  "$KIT_DIR/bin/mosaid" \
  "$KIT_DIR/BINARY.sha256" \
  "$KIT_DIR/config/product.template.json"; do
  [[ -f "$f" ]] || fail "kit is incomplete: $f"
done
( cd "$KIT_DIR" && sha256sum -c BINARY.sha256 ) || fail "phone-kit binary checksum failed"

printf 'Numeric Telegram owner user ID: '
IFS= read -r owner
[[ "$owner" =~ ^[0-9]+$ ]] || fail "owner ID must contain digits only"
(( owner > 0 )) || fail "owner ID must be positive"

printf 'Telegram bot token (hidden): '
IFS= read -rs bot_token; echo
[[ "$bot_token" =~ ^[0-9]{7,12}:[A-Za-z0-9_-]{30,}$ ]] || fail "Telegram token format is invalid"

printf 'OpenAI-compatible HTTPS base URL [https://api.openai.com/v1]: '
IFS= read -r api_base
api_base="${api_base:-https://api.openai.com/v1}"
[[ "$api_base" == https://* ]] || fail "an HTTPS model endpoint is required"
api_base="${api_base%/}"

printf 'Model ID: '
IFS= read -r model_id
[[ -n "$model_id" && "$model_id" != *$'\n'* ]] || fail "model ID is required"
(( ${#model_id} <= 128 )) || fail "model ID must be at most 128 characters"

printf 'OpenAI-compatible API key (hidden): '
IFS= read -rs api_key; echo
[[ -n "$api_key" && "$api_key" != *$'\n'* ]] || fail "API key is required"
(( ${#api_key} <= 65536 )) || fail "API key is unreasonably long"

mkdir -p "$MOSAID_HOME"/{bin,scripts,workspace,logs,runtime,tmp,reports,tests,licenses}
mkdir -p "$CONFIG_HOME"
chmod 700 "$CONFIG_HOME"

install -m 0700 "$KIT_DIR/bin/mosaid" "$MOSAID_HOME/bin/mosaid"
install -m 0600 "$KIT_DIR/BINARY.sha256" "$MOSAID_HOME/BINARY.sha256"
for script in verify-config.sh preflight.sh supervisor.sh redact-stream.sh health-sampler.sh collect-diagnostics.sh uninstall-product.sh; do
  install -m 0700 "$KIT_DIR/phone/$script" "$MOSAID_HOME/scripts/$script"
done
[[ -f "$KIT_DIR/licenses/MIT-LICENSE" ]] && install -m 0600 "$KIT_DIR/licenses/MIT-LICENSE" "$MOSAID_HOME/licenses/MIT-LICENSE"
[[ -f "$KIT_DIR/licenses/THIRD_PARTY_NOTICES.md" ]] && install -m 0600 "$KIT_DIR/licenses/THIRD_PARTY_NOTICES.md" "$MOSAID_HOME/licenses/THIRD_PARTY_NOTICES.md"

printf '%s\n' "$bot_token" > "$CONFIG_HOME/telegram.token"
printf '%s\n' "$api_key" > "$CONFIG_HOME/model.key"
chmod 0600 "$CONFIG_HOME/telegram.token" "$CONFIG_HOME/model.key"

jq --arg home "$MOSAID_HOME" --arg chome "$CONFIG_HOME" \
   --arg base "$api_base" --arg model "$model_id" --argjson owner "$owner" '
  .data_dir=$home
  | .owner_telegram_id=$owner
  | .telegram.token_file=($chome+"/telegram.token")
  | .model.api_key_file=($chome+"/model.key")
  | .model.base_url=$base
  | .model.name=$model
' "$KIT_DIR/config/product.template.json" > "$MOSAID_HOME/config.json"
chmod 0600 "$MOSAID_HOME/config.json"
unset bot_token api_key

# runit service: the supervisor supplies locking, backoff, redaction,
# health sampling and clean shutdown.
SERVICE="$PREFIX/var/service/mosaid"
mkdir -p "$SERVICE/log"
cat > "$SERVICE/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec "$MOSAID_HOME/scripts/supervisor.sh" 2>&1
EOF
cat > "$SERVICE/log/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$MOSAID_HOME/logs"
EOF
chmod 0700 "$SERVICE/run" "$SERVICE/log/run"
cat > "$MOSAID_HOME/logs/config" <<'EOF'
s1000000
n10
t
EOF
rm -f "$SERVICE/down"

mkdir -p "$HOME/.termux/boot"
install -m 0700 "$KIT_DIR/phone/10-mosaid.boot" "$HOME/.termux/boot/10-mosaid"

MOSAID_HOME="$MOSAID_HOME" "$MOSAID_HOME/scripts/verify-config.sh"
"$MOSAID_HOME/scripts/preflight.sh"

# Start termux-services in the current session if it has not already started.
# Termux:Boot will source the same file after reboot.
# shellcheck disable=SC1091
. "$PREFIX/etc/profile.d/start-services.sh"
sv up mosaid || true

cat <<EOF

Mosaid installed in: $MOSAID_HOME
Secrets written to:  $CONFIG_HOME (mode 0600)
No shared-storage permission was requested.

Manual Android actions still required:
1. Install Termux:Boot from the same source/signing family as Termux and open it once.
2. Set Android battery usage for Termux and Termux:Boot to Unrestricted.
3. Verify the persistent Termux notification shows a wake lock.
4. Run: $MOSAID_HOME/scripts/preflight.sh --network
5. In Telegram, send /status and /help to the bot.
6. Follow docs/PRODUCT-CHECKLIST.md for the first 30-minute scenario.

Uninstall: bash $MOSAID_HOME/scripts/uninstall-product.sh
EOF
