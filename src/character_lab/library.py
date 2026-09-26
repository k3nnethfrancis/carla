"""Shared source documents, independent of workspaces and model configuration."""

import json
from pathlib import Path

from .workspaces import HOME

BUNDLED = Path(__file__).with_name("seeds")


def _read_library(folder):
    folder = Path(folder)
    sources = []
    keys = set()
    for path in sorted(folder.glob("*.json")):
        source = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(source, dict):
            raise ValueError(f"Library document must be an object: {path.name}")
        key = source.get("key")
        if not isinstance(key, str) or not key or key in keys or ":" in key:
            raise ValueError(f"Invalid or duplicate library key in {path.name}")
        if not isinstance(source.get("title"), str) or not isinstance(
            source.get("passages"), list
        ):
            raise ValueError(f"Library document needs title and passages: {path.name}")
        ids = set()
        for passage in source["passages"]:
            if not isinstance(passage, dict):
                raise ValueError(f"Passage must be an object: {path.name}")
            ref = passage.get("id")
            if (
                not isinstance(ref, str)
                or not ref
                or ref in ids
                or not isinstance(passage.get("text"), str)
            ):
                raise ValueError(f"Invalid passage in {path.name}")
            ids.add(ref)
        keys.add(key)
        sources.append(source)
    return sources


def load_library(folder=None):
    """Bundled starters plus local documents; a local key overrides its starter."""
    local = _read_library(folder if folder is not None else HOME / "library")
    sources = {source["key"]: source for source in _read_library(BUNDLED)}
    sources.update((source["key"], source) for source in local)
    return list(sources.values())


def main():
    """Import a UTF-8 document without editing application source or workspace data."""
    import argparse
    import hashlib
    import re

    parser = argparse.ArgumentParser(
        description="Import a plain-text seed into Carla's shared library"
    )
    parser.add_argument("file", type=Path)
    parser.add_argument("--key", required=True, help="Stable identifier, e.g. paths")
    parser.add_argument("--title", required=True)
    parser.add_argument("--author", default="")
    parser.add_argument("--source-url", default="")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", args.key):
        parser.error("Key must use lowercase letters, digits, underscores or hyphens")
    folder = HOME / "library"
    if any(s["key"] == args.key for s in load_library(folder)) and list(
        folder.glob("*.json")
    ):
        parser.error(
            "That key already exists; choose a new key to preserve existing provenance"
        )
    raw = args.file.read_bytes()
    source = dict(
        key=args.key,
        title=args.title,
        author=args.author,
        url=args.source_url,
        source_sha256=hashlib.sha256(raw).hexdigest(),
        normalization="UTF-8 decoded without reflow or paraphrase.",
        passages=[dict(id="1", label=args.title, text=raw.decode("utf-8"))],
    )
    folder.mkdir(parents=True, exist_ok=True)
    output = folder / f"{args.key}.json"
    with output.open("x", encoding="utf-8") as stream:
        stream.write(json.dumps(source, ensure_ascii=False, indent=2) + "\n")
    print(output)


if __name__ == "__main__":
    main()
