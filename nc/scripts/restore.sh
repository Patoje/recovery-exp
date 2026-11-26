#!/usr/bin/env bash
# Procedimiento de restauración de referencia de Nextcloud, en el proyecto aislado ncexp-restore:
#   volúmenes nuevos -> restic restore -> pg_restore --exit-on-error -> arrancar Nextcloud
#   -> desactivar el modo mantenimiento (el backup se tomó con el modo activado)
#   -> occ maintenance:data-fingerprint (como indica la documentación de Nextcloud al restaurar).
# Uso: restore.sh [SNAPSHOT|latest] [RUN_DIR]
# Salida: <run>/restore.json. Código de salida distinto de 0 si falla cualquier paso.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SNAPSHOT="${1:-latest}"
RUN_DIR="${2:-$(current_run_dir)}"
OUT="$RUN_DIR/restore.json"
SNAP_ID=""
T_START="$(now_ms)"
T_RESTORE_END=""
T_FIRST_START=""

finish() {
  local rc=$? status t_end
  t_end="$(now_ms)"
  status=ok; [ "$rc" -eq 0 ] || status=failed
  cat > "$OUT" <<EOF
{
  "run_id": $(json_str "$(basename "$RUN_DIR")"),
  "status": "$status",
  "exit_code": $rc,
  "snapshot_requested": $(json_str "$SNAPSHOT"),
  "snapshot_id": $(json_str "$SNAP_ID"),
  "restore_started_at": $(json_str "$(iso_ms "$T_START")"),
  "restore_finished_at": $([ -n "$T_RESTORE_END" ] && json_str "$(iso_ms "$T_RESTORE_END")" || echo null),
  "first_start_at": $([ -n "$T_FIRST_START" ] && json_str "$(iso_ms "$T_FIRST_START")" || echo null),
  "finished_at": $(json_str "$(iso_ms "$t_end")"),
  "steps": [$STEPS]
}
EOF
  log "restore.json: $status"
  exit "$rc"
}
trap finish EXIT

RESTIC=("${REST[@]}" --profile tools run --rm --no-deps restic --no-lock --no-cache)

fresh_volumes() {
  "${REST[@]}" --profile tools down -v --remove-orphans
}

resolve_snapshot() {
  local sel=("$SNAPSHOT")
  [ "$SNAPSHOT" = "latest" ] && sel=(--latest 1)
  SNAP_ID="$("${RESTIC[@]}" snapshots --json "${sel[@]}" | grep -o '"id":"[0-9a-f]\{64\}"' | tail -1 | cut -d'"' -f4)"
  [ -n "$SNAP_ID" ] || { STEP_DETAIL="snapshot no encontrado: $SNAPSHOT"; return 1; }
  STEP_DETAIL="$SNAP_ID"
}

restic_restore() {
  "${RESTIC[@]}" restore "$SNAP_ID" --target /
}

start_postgres() {
  "${REST[@]}" up -d --wait postgres
}

pg_restore_step() {
  "${REST[@]}" exec -T postgres sh -ec \
    'pg_restore --exit-on-error -U "$POSTGRES_USER" -d "$POSTGRES_DB" /dump/nextcloud.dump'
}

start_nextcloud() {
  "${REST[@]}" up -d --wait nextcloud
}

maintenance_off() {
  occ REST maintenance:mode --off
}

data_fingerprint() {
  occ REST maintenance:data-fingerprint
}

step fresh_volumes fresh_volumes
step resolve_snapshot resolve_snapshot
step restic_restore restic_restore
step start_postgres start_postgres
step pg_restore pg_restore_step
T_RESTORE_END="$(now_ms)"
step start_nextcloud start_nextcloud
T_FIRST_START="$(now_ms)"
step maintenance_off maintenance_off
step data_fingerprint data_fingerprint
