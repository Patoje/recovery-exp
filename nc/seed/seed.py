"""Deterministic Nextcloud dataset: users, folders, text and binary files, shares.

Same SEED -> same users, paths and file contents. Writes <run>/dataset.json.
"""

import argparse
import os
import random
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import nextcloud as nc  # noqa: E402

USERS = ["alice", "bob", "carol", "dave", "erin"]
FOLDERS = ["Documents", "Notes", "Photos"]
WORDS = ("backup restore snapshot verify contract witness inventory retention archive volume "
         "database config service network storage digest").split()
TEXT_FILES_PER_FOLDER = 6
BINARY_FILES = 3          # per user, in Photos
BINARY_SIZE = 256 * 1024  # bytes


def sentence(rng, n):
    return " ".join(rng.choice(WORDS) for _ in range(n)).capitalize() + "."


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-dir", required=True)
    args = ap.parse_args()
    seed = int(os.environ["SEED"])
    password = os.environ["SEED_USER_PASSWORD"]
    admin = nc.Client(os.environ["NC_ADMIN_USER"], os.environ["NC_ADMIN_PASSWORD"])
    nc.wait_for_nextcloud()
    rng = random.Random(seed)
    files, shares = [], []
    for u in USERS:
        admin.create_user(u, password)
    for u in USERS:
        c = nc.Client(u, password)
        for folder in FOLDERS:
            c.mkdir(folder)
            if folder == "Photos":
                for i in range(BINARY_FILES):
                    content = rng.randbytes(BINARY_SIZE)
                    path = f"{folder}/img-{i:02d}.bin"
                    c.put(path, content)
                    files.append({"user": u, "path": path, "size": len(content), "sha256": nc.sha256_bytes(content)})
                continue
            for i in range(TEXT_FILES_PER_FOLDER):
                content = "\n".join(sentence(rng, rng.randint(6, 14)) for _ in range(rng.randint(3, 30))).encode() + b"\n"
                path = f"{folder}/{folder.lower()}-{i:02d}.txt"
                c.put(path, content)
                files.append({"user": u, "path": path, "size": len(content), "sha256": nc.sha256_bytes(content)})
    # Shares: each user shares Documents with the next one; alice publishes a link to Notes.
    for i, u in enumerate(USERS):
        s = nc.Client(u, password).share_with_user("Documents", USERS[(i + 1) % len(USERS)])
        shares.append({"owner": u, "path": "Documents", "share_with": s.get("share_with"), "id": s.get("id")})
    link = nc.Client("alice", password).public_link("Notes")
    shares.append({"owner": "alice", "path": "Notes", "share_type": "link", "id": link.get("id")})
    nc.write_json(os.path.join(args.run_dir, "dataset.json"),
                  {"seed": seed, "users": USERS, "files": files, "shares": shares, "created_at": nc.now_iso()})
    print(f"seed: {len(USERS)} usuarios, {len(files)} archivos, {len(shares)} recursos compartidos", flush=True)


if __name__ == "__main__":
    main()
