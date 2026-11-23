#!/usr/bin/env bash
# Corrida completa desde un estado limpio (ejecutar `make reset` antes):
#   up -> seed -> testigos previos -> backup (+ inventario) -> testigos posteriores -> B0.
# Salida: runs/<RUN_ID>/ con todos los JSON.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

S="$ROOT/scripts"
"$S/up.sh"
RUN_DIR="$(current_run_dir)"
"$S/seed.sh" "$RUN_DIR"
"$S/witness.sh" pre "$RUN_DIR"
INVENTORY_HOOK="$S/inventory.sh" "$S/backup.sh" "$RUN_DIR"
"$S/witness.sh" post "$RUN_DIR"
"$S/b0.sh" latest "$RUN_DIR"
log "corrida completa: $RUN_DIR"
ls -1 "$RUN_DIR"
