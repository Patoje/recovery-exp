#!/usr/bin/env bash
# Inventario de referencia de Nextcloud. Solo lectura: datos y config/ montados ro y sesión de base
# de datos read-only. Pensado para correr en modo mantenimiento (hook de backup.sh).
# Salida: <run>/inventory.json.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
RUN_DIR="${1:-$(current_run_dir)}"
"${ORIG[@]}" --profile tools run --rm --no-deps -T runner \
  python3 nc/witness/inventory.py --run-dir "/runs/$(basename "$RUN_DIR")"
