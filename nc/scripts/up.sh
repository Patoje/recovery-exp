#!/usr/bin/env bash
# Crea una corrida nueva y levanta el entorno original de Nextcloud desde cero:
# runner, PostgreSQL + Nextcloud (instalación con administrador) y repositorio de Restic.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [ -n "$(docker volume ls -q --filter label=com.docker.compose.project=ncexp)" ]; then
  echo "el entorno original ya tiene volúmenes: ejecutá nc/scripts/reset.sh antes de una corrida nueva" >&2
  exit 1
fi

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$ROOT/runs/$RUN_ID"
mkdir -p "$RUN_DIR"
echo "$RUN_ID" > "$ROOT/runs/CURRENT_NC"
log "corrida $RUN_ID (Nextcloud)"

log "construyendo runner"
"${ORIG[@]}" --profile tools build runner

# El entrypoint de la imagen instala Nextcloud en el primer arranque (variables NEXTCLOUD_ADMIN_* y
# POSTGRES_*): genera config.php con instanceid, passwordsalt y secret.
log "levantando postgres + nextcloud (instalación)"
"${ORIG[@]}" up -d --wait postgres nextcloud
installed=""
for _ in $(seq 1 120); do
  installed="$(occ ORIG status --output=json 2>/dev/null | grep -o '"installed":true' || true)"
  [ -n "$installed" ] && break
  sleep 2
done
[ -n "$installed" ] || { echo "up: Nextcloud no quedó instalado" >&2; exit 1; }
# Sin tareas de fondo por web (cron) ni chequeos de red: el entorno no tiene salida.
occ ORIG background:cron
occ ORIG config:system:set skeletondirectory --value=''
occ ORIG config:system:set has_internet_connection --type=boolean --value=false
occ ORIG config:system:set appstoreenabled --type=boolean --value=false

log "inicializando repositorio de Restic"
"${ORIG[@]}" --profile tools run --rm --no-deps restic init

restic_version="$("${ORIG[@]}" --profile tools run --rm --no-deps restic version | head -1)"
cat > "$RUN_DIR/run.json" <<EOF
{
  "run_id": $(json_str "$RUN_ID"),
  "app": "nextcloud",
  "env_version": $(json_str "$(cat "$NC/ENV_VERSION")"),
  "created_at": $(json_str "$(iso_ms "$(now_ms)")"),
  "incident_minutes": ${INCIDENT_MINUTES},
  "seed": ${SEED},
  "docker_version": $(json_str "$(docker version --format '{{.Server.Version}}')"),
  "compose_version": $(json_str "$(docker compose version --short)"),
  "restic_version": $(json_str "$restic_version"),
  "images": $(json_str "$("${ORIG[@]}" --profile tools config --images | sort | tr '\n' ' ')")
}
EOF
log "listo: $RUN_DIR"
