"""Pushes two branches of alice/notes whose names differ only in case, as loose refs."""

import json
import os
import sys
import tempfile

sys.path.insert(0, "/work/pylib")
import gitea  # noqa: E402

pw = os.environ["SEED_USER_PASSWORD"]
with tempfile.TemporaryDirectory() as wd:
    gitea.git(["clone", "-q", gitea.repo_url("alice", "notes", "alice", pw), wd])
    upper = gitea.git(["rev-parse", "HEAD"], cwd=wd, capture=True)
    lower = gitea.git(["rev-parse", "HEAD~1"], cwd=wd, capture=True)
    gitea.git(["push", "-q", "origin", f"{upper}:refs/heads/Case-Test", f"{lower}:refs/heads/case-test"], cwd=wd)
print(json.dumps({"repo": "alice/notes", "Case-Test": upper, "case-test": lower}))
