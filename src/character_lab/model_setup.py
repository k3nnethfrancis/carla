"""Model acquisition primitives and a cancellable transfer worker.

The Hub owns transfer progress, cache and resumption. Carla pins the resolved
revision, downloads only the chosen GGUF (and its shards), and registers it only
when every file has arrived. Local files are referenced in place.
"""

import hashlib
import json
import re
from pathlib import Path
from urllib.parse import unquote, urlparse

from huggingface_hub import HfApi, hf_hub_download

from .models import load_models
from .workspaces import HOME, acquire

PRESETS = [
    (
        "Qwen3-8B Base · Q4_K_M",
        "Antigma/Qwen3-8B-Base-GGUF",
        "39e3e606b9f228715a5663ac9f538d9c47efa81c",
        "qwen3-8b-base-q4_k_m.gguf",
    ),
    (
        "Qwen3-14B Base · Q4_K_M",
        "Antigma/Qwen3-14B-Base-GGUF",
        "f9bed82e088510d340a5db6507e2664f96a60b49",
        "qwen3-14b-base-q4_k_m.gguf",
    ),
    (
        "Qwen3-30B-A3B Base · Q4_K_M",
        "Antigma/Qwen3-30B-A3B-Base-GGUF",
        "a2e5ba0daba1b56c6e85af52da5d177ab5e39e73",
        "qwen3-30b-a3b-base-q4_k_m.gguf",
    ),
]
SHARD = re.compile(r"^(.*)-(\d{5})-of-(\d{5})\.gguf$", re.I)


def parse_hub_source(value):
    """Accept owner/repo, repository URLs, or exact blob/resolve file URLs."""
    value = value.strip()
    if "://" not in value:
        if not re.fullmatch(r"[\w.-]+/[\w.-]+", value):
            raise ValueError("Use owner/repo or a huggingface.co model/file URL")
        return value, "main", None
    url = urlparse(value)
    if url.scheme != "https" or url.netloc != "huggingface.co":
        raise ValueError("Use an https://huggingface.co URL")
    parts = [unquote(part) for part in url.path.strip("/").split("/")]
    if len(parts) == 2:
        return "/".join(parts), "main", None
    if len(parts) >= 4 and parts[2] in {"blob", "resolve", "tree"}:
        return "/".join(parts[:2]), parts[3], "/".join(parts[4:]) or None
    raise ValueError("Use a repository URL or a direct GGUF blob/resolve URL")


def shard_files(filename, available):
    match = SHARD.fullmatch(filename)
    wanted = (
        [f"{match[1]}-{i:05d}-of-{match[3]}.gguf" for i in range(1, int(match[3]) + 1)]
        if match
        else [filename]
    )
    if not wanted or any(name not in available for name in wanted):
        raise ValueError("The selected GGUF or one of its shards is missing")
    return wanted


def local_gguf(value):
    path = Path(value).expanduser().absolute()
    if path.suffix.lower() != ".gguf":
        raise ValueError("Select a .gguf file")
    names = shard_files(path.name, {p.name for p in path.parent.glob("*.gguf")})
    for name in names:
        with (path.parent / name).open("rb") as stream:
            if stream.read(4) != b"GGUF":
                raise ValueError(f"Not a GGUF file: {name}")
    return path.parent / names[0]


def plans(repo, revision, filename=None):
    """Resolve metadata only; the returned immutable plans require user confirmation."""
    info = HfApi().model_info(repo, revision=revision, files_metadata=True)
    files = {
        f.rfilename: f for f in info.siblings if f.rfilename.lower().endswith(".gguf")
    }
    options = (
        [filename]
        if filename
        else sorted(
            name
            for name in files
            if not SHARD.fullmatch(name) or SHARD.fullmatch(name)[2] == "00001"
        )
    )
    if not options:
        raise ValueError("No GGUF files found in that repository")
    result = []
    for option in options:
        names = shard_files(option, files)
        sizes = [files[name].size for name in names]
        result.append(
            dict(
                repo=repo,
                revision=info.sha,
                files=names,
                name=option,
                size=sum(sizes) if all(n is not None for n in sizes) else None,
                license=(info.card_data or {}).get(
                    "license", "Not specified; check the model card"
                ),
            )
        )
    return result


def register(path, name, kind, source):
    """Serialize registry changes; never replace an unrelated entry or partial download."""
    with acquire(HOME / ".model-setup"):
        registry = HOME / ("models.json" if kind == "base" else "policy-model.json")
        existing = load_models(registry, kind) if registry.exists() else []
        if any(Path(model["path"]).resolve() == path.resolve() for model in existing):
            return next(
                m for m in existing if Path(m["path"]).resolve() == path.resolve()
            )
        if kind == "instruct" and existing:
            raise ValueError(
                "A Grow policy model is already configured; edit policy-model.json to replace it"
            )
        used = set()
        for config, role in [
            (HOME / "models.json", "base"),
            (HOME / "policy-model.json", "instruct"),
        ]:
            if config.exists():
                used.update(m["port"] for m in load_models(config, role))
        port = next(p for p in range(18986, 65536) if p not in used)
        alias = "carla-" + hashlib.sha256(str(path).encode()).hexdigest()[:12]
        model = dict(
            name=name,
            alias=alias,
            kind=kind,
            path=str(path),
            port=port,
            url=f"http://127.0.0.1:{port}",
            context=8192,
            gpu_layers=99,
            source=source,
        )
        temp = registry.with_suffix(".tmp")
        temp.write_text(
            json.dumps([*existing, model] if kind == "base" else model, indent=2) + "\n"
        )
        temp.replace(registry)
    return model


def download_worker():
    """Separate process: cancellation terminates network transfers, not just an await."""
    import sys
    import time

    from tqdm import tqdm

    plan = json.loads(sys.stdin.readline())

    def emit(data):
        print(json.dumps(data), flush=True)

    class Progress(tqdm):
        def __init__(self, *args, **kwargs):
            kwargs["disable"] = True
            self.received = 0
            self.reported = 0
            super().__init__(*args, **kwargs)

        def update(self, amount=1):
            self.received += amount
            if time.monotonic() - self.reported >= 0.2:
                self.reported = time.monotonic()
                emit(
                    dict(
                        stage="progress",
                        downloaded=int(self.received),
                        total=int(self.total or 0),
                    )
                )

        def close(self):
            pass

    try:
        paths = []
        for index, name in enumerate(plan["files"]):
            emit(
                dict(
                    stage="progress",
                    file=name,
                    index=index + 1,
                    count=len(plan["files"]),
                )
            )
            paths.append(
                hf_hub_download(
                    repo_id=plan["repo"],
                    filename=name,
                    revision=plan["revision"],
                    cache_dir=str(HOME / "models"),
                    tqdm_class=Progress,
                )
            )
        emit(dict(stage="downloaded", path=str(local_gguf(paths[0]))))
    except Exception as exc:
        emit(dict(stage="error", message=str(exc)))
        raise SystemExit(1) from None


if __name__ == "__main__":
    download_worker()
