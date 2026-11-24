#!/usr/bin/env bash
# Línea base B1: ejecuta B0 completa y además verificaciones convencionales.
# Independiente de B0 y de los escenarios de falla: solo usa los artefactos de la corrida
# (runs/<RUN_ID>/*.json, secrets/), los valores declarados (declared.json, objectives.env)
# y el entorno (.env, compose/restore.yml).
#
# Uso: b1.sh [SNAPSHOT|latest] RUN_DIR
# Salida: <run>/b1.json y <run>/b1/ (log de B0 y datos crudos). Código 0 si todo pasa.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
SNAPSHOT="${1:-latest}"
RUN_DIR="${2:?uso: b1.sh [SNAPSHOT] RUN_DIR}"
RUN_ID="$(basename "$RUN_DIR")"
OUT="$RUN_DIR/b1"
RAW="$OUT/raw"
mkdir -p "$RAW"

ENVF="$ROOT/.env"
REST=(docker compose -f "$ROOT/compose/restore.yml" --env-file "$ENVF")
HOST_UID="$(id -u)"; HOST_GID="$(id -g)"; export HOST_UID HOST_GID
DB_USER="$(grep '^POSTGRES_USER=' "$ENVF" | cut -d= -f2-)"
DB_NAME="$(grep '^POSTGRES_DB=' "$ENVF" | cut -d= -f2-)"

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
echo "[b1] inicio $(now) corrida $RUN_ID" >&2
echo "{\"b1_started_at\": \"$(now)\"}" > "$RAW/times.json"

# --- 1. Antes de B0 (antes de cualquier migración): imagen y versión configuradas para la restauración.
"${REST[@]}" config --format json > "$RAW/restore_config.json" 2> "$RAW/restore_config.err"
image="$(docker run --rm -i --entrypoint python3 recexp-runner:local -c \
  'import json, sys; print(json.load(sys.stdin)["services"]["gitea"]["image"])' < "$RAW/restore_config.json" 2>/dev/null)"
printf '%s\n' "$image" > "$RAW/pre_image.txt"
if [ -n "$image" ]; then
  docker run --rm --entrypoint sh "$image" -c \
    'b=$(command -v gitea || find / -xdev -type f -name gitea -perm -u+x 2>/dev/null | head -1); exec "$b" --version' \
    > "$RAW/pre_version.txt" 2>&1
fi
echo "[b1] imagen configurada: $image ($(head -1 "$RAW/pre_version.txt" 2>/dev/null))" >&2

# --- 2. B0 completa, con su log.
echo "[b1] ejecutando B0" >&2
b0_rc=0
"$ROOT/scripts/b0.sh" "$SNAPSHOT" "$RUN_DIR" > "$OUT/b0.log" 2>&1 || b0_rc=$?
echo "$b0_rc" > "$RAW/b0_exit_code.txt"
echo "[b1] B0 salió con $b0_rc" >&2

# --- 3. Datos crudos del sistema restaurado (solo lectura).
COUNT_SQL="select coalesce(json_object_agg(t.table_name, (xpath('/row/c/text()',
  query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint
  order by t.table_name), '{}'::json) from information_schema.tables t
  where t.table_schema = 'public' and t.table_type = 'BASE TABLE'"
psql_ro() {
  "${REST[@]}" exec -T -e PGOPTIONS='-c default_transaction_read_only=on' postgres \
    psql -U "$DB_USER" -d "$DB_NAME" -At -v ON_ERROR_STOP=1 -c "$1"
}
psql_ro "$COUNT_SQL" > "$RAW/db_row_counts.json" 2> "$RAW/db_row_counts.err"
psql_ro "select u.lower_name || '/' || r.lower_name from repository r join \"user\" u on u.id = r.owner_id order by 1" \
  > "$RAW/db_repos.txt" 2> "$RAW/db_repos.err"
psql_ro "show server_encoding" > "$RAW/db_encoding.txt" 2> "$RAW/db_encoding.err"
"${REST[@]}" exec -T gitea sh -c 'cd /data/git/repositories && for d in */*.git; do
    [ -d "$d" ] || continue
    echo "## ${d%.git}"
    git -c safe.directory="*" --git-dir "$d" for-each-ref --format="%(refname) %(objectname)"
  done' > "$RAW/disk_refs.txt" 2> "$RAW/disk_refs.err"
"${REST[@]}" exec -T gitea sh -c \
  'awk -F" *= *" "/^\[/{s=\$0} s==\"[security]\" && \$1==\"SECRET_KEY\"{printf \"%s\", \$2}" /data/gitea/conf/app.ini | sha256sum | cut -d" " -f1' \
  > "$RAW/secret_key_sha256.txt" 2> "$RAW/secret_key_sha256.err"
cid="$("${REST[@]}" ps -q gitea 2>/dev/null | head -1)"
[ -n "$cid" ] && docker inspect --format '{{.Config.Image}}' "$cid" > "$RAW/running_image.txt" 2>&1

# --- 4. Verificaciones (en el runner, dentro de la red de restauración) y reporte.
"${REST[@]}" --profile tools run --rm --no-deps -T runner \
  python3 /work/baselines/b1/checks.py "/runs/$RUN_ID" /work/objectives.env
rc=$?
echo "[b1] fin $(now): $( [ $rc -eq 0 ] && echo pass || echo fail ) ($RUN_DIR/b1.json)" >&2
exit "$rc"
