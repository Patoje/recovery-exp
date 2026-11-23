#!/usr/bin/env bash
# Procedimiento de backup de referencia:
#   detener Gitea -> pg_dump -> restic backup (dump + /data completo) -> arrancar Gitea.
# Excluye solo /data/gitea/log y /data/gitea/indexers.
# Si INVENTORY_HOOK está definido, se ejecuta con Gitea detenido, antes de pg_dump.
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
GITEA_STOPPED=0
T_START="$(now_ms)"

finish() {
  local rc=$?
  if [ "$GITEA_STOPPED" = "1" ]; then
    log "el backup falló con Gitea detenido: volviendo a arrancarlo"
    step start_gitea_after_failure "${ORIG[@]}" up -d --wait gitea || true
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

stop_gitea() {
  "${ORIG[@]}" stop gitea || return
  GITEA_STOPPED=1
}

pg_dump_step() {
  "${ORIG[@]}" exec -T postgres sh -ec \
    'rm -f /dump/gitea.dump; pg_dump -U "$POSTGRES_USER" -Fc -f /dump/gitea.dump "$POSTGRES_DB"; ls -l /dump/gitea.dump'
}

restic_backup() {
  local out="$RUN_DIR/restic-backup.jsonl"
  "${ORIG[@]}" --profile tools run --rm --no-deps restic backup --json \
    --host gitea-original --tag "run=$RUN_ID" \
    --exclude /data/gitea/log --exclude /data/gitea/indexers \
    /dump /data > "$out" || return
  SNAP_ID="$(grep '"message_type":"summary"' "$out" | grep -o '"snapshot_id":"[0-9a-f]*"' | cut -d'"' -f4)"
  [ -n "$SNAP_ID" ] || { STEP_DETAIL="restic no informó snapshot_id"; return 1; }
  SNAP_TIME="$("${ORIG[@]}" --profile tools run --rm --no-deps restic snapshots --json "$SNAP_ID" \
    | grep -o '"time":"[^"]*"' | head -1 | cut -d'"' -f4)" || return
  STEP_DETAIL="snapshot $SNAP_ID"
}

start_gitea() {
  "${ORIG[@]}" up -d --wait gitea || return
  GITEA_STOPPED=0
}

step stop_gitea stop_gitea

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
step start_gitea start_gitea
