"""Creates alice/late and pushes one commit. Prints the commit SHA."""

import os
import sys
import tempfile

sys.path.insert(0, "/work/pylib")
import gitea  # noqa: E402

pw = os.environ["SEED_USER_PASSWORD"]
gitea.wait_for_gitea()
gitea.Client(os.environ["GITEA_ADMIN_USER"], os.environ["GITEA_ADMIN_PASSWORD"]).post(
    "/admin/users/alice/repos", {"name": "late", "private": False, "default_branch": "main"})
env = {"GIT_AUTHOR_NAME": "alice", "GIT_AUTHOR_EMAIL": "alice@example.test",
       "GIT_COMMITTER_NAME": "alice", "GIT_COMMITTER_EMAIL": "alice@example.test"}
with tempfile.TemporaryDirectory() as wd:
    gitea.git(["init", "-q", "-b", "main", wd])
    with open(os.path.join(wd, "README.md"), "w") as f:
        f.write("created after pg_dump\n")
    gitea.git(["add", "-A"], cwd=wd)
    gitea.git(["commit", "-q", "-m", "late"], cwd=wd, env=env)
    gitea.git(["push", "-q", gitea.repo_url("alice", "late", "alice", pw), "HEAD:main"], cwd=wd)
    print(gitea.git(["rev-parse", "HEAD"], cwd=wd, capture=True))
