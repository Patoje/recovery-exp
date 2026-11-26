"""Witness objects of the Nextcloud environment: reference truth of the experiment (not a verification).

  pre:  one text file, one binary file and one file shared with another user, before the backup.
  post: one witness per minute after the backup, rotating file -> binary -> share,
        until the incident time T = start + INCIDENT_MINUTES. An attempt that fails (for example,
        Nextcloud in maintenance mode) is recorded under "failed_attempts" and the next minute goes on.

Every witness records type, unique id (uuid4), creation time and sha256 of its content.
A share witness also records the path under which the recipient sees it.
Writes <run>/witnesses_pre.json or <run>/witnesses_post.json.
"""

import argparse
import datetime
import json
import os
import sys
import time
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import nextcloud as nc  # noqa: E402

OWNER, RECIPIENT, FOLDER = "alice", "bob", "witness"
ROTATION = ["file", "binary", "share"]


class Witnesses:
    def __init__(self):
        self.password = os.environ["SEED_USER_PASSWORD"]
        self.api = nc.Client(OWNER, self.password)

    def ensure_folder(self):
        self.api.mkdir(FOLDER)

    def file(self, wid, created_at, **_):
        content = f"witness {wid}\ncreated_at {created_at}\n".encode()
        path = f"{FOLDER}/file-{wid}.txt"
        self.api.put(path, content)
        return {"user": OWNER, "path": path, "sha256": nc.sha256_bytes(content)}

    def binary(self, wid, **_):
        content = os.urandom(32 * 1024)
        path = f"{FOLDER}/bin-{wid}.bin"
        self.api.put(path, content)
        return {"user": OWNER, "path": path, "size": len(content), "sha256": nc.sha256_bytes(content)}

    def share(self, wid, created_at, **_):
        content = f"shared witness {wid}\ncreated_at {created_at}\n".encode()
        name = f"share-{wid}.txt"
        path = f"{FOLDER}/{name}"
        self.api.put(path, content)
        s = self.api.share_with_user(path, RECIPIENT)
        return {"user": OWNER, "path": path, "share_id": s.get("id"), "share_with": RECIPIENT,
                "recipient_path": name, "sha256": nc.sha256_bytes(content)}

    def make(self, kind, **kw):
        wid, created_at = str(uuid.uuid4()), nc.now_iso()
        rec = {"type": kind, "id": wid, "created_at": created_at}
        rec.update(getattr(self, kind)(wid=wid, created_at=created_at, **kw))
        print(f"witness {kind} {wid}", flush=True)
        return rec


def run_pre(w, run_dir):
    w.ensure_folder()
    started = nc.now_iso()
    items = [w.make("file"), w.make("binary"), w.make("share")]
    nc.write_json(os.path.join(run_dir, "witnesses_pre.json"),
                  {"phase": "pre_backup", "started_at": started, "finished_at": nc.now_iso(), "witnesses": items})


def run_post(w, run_dir, minutes):
    t0 = time.time()
    incident_at = t0 + 60 * minutes
    iso = lambda t: datetime.datetime.fromtimestamp(t, datetime.timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")  # noqa: E731
    out = {"phase": "post_backup", "incident_minutes": minutes, "started_at": iso(t0),
           "incident_at": iso(incident_at), "witnesses": []}
    path = os.path.join(run_dir, "witnesses_post.json")
    for i in range(1, minutes + 1):
        delay = t0 + 60 * i - time.time()
        if delay > 0:
            time.sleep(delay)
        kind = ROTATION[(i - 1) % len(ROTATION)]
        attempted_at = nc.now_iso()
        try:
            rec = w.make(kind)
        except Exception as e:  # Nextcloud unavailable: record the attempt and go on with the next minute
            out.setdefault("failed_attempts", []).append(
                {"type": kind, "minute": i, "status": "failed", "attempted_at": attempted_at, "error": repr(e)[:300]})
            print(f"witness {kind} failed at minute {i}: {e!r}", flush=True)
            nc.write_json(path, out)
            continue
        rec["minute"] = i
        out["witnesses"].append(rec)
        nc.write_json(path, out)
    out["finished_at"] = nc.now_iso()
    nc.write_json(path, out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("phase", choices=["pre", "post"])
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()
    w = Witnesses()
    if args.phase == "pre":
        run_pre(w, args.run_dir)
    else:
        run_post(w, args.run_dir, int(os.environ.get("INCIDENT_MINUTES", "5")))


if __name__ == "__main__":
    main()
