#!/usr/bin/env bash
# Carga el dataset determinista en el entorno original de Nextcloud. Salida: <run>/dataset.json.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
RUN_DIR="${1:-$(current_run_dir)}"
"${ORIG[@]}" --profile tools run --rm --no-deps -T runner \
  python3 nc/seed/seed.py --run-dir "/runs/$(basename "$RUN_DIR")"
