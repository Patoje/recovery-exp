#!/usr/bin/env bash
# Crea testigos en Nextcloud. Uso: witness.sh pre|post [RUN_DIR]
# Salida: <run>/witnesses_pre.json o <run>/witnesses_post.json.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
PHASE="${1:?uso: witness.sh pre|post [RUN_DIR]}"
RUN_DIR="${2:-$(current_run_dir)}"
"${ORIG[@]}" --profile tools run --rm --no-deps -T runner \
  python3 nc/witness/witness.py "$PHASE" --run-dir "/runs/$(basename "$RUN_DIR")"
