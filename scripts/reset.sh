#!/usr/bin/env bash
# Vuelve a un estado limpio: borra contenedores, redes y volúmenes de ambos proyectos
# (incluido el repositorio de Restic) y el puntero a la corrida actual.
# No borra los resultados en runs/<RUN_ID>/ (ver `make clean-runs`).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ -f "$ROOT/.env" ] || cp "$ROOT/.env.example" "$ROOT/.env"
# shellcheck source=lib.sh
. "$ROOT/scripts/lib.sh"

log "borrando proyecto de restauración"
"${REST[@]}" --profile tools down -v --remove-orphans
log "borrando proyecto original"
docker compose -f "$ROOT/compose/original.yml" -f "$ROOT/compose/original.minio.yml" --env-file "$ROOT/.env" \
  --profile minio --profile tools down -v --remove-orphans
rm -f "$ROOT/runs/CURRENT"

left="$(docker volume ls -q --filter label=com.docker.compose.project=recexp; docker volume ls -q --filter label=com.docker.compose.project=recexp-restore)"
if [ -n "$left" ]; then
  echo "quedaron volúmenes: $left" >&2
  exit 1
fi
log "estado limpio"
