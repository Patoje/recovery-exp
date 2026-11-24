"""Builds results/summary.md from results/faults/*/*/result.json (prints Markdown).

For each scenario and baseline, the latest batch is reported. Earlier batches are
counted but never deleted.
"""

import glob
import json
import os
from collections import defaultdict

ROOT = "/work"
CONTROLS = {"02-db-creds-missing", "15-corrupt-pack"}
BASELINES = ["b0", "b1"]

results = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))  # scen -> baseline -> batch -> [r]
for path in sorted(glob.glob(f"{ROOT}/results/faults/*/*/result.json")):
    with open(path) as f:
        r = json.load(f)
    r["_dir"] = os.path.relpath(os.path.dirname(path), ROOT)
    results[r["scenario"]][r["baseline"]][r["batch"]].append(r)


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
         "Generado por `make summary` a partir de `results/faults/*/*/result.json`. Para cada escenario y "
         "línea base se muestra el último lote (en escenarios repetidos, todas las repeticiones). "
         "Nada se borra: las corridas anteriores y las invalid se conservan.", "",
         "`confirm`: la falla ocurrió de verdad; si no, la corrida es `invalid` y no cuenta. Controles: 02 y 15. "
         "`Suspensión`: corridas invalid por \"harness: suspensión del host\" (evento Sleep de `pmset` o hueco "
         "> 90 s entre intentos consecutivos de testigos posteriores, exitosos o fallidos), sobre todos los lotes.", ""]
lines += ["| Falla | confirm | B0 | B1 | Suspensión (invalid / total) | t restauración→fin B0 (s) | Resultados |",
          "|---|---|---|---|---|---|---|"]
repeated_detail = []
for scen in sorted(results):
    row, dirs, rto, confirm_txt = {}, [], "—", "—"
    susp_total = susp_inv = 0
    for b in BASELINES:
        batches = results[scen].get(b, {})
        if not batches:
            row[b] = "—"
            continue
        allruns = [r for v in batches.values() for r in v]
        susp_total += len(allruns)
        susp_inv += sum(1 for r in allruns if is_susp(r))
        c, res, single, runs = summarize(batches)
        row[b] = res
        if b == "b0" or confirm_txt == "—":
            confirm_txt = c
        if single:
            dirs.append(f"`{single['_dir']}`")
            if b == "b0" and single.get("restore_to_baseline_end_s") is not None:
                rto = str(single["restore_to_baseline_end_s"])
        else:
            dirs.append(f"`results/faults/{scen}/` ({len(runs)} corridas)")
            repeated_detail.append((scen, b, runs))
    name = scen + (" (control)" if scen in CONTROLS else "")
    lines.append(f"| {name} | {confirm_txt} | {row.get('b0', '—')} | {row.get('b1', '—')} | "
                 f"{susp_inv} / {susp_total} | {rto} | {'<br>'.join(dirs)} |")

for scen, b, runs in repeated_detail:
    lines += ["", f"## {scen}: repeticiones ({b.upper()})", "",
              f"| Corrida | Estado | confirm | {b.upper()} | Detalle |", "|---|---|---|---|---|"]
    for r in runs:
        if is_susp(r):
            hs = r["host_suspension"]
            det = f"suspensión: {', '.join(hs['triggered_by'])}; {len(hs['pmset_sleep']['events_in_run'])} Sleep; hueco máx. {hs['witness_gap']['max_gap_s']} s"
        else:
            det = "; ".join(c for c in r["confirm"]["checks"] if "inconsistencia" in c or c.startswith("[fail]"))
        lines.append(f"| `{r['batch']}-r{r['repetition']:02d}` | {r['status']} | {r['confirm']['status']} | {step_txt(r)} | {det} |")
findings = os.path.join(ROOT, "faults", "findings.md")
if os.path.exists(findings):
    with open(findings) as f:
        lines += ["", f.read().rstrip()]
print("\n".join(lines))
