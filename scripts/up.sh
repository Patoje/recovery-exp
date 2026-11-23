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
