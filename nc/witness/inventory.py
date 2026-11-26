"""Reference inventory of the Nextcloud environment at backup time (reference truth, not a verification).

Runs in maintenance mode. Read-only by construction: the data directory and config/ are mounted
read-only and the database session uses default_transaction_read_only=on.

  - row count per table (schema public)
  - user files known to the database (oc_filecache, home storages, files/ only, no directories)
  - user files on disk (<data>/<user>/files/...), with size
  - sha256 of the pre-backup witness files as stored on disk

Writes <run>/inventory.json.
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import nextcloud as nc  # noqa: E402

DATA_ROOT = "/data"

COUNT_SQL = """
select coalesce(json_object_agg(t.table_name, (xpath('/row/c/text()',
         query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint
       order by t.table_name), '{}'::json)
from information_schema.tables t
where t.table_schema = 'public' and t.table_type = 'BASE TABLE'
"""

DB_FILES_SQL = """
select coalesce(json_agg(x order by x), '[]'::json) from (
  select substr(s.id, 7) || '/' || f.path as x
  from oc_filecache f join oc_storages s on s.numeric_id = f.storage
  where s.id like 'home::%' and f.path like 'files/%'
    and f.mimetype <> (select id from oc_mimetypes where mimetype = 'httpd/unix-directory')) q
"""


def psql_json(sql):
    env = dict(os.environ, PGOPTIONS="-c default_transaction_read_only=on")
    out = subprocess.run(["psql", "-At", "-v", "ON_ERROR_STOP=1", "-c", sql],
                         check=True, stdout=subprocess.PIPE, env=env).stdout.decode()
    return json.loads(out)


def disk_files():
    out = {}
    for user in sorted(os.listdir(DATA_ROOT)):
        root = os.path.join(DATA_ROOT, user, "files")
        if not os.path.isdir(root):
            continue
        for d, _, names in os.walk(root):
            for n in names:
                p = os.path.join(d, n)
                out[f"{user}/{os.path.relpath(p, os.path.join(DATA_ROOT, user))}"] = os.path.getsize(p)
    return dict(sorted(out.items()))


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()
    taken = nc.now_iso()
    with open(os.path.join(args.run_dir, "witnesses_pre.json")) as f:
        pre = json.load(f)
    witness_files = []
    for w in pre["witnesses"]:
        loc = os.path.join(DATA_ROOT, w["user"], "files", w["path"])
        witness_files.append({"witness_id": w["id"], "type": w["type"], "location": loc,
                              "sha256": sha256_file(loc) if os.path.exists(loc) else None})
    inv = {"taken_at": taken, "maintenance_mode": True, "table_row_counts": psql_json(COUNT_SQL),
           "db_user_files": psql_json(DB_FILES_SQL), "disk_user_files": disk_files(),
           "witness_files": witness_files, "finished_at": nc.now_iso()}
    nc.write_json(os.path.join(args.run_dir, "inventory.json"), inv)
    print(f"inventario: {len(inv['table_row_counts'])} tablas, {len(inv['db_user_files'])} archivos en la base, "
          f"{len(inv['disk_user_files'])} en disco", flush=True)


if __name__ == "__main__":
    main()
