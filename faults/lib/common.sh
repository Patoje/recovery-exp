# Ayudas para inject y confirm de los escenarios. Se carga con `source`.
# Requiere scripts/lib.sh (ORIG, REST, log, json_str) y las variables FAULT_* del runner.
# shellcheck source=../../scripts/lib.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/lib.sh"
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

rest_gitea() { "${REST[@]}" exec -T gitea "$@" 2>/dev/null; }
orig_gitea() { "${ORIG[@]}" exec -T gitea "$@" 2>/dev/null; }

# Python en el runner del proyecto original (red original, /data montado ro).
orig_py() { "${ORIG[@]}" --profile tools run --rm --no-deps -T runner python3 "$@"; }

# Valor de clave en una sección de app.ini leído por stdin.
ini_get() { awk -F' *= *' -v sec="[$1]" -v key="$2" '/^\[/{s=$0} s==sec && $1==key{print $2}'; }

# Ramas en la base restaurada y refs en disco restaurado: "owner/repo branch sha".
rest_db_branches() {
  rest_sql "select u.lower_name||'/'||r.lower_name||' '||b.name||' '||b.commit_id from branch b
            join repository r on r.id=b.repo_id join \"user\" u on u.id=r.owner_id
            where not b.is_deleted order by 1"
}
rest_disk_branches() {
  rest_gitea sh -c 'cd /data/git/repositories && for d in */*.git; do
      git -c safe.directory="*" --git-dir "$d" for-each-ref --format="%(refname:strip=2) %(objectname)" refs/heads \
      | sed "s#^#${d%.git} #"; done' | sort
}

RESTIC_IMG="restic/restic@sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a"
RUNNER_IMG="recexp-runner:local"

# Comando de shell sobre un volumen, con la imagen de Restic (Alpine, fijada por digest).
vol_sh() { # volumen script
  docker run --rm -v "$1:/data" --entrypoint sh "$RESTIC_IMG" -c "$2"
}

# Python del runner sin red, con el repo en /work y la corrida en /run.
runner_py() { docker run --rm -v "$ROOT:/work:ro" -v "$RUN_DIR:/run" -w /work "$@"; }

json_field() { # archivo campo -> primer valor string
  grep -o "\"$2\": *\"[^\"]*\"" "$1" | head -1 | sed 's/.*: *"//; s/"$//'
}

# Éxito si el comando falla (para checks negativos).
fails() { ! "$@" >/dev/null 2>&1; }

sha256_stdin() { if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi | cut -d' ' -f1; }
