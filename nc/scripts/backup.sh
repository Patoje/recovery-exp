#!/usr/bin/env bash
# Procedimiento de backup de referencia de Nextcloud:
#   modo mantenimiento activado -> pg_dump -> restic backup (dump + datos + config/) -> modo mantenimiento desactivado.
# Si INVENTORY_HOOK está definido, se ejecuta en modo mantenimiento, antes de pg_dump.
# El hook es del orquestador, no del procedimiento: su tiempo se registra aparte.
# Salida: <run>/backup.json. Código de salida distinto de 0 si falla cualquier paso.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

RUN_DIR="${1:-$(current_run_dir)}"
OUT="$RUN_DIR/backup.json"
RUN_ID="$(basename "$RUN_DIR")"
SNAP_ID=""
SNAP_TIME=""
HOOK_JSON="null"
IN_MAINTENANCE=0
T_START="$(now_ms)"

finish() {
  local rc=$?
  if [ "$IN_MAINTENANCE" = "1" ]; then
    log "el backup falló en modo mantenimiento: desactivándolo"
    step maintenance_off_after_failure occ ORIG maintenance:mode --off || true
  fi
  local t_end status
  t_end="$(now_ms)"
  status=ok; [ "$rc" -eq 0 ] || status=failed
  cat > "$OUT" <<EOF
{
  "run_id": $(json_str "$RUN_ID"),
  "status": "$status",
  "exit_code": $rc,
  "started_at": $(json_str "$(iso_ms "$T_START")"),
  "finished_at": $(json_str "$(iso_ms "$t_end")"),
  "duration_ms": $((t_end - T_START)),
  "snapshot": {"id": $(json_str "$SNAP_ID"), "time": $(json_str "$SNAP_TIME")},
  "steps": [$STEPS],
  "inventory_hook": $HOOK_JSON
}
EOF
  log "backup.json: $status"
  exit "$rc"
}
trap finish EXIT

maintenance_on() {
  occ ORIG maintenance:mode --on || return
  IN_MAINTENANCE=1
}

pg_dump_step() {
  "${ORIG[@]}" exec -T postgres sh -ec \
    'rm -f /dump/nextcloud.dump; pg_dump -U "$POSTGRES_USER" -Fc -f /dump/nextcloud.dump "$POSTGRES_DB"; ls -l /dump/nextcloud.dump'
}

restic_backup() {
  local out="$RUN_DIR/restic-backup.jsonl"
  "${ORIG[@]}" --profile tools run --rm --no-deps restic backup --json \
    --host nextcloud-original --tag "run=$RUN_ID" \
    /dump /data /config > "$out" || return
  SNAP_ID="$(grep '"message_type":"summary"' "$out" | grep -o '"snapshot_id":"[0-9a-f]*"' | cut -d'"' -f4)"
  [ -n "$SNAP_ID" ] || { STEP_DETAIL="restic no informó snapshot_id"; return 1; }
  SNAP_TIME="$("${ORIG[@]}" --profile tools run --rm --no-deps restic snapshots --json "$SNAP_ID" \
    | grep -o '"time":"[^"]*"' | head -1 | cut -d'"' -f4)" || return
  STEP_DETAIL="snapshot $SNAP_ID"
}

maintenance_off() {
  occ ORIG maintenance:mode --off || return
  IN_MAINTENANCE=0
}

step maintenance_on maintenance_on

if [ -n "${INVENTORY_HOOK:-}" ]; then
  h0="$(now_ms)"; hrc=0
  log "hook de inventario: $INVENTORY_HOOK"
  $INVENTORY_HOOK "$RUN_DIR" || hrc=$?
  h1="$(now_ms)"
  HOOK_JSON="{\"command\":$(json_str "$INVENTORY_HOOK"),\"exit_code\":$hrc,\"started_at\":$(json_str "$(iso_ms "$h0")"),\"duration_ms\":$((h1 - h0))}"
  [ "$hrc" -eq 0 ] || exit "$hrc"
fi

step pg_dump pg_dump_step
step restic_backup restic_backup
step maintenance_off maintenance_off
