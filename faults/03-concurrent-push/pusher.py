"""Pushes a new commit on a new branch of alice/concurrent every 0.2 s until killed."""

import os
import sys
import tempfile
import time

sys.path.insert(0, "/work/pylib")
import gitea  # noqa: E402

pw = os.environ["SEED_USER_PASSWORD"]
admin = gitea.Client(os.environ["GITEA_ADMIN_USER"], os.environ["GITEA_ADMIN_PASSWORD"])
admin.post("/admin/users/alice/repos", {"name": "concurrent", "private": False, "default_branch": "main"})
env = {"GIT_AUTHOR_NAME": "alice", "GIT_AUTHOR_EMAIL": "alice@example.test",
       "GIT_COMMITTER_NAME": "alice", "GIT_COMMITTER_EMAIL": "alice@example.test"}
wd = tempfile.mkdtemp()
gitea.git(["init", "-q", "-b", "main", wd])
gitea.git(["remote", "add", "origin", gitea.repo_url("alice", "concurrent", "alice", pw)], cwd=wd)
i = 0
while True:
    with open(os.path.join(wd, "counter.txt"), "w") as f:
        f.write(f"{i}\n")
    gitea.git(["add", "-A"], cwd=wd)
    gitea.git(["commit", "-q", "-m", f"push {i}"], cwd=wd, env=env)
    gitea.git(["push", "-q", "origin", f"HEAD:refs/heads/main", f"HEAD:refs/heads/c-{i:05d}"], cwd=wd)
    print(f"{gitea.now_iso()} push {i}", flush=True)
    i += 1
    time.sleep(0.2)
