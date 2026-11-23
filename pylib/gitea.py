"""Small Gitea client and helpers shared by seed, witness and inventory (stdlib only)."""

import base64
import datetime
import hashlib
import hmac
import http.cookiejar
import json
import os
import re
import struct
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

GITEA_URL = os.environ.get("GITEA_URL", "http://gitea:3000").rstrip("/")


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
    """Gitea API client authenticated with HTTP basic auth."""

    def __init__(self, user, password, otp=None):
        self.user = user
        self.password = password
        self.otp = otp

    def _headers(self):
        token = base64.b64encode(f"{self.user}:{self.password}".encode()).decode()
        h = {"Authorization": f"Basic {token}", "Accept": "application/json"}
        if self.otp:
            h["X-Gitea-OTP"] = self.otp()
        return h

    def request(self, method, path, body=None, ok=(200, 201, 204), raw_body=None, content_type=None):
        headers = self._headers()
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        elif raw_body is not None:
            data = raw_body
            headers["Content-Type"] = content_type
        req = urllib.request.Request(GITEA_URL + "/api/v1" + path, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                status, payload = resp.status, resp.read()
        except urllib.error.HTTPError as e:
            status, payload = e.code, e.read()
        if status not in ok:
            raise ApiError(f"{method} {path} -> {status}: {payload[:300]!r}")
        return json.loads(payload) if payload else None

    def get(self, path, **kw):
        return self.request("GET", path, **kw)

    def post(self, path, body=None, **kw):
        return self.request("POST", path, body=body, **kw)

    def upload_attachment(self, owner, repo, issue_index, filename, content):
        boundary = "----recexp" + sha256_bytes(content)[:24]
        parts = [
            f"--{boundary}\r\n".encode(),
            f'Content-Disposition: form-data; name="attachment"; filename="{filename}"\r\n'.encode(),
            b"Content-Type: application/octet-stream\r\n\r\n",
            content,
            f"\r\n--{boundary}--\r\n".encode(),
        ]
        return self.request(
            "POST",
            f"/repos/{owner}/{repo}/issues/{issue_index}/assets?name={urllib.parse.quote(filename)}",
            raw_body=b"".join(parts),
            content_type=f"multipart/form-data; boundary={boundary}",
        )


def wait_for_gitea(timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(GITEA_URL + "/api/healthz", timeout=5) as r:
                if r.status == 200:
                    return
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError("gitea not healthy")


# --- TOTP (RFC 6238, SHA1, 6 digits, 30 s) ---

def totp(secret_b32, at=None):
    key = base64.b32decode(secret_b32.upper() + "=" * (-len(secret_b32) % 8))
    counter = int((at if at is not None else time.time()) // 30)
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    code = (struct.unpack(">I", digest[offset:offset + 4])[0] & 0x7FFFFFFF) % 1_000_000
    return f"{code:06d}"


class WebSession:
    """Browser-like session for flows without an API (2FA enrollment)."""

    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def get(self, path):
        with self.opener.open(GITEA_URL + path, timeout=60) as r:
            return r.status, r.read().decode()

    def post(self, path, fields):
        data = urllib.parse.urlencode(fields).encode()
        req = urllib.request.Request(GITEA_URL + path, data=data, method="POST")
        try:
            with self.opener.open(req, timeout=60) as r:
                return r.status, r.geturl(), r.read().decode()
        except urllib.error.HTTPError as e:
            return e.code, path, e.read().decode()

    @staticmethod
    def form(html, **fields):
        """Form fields, plus the _csrf token when the page has one (Gitea 28 uses origin checks instead)."""
        m = re.search(r'name="_csrf"\s+value="([^"]+)"', html)
        if m:
            fields["_csrf"] = m.group(1)
        return fields


# --- git over HTTP ---

def git(args, cwd=None, env=None, capture=False):
    full_env = dict(os.environ)
    full_env.update(env or {})
    res = subprocess.run(["git"] + args, cwd=cwd, env=full_env, check=True,
                         stdout=subprocess.PIPE if capture else None)
    return res.stdout.decode().strip() if capture else None


def repo_url(owner, repo, user, password):
    host = GITEA_URL.split("://", 1)[1]
    return f"http://{urllib.parse.quote(user)}:{urllib.parse.quote(password)}@{host}/{owner}/{repo}.git"
