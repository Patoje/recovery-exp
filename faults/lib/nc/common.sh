# Ayudas para inject y confirm de los escenarios de Nextcloud. Se carga con `source`.
# Copia de faults/lib/common.sh (Gitea) con el entorno de nc/. Requiere las variables FAULT_* del runner.
# shellcheck source=../../../nc/scripts/lib.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/nc/scripts/lib.sh"
set +e

CONFIRM_FAILS=0

# check "descripción" cmd...: condición obligatoria de confirm.
check() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then echo "[ok] $desc"; else echo "[fail] $desc"; CONFIRM_FAILS=$((CONFIRM_FAILS + 1)); fi
}
# observe "descripción" valor: dato registrado, no condiciona confirm.
observe() { echo "[obs] $1: $2"; }
confirm_done() { [ "$CONFIRM_FAILS" -eq 0 ]; }

# Restic sobre el repositorio original, sin lock (solo lectura).
orig_restic() { "${ORIG[@]}" --profile tools run --rm --no-deps -T restic --no-lock "$@" 2>/dev/null; }
snap_ls() { orig_restic ls latest "$@"; }

# SQL en la base restaurada / original.
rest_sql() { "${REST[@]}" exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"' _ "$1" 2>/dev/null; }
orig_sql() { "${ORIG[@]}" exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"' _ "$1" 2>/dev/null; }

rest_nc() { "${REST[@]}" exec -T nextcloud "$@" 2>/dev/null; }
orig_nc() { "${ORIG[@]}" exec -T nextcloud "$@" 2>/dev/null; }

# curl desde el runner de la restauración (red restaurada).
rest_curl() { "${REST[@]}" --profile tools run --rm --no-deps -T runner curl "$@" 2>/dev/null; }

RESTIC_IMG="restic/restic@sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a"
RUNNER_IMG="recexp-runner:local"

# Comando de shell sobre un volumen, con la imagen de Restic (Alpine, fijada por digest).
vol_sh() { # volumen script
  docker run --rm -v "$1:/data" --entrypoint sh "$RESTIC_IMG" -c "$2"
}

json_field() { # archivo campo -> primer valor string
  grep -o "\"$2\": *\"[^\"]*\"" "$1" | head -1 | sed 's/.*: *"//; s/"$//'
}

# Éxito si el comando falla (para checks negativos).
fails() { ! "$@" >/dev/null 2>&1; }
