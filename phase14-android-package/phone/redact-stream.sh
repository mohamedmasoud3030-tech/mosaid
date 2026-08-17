#!/data/data/com.termux/files/usr/bin/bash
# Filter Mosaid product output so Telegram token and model API key never
# reach the on-disk logs. Secrets travel to awk through its inherited
# environment, not argv. This is defense in depth only: processes under
# the same Termux UID are not isolated from each other.
set -euo pipefail
umask 077

MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"

export PHASE14_REDACT_TELEGRAM="$(cat "$CONFIG_HOME/telegram.token" 2>/dev/null || true)"
export PHASE14_REDACT_MODEL="$(cat "$CONFIG_HOME/model.key" 2>/dev/null || true)"

exec awk '
function replace_literal(s, secret,    p) {
  if (secret == "") return s
  while ((p = index(s, secret)) > 0) {
    s = substr(s, 1, p-1) "[REDACTED]" substr(s, p+length(secret))
  }
  return s
}
{
  line = replace_literal($0, ENVIRON["PHASE14_REDACT_TELEGRAM"])
  line = replace_literal(line, ENVIRON["PHASE14_REDACT_MODEL"])
  gsub(/[0-9]{7,12}:[A-Za-z0-9_-]{20,}/, "[REDACTED_TELEGRAM_TOKEN]", line)
  gsub(/sk-[A-Za-z0-9_-]{12,}/, "[REDACTED_API_KEY]", line)
  print line
  fflush()
}'
