"""Reference inventory at backup time (reference truth, not a verification).

Runs while Gitea is stopped. Read-only by construction: /data is mounted read-only
and the database session uses default_transaction_read_only=on.

  - row count per table (schema public)
  - refs of every repository (name -> SHA)
  - sha256 of the pre-backup witness files as stored in /data

Writes <run>/inventory.json.
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import gitea  # noqa: E402

REPOS_ROOT = "/data/git/repositories"
LFS_ROOT = "/data/git/lfs"
ATTACH_ROOT = "/data/gitea/attachments"

COUNT_SQL = """
select coalesce(json_object_agg(t.table_name, (xpath('/row/c/text()',
         query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint
       order by t.table_name), '{}'::json)
from information_schema.tables t
where t.table_schema = 'public' and t.table_type = 'BASE TABLE'
"""


def table_counts():
    env = dict(os.environ, PGOPTIONS="-c default_transaction_read_only=on")
    out = subprocess.run(["psql", "-At", "-v", "ON_ERROR_STOP=1", "-c", COUNT_SQL],
                         check=True, stdout=subprocess.PIPE, env=env).stdout.decode()
    return json.loads(out)


def git_ro(git_dir, args, binary=False):
    out = subprocess.run(["git", "-c", "safe.directory=*", "--git-dir", git_dir] + args,
                         check=True, stdout=subprocess.PIPE).stdout
    return out if binary else out.decode()


def repo_refs():
    refs = {}
    for owner in sorted(os.listdir(REPOS_ROOT)):
        odir = os.path.join(REPOS_ROOT, owner)
        if not os.path.isdir(odir):
            continue
        for repo in sorted(os.listdir(odir)):
            if not repo.endswith(".git"):
                continue
            out = git_ro(os.path.join(odir, repo), ["for-each-ref", "--format=%(refname) %(objectname)"])
            refs[f"{owner}/{repo[:-4]}"] = dict(line.split(" ", 1) for line in out.splitlines() if line)
    return refs


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def witness_files(pre):
    files = []
    for w in pre["witnesses"]:
        if w["type"] == "commit":
            owner, name = w["repo"].split("/")
            git_dir = os.path.join(REPOS_ROOT, owner, name + ".git")
            blob = git_ro(git_dir, ["show", f"{w['commit_sha']}:{w['path']}"], binary=True)
            files.append({"witness_id": w["id"], "type": "commit",
                          "location": f"{w['repo']}@{w['commit_sha']}:{w['path']}",
                          "sha256": hashlib.sha256(blob).hexdigest()})
        elif w["type"] == "lfs":
            oid = w["lfs_oid"]
            path = os.path.join(LFS_ROOT, oid[0:2], oid[2:4], oid[4:])
            files.append({"witness_id": w["id"], "type": "lfs", "location": path, "sha256": sha256_file(path)})
        elif w["type"] == "attachment":
            u = w["attachment_uuid"]
            path = os.path.join(ATTACH_ROOT, u[0], u[1], u)
            files.append({"witness_id": w["id"], "type": "attachment", "location": path, "sha256": sha256_file(path)})
    return files


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()
    with open(os.path.join(args.run_dir, "witnesses_pre.json")) as f:
        pre = json.load(f)
    started = gitea.now_iso()
    inv = {
        "taken_at": started,
        "gitea_stopped": True,
        "table_row_counts": table_counts(),
        "repository_refs": repo_refs(),
        "witness_files": witness_files(pre),
    }
    inv["finished_at"] = gitea.now_iso()
    gitea.write_json(os.path.join(args.run_dir, "inventory.json"), inv)
    print(f"inventory ok: {len(inv['table_row_counts'])} tables, {len(inv['repository_refs'])} repos")


if __name__ == "__main__":
    main()
