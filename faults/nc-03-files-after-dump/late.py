"""Uploads a folder of alice with three files after pg_dump. Prints JSON with paths and sha256."""

import json
import os
import sys
import uuid

sys.path.insert(0, "/work/nc/pylib")
import nextcloud as nc  # noqa: E402

c = nc.Client("alice", os.environ["SEED_USER_PASSWORD"])
nc.wait_for_nextcloud()
tag = str(uuid.uuid4())
c.mkdir("late")
files = []
for i in range(3):
    content = f"created after pg_dump {tag} #{i}\n".encode()
    path = f"late/late-{tag}-{i}.txt"
    c.put(path, content)
    files.append({"path": path, "sha256": nc.sha256_bytes(content)})
print(json.dumps({"user": "alice", "tag": tag, "files": files}, indent=2))
