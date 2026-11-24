"""Locate the byte range of the witness commit object inside the Restic repository.

Runs in the runner image with the restic binary at /opt/restic/restic and the repository
at /repo (read-only). Walks the latest snapshot to the loose object file of the commit,
maps its blob to (pack, offset, length) with the index, and checks that the range
touches only that blob and none of the blobs of /dump/gitea.dump or app.ini.
Prints a JSON plan.
"""

import json
import subprocess
import sys

RESTIC = ["/opt/restic/restic", "--no-lock", "--no-cache", "-r", "/repo"]


def restic(*args):
    return subprocess.run(RESTIC + list(args), check=True, stdout=subprocess.PIPE).stdout


def node_at(root_tree, path):
    tree = root_tree
    parts = [p for p in path.split("/") if p]
    for i, name in enumerate(parts):
        nodes = json.loads(restic("cat", "blob", tree))["nodes"]
        node = next(n for n in nodes if n["name"] == name)
        if i == len(parts) - 1:
            return node
        tree = node["subtree"]


def main():
    commit_sha = sys.argv[1]
    snap = json.loads(restic("snapshots", "--json", "--latest", "1"))[0]
    root = json.loads(restic("cat", "snapshot", snap["id"]))["tree"]
    obj_path = f"/data/git/repositories/alice/witness.git/objects/{commit_sha[:2]}/{commit_sha[2:]}"
    target = node_at(root, obj_path)["content"]
    protected = {
        "/dump/gitea.dump": node_at(root, "/dump/gitea.dump")["content"],
        "/data/gitea/conf/app.ini": node_at(root, "/data/gitea/conf/app.ini")["content"],
    }
    entries = []
    for idx in restic("list", "index").decode().split():
        for pack in json.loads(restic("cat", "index", idx))["packs"]:
            for b in pack["blobs"]:
                entries.append({"pack": pack["id"], **b})
    by_id = {}
    for e in entries:
        by_id.setdefault(e["id"], []).append(e)
    if len(target) != 1 or len(by_id.get(target[0], [])) != 1:
        raise SystemExit(f"unexpected layout: content={target} index entries={by_id.get(target[0] if target else '')}")
    t = by_id[target[0]][0]
    start, end = t["offset"], t["offset"] + t["length"]
    overlapping = [e["id"] for e in entries
                   if e["pack"] == t["pack"] and e["offset"] < end and start < e["offset"] + e["length"]]
    protected_in_range = {p: [b for b in blobs if b in overlapping] for p, blobs in protected.items()}
    print(json.dumps({
        "snapshot": snap["id"], "object_path": obj_path, "blob_id": t["id"], "pack": t["pack"],
        "pack_path": f"data/{t['pack'][:2]}/{t['pack']}", "offset": start, "length": t["length"],
        "blobs_overlapping_range": overlapping, "protected_blobs": protected,
        "protected_blobs_in_range": protected_in_range,
        "only_witness_blob_in_range": overlapping == [t["id"]],
        "protected_untouched": all(not v for v in protected_in_range.values()),
    }, indent=2))


if __name__ == "__main__":
    main()
