# Funciones comunes. Se carga con `source`; compatible con bash 3.2.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

[ -f "$ROOT/.env" ] || { echo "falta .env (ejecutá 'make reset')" >&2; exit 2; }
set -a
# shellcheck disable=SC1091
. "$ROOT/.env"
set +a

HOST_UID="$(id -u)"
HOST_GID="$(id -g)"
export HOST_UID HOST_GID

ORIG=(docker compose -f "$ROOT/compose/original.yml" --env-file "$ROOT/.env")
if [ "${MINIO:-0}" = "1" ]; then
  ORIG=(docker compose -f "$ROOT/compose/original.yml" -f "$ROOT/compose/original.minio.yml" --env-file "$ROOT/.env" --profile minio)
fi
REST=(docker compose -f "$ROOT/compose/restore.yml" --env-file "$ROOT/.env")

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }

# Corrida actual: runs/CURRENT contiene el RUN_ID creado por `make up`.
current_run_dir() {
  [ -f "$ROOT/runs/CURRENT" ] || { echo "no hay corrida actual (ejecutá 'make up')" >&2; return 2; }
  echo "$ROOT/runs/$(cat "$ROOT/runs/CURRENT")"
}

# Milisegundos desde epoch. GNU date en Linux; perl como respaldo (macOS).
now_ms() {
  local t
  t="$(date +%s%N 2>/dev/null || true)"
  case "$t" in
    ''|*N*) perl -MTime::HiRes=time -e 'printf("%d\n", time()*1000)' ;;
    *) echo $((t / 1000000)) ;;
  esac
}

# ms desde epoch -> ISO 8601 UTC con milisegundos.
iso_ms() {
  local ms="$1" s d
  s=$((ms / 1000))
  d="$(date -u -d "@$s" +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -r "$s" +%Y-%m-%dT%H:%M:%S)"
  printf '%s.%03dZ' "$d" $((ms % 1000))
}

json_str() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/}"
  s="${s//$'\t'/ }"
  printf '"%s"' "$s"
}

# Registro de pasos. STEPS acumula objetos JSON separados por coma.
STEPS=""
STEP_DETAIL=""
add_step() { # name status t0 t1 exit_code [detail]
  local obj
  obj="{\"name\":$(json_str "$1"),\"status\":$(json_str "$2"),\"started_at\":$(json_str "$(iso_ms "$3")"),\"finished_at\":$(json_str "$(iso_ms "$4")"),\"duration_ms\":$(($4 - $3)),\"exit_code\":$5"
  [ -n "${6:-}" ] && obj="$obj,\"detail\":$(json_str "$6")"
  obj="$obj}"
  if [ -z "$STEPS" ]; then STEPS="$obj"; else STEPS="$STEPS,$obj"; fi
}

# step NAME CMD...: ejecuta CMD, registra el paso y devuelve su código de salida.
# CMD puede dejar texto en STEP_DETAIL. Las funciones usadas como CMD deben
# encadenar sus comandos con && o `|| return`, porque errexit no aplica dentro de `if`.
step() {
  local name="$1" t0 t1 rc=0
  shift
  STEP_DETAIL=""
  log "paso: $name"
  t0="$(now_ms)"
  "$@" || rc=$?
  t1="$(now_ms)"
  if [ "$rc" -eq 0 ]; then
    add_step "$name" ok "$t0" "$t1" 0 "$STEP_DETAIL"
  else
    add_step "$name" failed "$t0" "$t1" "$rc" "$STEP_DETAIL"
    log "falló: $name (código $rc)"
  fi
  return "$rc"
}
