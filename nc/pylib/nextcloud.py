"""Small Nextcloud client (WebDAV + OCS) shared by seed, witness and inventory (stdlib only)."""

import base64
import datetime
import hashlib
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request

NC_URL = os.environ.get("NC_URL", "http://nextcloud").rstrip("/")


def now_iso():
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def sha256_bytes(data):
    return hashlib.sha256(data).hexdigest()


def write_json(path, obj):
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(obj, f, indent=2, sort_keys=False)
        f.write("\n")
    os.replace(tmp, path)


class ApiError(Exception):
    pass


class Client:
    """Nextcloud client authenticated with HTTP basic auth."""

    def __init__(self, user, password):
        self.user = user
        self.password = password

    def request(self, method, url, data=None, headers=None, ok=(200, 201, 204, 207)):
        h = {"Authorization": "Basic " + base64.b64encode(f"{self.user}:{self.password}".encode()).decode()}
        h.update(headers or {})
        req = urllib.request.Request(url if url.startswith("http") else NC_URL + url, data=data, method=method, headers=h)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                status, payload = resp.status, resp.read()
        except urllib.error.HTTPError as e:
            status, payload = e.code, e.read()
        if ok and status not in ok:
            raise ApiError(f"{method} {url} -> {status}: {payload[:300]!r}")
        return status, payload

    # --- WebDAV (files of the authenticated user)
    def dav(self, path):
        return f"/remote.php/dav/files/{urllib.parse.quote(self.user)}/{urllib.parse.quote(path.lstrip('/'))}"

    def mkdir(self, path):
        self.request("MKCOL", self.dav(path), ok=(201, 405))  # 405: already exists

    def put(self, path, content):
        self.request("PUT", self.dav(path), data=content, headers={"Content-Type": "application/octet-stream"})

    def get(self, path):
        return self.request("GET", self.dav(path))[1]

    # --- OCS
    def ocs(self, method, path, fields=None):
        data = urllib.parse.urlencode(fields).encode() if fields else None
        headers = {"OCS-APIRequest": "true", "Accept": "application/json"}
        if data:
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        _, payload = self.request(method, f"/ocs/v2.php{path}?format=json", data=data, headers=headers)
        return json.loads(payload)["ocs"]["data"]

    def create_user(self, user, password):
        return self.ocs("POST", "/cloud/users", {"userid": user, "password": password})

    def share_with_user(self, path, share_with, permissions=1):
        return self.ocs("POST", "/apps/files_sharing/api/v1/shares",
                        {"path": "/" + path.lstrip("/"), "shareType": 0, "shareWith": share_with,
                         "permissions": permissions})

    def public_link(self, path):
        return self.ocs("POST", "/apps/files_sharing/api/v1/shares", {"path": "/" + path.lstrip("/"), "shareType": 3})


def wait_for_nextcloud(timeout=180):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(NC_URL + "/status.php", timeout=5) as r:
                s = json.loads(r.read())
                if r.status == 200 and s.get("installed") and not s.get("maintenance"):
                    return s
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError("nextcloud not ready")
