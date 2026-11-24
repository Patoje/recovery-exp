#!/usr/bin/env bash
# Ejecuta un escenario de falla: reset -> up -> seed -> testigos previos -> [inject] ->
# backup -> declare -> [inject] -> testigos posteriores -> línea base -> confirm.
# Uso: runner.sh <NN | nombre-de-carpeta>    (BASELINE=b0 por defecto)
# Resultados: results/faults/<escenario>/<batch>-rNN/{run.log,result.json,evidence/}.
# Nunca borra resultados.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
N="${1:?uso: runner.sh <NN|escenario>}"
BASELINE="${BASELINE:-b0}"

if [ -d "faults/$N" ]; then
  SCEN_DIR="$ROOT/faults/$N"
else
  matches=( $(ls -d faults/"$N"-* 2>/dev/null) )
  [ "${#matches[@]}" -eq 1 ] || { echo "escenario ambiguo o inexistente: $N (${matches[*]:-ninguno})" >&2; exit 2; }
  SCEN_DIR="$ROOT/${matches[0]}"
fi
SCEN="$(basename "$SCEN_DIR")"
BATCH="$(date -u +%Y%m%dT%H%M%SZ)"

REPEAT=1; SHIM=1; MINIO=0; POST_WITNESS=standard
set -a
# shellcheck disable=SC1091
. "$SCEN_DIR/scenario.env"
set +a
[ -n "${FAULT_REPEAT:-}" ] && REPEAT="$FAULT_REPEAT"
export BASELINE

# shellcheck source=../../scripts/lib.sh
. "$ROOT/scripts/lib.sh"
set +e

REAL_DOCKER="$(type -P docker)"

run_once() {
  local rep="$1" res
  res="$ROOT/results/faults/$SCEN/$BATCH-r$(printf %02d "$rep")"
  mkdir -p "$res/evidence"
  ( pipeline "$rep" "$res" ) 2>&1 | tee "$res/run.log"
  return "${PIPESTATUS[0]}"
}

pipeline() {
  local rep="$1" res="$2"
  export FAULT_DIR="$SCEN_DIR" FAULT_EVIDENCE="$res/evidence" FAULT_RUN_LOG="$res/run.log"
  export FAULT_REAL_DOCKER="$REAL_DOCKER" FAULT_SHIM_LOG="$res/evidence/shim.log" FAULT_REP="$rep"
  export MINIO
  if [ "$SHIM" = "1" ]; then
    export PATH="$ROOT/faults/lib/shim:$PATH"
  fi
  # Ganchos por defecto; el escenario los redefine en su archivo inject.
  prepare() { :; }; before_backup() { :; }; after_backup() { :; }; on_event() { :; }
  backup() { INVENTORY_HOOK="$ROOT/scripts/inventory.sh" "$ROOT/scripts/backup.sh" "$RUN_DIR"; }
  witness_post() { "$ROOT/scripts/witness.sh" post "$RUN_DIR"; }
  # shellcheck disable=SC1091
  . "$SCEN_DIR/inject"

  local t_start status="" reason="" harness_ok=1 phases="" docker_path
  t_start="$(now_ms)"
  RUN_DIR=""
  docker_path="$(type -P docker)"
  log "escenario $SCEN r$rep, baseline $BASELINE, docker=$docker_path, shim=$SHIM"

  phase() { # name cmd...
    local name="$1" p0 p1 rc=0
    shift
    log "== fase: $name"
    p0="$(now_ms)"
    "$@" || rc=$?
    p1="$(now_ms)"
    local obj="{\"name\":$(json_str "$name"),\"exit_code\":$rc,\"started_at\":$(json_str "$(iso_ms "$p0")"),\"duration_ms\":$((p1 - p0))}"
    if [ -z "$phases" ]; then phases="$obj"; else phases="$phases,$obj"; fi
    return "$rc"
  }

  run_up() { "$ROOT/scripts/up.sh" && RUN_DIR="$(current_run_dir)" && export RUN_DIR; }

  for ph in reset:"$ROOT/scripts/reset.sh" prepare:prepare up:run_up seed:"$ROOT/scripts/seed.sh" \
            witness_pre:"$ROOT/scripts/witness.sh pre" before_backup:before_backup backup:backup \
            declare:"$ROOT/scripts/declare.sh" after_backup:after_backup witness_post:witness_post; do
    name="${ph%%:*}"; cmd="${ph#*:}"
    # shellcheck disable=SC2086
    if ! phase "$name" $cmd ${RUN_DIR:+"$RUN_DIR"}; then
      harness_ok=0; reason="harness: falló la fase $name"; log "$reason"; break
    fi
  done

  local b_rc=-1 b_result="not-run" b_failed="" b_report="" confirm_rc=-1 confirm_lines=""
  if [ "$harness_ok" = 1 ]; then
    local bscript="$ROOT/scripts/$BASELINE.sh"
    [ "$BASELINE" = b1 ] && bscript="$ROOT/baselines/b1/b1.sh"
    phase baseline "$bscript" latest "$RUN_DIR"; b_rc=$?
    b_report="$RUN_DIR/$BASELINE.json"
    if [ -f "$b_report" ]; then
      b_result="$(grep -o '"result": "[a-z]*"' "$b_report" | cut -d'"' -f4)"
      b_failed="$(grep -o '"first_failed_step": "[a-z_0-9]*"' "$b_report" | head -1 | cut -d'"' -f4)"
      [ -n "$b_failed" ] || b_failed="$(grep -o '"name":"[a-z_]*","status":"failed"' "$b_report" | head -1 | cut -d'"' -f4)"
    else
      b_result="no-report"
    fi
    phase confirm sh -c '"$FAULT_DIR/confirm" > "$FAULT_EVIDENCE/confirm.log" 2>&1; rc=$?; cat "$FAULT_EVIDENCE/confirm.log"; exit $rc'
    confirm_rc=$?
    while IFS= read -r l; do
      case "$l" in "["*) if [ -z "$confirm_lines" ]; then confirm_lines="$(json_str "$l")"; else confirm_lines="$confirm_lines,$(json_str "$l")"; fi ;; esac
    done < "$FAULT_EVIDENCE/confirm.log"
  fi

  local confirm_status=invalid
  [ "$confirm_rc" = 0 ] && confirm_status=valid
  if [ "$harness_ok" != 1 ]; then status=invalid
  elif [ "$confirm_status" != valid ]; then status=invalid; reason="confirm no se cumplió"
  elif [ "$b_result" = pass ]; then status=pass
  else status=fail
  fi

  local shim_mods=0 rto="null"
  [ -f "$FAULT_SHIM_LOG" ] && shim_mods="$(grep -c '' "$FAULT_SHIM_LOG")"
  if [ -n "$RUN_DIR" ] && [ -f "$RUN_DIR/restore.json" ] && [ -f "$b_report" ]; then
    rto="$(docker run --rm -v "$RUN_DIR:/r:ro" --entrypoint python3 recexp-runner:local -c '
import json, datetime
p = lambda s: datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))
r = json.load(open("/r/restore.json")); b = json.load(open("/r/'"$BASELINE"'.json"))
print(round((p(b["finished_at"]) - p(r["restore_started_at"])).total_seconds(), 1))' 2>/dev/null || echo null)"
  fi
  local t_end; t_end="$(now_ms)"
  cat > "$res/result.json" <<EOF
{
  "scenario": $(json_str "$SCEN"),
  "batch": $(json_str "$BATCH"),
  "repetition": $rep,
  "run_id": $(json_str "$(basename "${RUN_DIR:-none}")"),
  "baseline": $(json_str "$BASELINE"),
  "env_version": $(json_str "$(cat "$ROOT/ENV_VERSION")"),
  "status": "$status",
  "reason": $(json_str "$reason"),
  "started_at": $(json_str "$(iso_ms "$t_start")"),
  "finished_at": $(json_str "$(iso_ms "$t_end")"),
  "shim": {"enabled": $([ "$SHIM" = 1 ] && echo true || echo false), "docker_path": $(json_str "$docker_path"), "modifications_logged": $shim_mods},
  "phases": [$phases],
  "baseline_result": {"exit_code": $b_rc, "result": $(json_str "$b_result"), "first_failed_step": $(json_str "$b_failed")},
  "restore_to_baseline_end_s": $rto,
  "confirm": {"status": "$confirm_status", "exit_code": $confirm_rc, "checks": [$confirm_lines]}
}
EOF
  "$ROOT/faults/lib/suspension_check.sh" "$res"
  grep -q '"status": "invalid"' "$res/result.json" && status=invalid && reason="$(grep -o '"reason": "[^"]*"' "$res/result.json" | head -1 | cut -d'"' -f4)"
  log "resultado: $status ${reason:+($reason)} -> $res/result.json"
  [ "$status" != invalid ]
}

rc_all=0
for rep in $(seq 1 "$REPEAT"); do
  run_once "$rep" || rc_all=1
done
exit "$rc_all"
