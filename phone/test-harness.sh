#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
TESTS="$P0_HOME/tests"
PLAN="$P0_HOME/test-plan.json"
ACTIVE="$TESTS/active.json"
mkdir -p "$TESTS"

usage() {
  cat <<'EOF'
Usage:
  test-harness.sh list
  test-harness.sh arm <scenario-id>
  test-harness.sh note <text>
  test-harness.sh status
  test-harness.sh finalize
  test-harness.sh abort
EOF
}

max_seen_message_id() {
  grep -h 'Qualification message handled' "$P0_HOME"/logs/* 2>/dev/null \
    | grep -o 'message_id=[0-9]*' | cut -d= -f2 | sort -n | tail -n1 || true
}
error_count() {
  { grep -hEi '(^|[[:space:]])(ERR|ERROR|FATAL|panic)([[:space:]:]|$)' "$P0_HOME"/logs/* 2>/dev/null || true; } | wc -l | awk '{print $1}'
}

cmd="${1:-}"
case "$cmd" in
  list)
    jq -r '.scenarios[] | [.id, (.duration_seconds|tostring), (.expected_echo|tostring), .action] | @tsv' "$PLAN"
    ;;
  arm)
    id="${2:-}"
    [[ "$id" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "Invalid scenario id" >&2; exit 2; }
    scenario="$(jq -c --arg id "$id" '.scenarios[] | select(.id==$id)' "$PLAN")"
    [[ -n "$scenario" ]] || { echo "Unknown scenario: $id" >&2; exit 2; }
    if [[ -f "$ACTIVE" ]]; then
      old_end="$(jq -r '.end_epoch // 0' "$ACTIVE" 2>/dev/null || echo 0)"
      if (( $(date +%s) <= old_end )); then echo "Another test is active" >&2; exit 73; fi
    fi
    now="$(date +%s)"
    duration="$(jq -r '.duration_seconds' <<<"$scenario")"
    end=$((now+duration))
    restart="$(cat "$P0_HOME/runtime/restart.count" 2>/dev/null || echo 0)"
    baseline_mid="$(max_seen_message_id)"; baseline_mid="${baseline_mid:-0}"
    baseline_errors="$(error_count)"
    dir="$TESTS/$id"
    mkdir -p "$dir"
    jq -n --argjson scenario_obj "$scenario" --arg id "$id" --arg started_at "$(date -u +%FT%TZ)" \
      --argjson start_epoch "$now" --argjson end_epoch "$end" --argjson baseline_restart "${restart:-0}" \
      --argjson baseline_message_id "$baseline_mid" --argjson baseline_errors "$baseline_errors" \
      '{scenario:$id,scenario_definition:$scenario_obj,started_at:$started_at,start_epoch:$start_epoch,end_epoch:$end,baseline_restart_count:$baseline_restart,baseline_message_id:$baseline_message_id,baseline_error_count:$baseline_errors}' > "$ACTIVE"
    cp "$ACTIVE" "$dir/test.json"
    printf '# Manual notes for %s\n\n- Armed: %s\n- Required action: %s\n' "$id" "$(date -u +%FT%TZ)" "$(jq -r '.action' <<<"$scenario")" > "$dir/notes.md"
    chmod 600 "$ACTIVE" "$dir/test.json" "$dir/notes.md"
    echo "Armed $id until $(date -d "@$end" -u +%FT%TZ 2>/dev/null || echo "$end")"
    echo "Action: $(jq -r '.action' <<<"$scenario")"
    ;;
  note)
    shift || true
    [[ -f "$ACTIVE" ]] || { echo "No active test" >&2; exit 2; }
    id="$(jq -r '.scenario' "$ACTIVE")"
    printf -- '- %s — %s\n' "$(date -u +%FT%TZ)" "$*" >> "$TESTS/$id/notes.md"
    ;;
  status)
    if [[ ! -f "$ACTIVE" ]]; then echo "No active test"; exit 0; fi
    jq . "$ACTIVE"
    [[ -f "$P0_HOME/runtime/health.json" ]] && jq . "$P0_HOME/runtime/health.json"
    ;;
  finalize)
    [[ -f "$ACTIVE" ]] || { echo "No active test" >&2; exit 2; }
    bash "$P0_HOME/scripts/collect-results.sh" "$ACTIVE"
    id="$(jq -r '.scenario' "$ACTIVE")"
    mv "$ACTIVE" "$TESTS/$id/completed-test.json"
    echo "Finalized $id"
    ;;
  abort)
    [[ -f "$ACTIVE" ]] || exit 0
    id="$(jq -r '.scenario' "$ACTIVE")"
    mv "$ACTIVE" "$TESTS/$id/aborted-test.json"
    echo "Aborted $id"
    ;;
  *) usage; exit 2 ;;
esac
