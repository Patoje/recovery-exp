#!/usr/bin/env bash
# Crea una corrida nueva y levanta el entorno original desde cero:
# runner, PostgreSQL + Gitea, usuario administrador y repositorio de Restic.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [ -n "$(docker volume ls -q --filter label=com.docker.compose.project=recexp)" ]; then
  echo "el entorno original ya tiene volúmenes: ejecutá 'make reset' antes de una corrida nueva" >&2
  exit 1
fi

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$ROOT/runs/$RUN_ID"
mkdir -p "$RUN_DIR"
echo "$RUN_ID" > "$ROOT/runs/CURRENT"
log "corrida $RUN_ID"

log "construyendo runner"
"${ORIG[@]}" --profile tools build runner

log "levantando postgres + gitea"
"${ORIG[@]}" up -d --wait postgres gitea

# Como una instalación normal: SECRET_KEY aleatoria, generada una sola vez y escrita solo en app.ini.
# Con INSTALL_LOCK=true Gitea 28.4.0 deja SECRET_KEY vacía; INTERNAL_TOKEN, LFS_JWT_SECRET,
# [oauth2] JWT_SECRET y jwt/private.pem sí los genera en el primer arranque y quedan en /data.
log "generando SECRET_KEY en app.ini"
secret_key="$("${ORIG[@]}" exec -T -u git gitea gitea generate secret SECRET_KEY)"
[ -n "$secret_key" ] || { echo "up: no se pudo generar SECRET_KEY" >&2; exit 1; }
"${ORIG[@]}" exec -T -u git gitea sh -c 'f=/data/gitea/conf/app.ini
  awk -v k="$1" '"'"'/^\[/{s=$0} s=="[security]" && $1=="SECRET_KEY"{print "SECRET_KEY = " k; next} {print}'"'"' "$f" > "$f.new" &&
  cat "$f.new" > "$f" && rm "$f.new"' sh "$secret_key"
"${ORIG[@]}" restart gitea
"${ORIG[@]}" up -d --wait gitea
in_ini="$("${ORIG[@]}" exec -T gitea awk -F' *= *' '/^\[/{s=$0} s=="[security]" && $1=="SECRET_KEY"{printf "%s", $2}' /data/gitea/conf/app.ini)"
[ "$in_ini" = "$secret_key" ] || { echo "up: SECRET_KEY no quedó en app.ini" >&2; exit 1; }
unset secret_key in_ini

log "creando administrador"
"${ORIG[@]}" exec -T -u git gitea gitea admin user create --admin \
  --username "$GITEA_ADMIN_USER" --password "$GITEA_ADMIN_PASSWORD" \
  --email "$GITEA_ADMIN_EMAIL" --must-change-password=false

log "inicializando repositorio de Restic"
"${ORIG[@]}" --profile tools run --rm --no-deps restic init

restic_version="$("${ORIG[@]}" --profile tools run --rm --no-deps restic version | head -1)"
cat > "$RUN_DIR/run.json" <<EOF
{
  "run_id": $(json_str "$RUN_ID"),
  "env_version": $(json_str "$(cat "$ROOT/ENV_VERSION")"),
  "created_at": $(json_str "$(iso_ms "$(now_ms)")"),
  "incident_minutes": ${INCIDENT_MINUTES},
  "seed": ${SEED},
  "minio": $([ "${MINIO:-0}" = "1" ] && echo true || echo false),
  "docker_version": $(json_str "$(docker version --format '{{.Server.Version}}')"),
  "compose_version": $(json_str "$(docker compose version --short)"),
  "restic_version": $(json_str "$restic_version"),
  "images": $(json_str "$("${ORIG[@]}" --profile tools config --images | sort | tr '\n' ' ')")
}
EOF
log "listo: $RUN_DIR"
