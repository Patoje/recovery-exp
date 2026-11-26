#!/usr/bin/env bash
# Línea base B0 de Nextcloud: la B0 de Gitea (scripts/b0.sh) adaptada lo mínimo.
# Exactamente estos pasos, en orden; se detiene en la primera falla:
#   1. restic check --read-data
#   2. restaurar con el procedimiento de referencia (nc/scripts/restore.sh)
#   3. docker compose up hasta que pasen los healthchecks
#   4. GET /index.php/login responde 200            (Gitea: GET /)
#   5. GET /status.php responde 200                 (Gitea: GET /api/v1/version)
#   6. login por API con un usuario de prueba       (OCS /cloud/user)
#   7. el listado de archivos devuelve al menos uno (WebDAV PROPFIND; Gitea: listado de repositorios)
#   8. descarga de un archivo del usuario           (WebDAV GET; Gitea: git clone)
# Uso: b0.sh [SNAPSHOT|latest] [RUN_DIR]
# Salida: <run>/b0.json; código 0 si todo pasa, distinto de 0 si algo falla.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SNAPSHOT="${1:-latest}"
RUN_DIR="${2:-$(current_run_dir)}"
OUT="$RUN_DIR/b0.json"
T_START="$(now_ms)"
RUNNER=("${REST[@]}" --profile tools run --rm --no-deps -T runner)

restic_check() {
  "${REST[@]}" --profile tools run --rm --no-deps restic --no-lock --no-cache check --read-data
}

restore() {
  "$NC/scripts/restore.sh" "$SNAPSHOT" "$RUN_DIR"
}

compose_up_healthy() {
  "${REST[@]}" up -d --wait --wait-timeout 300
}

expect_code() { # code path [curl args...]
  local want="$1" path="$2" code
  shift 2
  code="$("${RUNNER[@]}" curl -s -o /dev/null -w '%{http_code}' "$@" "http://nextcloud$path")" || return
  STEP_DETAIL="HTTP $code"
  [ "$code" = "$want" ]
}

http_login_page() { expect_code 200 /index.php/login; }
http_status() { expect_code 200 /status.php; }
api_login() { expect_code 200 "/ocs/v2.php/cloud/user?format=json" -u "$B0_USER:$B0_PASSWORD" -H "OCS-APIRequest: true"; }

file_list() {
  local n
  n="$("${RUNNER[@]}" sh -c 'curl -fsS -X PROPFIND -H "Depth: 1" -u "$B0_USER:$B0_PASSWORD" \
        "http://nextcloud/remote.php/dav/files/$B0_USER/" | grep -o "<d:response>" | wc -l')" || return
  n=$((n - 1))  # la primera respuesta es la carpeta raíz
  STEP_DETAIL="$n entradas en la raíz de $B0_USER"
  [ "$n" -ge 1 ]
}

file_download() {
  local sum
  sum="$("${RUNNER[@]}" sh -c 'curl -fsS -u "$B0_USER:$B0_PASSWORD" \
        "http://nextcloud/remote.php/dav/files/$B0_USER/Documents/documents-00.txt" | sha256sum | cut -d" " -f1')" || return
  STEP_DETAIL="Documents/documents-00.txt sha256 $sum"
  [ -n "$sum" ] && [ "$sum" != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]
}

NAMES="restic_check restore compose_up_healthy http_login_page http_status api_login file_list file_download"
RC=0
FAILED=""
for name in $NAMES; do
  if [ -n "$FAILED" ]; then
    now="$(now_ms)"
    add_step "$name" skipped "$now" "$now" 0 "omitido: falló $FAILED"
    continue
  fi
  step "$name" "$name" || { RC=$?; FAILED="$name"; }
done

T_END="$(now_ms)"
STATUS=pass; [ "$RC" -eq 0 ] || STATUS=fail
cat > "$OUT" <<EOF
{
  "baseline": "B0",
  "app": "nextcloud",
  "run_id": $(json_str "$(basename "$RUN_DIR")"),
  "snapshot_requested": $(json_str "$SNAPSHOT"),
  "result": "$STATUS",
  "exit_code": $RC,
  "started_at": $(json_str "$(iso_ms "$T_START")"),
  "finished_at": $(json_str "$(iso_ms "$T_END")"),
  "duration_ms": $((T_END - T_START)),
  "steps": [$STEPS]
}
EOF
log "B0 (Nextcloud): $STATUS ($OUT)"
exit "$RC"
