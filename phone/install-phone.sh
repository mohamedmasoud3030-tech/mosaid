#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"

fail() { echo "INSTALL FAIL: $*" >&2; exit 1; }
case "${PREFIX:-}" in
  /data/data/com.termux/files/usr|/data/user/0/com.termux/files/usr) ;;
  *) fail "run this only inside the official Termux application" ;;
esac
[[ "$(uname -m)" == "aarch64" ]] || fail "Phase 0 artifact supports aarch64 only"
[[ "$(getprop ro.product.cpu.abi 2>/dev/null)" == "arm64-v8a" ]] || fail "Android ABI must be arm64-v8a"

command -v pkg >/dev/null 2>&1 || fail "pkg command is missing"
pkg update -y
pkg install -y ca-certificates curl jq coreutils procps findutils gawk grep sed termux-services termux-api

for f in \
  "$KIT_DIR/bin/picoclaw-phase0" \
  "$KIT_DIR/BINARY.sha256" \
  "$KIT_DIR/config/config.phase0.template.json" \
  "$KIT_DIR/config/test-plan.json"; do
  [[ -f "$f" ]] || fail "kit is incomplete: $f"
done
( cd "$KIT_DIR" && sha256sum -c BINARY.sha256 ) || fail "phone-kit binary checksum failed"

printf 'Numeric Telegram owner user ID: '
IFS= read -r owner
[[ "$owner" =~ ^[0-9]+$ ]] || fail "owner ID must contain digits only"
printf 'Telegram bot token (hidden): '
IFS= read -rs bot_token; echo
[[ "$bot_token" =~ ^[0-9]{7,12}:[A-Za-z0-9_-]{30,}$ ]] || fail "Telegram token format is invalid"
printf 'OpenAI-compatible HTTPS base URL [https://api.openai.com/v1]: '
IFS= read -r api_base
api_base="${api_base:-https://api.openai.com/v1}"
[[ "$api_base" == https://* ]] || fail "Phase 0 requires an HTTPS model endpoint"
printf 'Model ID (without openai/ prefix): '
IFS= read -r model_id
[[ -n "$model_id" && "$model_id" != *$'\n'* ]] || fail "model ID is required"
printf 'OpenAI-compatible API key (hidden): '
IFS= read -rs api_key; echo
[[ -n "$api_key" && "$api_key" != *$'\n'* ]] || fail "API key is required"

mkdir -p "$P0_HOME"/{bin,scripts,workspace,logs,runtime,tmp,reports,tests,licenses}
install -m 0700 "$KIT_DIR/bin/picoclaw-phase0" "$P0_HOME/bin/picoclaw-phase0"
install -m 0600 "$KIT_DIR/BINARY.sha256" "$P0_HOME/BINARY.sha256"
install -m 0600 "$KIT_DIR/config/test-plan.json" "$P0_HOME/test-plan.json"
for script in verify-config.sh preflight.sh supervisor.sh run-foreground.sh redact-stream.sh health-sampler.sh test-harness.sh collect-results.sh collect-diagnostics.sh; do
  install -m 0700 "$KIT_DIR/phone/$script" "$P0_HOME/scripts/$script"
done
[[ -f "$KIT_DIR/licenses/PicoClaw-LICENSE" ]] && install -m 0600 "$KIT_DIR/licenses/PicoClaw-LICENSE" "$P0_HOME/licenses/PicoClaw-LICENSE"

jq --arg workspace "$P0_HOME/workspace" --arg owner "$owner" --arg base "${api_base%/}" --arg model "$model_id" '
  .agents.defaults.workspace=$workspace
  | .channel_list.telegram.allow_from=[$owner]
  | .model_list[0].api_base=$base
  | .model_list[0].model=("openai/"+$model)
' "$KIT_DIR/config/config.phase0.template.json" > "$P0_HOME/config.json"

jq -n --arg token "$bot_token" --arg key "$api_key" '{
  channels:{telegram:{settings:{token:$token}}},
  model_list:{"phase0-model":{api_keys:[$key]}}
}' > "$P0_HOME/.security.yml"
chmod 0600 "$P0_HOME/config.json" "$P0_HOME/.security.yml"
unset bot_token api_key

# runit service: the outer supervisor supplies locking, backoff, metrics and clean shutdown.
SERVICE="$PREFIX/var/service/picoclaw-phase0"
mkdir -p "$SERVICE/log"
cat > "$SERVICE/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec "$P0_HOME/scripts/supervisor.sh" 2>&1
EOF
cat > "$SERVICE/log/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$P0_HOME/logs"
EOF
chmod 0700 "$SERVICE/run" "$SERVICE/log/run"
cat > "$P0_HOME/logs/config" <<'EOF'
s1000000
n10
t
EOF
rm -f "$SERVICE/down"

mkdir -p "$HOME/.termux/boot"
install -m 0700 "$KIT_DIR/phone/10-picoclaw-phase0.boot" "$HOME/.termux/boot/10-picoclaw-phase0"

"$P0_HOME/scripts/verify-config.sh"
"$P0_HOME/scripts/preflight.sh"

# Start termux-services in the current session if it has not already started.
# Termux:Boot will source the same file after reboot.
# shellcheck disable=SC1091
. "$PREFIX/etc/profile.d/start-services.sh"
sv up picoclaw-phase0 || true

cat <<EOF

Phase 0 installed in: $P0_HOME
No shared-storage permission was requested.

Manual Android actions still required:
1. Install Termux:Boot from the same source/signing family as Termux and open it once.
2. Set Android battery usage for Termux and Termux:Boot to Unrestricted.
3. Verify the persistent Termux notification shows a wake lock.
4. Run: $P0_HOME/scripts/preflight.sh --network
5. In Telegram, send /status and /echo hello.
6. Start the first scenario with:
   $P0_HOME/scripts/test-harness.sh arm 01-initial-30m
EOF
