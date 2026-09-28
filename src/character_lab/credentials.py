"""Local provider credentials, kept out of workspaces, traces and exports."""

import json
import os
import tempfile
from pathlib import Path

from .workspaces import HOME


def openrouter_key():
    path = HOME / "credentials.json"
    if path.exists():
        key = json.loads(path.read_text()).get("openrouter_api_key", "")
        if key:
            return key, "saved"
    key = os.environ.get("OPENROUTER_API_KEY", "").strip()
    return key, "environment" if key else ""


def save_openrouter_key(key):
    if (
        not isinstance(key, str)
        or not key.strip()
        or any(c.isspace() for c in key.strip())
    ):
        raise ValueError("Enter an OpenRouter API key without spaces")
    HOME.mkdir(parents=True, exist_ok=True)
    path = HOME / "credentials.json"
    data = json.loads(path.read_text()) if path.exists() else {}
    data["openrouter_api_key"] = key.strip()
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", dir=HOME, delete=False) as f:
            temporary = Path(f.name)
            os.fchmod(f.fileno(), 0o600)
            json.dump(data, f)
            f.flush()
            os.fsync(f.fileno())
        temporary.replace(path)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
