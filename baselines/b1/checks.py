"""B1 checks, run in the runner container inside the restore network.

Inputs (all from the run, never from fault scenarios):
  <run>/b0.json, restore.json, declared.json, inventory.json,
  witnesses_pre.json, witnesses_post.json, secrets/totp.json,
  <run>/b1/b0.log and <run>/b1/raw/* (gathered by b1.sh), objectives.env.
Writes <run>/b1.json. Exit 0 only if every check passes.
"""

import base64
import datetime
import hashlib
import hmac
import json
import os
import re
import struct
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

GITEA = "http://gitea:3000"

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


def totp(secret_b32, at=None):
    key = base64.b32decode(secret_b32.upper() + "=" * (-len(secret_b32) % 8))
    counter = int((at if at is not None else time.time()) // 30)
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    o = digest[-1] & 0x0F
    return f"{(struct.unpack('>I', digest[o:o + 4])[0] & 0x7FFFFFFF) % 1_000_000:06d}"


def http(path, user=None, password=None, otp=None, raw=False):
    url = path if path.startswith("http") else GITEA + path
    headers = {}
    if user:
        headers["Authorization"] = "Basic " + base64.b64encode(f"{user}:{password}".encode()).decode()
    if otp:
        headers["X-Gitea-OTP"] = otp
    req = urllib.request.Request(url, headers=headers)
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
        ok = bool(img) and img == declared.get("gitea_image")
        return ok, f"configurada {img or '?'}; declarada {declared.get('gitea_image', '?')}"

    def pre_version():
        m = re.search(r"version (\S+)", read(os.path.join(raw, "pre_version.txt")))
        v = m.group(1) if m else None
        return v is not None and v == declared.get("gitea_version"), f"imagen configurada {v or '?'}; declarada {declared.get('gitea_version', '?')}"

    rep.run("gitea_image_before_migrations", pre_image)
    rep.run("gitea_version_before_migrations", pre_version)

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
        return img == declared.get("gitea_image"), f"en marcha {img or '?'}"

    def running_version():
        st, body = http("/api/v1/version")
        v = (body or {}).get("version") if st == 200 else None
        return v == declared.get("gitea_version"), f"API {v or st}; declarada {declared.get('gitea_version', '?')}"

    def secret_key():
        fp = read(os.path.join(raw, "secret_key_sha256.txt")).strip()
        ok = bool(fp) and fp == declared.get("secret_key_sha256")
        return ok, f"huella en uso {fp[:12] or '?'}…; declarada {declared.get('secret_key_sha256', '?')[:12]}…"

    def encoding():
        enc = read(os.path.join(raw, "db_encoding.txt")).strip()
        return enc == declared.get("database_encoding"), f"base {enc or '?'}; declarada {declared.get('database_encoding', '?')}"

    def external_deps():
        deps = declared.get("external_dependencies", [])
        if not deps:
            return True, "sin dependencias externas declaradas"
        bad = []
        for d in deps:
            url = f"http://{d['endpoint']}/minio/health/live"
            try:
                st, _ = http(url, raw=True)
            except Exception as e:
                st = repr(e)[:80]
            if st != 200:
                bad.append(f"{d['section']}→{d['endpoint']}: {st}")
        return not bad, "disponibles" if not bad else "; ".join(bad)

    rep.run("gitea_image_running", running_image)
    rep.run("gitea_version_running", running_version)
    rep.run("secret_key_fingerprint", secret_key)
    rep.run("database_encoding", encoding)
    rep.run("external_dependencies", external_deps)

    # --- inventories (before any write by B1)
    def row_counts():
        counts = load(os.path.join(raw, "db_row_counts.json"))
        ref = inventory.get("table_row_counts")
        if counts is None or ref is None:
            return False, "faltan conteos (base restaurada o inventario)"
        diff = {t: (ref.get(t), counts.get(t)) for t in sorted(set(ref) | set(counts)) if ref.get(t) != counts.get(t)}
        return not diff, f"{len(ref)} tablas; diferencias (inventario, restaurada): {diff}" if diff else f"{len(ref)} tablas iguales"

    disk = {}
    cur = None
    for line in read(os.path.join(raw, "disk_refs.txt")).splitlines():
        if line.startswith("## "):
            cur = line[3:]
            disk[cur] = {}
        elif cur and " " in line:
            k, v = line.split(" ", 1)
            disk[cur][k] = v

    def repos_db_vs_disk():
        db = set(read(os.path.join(raw, "db_repos.txt")).split())
        if not db and not disk:
            return False, "no se pudo leer la base ni el disco restaurados"
        only_db, only_disk = sorted(db - set(disk)), sorted(set(disk) - db)
        ok = not only_db and not only_disk
        return ok, f"{len(db)} en la base, {len(disk)} en disco" + ("" if ok else f"; solo en base {only_db}; solo en disco {only_disk}")

    def refs_vs_inventory():
        ref = inventory.get("repository_refs")
        if ref is None or not disk:
            return False, "faltan refs (inventario o disco restaurado)"
        diffs = []
        for repo in sorted(set(ref) | set(disk)):
            a, b = ref.get(repo), disk.get(repo)
            if a != b:
                if a is None or b is None:
                    diffs.append(f"{repo}: {'falta en disco' if b is None else 'no está en el inventario'}")
                else:
                    keys = sorted(k for k in set(a) | set(b) if a.get(k) != b.get(k))
                    diffs.append(f"{repo}: {keys[:5]}")
        return not diffs, f"{len(ref)} repositorios iguales" if not diffs else "; ".join(diffs[:8])

    rep.run("table_row_counts_vs_inventory", row_counts)
    rep.run("repositories_db_vs_disk", repos_db_vs_disk)
    rep.run("refs_vs_inventory", refs_vs_inventory)

    # --- witnesses
    def witness_present(w):
        repo = w["repo"]
        if w["type"] == "commit":
            st, _ = http(f"/api/v1/repos/{repo}/git/commits/{w['commit_sha']}", user, password)
            return st == 200
        if w["type"] == "issue":
            st, body = http(f"/api/v1/repos/{repo}/issues/{w['issue_number']}", user, password)
            return st == 200 and (body or {}).get("title") == w["title"]
        if w["type"] == "attachment":
            return attachment_sha(w) == w["sha256"]
        if w["type"] == "lfs":
            return lfs_sha(w) == w["lfs_oid"]
        return False

    def attachment_sha(w):
        st, body = http(f"/api/v1/repos/{w['repo']}/issues/{w['issue_number']}/assets/{w['attachment_id']}", user, password)
        if st != 200:
            return f"asset HTTP {st}"
        st, data = http(body["browser_download_url"], user, password, raw=True)
        return hashlib.sha256(data).hexdigest() if st == 200 else f"descarga HTTP {st}"

    def lfs_sha(w):
        st, data = http(f"/{w['repo']}/media/commit/{w['commit_sha']}/{w['path']}", user, password, raw=True)
        return hashlib.sha256(data).hexdigest() if st == 200 else f"HTTP {st}"

    def pre_of(kind):
        return [w for w in pre.get("witnesses", []) if w["type"] == kind]

    def attachment_hash():
        ws = pre_of("attachment")
        bad = [f"{w['id']}: {attachment_sha(w)}" for w in ws if attachment_sha(w) != w["sha256"]]
        return bool(ws) and not bad, f"{len(ws)} adjuntos testigo" + ("" if not bad else f"; distinto: {bad}")

    def lfs_content():
        ws = pre_of("lfs")
        bad = [f"{w['id']}: {lfs_sha(w)}" for w in ws if lfs_sha(w) != w["lfs_oid"]]
        return bool(ws) and not bad, f"{len(ws)} objetos LFS testigo" + ("" if not bad else f"; distinto: {bad}")

    def issue_exists():
        ws = pre_of("issue")
        bad = [w["id"] for w in ws if not witness_present(w)]
        return bool(ws) and not bad, f"{len(ws)} issues testigo" + ("" if not bad else f"; faltan: {bad}")

    def totp_login():
        s = load(os.path.join(run, "secrets", "totp.json"))
        st, body = http("/api/v1/user", s["user"], env["SEED_USER_PASSWORD"], otp=totp(s["secret_base32"]))
        return st == 200 and (body or {}).get("login") == s["user"], f"{s['user']}: HTTP {st}"

    def git_push():
        repo = next((w["repo"] for w in pre.get("witnesses", []) if w["type"] == "commit"), None)
        if not repo:
            return False, "sin repositorio testigo"
        url = f"http://{user}:{password}@gitea:3000/{repo}.git"
        genv = dict(os.environ, GIT_LFS_SKIP_SMUDGE="1", GIT_AUTHOR_NAME="b1", GIT_AUTHOR_EMAIL="b1@example.test",
                    GIT_COMMITTER_NAME="b1", GIT_COMMITTER_EMAIL="b1@example.test", GIT_TERMINAL_PROMPT="0")
        with tempfile.TemporaryDirectory() as wd:
            def g(*a):
                return subprocess.run(["git", *a], cwd=wd, env=genv, capture_output=True, text=True, timeout=120)
            r = g("clone", "-q", url, ".")
            if r.returncode:
                return False, f"clone: {r.stderr.strip()[-200:]}"
            with open(os.path.join(wd, f"b1-push-{int(time.time())}.txt"), "w") as f:
                f.write("b1 push check\n")
            g("add", "-A")
            g("commit", "-q", "-m", "b1 push check")
            r = g("push", "-q", "origin", "HEAD")
            return r.returncode == 0, f"push a {repo}: " + ("ok" if r.returncode == 0 else r.stderr.strip()[-200:])

    rep.run("witness_attachment_hash", attachment_hash)
    rep.run("witness_lfs_content", lfs_content)
    rep.run("witness_issue_exists", issue_exists)
    rep.run("totp_login", totp_login)
    rep.run("git_push_witness_repo", git_push)
    functional_end = now()

    # --- objectives
    def rto():
        if "restore_started_at" not in restore:
            return False, "restore.json sin restore_started_at"
        functional_ok = all(s["status"] == "ok" for s in rep.steps
                            if s["name"] in ("b0", "witness_attachment_hash", "witness_lfs_content",
                                             "witness_issue_exists", "totp_login", "git_push_witness_repo"))
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
    report = {"baseline": "B1", "run_id": os.path.basename(run), "result": "pass" if ok else "fail",
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
