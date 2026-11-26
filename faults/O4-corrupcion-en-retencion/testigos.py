"""Witnesses of hidden fault D: testigo-$WID.txt (one push of one small commit) and
testigo-$WID.bin (attachment), with their sha256 at creation time. Prints JSON.

Usage (runner of the original project): python3 faults/O4-corrupcion-en-retencion/testigos.py ISSUE_NUMBER
"""

import json
import os
import sys
import tempfile
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "pylib"))
import gitea  # noqa: E402

OWNER, REPO = "alice", "witness"
issue_number = int(sys.argv[1])
password = os.environ["SEED_USER_PASSWORD"]
wid = str(uuid.uuid4())
txt_name, bin_name = f"testigo-{wid}.txt", f"testigo-{wid}.bin"
txt = f"testigo {wid}\ncreated_at {gitea.now_iso()}\n".encode()

with tempfile.TemporaryDirectory() as wd:
    gitea.git(["clone", "-q", gitea.repo_url(OWNER, REPO, OWNER, password), wd], env={"GIT_LFS_SKIP_SMUDGE": "1"})
    with open(os.path.join(wd, txt_name), "wb") as f:
        f.write(txt)
    env = {"GIT_AUTHOR_NAME": OWNER, "GIT_AUTHOR_EMAIL": f"{OWNER}@example.test",
           "GIT_COMMITTER_NAME": OWNER, "GIT_COMMITTER_EMAIL": f"{OWNER}@example.test"}
    gitea.git(["add", txt_name], cwd=wd)
    gitea.git(["commit", "-q", "-m", f"testigo {wid}"], cwd=wd, env=env)
    gitea.git(["push", "-q", "origin", "HEAD:main"], cwd=wd, env={"GIT_LFS_SKIP_SMUDGE": "1"})
    commit = gitea.git(["rev-parse", "HEAD"], cwd=wd, capture=True)
    blob = gitea.git(["rev-parse", f"HEAD:{txt_name}"], cwd=wd, capture=True)

content = os.urandom(8 * 1024)
att = gitea.Client(OWNER, password).upload_attachment(OWNER, REPO, issue_number, bin_name, content)
print(json.dumps({"wid": wid, "repo": f"{OWNER}/{REPO}", "created_at": gitea.now_iso(),
                  "txt": {"path": txt_name, "commit_sha": commit, "blob_sha": blob, "sha256": gitea.sha256_bytes(txt)},
                  "bin": {"name": bin_name, "uuid": att["uuid"], "issue_number": issue_number, "size": len(content),
                          "sha256": gitea.sha256_bytes(content)}}, indent=2))
