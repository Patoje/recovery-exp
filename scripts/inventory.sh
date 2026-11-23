#!/usr/bin/env bash
# Inventario de referencia. Solo lectura: /data montado ro y sesión de base de datos read-only.
# Pensado para correr con Gitea detenido (hook de backup.sh). Salida: <run>/inventory.json.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
RUN_DIR="${1:-$(current_run_dir)}"
"${ORIG[@]}" --profile tools run --rm --no-deps -T runner \
  python3 witness/inventory.py --run-dir "/runs/$(basename "$RUN_DIR")"
