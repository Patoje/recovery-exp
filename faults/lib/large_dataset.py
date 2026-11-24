"""Fixed-size dataset for scenarios 00-healthy-large and 12: 512 MiB of LFS objects.

16 files of 32 MiB, generated from SEED, pushed to alice/large. Prints a JSON record.
"""

import json
import os
import random
import sys
import tempfile

sys.path.insert(0, "/work/pylib")
import gitea  # noqa: E402

FILES, SIZE = 16, 32 * 1024 * 1024
OWNER, REPO = "alice", "large"


def main():
    password = os.environ["SEED_USER_PASSWORD"]
    admin = gitea.Client(os.environ["GITEA_ADMIN_USER"], os.environ["GITEA_ADMIN_PASSWORD"])
    admin.post(f"/admin/users/{OWNER}/repos", {"name": REPO, "private": False, "default_branch": "main"})
    rng = random.Random(int(os.environ["SEED"]) + 512)
    oids = []
    with tempfile.TemporaryDirectory(dir="/tmp") as wd:
        gitea.git(["init", "-q", "-b", "main", wd])
        gitea.git(["lfs", "install", "--local"], cwd=wd)
        with open(os.path.join(wd, ".gitattributes"), "w") as f:
            f.write("*.bin filter=lfs diff=lfs merge=lfs -text\n")
        for i in range(FILES):
            data = rng.randbytes(SIZE)
            oids.append(gitea.sha256_bytes(data))
            with open(os.path.join(wd, f"large-{i:02d}.bin"), "wb") as f:
                f.write(data)
        env = {"GIT_AUTHOR_NAME": OWNER, "GIT_AUTHOR_EMAIL": f"{OWNER}@example.test",
               "GIT_COMMITTER_NAME": OWNER, "GIT_COMMITTER_EMAIL": f"{OWNER}@example.test",
               "GIT_AUTHOR_DATE": "2026-02-01T00:00:00Z", "GIT_COMMITTER_DATE": "2026-02-01T00:00:00Z"}
        gitea.git(["add", "-A"], cwd=wd)
        gitea.git(["commit", "-q", "-m", "large dataset"], cwd=wd, env=env)
        gitea.git(["remote", "add", "origin", gitea.repo_url(OWNER, REPO, OWNER, password)], cwd=wd)
        gitea.git(["push", "-q", "origin", "main"], cwd=wd)
    print(json.dumps({"repo": f"{OWNER}/{REPO}", "files": FILES, "file_bytes": SIZE,
                      "total_bytes": FILES * SIZE, "lfs_oids": oids, "created_at": gitea.now_iso()}))


if __name__ == "__main__":
    main()
