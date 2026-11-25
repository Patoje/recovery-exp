#!/usr/bin/env bash
# Línea base "contract": motor de contratos (engine/) con contracts/gitea.yaml.
#   1. engine verify --phase pre   (configuración tal como va a arrancar e integridad del snapshot)
#   2. scripts/restore.sh          (procedimiento de restauración de referencia, log en contract/restore.log)
#   3. engine verify --phase post  (todo lo demás y el reporte: contract.json, contract.md)
# Todas las llamadas a docker pasan por el PATH, así que el shim de fallas las ve igual que en B0 y B1.
#
# Uso: contract.sh [SNAPSHOT|latest] RUN_DIR      Código 0 solo si el contrato pasa.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
SNAPSHOT="${1:-latest}"
RUN_DIR="${2:?uso: contract.sh [SNAPSHOT] RUN_DIR}"
CONTRACT="${CONTRACT:-contracts/gitea.yaml}"
ENGINE="$ROOT/engine/bin/engine-darwin-arm64"
PROBE="$ROOT/engine/bin/engine-linux-arm64"
HOST_UID="$(id -u)"; HOST_GID="$(id -g)"; export HOST_UID HOST_GID
mkdir -p "$RUN_DIR/contract"

[ -x "$ENGINE" ] && [ -x "$PROBE" ] || { echo "[contract] falta el motor compilado: engine/build.sh" >&2; exit 2; }
now() { perl -MTime::HiRes=time -MPOSIX=strftime -e '$t=time; printf("%s.%03dZ\n", strftime("%Y-%m-%dT%H:%M:%S", gmtime($t)), ($t-int($t))*1000)'; }
verify() { "$ENGINE" verify --phase "$1" --contract "$CONTRACT" --run "$RUN_DIR" --root "$ROOT" --probe-bin "$PROBE"; }

t0="$(now)"; echo "[contract] fase pre $t0" >&2
verify pre; pre_rc=$?
t1="$(now)"; echo "[contract] restauración de referencia $t1" >&2
"$ROOT/scripts/restore.sh" "$SNAPSHOT" "$RUN_DIR" > "$RUN_DIR/contract/restore.log" 2>&1; restore_rc=$?
t2="$(now)"; echo "[contract] restore.sh salió con $restore_rc; fase post $t2" >&2
verify post; rc=$?
t3="$(now)"
cat > "$RUN_DIR/contract/timing.json" <<JSON
{"pre_started_at": "$t0", "restore_started_at": "$t1", "post_started_at": "$t2", "finished_at": "$t3",
 "pre_exit_code": $pre_rc, "restore_exit_code": $restore_rc, "post_exit_code": $rc}
JSON
echo "[contract] fin $t3: $( [ $rc -eq 0 ] && echo pass || echo fail ) ($RUN_DIR/contract.json)" >&2
exit "$rc"
