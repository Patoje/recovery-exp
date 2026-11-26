#!/usr/bin/env bash
# Vuelve a un estado limpio en el entorno de Nextcloud: borra contenedores, redes y volúmenes de
# ambos proyectos (incluido el repositorio de Restic) y el puntero a la corrida actual.
# No borra los resultados en runs/<RUN_ID>/.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
[ -f "$ROOT/nc/.env" ] || cp "$ROOT/nc/.env.example" "$ROOT/nc/.env"
# shellcheck source=lib.sh
. "$ROOT/nc/scripts/lib.sh"

log "borrando proyecto de restauración (Nextcloud)"
"${REST[@]}" --profile tools down -v --remove-orphans
log "borrando proyecto original (Nextcloud)"
"${ORIG[@]}" --profile tools down -v --remove-orphans
rm -f "$ROOT/runs/CURRENT_NC"

left="$(docker volume ls -q --filter label=com.docker.compose.project=ncexp; docker volume ls -q --filter label=com.docker.compose.project=ncexp-restore)"
if [ -n "$left" ]; then
  echo "quedaron volúmenes: $left" >&2
  exit 1
fi
log "estado limpio"
