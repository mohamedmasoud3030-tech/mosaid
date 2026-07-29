#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP"/{bin,scripts,workspace,logs,runtime,tmp,reports,tests}

for s in verify-config.sh supervisor.sh redact-stream.sh health-sampler.sh; do
  cp "$ROOT/phone/$s" "$TMP/scripts/$s"
  chmod 700 "$TMP/scripts/$s"
done

jq --arg workspace "$TMP/workspace" --arg owner 123456789 \
  --arg base https://example.invalid/v1 --arg model test-model '
  .agents.defaults.workspace=$workspace
  | .channel_list.telegram.allow_from=[$owner]
  | .model_list[0].api_base=$base
  | .model_list[0].model=("openai/"+$model)
' "$ROOT/config/config.phase0.template.json" > "$TMP/config.json"

# Deliberately synthetic values that do not match real provider token formats.
jq -n '{channels:{telegram:{settings:{token:"synthetic-telegram-value"}}},model_list:{"phase0-model":{api_keys:["synthetic-model-value"]}}}' > "$TMP/.security.yml"

cat > "$TMP/bin/picoclaw-phase0" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  status) echo 'status ok' ;;
  gateway)
    token=$(jq -r '.channels.telegram.settings.token' "$PICOCLAW_HOME/.security.yml")
    key=$(jq -r '.model_list["phase0-model"].api_keys[0]' "$PICOCLAW_HOME/.security.yml")
    echo "dummy token=$token key=$key"
    sleep 3
    ;;
  *) exit 2 ;;
esac
EOF
chmod 700 "$TMP/bin/picoclaw-phase0"
chmod 600 "$TMP/config.json" "$TMP/.security.yml"
sha256sum "$TMP/bin/picoclaw-phase0" > "$TMP/BINARY.sha256"
chmod 600 "$TMP/BINARY.sha256"

PICOCLAW_PHASE0_HOME="$TMP" bash "$TMP/scripts/verify-config.sh"
PICOCLAW_PHASE0_HOME="$TMP" bash "$TMP/scripts/supervisor.sh" --once > "$TMP/first.log" 2>&1 &
first=$!
sleep 1
set +e
PICOCLAW_PHASE0_HOME="$TMP" bash "$TMP/scripts/supervisor.sh" --once > "$TMP/second.log" 2>&1
second_rc=$?
set -e
wait "$first"

[[ "$second_rc" == 73 ]]
! grep -Fq 'synthetic-telegram-value' "$TMP/first.log"
! grep -Fq 'synthetic-model-value' "$TMP/first.log"
grep -q '\[REDACTED\]' "$TMP/first.log"
[[ ! -e "$TMP/runtime/supervisor.lock" ]]
[[ ! -e "$TMP/runtime/agent.pid" ]]
[[ ! -e "$TMP/runtime/supervisor.pid" ]]

echo 'Phase 0 config/redaction/singleton/clean-shutdown self-test: PASS'
