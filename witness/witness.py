"""Witness objects: reference truth of the experiment (not a verification).

  pre:  one commit, one issue, one attachment and one LFS object, before the backup.
  post: one witness per minute after the backup, rotating commit -> issue -> attachment -> LFS,
        until the incident time T = start + INCIDENT_MINUTES.

Every witness records type, unique id (uuid4), creation time and sha256.
Writes <run>/witnesses_pre.json or <run>/witnesses_post.json.
"""

import argparse
import datetime
import json
import os
import sys
import tempfile
import time
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import gitea  # noqa: E402

OWNER, REPO = "alice", "witness"
ROTATION = ["commit", "issue", "attachment", "lfs"]


class Witnesses:
    def __init__(self):
        self.password = os.environ["SEED_USER_PASSWORD"]
        self.api = gitea.Client(OWNER, self.password)
        self.admin = gitea.Client(os.environ["GITEA_ADMIN_USER"], os.environ["GITEA_ADMIN_PASSWORD"])

    def ensure_repo(self):
        try:
            self.api.get(f"/repos/{OWNER}/{REPO}")
        except gitea.ApiError:
            self.admin.post(f"/admin/users/{OWNER}/repos", {"name": REPO, "private": False, "default_branch": "main"})

    def _push_file(self, rel_path, content, wid, lfs):
        with tempfile.TemporaryDirectory() as wd:
            gitea.git(["clone", "-q", gitea.repo_url(OWNER, REPO, OWNER, self.password), wd],
                      env={"GIT_LFS_SKIP_SMUDGE": "1"})
            gitea.git(["lfs", "install", "--local"], cwd=wd)
            attrs = os.path.join(wd, ".gitattributes")
            if not os.path.exists(attrs):
                with open(attrs, "w") as f:
                    f.write("lfs/*.bin filter=lfs diff=lfs merge=lfs -text\n")
            os.makedirs(os.path.dirname(os.path.join(wd, rel_path)), exist_ok=True)
            with open(os.path.join(wd, rel_path), "wb") as f:
                f.write(content)
            env = {"GIT_AUTHOR_NAME": OWNER, "GIT_AUTHOR_EMAIL": f"{OWNER}@example.test",
                   "GIT_COMMITTER_NAME": OWNER, "GIT_COMMITTER_EMAIL": f"{OWNER}@example.test"}
            gitea.git(["add", "-A"], cwd=wd)
            gitea.git(["commit", "-q", "-m", f"witness {wid}"], cwd=wd, env=env)
            gitea.git(["push", "-q", "origin", "HEAD:main"], cwd=wd)
            return gitea.git(["rev-parse", "HEAD"], cwd=wd, capture=True)

    def commit(self, wid, created_at, **_):
        content = f"witness {wid}\ncreated_at {created_at}\n".encode()
        path = f"witness/commit-{wid}.txt"
        sha = self._push_file(path, content, wid, lfs=False)
        return {"repo": f"{OWNER}/{REPO}", "commit_sha": sha, "path": path, "sha256": gitea.sha256_bytes(content)}

    def lfs(self, wid, **_):
        content = os.urandom(32 * 1024)
        path = f"lfs/lfs-{wid}.bin"
        sha = self._push_file(path, content, wid, lfs=True)
        oid = gitea.sha256_bytes(content)
        return {"repo": f"{OWNER}/{REPO}", "commit_sha": sha, "path": path, "lfs_oid": oid,
                "size": len(content), "sha256": oid}

    def issue(self, wid, created_at, **_):
        title, body = f"witness-issue {wid}", f"witness {wid}\ncreated_at {created_at}\n"
        issue = self.api.post(f"/repos/{OWNER}/{REPO}/issues", {"title": title, "body": body})
        return {"repo": f"{OWNER}/{REPO}", "issue_number": issue["number"], "issue_id": issue["id"],
                "title": title, "sha256": gitea.sha256_bytes(body.encode())}

    def attachment(self, wid, issue_number, **_):
        content = os.urandom(8 * 1024).hex().encode()
        name = f"witness-{wid}.txt"
        att = self.api.upload_attachment(OWNER, REPO, issue_number, name, content)
        return {"repo": f"{OWNER}/{REPO}", "issue_number": issue_number, "attachment_uuid": att["uuid"],
                "attachment_id": att["id"], "name": name, "size": len(content),
                "sha256": gitea.sha256_bytes(content)}

    def make(self, kind, **kw):
        wid, created_at = str(uuid.uuid4()), gitea.now_iso()
        rec = {"type": kind, "id": wid, "created_at": created_at}
        rec.update(getattr(self, kind)(wid=wid, created_at=created_at, **kw))
        print(f"witness {kind} {wid}", flush=True)
        return rec


def run_pre(w, run_dir):
    w.ensure_repo()
    started = gitea.now_iso()
    items = [w.make("commit"), w.make("lfs")]
    issue = w.make("issue")
    items.append(issue)
    items.append(w.make("attachment", issue_number=issue["issue_number"]))
    gitea.write_json(os.path.join(run_dir, "witnesses_pre.json"),
                     {"phase": "pre_backup", "started_at": started, "finished_at": gitea.now_iso(), "witnesses": items})


def run_post(w, run_dir, minutes):
    with open(os.path.join(run_dir, "witnesses_pre.json")) as f:
        pre = json.load(f)
    issue_number = next(x["issue_number"] for x in pre["witnesses"] if x["type"] == "issue")
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
        rec = w.make(kind, issue_number=issue_number)
        rec["minute"] = i
        out["witnesses"].append(rec)
        gitea.write_json(path, out)
    out["finished_at"] = gitea.now_iso()
    gitea.write_json(path, out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("phase", choices=["pre", "post"])
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()
    gitea.wait_for_gitea()
    w = Witnesses()
    if args.phase == "pre":
        run_pre(w, args.run_dir)
    else:
        run_post(w, args.run_dir, int(os.environ["INCIDENT_MINUTES"]))


if __name__ == "__main__":
    main()
