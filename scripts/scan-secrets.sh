#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT"

patterns=(
  '-----BEGIN (RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----'
  '(^|[^A-Za-z0-9_])sk-[A-Za-z0-9_-]{20,}'
  '(^|[^A-Za-z0-9_])sk-or-v1-[A-Za-z0-9_-]{20,}'
  '(^|[^A-Za-z0-9_])[0-9]{7,12}:[A-Za-z0-9_-]{30,}'
  'xox[baprs]-[A-Za-z0-9-]{20,}'
  'gh[pousr]_[A-Za-z0-9]{30,}'
)

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  mapfile -d '' files < <(git ls-files -z --cached --others --exclude-standard)
else
  mapfile -d '' files < <(find . -type f -not -path './evidence/artifacts/*' -not -path './release/*' -print0)
fi

found=0
for file in "${files[@]}"; do
  [[ -f "$file" ]] || continue
  case "$file" in
    evidence/artifacts/*|release/*|*.png|*.jpg|*.zip|*.gz|*.tar|*.so|*.bin) continue ;;
  esac
  for pattern in "${patterns[@]}"; do
    if LC_ALL=C grep -IEn "$pattern" "$file" >/tmp/phase0-secret-scan.$$ 2>/dev/null; then
      echo "Potential secret in $file (pattern redacted)" >&2
      sed -E 's/:.*/:[REDACTED]/' /tmp/phase0-secret-scan.$$ >&2 || true
      found=1
    fi
  done
done
rm -f /tmp/phase0-secret-scan.$$

if (( found )); then
  echo "Secret scan: FAIL" >&2
  exit 1
fi
echo "Secret scan: PASS"
