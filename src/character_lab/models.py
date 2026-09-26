"""Explicit local model configuration; discovery never downloads weights."""

import json
from pathlib import Path
from urllib.parse import urlparse

from .runtime import DEFAULT_MODEL
from .workspaces import HOME


def load_models(path, kind="base"):
    path = Path(path).expanduser().resolve()
    data = json.loads(path.read_text(encoding="utf-8"))
    entries = data if isinstance(data, list) else [data]
    if not entries:
        raise ValueError("Model configuration must contain at least one model")
    if kind == "instruct" and len(entries) != 1:
        raise ValueError("Configure exactly one policy model")
    aliases = set()
    for model in entries:
        if not isinstance(model, dict):
            raise ValueError("Each model must be an object")
        alias = model.get("alias")
        if not isinstance(alias, str) or not alias.strip() or alias in aliases:
            raise ValueError("Model aliases must be nonempty and unique")
        aliases.add(alias)
        if model.get("kind") != kind:
            raise ValueError(f"This configuration requires {kind} models")
        location = model.get("path")
        if not isinstance(location, str) or not location.strip():
            raise ValueError("Each model requires a GGUF path")
        location = Path(location).expanduser()
        model["path"] = str((path.parent / location).absolute())
        model.setdefault("name", alias)
        model.setdefault("context", 8192)
        model.setdefault("gpu_layers", 99)
        port = model.get("port")
        if type(port) is not int or not 1 <= port <= 65535:
            raise ValueError("Model port must be between 1 and 65535")
        model.setdefault("url", f"http://127.0.0.1:{port}")
        if not isinstance(model["url"], str):
            raise ValueError("Model URL must be a string")
        url = urlparse(model["url"])
        if (
            url.scheme != "http"
            or url.hostname != "127.0.0.1"
            or url.port != port
            or url.username
            or url.password
            or url.path not in ("", "/")
            or url.query
            or url.fragment
        ):
            raise ValueError(
                "Model URL must match its local http://127.0.0.1:PORT endpoint"
            )
        model["url"] = model["url"].rstrip("/")
        if type(model["context"]) is not int or model["context"] < 0:
            raise ValueError(
                "Context must be zero (model default) or a positive token count"
            )
        if type(model["gpu_layers"]) is not int or model["gpu_layers"] < 0:
            raise ValueError("GPU layers must be a nonnegative integer; use 0 for CPU")
    return entries


def available_models(saved=()):
    path = HOME / "models.json"
    configured = load_models(path) if path.exists() else []
    catalog = {m["alias"]: m for m in configured}
    catalog.update({m["alias"]: m for m in saved if m.get("path") or not configured})
    return list(catalog.values()) or [DEFAULT_MODEL.copy()]
