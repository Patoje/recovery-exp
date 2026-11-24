"""Adds a pre-backup issue witness whose title has emoji and non-Latin characters."""

import json
import os
import sys
import uuid

sys.path.insert(0, "/work/pylib")
import gitea  # noqa: E402

path = sys.argv[1]
wid, created_at = str(uuid.uuid4()), gitea.now_iso()
title = f"witness-emoji 😀🚀 日本語 Ελληνικά Кириллица {wid}"
body = f"witness {wid}\ncreated_at {created_at}\n😀 日本語\n"
api = gitea.Client("alice", os.environ["SEED_USER_PASSWORD"])
issue = api.post("/repos/alice/witness/issues", {"title": title, "body": body})
rec = {"type": "issue", "id": wid, "created_at": created_at, "repo": "alice/witness",
       "issue_number": issue["number"], "issue_id": issue["id"], "title": title,
       "sha256": gitea.sha256_bytes(body.encode())}
with open(path) as f:
    pre = json.load(f)
pre["witnesses"].append(rec)
gitea.write_json(path, pre)
print(json.dumps(rec, ensure_ascii=False))
