#!/usr/bin/env bash
# Chequeo de suspensión del host sobre una o más carpetas de resultados (results/faults/<esc>/<lote>-rNN).
# Guarda las líneas Sleep de `pmset -g log` en evidence/pmset_sleep.txt y actualiza result.json.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
for res in "$@"; do
  res="${res%/}"; res="${res#"$ROOT"/}"
  mkdir -p "$res/evidence"
  if command -v pmset >/dev/null 2>&1; then
    pmset -g log | awk '$4=="Sleep"' > "$res/evidence/pmset_sleep.txt"
  else
    echo "# pmset no disponible en este host" > "$res/evidence/pmset_sleep.txt"
  fi
  docker run --rm --user "$(id -u):$(id -g)" -v "$ROOT:/work" -w /work recexp-runner:local \
    python3 faults/lib/suspension_check.py "$res"
done
