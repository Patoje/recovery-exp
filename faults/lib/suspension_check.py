"""Host-suspension check for a fault-scenario run (updates result.json in place).

A run is invalid ("harness: suspensión del host") if either:
  (a) `pmset -g log` shows a Sleep event between the run's start and end;
  (b) two consecutive post-backup witness attempts (successful or failed) are more than 90 s apart.
The triggering condition and its evidence are recorded under "host_suspension".
Idempotent: re-running recomputes from the status before the check.

Usage: suspension_check.py <results/.../RES_DIR>   (pmset lines in evidence/pmset_sleep.txt)
"""

import datetime
import json
import os
import sys

GAP_LIMIT_S = 90
REASON = "harness: suspensión del host"


def ts(s):
    return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))


def main():
    res = sys.argv[1]
    path = os.path.join(res, "result.json")
    with open(path) as f:
        r = json.load(f)
    prev = r.get("host_suspension", {}).get("status_before_check")
    status_before = prev or {"status": r["status"], "reason": r["reason"]}
    start, end = ts(r["started_at"]), ts(r["finished_at"])

    pm_file = os.path.join(res, "evidence", "pmset_sleep.txt")
    sleeps, pm_available = [], False
    with open(pm_file) as f:
        for line in f:
            if line.startswith("#"):
                continue
            pm_available = True
            try:
                t = datetime.datetime.strptime(line[:25], "%Y-%m-%d %H:%M:%S %z")
            except ValueError:
                continue
            if start <= t <= end:
                sleeps.append(line.strip()[:200])

    gaps, max_gap, witness_file = [], None, None
    wp = os.path.join("runs", r["run_id"], "witnesses_post.json")
    if r["run_id"] != "none" and os.path.exists(wp):
        witness_file = wp
        with open(wp) as f:
            post = json.load(f)
        # Attempts, successful (created_at) or failed (attempted_at), in time order.
        attempts = [(w["created_at"], w.get("minute")) for w in post["witnesses"]]
        attempts += [(a["attempted_at"], a.get("minute")) for a in post.get("failed_attempts", [])]
        attempts.sort(key=lambda x: ts(x[0]))
        for (ta, ma), (tb, mb) in zip(attempts, attempts[1:]):
            d = (ts(tb) - ts(ta)).total_seconds()
            max_gap = d if max_gap is None else max(max_gap, d)
            if d > GAP_LIMIT_S:
                gaps.append({"from_minute": ma, "to_minute": mb, "from": ta, "to": tb, "gap_s": round(d, 1)})

    triggered = []
    if sleeps:
        triggered.append("pmset_sleep")
    if gaps:
        triggered.append("witness_gap")
    r["host_suspension"] = {
        "checked_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "invalid": bool(triggered),
        "triggered_by": triggered,
        "pmset_sleep": {"available": pm_available, "events_in_run": sleeps},
        "witness_gap": {"limit_s": GAP_LIMIT_S, "measured_over": "attempts", "witness_file": witness_file,
                        "max_gap_s": None if max_gap is None else round(max_gap, 1), "gaps_over_limit": gaps},
        "status_before_check": status_before,
    }
    if triggered:
        r["status"], r["reason"] = "invalid", REASON
    else:
        r["status"], r["reason"] = status_before["status"], status_before["reason"]
    with open(path + ".tmp", "w") as f:
        json.dump(r, f, indent=2, ensure_ascii=False)
        f.write("\n")
    os.replace(path + ".tmp", path)
    print(f"{res}: status={r['status']} triggered={triggered or '-'} max_gap_s={r['host_suspension']['witness_gap']['max_gap_s']} sleeps={len(sleeps)}")


if __name__ == "__main__":
    main()
