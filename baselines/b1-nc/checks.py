"""B1 checks for Nextcloud (port of baselines/b1/checks.py), run in the runner container inside the restore network.

Inputs (all from the run, never from fault scenarios):
  <run>/b0.json, restore.json, declared.json, inventory.json,
  witnesses_pre.json, witnesses_post.json,
  <run>/b1/b0.log and <run>/b1/raw/* (gathered by b1.sh), nc/objectives.env.
Writes <run>/b1.json. Exit 0 only if every check passes.
"""

import base64
import datetime
import hashlib
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

NC = "http://nextcloud"  # S: el servicio responde en http://nextcloud
# Tables that B0 itself writes before B1 counts rows (login page and login): appdata of the theming
# app in oc_filecache and last-login preferences. User files are compared separately.
WRITTEN_BY_B0 = {"oc_filecache", "oc_preferences"}  # S: el login y la página de login escriben en oc_filecache (appdata) y oc_preferences

# Exact error patterns in the B0 log (restore tool and database load). Never the bare word "error".
LOG_ERROR_PATTERNS = [
    ("pg_restore", re.compile(r"pg_restore: error:")),
    ("pg_restore", re.compile(r"pg_restore: warning: errors ignored on restore:")),
    ("restic", re.compile(r"^Fatal: ")),
    ("restic", re.compile(r"^ignoring error for ")),
    ("restic", re.compile(r"^error: ")),
    ("restic", re.compile(r"^\s*error for tree ")),
]
SELFTEST_LINES = [
    ("no errors were found", False),
    ("using temporary cache in /tmp/restic-check-cache-123", False),
    ("check snapshots, trees and blobs", False),
    ('pg_restore: error: COPY failed for table "issue": ERROR:  character with byte sequence', True),
    ("pg_restore: warning: errors ignored on restore: 2", True),
    ("Fatal: repository contains errors", True),
    ("ignoring error for /data/git/repositories/alice/witness.git/objects/ab/cd: ciphertext verification failed", True),
]


def now():
    return datetime.datetime.now(datetime.timezone.utc)


def iso(t):
    return t.isoformat(timespec="milliseconds").replace("+00:00", "Z")


def ts(s):
    return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))


def load(path, default=None):
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError):
        return default


def read(path, default=""):
    try:
        with open(path) as f:
            return f.read()
    except OSError:
        return default


def env_file(path):
    out = {}
    for line in read(path).splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            out[k.strip()] = v.strip()
    return out


def http(path, user=None, password=None, raw=False, method="GET", data=None):
    url = path if path.startswith("http") else NC + path
    headers = {}
    if user:
        headers["Authorization"] = "Basic " + base64.b64encode(f"{user}:{password}".encode()).decode()  # S: WebDAV acepta autenticación básica
    req = urllib.request.Request(url, headers=headers, method=method, data=data)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            status, body = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, body = e.code, e.read()
    if raw:
        return status, body
    try:
        return status, json.loads(body) if body else None
    except ValueError:
        return status, None


class Report:
    def __init__(self):
        self.steps = []

    def run(self, name, fn):
        t0 = now()
        try:
            ok, detail = fn()
        except Exception as e:  # a check that cannot run is a failed check
            ok, detail = False, f"no se pudo verificar: {e!r}"[:500]
        t1 = now()
        self.steps.append({"name": name, "status": "ok" if ok else "failed", "started_at": iso(t0),
                           "duration_ms": int((t1 - t0).total_seconds() * 1000), "detail": detail})
        print(f"[b1] {'ok  ' if ok else 'FAIL'} {name}: {detail}"[:400], flush=True)
        return ok


def main():
    run, objectives_path = sys.argv[1], sys.argv[2]
    raw = os.path.join(run, "b1", "raw")
    env = os.environ
    obj = env_file(objectives_path)
    rto_limit, rpo_limit = float(obj["RTO_SECONDS"]), float(obj["RPO_SECONDS"])
    declared = load(os.path.join(run, "declared.json"), {})
    inventory = load(os.path.join(run, "inventory.json"), {})
    pre = load(os.path.join(run, "witnesses_pre.json"), {"witnesses": []})
    post = load(os.path.join(run, "witnesses_post.json"), {"witnesses": []})
    restore = load(os.path.join(run, "restore.json"), {})
    b0 = load(os.path.join(run, "b0.json"), {})
    user, password = env["B0_USER"], env["B0_PASSWORD"]
    rep = Report()
    started = load(os.path.join(raw, "times.json"), {}).get("b1_started_at")

    # --- before B0 (captured by b1.sh before any migration)
    def pre_image():
        img = read(os.path.join(raw, "pre_image.txt")).strip()
        ok = bool(img) and img == declared.get("nextcloud_image")
        return ok, f"configurada {img or '?'}; declarada {declared.get('nextcloud_image', '?')}"

    def pre_version():
        m = re.search(r"^(\d+(\.\d+)+)$", read(os.path.join(raw, "pre_version.txt")).strip())  # S: version.php da la versión con cuatro números (34.0.5.1)
        v = m.group(1) if m else None
        return v is not None and v == declared.get("nextcloud_version"), f"imagen configurada {v or '?'}; declarada {declared.get('nextcloud_version', '?')}"

    rep.run("nextcloud_image_before_migrations", pre_image)
    rep.run("nextcloud_version_before_migrations", pre_version)

    # --- B0
    def b0_complete():
        rc = read(os.path.join(raw, "b0_exit_code.txt")).strip()
        return b0.get("result") == "pass" and rc == "0", f"B0 {b0.get('result', 'sin reporte')}, código {rc or '?'}"

    rep.run("b0", b0_complete)

    # --- log of the restore tool and database load (B0 log), with a self-test of the patterns
    def match_errors(line):
        return [src for src, rx in LOG_ERROR_PATTERNS if rx.search(line)]

    def patterns_selftest():
        wrong = [(l, exp) for l, exp in SELFTEST_LINES if bool(match_errors(l)) != exp]
        return not wrong, ("patrones correctos; 'no errors were found' no cuenta como error"
                           if not wrong else f"clasificación incorrecta: {wrong}")

    def log_errors():
        lines = read(os.path.join(run, "b1", "b0.log")).splitlines()
        if not lines:
            return False, "log de B0 vacío"
        hits = [(i + 1, l.strip()[:200]) for i, l in enumerate(lines) if match_errors(l)]
        no_err = sum(1 for l in lines if "no errors were found" in l)
        detail = f"{len(hits)} líneas de error; líneas 'no errors were found' (no cuentan): {no_err}"
        if hits:
            detail += "; " + " | ".join(f"l.{n}: {t}" for n, t in hits[:5])
        return not hits, detail

    rep.run("log_patterns_selftest", patterns_selftest)
    rep.run("restore_and_load_errors_in_log", log_errors)

    # --- declared values on the running system
    def running_image():
        img = read(os.path.join(raw, "running_image.txt")).strip()
        return img == declared.get("nextcloud_image"), f"en marcha {img or '?'}"

    def running_version():
        st, body = http("/status.php")  # S: /status.php sin autenticación
        v = (body or {}).get("version") if st == 200 else None  # S: campo "version" de status.php
        return v == declared.get("nextcloud_version"), f"status.php {v or st}; declarada {declared.get('nextcloud_version', '?')}"

    def status_ready():
        st, body = http("/status.php")
        b = body or {}
        ok = st == 200 and b.get("installed") is True and b.get("maintenance") is False and b.get("needsDbUpgrade") is False  # S: campos installed, maintenance y needsDbUpgrade
        return ok, f"HTTP {st}; installed={b.get('installed')}, maintenance={b.get('maintenance')}, needsDbUpgrade={b.get('needsDbUpgrade')}"

    def secret_key():
        fp = read(os.path.join(raw, "secret_sha256.txt")).strip()
        ok = bool(fp) and fp == declared.get("secret_sha256")
        return ok, f"huella en uso {fp[:12] or '?'}…; declarada {declared.get('secret_sha256', '?')[:12]}…"

    def encoding():
        enc = read(os.path.join(raw, "db_encoding.txt")).strip()
        return enc == declared.get("database_encoding"), f"base {enc or '?'}; declarada {declared.get('database_encoding', '?')}"

    def external_deps():
        deps = declared.get("external_dependencies", [])
        if not deps:
            return True, "sin dependencias externas declaradas"
        bad = []
        for d in deps:
            url = f"http://{d['endpoint']}/minio/health/live"  # S: dependencias declaradas de tipo MinIO (ninguna en este entorno)
            try:
                st, _ = http(url, raw=True)
            except Exception as e:
                st = repr(e)[:80]
            if st != 200:
                bad.append(f"{d['section']}→{d['endpoint']}: {st}")
        return not bad, "disponibles" if not bad else "; ".join(bad)

    rep.run("nextcloud_image_running", running_image)
    rep.run("nextcloud_version_running", running_version)
    rep.run("nextcloud_status_ready", status_ready)
    rep.run("secret_fingerprint", secret_key)
    rep.run("database_encoding", encoding)
    rep.run("external_dependencies", external_deps)

    # --- inventories (before any write by B1)
    def row_counts():
        counts = load(os.path.join(raw, "db_row_counts.json"))
        ref = inventory.get("table_row_counts")
        if counts is None or ref is None:
            return False, "faltan conteos (base restaurada o inventario)"
        diff = {t: (ref.get(t), counts.get(t)) for t in sorted(set(ref) | set(counts))
                if ref.get(t) != counts.get(t) and t not in WRITTEN_BY_B0}
        return not diff, (f"{len(ref)} tablas; diferencias (inventario, restaurada): {diff}" if diff else
                          f"{len(ref)} tablas iguales (sin contar {', '.join(sorted(WRITTEN_BY_B0))})")

    disk = {}
    for line in read(os.path.join(raw, "disk_files.txt")).splitlines():
        if " " in line:
            k, v = line.rsplit(" ", 1)
            disk[k] = int(v)

    def files_db_vs_disk():
        db = set(read(os.path.join(raw, "db_files.txt")).splitlines())
        if not db and not disk:
            return False, "no se pudo leer la base ni el disco restaurados"
        only_db, only_disk = sorted(db - set(disk)), sorted(set(disk) - db)
        ok = not only_db and not only_disk
        return ok, f"{len(db)} en la base, {len(disk)} en disco" + ("" if ok else f"; solo en base {only_db[:5]} ({len(only_db)}); solo en disco {only_disk[:5]} ({len(only_disk)})")

    def files_vs_inventory():
        ref = inventory.get("disk_user_files")
        if ref is None or not disk:
            return False, "faltan archivos (inventario o disco restaurado)"
        diffs = sorted(k for k in set(ref) | set(disk) if ref.get(k) != disk.get(k))
        return not diffs, f"{len(ref)} archivos iguales (ruta y tamaño)" if not diffs else f"{len(diffs)} distintos: {diffs[:8]}"

    rep.run("table_row_counts_vs_inventory", row_counts)
    rep.run("files_db_vs_disk", files_db_vs_disk)
    rep.run("files_vs_inventory", files_vs_inventory)

    # --- witnesses
    def dav_get(user, path):
        st, data = http(f"/remote.php/dav/files/{urllib.parse.quote(user)}/{urllib.parse.quote(path)}", user,
                        env["SEED_USER_PASSWORD"], raw=True)  # S: WebDAV de archivos en /remote.php/dav/files/<usuario>/<ruta>
        return hashlib.sha256(data).hexdigest() if st == 200 else f"HTTP {st}"

    def witness_sha(w):
        if w["type"] == "share":
            return dav_get(w["share_with"], w["recipient_path"])  # S: el destinatario ve el archivo compartido en su raíz
        return dav_get(w["user"], w["path"])

    def witness_present(w):
        return witness_sha(w) == w["sha256"]

    def pre_of(kind):
        return [w for w in pre.get("witnesses", []) if w["type"] == kind]

    def witness_check(kind, label):
        def fn():
            ws = pre_of(kind)
            bad = [f"{w['id']}: {witness_sha(w)}" for w in ws if not witness_present(w)]
            return bool(ws) and not bad, f"{len(ws)} {label}" + ("" if not bad else f"; distinto: {bad}")
        return fn

    def write_probe():
        path = f"b1-write-{int(time.time())}.txt"
        content = b"b1 write check\n"
        url = f"/remote.php/dav/files/{urllib.parse.quote(user)}/{path}"
        st, _ = http(url, user, password, raw=True, method="PUT", data=content)  # S: subir un archivo es un PUT de WebDAV
        if st not in (201, 204):
            return False, f"PUT {path}: HTTP {st}"
        back = dav_get(user, path)
        return back == hashlib.sha256(content).hexdigest(), f"PUT {path}: HTTP {st}; lectura {'igual' if back == hashlib.sha256(content).hexdigest() else back}"

    rep.run("witness_file_hash", witness_check("file", "archivos testigo"))
    rep.run("witness_binary_hash", witness_check("binary", "binarios testigo"))
    rep.run("witness_share_hash", witness_check("share", "archivos compartidos testigo"))
    rep.run("write_probe", write_probe)
    functional_end = now()

    # --- objectives
    def rto():
        if "restore_started_at" not in restore:
            return False, "restore.json sin restore_started_at"
        functional_ok = all(s["status"] == "ok" for s in rep.steps
                            if s["name"] in ("b0", "witness_file_hash", "witness_binary_hash",
                                             "witness_share_hash", "write_probe"))
        secs = (functional_end - ts(restore["restore_started_at"])).total_seconds()
        if not functional_ok:
            return False, f"las pruebas funcionales no pasaron ({secs:.1f} s); RTO {rto_limit:.0f} s"
        return secs <= rto_limit, f"{secs:.1f} s hasta las pruebas funcionales; RTO declarado {rto_limit:.0f} s"

    def rpo():
        incident = post.get("incident_at")
        if not incident:
            return False, "witnesses_post.json sin incident_at"
        allw = pre.get("witnesses", []) + post.get("witnesses", [])
        recovered = [w for w in allw if witness_present(w)]
        if not recovered:
            return False, "ningún testigo recuperado"
        newest = max(recovered, key=lambda w: ts(w["created_at"]))
        secs = (ts(incident) - ts(newest["created_at"])).total_seconds()
        return secs <= rpo_limit, (f"{secs:.1f} s (T {incident}; testigo más reciente recuperado {newest['type']} "
                                   f"{newest['created_at']}; {len(recovered)}/{len(allw)} recuperados); RPO declarado {rpo_limit:.0f} s")

    rep.run("rto", rto)
    rep.run("rpo", rpo)

    ok = all(s["status"] == "ok" for s in rep.steps)
    report = {"baseline": "B1", "app": "nextcloud", "run_id": os.path.basename(run), "result": "pass" if ok else "fail",
              "exit_code": 0 if ok else 1, "started_at": started, "finished_at": iso(now()),
              "objectives": {"rto_seconds": rto_limit, "rpo_seconds": rpo_limit},
              "first_failed_step": next((x["name"] for x in rep.steps if x["status"] != "ok"), ""),
              "functional_tests_finished_at": iso(functional_end), "steps": rep.steps}
    with open(os.path.join(run, "b1.json"), "w") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
        f.write("\n")
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
