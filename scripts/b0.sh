#!/usr/bin/env bash
# Línea base B0. Exactamente estos pasos, en orden; se detiene en la primera falla:
#   1. restic check --read-data
#   2. restaurar con el procedimiento de referencia (restore.sh)
#   3. docker compose up hasta que pasen los healthchecks
#   4. GET / responde 200
#   5. GET /api/v1/version responde 200
#   6. login por API con un usuario de prueba sin 2FA
#   7. el listado de repositorios devuelve al menos uno
#   8. git clone de un repositorio
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
  "$ROOT/scripts/restore.sh" "$SNAPSHOT" "$RUN_DIR"
}

compose_up_healthy() {
  "${REST[@]}" up -d --wait --wait-timeout 300
}

expect_200() { # path [curl args...]
  local path="$1" code
  shift
  code="$("${RUNNER[@]}" curl -s -o /dev/null -w '%{http_code}' "$@" "http://gitea:3000$path")" || return
  STEP_DETAIL="HTTP $code"
  [ "$code" = "200" ]
}

http_root() { expect_200 /; }
http_version() { expect_200 /api/v1/version; }
api_login() { expect_200 /api/v1/user -u "$B0_USER:$B0_PASSWORD"; }

repo_list() {
  local n
  n="$("${RUNNER[@]}" sh -c 'curl -fsS -u "$B0_USER:$B0_PASSWORD" "$GITEA_URL/api/v1/repos/search?limit=50" \
        | python3 -c "import sys, json; print(len(json.load(sys.stdin)[\"data\"]))"')" || return
  STEP_DETAIL="$n repositorios"
  [ "$n" -ge 1 ]
}

git_clone() {
  local head
  head="$("${RUNNER[@]}" sh -c 'git clone -q "http://$B0_USER:$B0_PASSWORD@gitea:3000/$B0_CLONE_REPO.git" /tmp/clone \
        && git -C /tmp/clone rev-parse HEAD')" || return
  STEP_DETAIL="$B0_CLONE_REPO HEAD $head"
}

NAMES="restic_check restore compose_up_healthy http_root http_version api_login repo_list git_clone"
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
log "B0: $STATUS ($OUT)"
exit "$RC"
