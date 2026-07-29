#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
SECRETS="$P0_HOME/.security.yml"

# Secrets travel to awk through its inherited environment, not argv. This is
# defense in depth only: processes under the same Termux UID are not isolated.
export PHASE0_REDACT_TELEGRAM="$(jq -r '.channels.telegram.settings.token // .channel_list.telegram.settings.token // ""' "$SECRETS")"
export PHASE0_REDACT_MODEL="$(jq -r '.model_list["phase0-model"].api_keys[0] // ""' "$SECRETS")"

exec awk '
function replace_literal(s, secret,    p) {
  if (secret == "") return s
  while ((p = index(s, secret)) > 0) {
    s = substr(s, 1, p-1) "[REDACTED]" substr(s, p+length(secret))
  }
  return s
}
{
  line = replace_literal($0, ENVIRON["PHASE0_REDACT_TELEGRAM"])
  line = replace_literal(line, ENVIRON["PHASE0_REDACT_MODEL"])
  gsub(/[0-9]{7,12}:[A-Za-z0-9_-]{20,}/, "[REDACTED_TELEGRAM_TOKEN]", line)
  gsub(/sk-[A-Za-z0-9_-]{12,}/, "[REDACTED_API_KEY]", line)
  print line
  fflush()
}'
