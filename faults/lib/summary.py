"""Builds results/summary.md from results/faults/*/*/result.json (prints Markdown).

For each environment version, scenario and baseline, the latest batch is reported.
Earlier batches are counted but never deleted. Results without "env_version" are v1.
"""

import glob
import json
import os
from collections import defaultdict

ROOT = "/work"
CONTROLS = {"02-db-creds-missing", "15-corrupt-pack"}
BASELINES = ["b0", "b1", "contract"]
ENVS = ["v2", "v1"]  # current first

# Frozen contract baseline (tags engine-v1 and contract-gitea-v1). Contract runs made with another
# engine commit or contract file are earlier development runs: listed apart, not counted.
FROZEN_ENGINE = "133675b"
FROZEN_CONTRACT_SHA256 = "f393f6d65e2f9f1136cdb7b9e14628ad77d9b4903562af242c2aca83d9582240"


def contract_version(r):
    try:
        with open(f"{ROOT}/runs/{r['run_id']}/contract.json") as f:
            c = json.load(f)
        return c["engine"]["commit"], c["contract"]["sha256"]
    except (OSError, KeyError, ValueError):
        return None, None


def wrapper_version(r):
    """Version of baselines/contract/contract.sh: v2 also copies restore.sh's output to run.log."""
    try:
        with open(f"{ROOT}/runs/{r['run_id']}/contract/restore.log") as f:
            first = next((l.strip() for l in f if l.strip()), "")
        with open(f"{ROOT}/{r['_dir']}/run.log") as f:
            return 2 if first and first in f.read() else 1
    except OSError:
        return None


# env -> scen -> baseline -> batch -> [r]
results = defaultdict(lambda: defaultdict(lambda: defaultdict(lambda: defaultdict(list))))
pre_freeze = []  # contract runs before the freeze
hidden = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))  # hidden faults O1..: scen -> baseline -> batch -> [r]
HIDDEN_ALL = {"O1-secuencias-en-cero": "A", "O2-minio-original": "B", "O3-latest-de-otro": "C",
              "O4-corrupcion-en-retencion": "D"}
HIDDEN_NOT_RUN = {"O2-minio-original": "no implementada: el orden de la inyección no se puede seguir (pregunta 7)"}
for path in sorted(glob.glob(f"{ROOT}/results/faults/*/*/result.json")):
    with open(path) as f:
        r = json.load(f)
    r["_dir"] = os.path.relpath(os.path.dirname(path), ROOT)
    if r["baseline"] == "contract" and contract_version(r) != (FROZEN_ENGINE, FROZEN_CONTRACT_SHA256):
        pre_freeze.append(r)
        continue
    if r["scenario"] in HIDDEN_ALL:
        hidden[r["scenario"]][r["baseline"]][r["batch"]].append(r)
        continue
    results[r.get("env_version", "v1")][r["scenario"]][r["baseline"]][r["batch"]].append(r)


SUSPENSION = "harness: suspensión del host"


def is_susp(r):
    return r["reason"] == SUSPENSION


def step_txt(r):
    res = r["baseline_result"]["result"]
    step = r["baseline_result"]["first_failed_step"]
    return res + (f" en `{step}`" if res != "pass" and step else "")


def summarize(batches):
    """Repeated scenario: every repetition of every batch. Otherwise: the latest batch."""
    repeated = any(len(v) > 1 for v in batches.values())
    if repeated:
        runs = [r for b in sorted(batches) for r in batches[b]]
    else:
        runs = batches[max(batches)]
    valid = [r for r in runs if r["status"] != "invalid"]
    if not repeated:
        r = runs[0]
        confirm = "valid" if r["status"] != "invalid" else f"invalid ({r['reason'] or 'confirm'})"
        return confirm, step_txt(r), r, runs
    confirm = f"valid {len(valid)}/{len(runs)} corridas"
    passes = sum(1 for r in valid if r["baseline_result"]["result"] == "pass")
    return confirm, f"pass {passes}/{len(valid)}, fail {len(valid) - passes}/{len(valid)} (válidas)", None, runs


lines = ["# Resumen de escenarios de falla", "",
         "Generado por `make summary` a partir de `results/faults/*/*/result.json`. Para cada versión del entorno, "
         "escenario y línea base se muestra el último lote (en escenarios repetidos, todas las repeticiones). "
         "Nada se borra: las corridas anteriores y las invalid se conservan.", "",
         "Entornos: **v1** (`b0-v1`, `SECRET_KEY` vacía en `app.ini`) y **v2** (`make up` genera una `SECRET_KEY` "
         "aleatoria y la escribe solo en `app.ini`). Los resultados sin `env_version` son del entorno v1.", "",
         "`confirm`: la falla ocurrió de verdad; si no, la corrida es `invalid` y no cuenta. Controles: 02 y 15. "
         "`Suspensión`: corridas invalid por \"harness: suspensión del host\" (evento Sleep de `pmset` o hueco "
         "> 90 s entre intentos consecutivos de testigos posteriores, exitosos o fallidos), sobre todos los lotes."]
repeated_detail = []
b0_by_env = defaultdict(dict)  # env -> scen -> (confirm, b0)
for env in ENVS:
    if not results.get(env):
        continue
    lines += ["", f"## Entorno {env}", "",
              "| Falla | confirm | B0 | B1 | Contrato | Suspensión (invalid / total) | t restauración→fin B0 (s) | Resultados |",
              "|---|---|---|---|---|---|---|---|"]
    for scen in sorted(results[env]):
        row, dirs, rto, confirm_txt = {}, [], "—", "—"
        susp_total = susp_inv = 0
        for b in BASELINES:
            batches = results[env][scen].get(b, {})
            if not batches:
                row[b] = "—"
                continue
            allruns = [r for v in batches.values() for r in v]
            susp_total += len(allruns)
            susp_inv += sum(1 for r in allruns if is_susp(r))
            c, res, single, runs = summarize(batches)
            if single and single["status"] == "invalid" and b != "b0":
                res += f" (corrida invalid: {single['reason'] or 'confirm'})"
            row[b] = res
            if b == "b0" or confirm_txt == "—":
                confirm_txt = c
            if single:
                dirs.append(f"`{single['_dir']}`")
                if b == "b0" and single.get("restore_to_baseline_end_s") is not None:
                    rto = str(single["restore_to_baseline_end_s"])
            else:
                dirs.append(f"`results/faults/{scen}/` ({len(runs)} corridas {env})")
                repeated_detail.append((env, scen, b, runs))
        if "b0" in results[env][scen]:
            b0_by_env[env][scen] = (confirm_txt, row["b0"], rto)
        name = scen + (" (control)" if scen in CONTROLS else "")
        lines.append(f"| {name} | {confirm_txt} | {row.get('b0', '—')} | {row.get('b1', '—')} | {row.get('contract', '—')} | "
                     f"{susp_inv} / {susp_total} | {rto} | {'<br>'.join(dirs)} |")
    if env == "v2":
        lines += ["", "`confirm` es el de la corrida de B0 (o de la primera línea base con corridas). Si la corrida de "
                  "otra línea base fue invalid, su celda lo indica. Contrato: motor `" + FROZEN_ENGINE + "` y "
                  "`contracts/gitea.yaml` sha256 `" + FROZEN_CONTRACT_SHA256[:12] + "…` (tags `engine-v1`, "
                  "`contract-gitea-v1`)."]
        contract_runs = [r for sc in results[env].values() for runs in sc.get("contract", {}).values() for r in runs]
        v2_runs = [r for r in contract_runs if wrapper_version(r) == 2]
        lines.append("Envoltorio `baselines/contract/contract.sh`: v1 en la tanda inicial; la v2 (pregunta 6) copia "
                     "también la salida de `restore.sh` a `run.log` con `tee`, sin otros cambios. Corridas con la v2: " +
                     (", ".join(f"`{r['_dir']}`" for r in v2_runs) or "ninguna") + ". Las corridas anteriores con la v1 "
                     "se conservan (14: `invalid` porque su `confirm` busca la salida de `pg_restore` en `run.log`).")
        if pre_freeze:
            lines.append("Corridas del contrato previas al congelamiento (no cuentan): " + ", ".join(
                f"`{r['_dir']}` (motor `{contract_version(r)[0]}`, contrato `{(contract_version(r)[1] or '?')[:8]}`)"
                for r in pre_freeze) + ".")

if b0_by_env.get("v1") and b0_by_env.get("v2"):
    lines += ["", "## Comparación de B0: entorno v1 contra v2", "",
              "| Falla | confirm v1 | B0 v1 | confirm v2 | B0 v2 | t v1 (s) | t v2 (s) | ¿Cambió B0? |",
              "|---|---|---|---|---|---|---|---|"]
    for scen in sorted(set(b0_by_env["v1"]) | set(b0_by_env["v2"])):
        c1, r1, t1 = b0_by_env["v1"].get(scen, ("—", "—", "—"))
        c2, r2, t2 = b0_by_env["v2"].get(scen, ("—", "—", "—"))
        norm = lambda t: "pass" if t.startswith("pass ") and " fail 0/" in t else t  # all valid repetitions passed
        changed = "—" if "—" in (r1, r2) else ("no" if norm(r1) == norm(r2) else "**sí**")
        lines.append(f"| {scen} | {c1} | {r1} | {c2} | {r2} | {t1} | {t2} | {changed} |")

HEALTHY = "00-healthy"


def healthy_series(env, b):
    """Runs of the healthy series: from the first batch with several repetitions on (top-ups included)."""
    batches = results[env].get(HEALTHY, {}).get(b, {})
    multi = sorted(k for k, v in batches.items() if len(v) > 1)
    if not multi:
        return []
    return [r for k in sorted(batches) if k >= multi[0] for r in batches[k]]


for env in ENVS:
    series = {b: healthy_series(env, b) for b in BASELINES}
    if not any(series.values()):
        continue
    lines += ["", f"## Restauraciones sanas ({HEALTHY}, entorno {env})", "",
              "Cada corrida arranca desde `make reset`. Una falsa alarma es un `fail` de la línea base en una "
              "restauración sana válida.", "",
              "| Línea base | Corrida | Estado | Resultado | ¿Falsa alarma? | Detalle |", "|---|---|---|---|---|---|"]
    for b in BASELINES:
        for r in series[b]:
            if r["status"] == "invalid":
                fa, det = "— (invalid)", r["reason"]
            else:
                fa = "**sí**" if r["baseline_result"]["result"] != "pass" else "no"
                det = ""
            lines.append(f"| {b.upper()} | `{r['batch']}-r{r['repetition']:02d}` | {r['status']} | {step_txt(r)} | {fa} | {det} |")
    for b in BASELINES:
        v = [r for r in series[b] if r["status"] != "invalid"]
        if series[b]:
            fas = sum(1 for r in v if r["baseline_result"]["result"] != "pass")
            lines.append(f"\n**{b.upper()}:** {len(v)} válidas de {len(series[b])}; falsas alarmas: {fas} de {len(v)}.")

for env, scen, b, runs in repeated_detail:
    if scen == HEALTHY:
        continue
    lines += ["", f"## {scen}: repeticiones ({b.upper()}, entorno {env})", "",
              f"| Corrida | Estado | confirm | {b.upper()} | Detalle |", "|---|---|---|---|---|"]
    for r in runs:
        if is_susp(r):
            hs = r["host_suspension"]
            det = f"suspensión: {', '.join(hs['triggered_by'])}; {len(hs['pmset_sleep']['events_in_run'])} Sleep; hueco máx. {hs['witness_gap']['max_gap_s']} s"
        else:
            det = "; ".join(c for c in r["confirm"]["checks"] if "inconsistencia" in c or c.startswith("[fail]"))
        lines.append(f"| `{r['batch']}-r{r['repetition']:02d}` | {r['status']} | {r['confirm']['status']} | {step_txt(r)} | {det} |")
# Hidden faults (docs/fallas-ocultas.md), apart from the known scenarios.
lines += ["", "## Fallas ocultas", "",
          "Escenarios `faults/O1`–`O4` a partir de `docs/fallas-ocultas.md` (fallas A–D). Cada línea base corre "
          "en su propia corrida desde `make reset`, con el mismo shim y los mismos criterios de invalidez; "
          "`confirm` es el de cada corrida. Se muestra el último lote. Detalle de la implementación y de los "
          "supuestos a verificar: `docs/fallas-ocultas/registro.md`.", "",
          "| Falla | confirm | B0 | B1 | Contrato (primer paso que falla) |", "|---|---|---|---|---|"]
hidden_dirs = []
for scen, letter in HIDDEN_ALL.items():
    if scen in HIDDEN_NOT_RUN and not hidden.get(scen):
        lines.append(f"| {scen} ({letter}) | {HIDDEN_NOT_RUN[scen]} | — | — | — |")
        continue
    cells, confirms = {}, []
    for b in BASELINES:
        batches = hidden[scen].get(b)
        if not batches:
            cells[b] = "—"
            continue
        r = batches[max(batches)][-1]
        hidden_dirs.append(f"`{r['_dir']}`")
        cells[b] = step_txt(r) + (" (corrida invalid)" if r["status"] == "invalid" else "")
        confirms.append((b, "valid" if r["status"] != "invalid" else f"invalid ({r['reason'] or 'confirm'})"))
    if confirms and all(c == "valid" for _, c in confirms):
        ctxt = "valid " + "/".join(b.upper() for b, _ in confirms)
    else:
        ctxt = "; ".join(f"{b.upper()}: {c}" for b, c in confirms) or "—"
    lines.append(f"| {scen} ({letter}) | {ctxt} | {cells['b0']} | {cells['b1']} | {cells['contract']} |")
if hidden_dirs:
    lines += ["", "Resultados: " + ", ".join(hidden_dirs) + "."]

findings = os.path.join(ROOT, "faults", "findings.md")
if os.path.exists(findings):
    with open(findings) as f:
        lines += ["", f.read().rstrip()]
print("\n".join(lines))
