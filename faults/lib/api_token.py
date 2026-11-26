"""Creates an API token for a test user in the original Gitea and prints it (for hidden faults O1, O2).

Usage (runner of the original project): python3 faults/lib/api_token.py USER NAME
Scopes: write:issue, write:repository, read:user. The password is SEED_USER_PASSWORD.
"""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "pylib"))
import gitea  # noqa: E402

user, name = sys.argv[1], sys.argv[2]
api = gitea.Client(user, os.environ["SEED_USER_PASSWORD"])
tok = api.post(f"/users/{user}/tokens", {"name": name, "scopes": ["write:issue", "write:repository", "read:user"]})
print(tok["sha1"])
