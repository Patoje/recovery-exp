"""Deterministic test dataset for Gitea.

All content (users, repos, commits, branches, issues, comments, attachments, LFS
objects) derives from SEED. Commit dates are fixed, so commit SHAs are reproducible.
Not deterministic: Gitea-internal ids and timestamps, and the TOTP secret (Gitea
generates it during enrollment).

Writes <run>/dataset.json and <run>/secrets/totp.json.
"""

import argparse
import datetime
import os
import random
import sys
import tempfile

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import gitea  # noqa: E402

USERS = ["alice", "bob", "carol", "dave", "erin"]
TOTP_USER = "erin"
# (owner, name, uses LFS)
REPOS = [
    ("alice", "notes", False),
    ("alice", "assets", True),
    ("bob", "api", False),
    ("bob", "cli", False),
    ("carol", "website", False),
    ("carol", "media", True),
    ("dave", "infra", False),
    ("erin", "docs", False),
]
WORDS = ("backup restore snapshot volume disk index query commit branch merge issue "
         "comment release deploy config cache queue token report metric alert").split()
BASE_DATE = datetime.datetime(2026, 1, 1, tzinfo=datetime.timezone.utc)


def sentence(rng, n):
    return " ".join(rng.choice(WORDS) for _ in range(n))


class Seeder:
    def __init__(self, seed, password, admin):
        self.rng = random.Random(seed)
        self.password = password
        self.admin = admin
        self.clock = 0
        self.summary = {"users": [], "repos": [], "issues": 0, "comments": 0,
                        "attachments": 0, "attachment_bytes": 0, "lfs_objects": 0, "lfs_bytes": 0,
                        "commits": 0, "branches": 0}

    def client(self, user):
        return gitea.Client(user, self.password)

    def next_date(self):
        self.clock += 1
        return (BASE_DATE + datetime.timedelta(minutes=17 * self.clock)).strftime("%Y-%m-%dT%H:%M:%SZ")

    def create_users(self):
        for u in USERS:
            self.admin.post("/admin/users", {
                "username": u, "email": f"{u}@example.test", "password": self.password,
                "must_change_password": False, "full_name": u.capitalize(),
            })
            self.summary["users"].append(u)

    def commit(self, wd, owner, message):
        date = self.next_date()
        env = {"GIT_AUTHOR_NAME": owner, "GIT_AUTHOR_EMAIL": f"{owner}@example.test",
               "GIT_COMMITTER_NAME": owner, "GIT_COMMITTER_EMAIL": f"{owner}@example.test",
               "GIT_AUTHOR_DATE": date, "GIT_COMMITTER_DATE": date}
        gitea.git(["add", "-A"], cwd=wd)
        gitea.git(["commit", "-q", "-m", message], cwd=wd, env=env)
        self.summary["commits"] += 1

    def write_text_change(self, wd, i):
        name = f"doc-{self.rng.randint(1, 6)}.md"
        with open(os.path.join(wd, name), "a") as f:
            f.write(f"## {i}\n\n{sentence(self.rng, self.rng.randint(8, 40))}\n\n")

    def write_lfs_object(self, wd, i):
        size = self.rng.randint(64 * 1024, 512 * 1024)
        data = self.rng.randbytes(size)
        with open(os.path.join(wd, f"blob-{i}.bin"), "wb") as f:
            f.write(data)
        self.summary["lfs_objects"] += 1
        self.summary["lfs_bytes"] += size

    def create_repo(self, owner, name, lfs):
        self.admin.post(f"/admin/users/{owner}/repos", {"name": name, "private": False, "default_branch": "main"})
        info = {"name": f"{owner}/{name}", "lfs": lfs, "branches": ["main"]}
        with tempfile.TemporaryDirectory() as wd:
            gitea.git(["init", "-q", "-b", "main", wd])
            if lfs:
                gitea.git(["lfs", "install", "--local"], cwd=wd)
                with open(os.path.join(wd, ".gitattributes"), "w") as f:
                    f.write("*.bin filter=lfs diff=lfs merge=lfs -text\n")
            with open(os.path.join(wd, "README.md"), "w") as f:
                f.write(f"# {name}\n\n{sentence(self.rng, 12)}\n")
            self.commit(wd, owner, "initial commit")
            for i in range(self.rng.randint(8, 20)):
                self.write_text_change(wd, i)
                if lfs and i % 4 == 0:
                    self.write_lfs_object(wd, i)
                self.commit(wd, owner, sentence(self.rng, 5))
            for b in range(self.rng.randint(1, 3)):
                branch = f"feature/{self.rng.choice(WORDS)}-{b}"
                back = self.rng.randint(0, 4)
                gitea.git(["checkout", "-q", "-b", branch, f"main~{back}"], cwd=wd)
                for i in range(self.rng.randint(1, 5)):
                    self.write_text_change(wd, 100 + i)
                    self.commit(wd, owner, f"{branch}: {sentence(self.rng, 4)}")
                gitea.git(["checkout", "-q", "main"], cwd=wd)
                info["branches"].append(branch)
                self.summary["branches"] += 1
            gitea.git(["remote", "add", "origin", gitea.repo_url(owner, name, owner, self.password)], cwd=wd)
            gitea.git(["push", "-q", "origin", "--all"], cwd=wd)
        self.summary["repos"].append(info)

    def create_issues(self, owner, name, authors):
        for _ in range(self.rng.randint(2, 5)):
            author = self.rng.choice(authors)
            issue = self.client(author).post(f"/repos/{owner}/{name}/issues", {
                "title": sentence(self.rng, 5), "body": sentence(self.rng, 30)})
            self.summary["issues"] += 1
            for _ in range(self.rng.randint(0, 3)):
                self.client(self.rng.choice(authors)).post(
                    f"/repos/{owner}/{name}/issues/{issue['number']}/comments",
                    {"body": sentence(self.rng, 15)})
                self.summary["comments"] += 1
            if self.rng.random() < 0.5:
                content = self.rng.randbytes(self.rng.randint(1024, 64 * 1024)).hex().encode()
                self.client(author).upload_attachment(owner, name, issue["number"],
                                                      f"attachment-{issue['number']}.txt", content)
                self.summary["attachments"] += 1
                self.summary["attachment_bytes"] += len(content)

    def enroll_totp(self, user):
        web = gitea.WebSession()
        _, html = web.get("/user/login")
        status, url, html = web.post("/user/login", gitea.WebSession.form(
            html, user_name=user, password=self.password))
        if status != 200 or "/user/login" in url:
            raise RuntimeError(f"web login failed for {user}: {status} {url}")
        _, html = web.get("/user/settings/security/two_factor/enroll")
        secrets = set(gitea.re.findall(r"\b[A-Z2-7]{32,}\b", html))
        if len(secrets) != 1:
            raise RuntimeError(f"could not read TOTP secret from enrollment page ({len(secrets)} candidates)")
        secret = secrets.pop()
        status, url, html = web.post("/user/settings/security/two_factor/enroll",
                                     gitea.WebSession.form(html, passcode=gitea.totp(secret)))
        if status != 200 or "two_factor/enroll" in url:
            raise RuntimeError(f"TOTP enrollment failed: {status} {url}")
        # Basic auth must now require the OTP header.
        try:
            self.client(user).get("/user")
            raise RuntimeError("2FA user could log in without OTP")
        except gitea.ApiError:
            pass
        gitea.Client(user, self.password, otp=lambda: gitea.totp(secret)).get("/user")
        return secret


def data_size(root="/data"):
    total = 0
    for dirpath, _, files in os.walk(root):
        for fn in files:
            try:
                total += os.lstat(os.path.join(dirpath, fn)).st_size
            except FileNotFoundError:
                pass
    return total


def db_size():
    out = gitea.subprocess.run(["psql", "-At", "-c", "select pg_database_size(current_database())"],
                               check=True, stdout=gitea.subprocess.PIPE).stdout.decode().strip()
    return int(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()

    started = gitea.now_iso()
    gitea.wait_for_gitea()
    admin = gitea.Client(os.environ["GITEA_ADMIN_USER"], os.environ["GITEA_ADMIN_PASSWORD"])
    s = Seeder(int(os.environ["SEED"]), os.environ["SEED_USER_PASSWORD"], admin)

    s.create_users()
    for owner, name, lfs in REPOS:
        s.create_repo(owner, name, lfs)
    for owner, name, _ in REPOS:
        s.create_issues(owner, name, [u for u in USERS if u != TOTP_USER])
    secret = s.enroll_totp(TOTP_USER)

    secrets_dir = os.path.join(args.run_dir, "secrets")
    os.makedirs(secrets_dir, exist_ok=True)
    gitea.write_json(os.path.join(secrets_dir, "totp.json"), {
        "note": "TEST secret, valid only inside this experiment",
        "user": TOTP_USER, "secret_base32": secret, "algorithm": "SHA1", "digits": 6, "period": 30})

    d_bytes, db_bytes = data_size(), db_size()
    gitea.write_json(os.path.join(args.run_dir, "dataset.json"), {
        "seed": int(os.environ["SEED"]),
        "started_at": started,
        "finished_at": gitea.now_iso(),
        "totp_user": TOTP_USER,
        **s.summary,
        "size": {"data_dir_bytes": d_bytes, "database_bytes": db_bytes, "total_bytes": d_bytes + db_bytes},
    })
    print(f"seed ok: {len(REPOS)} repos, {s.summary['issues']} issues, total {d_bytes + db_bytes} bytes")


if __name__ == "__main__":
    main()
