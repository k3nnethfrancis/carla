"""Explicit first-run model acquisition, before the terminal renderer starts.

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


def choose(labels, ask=input):
    for index, label in enumerate(labels, 1):
        print(f"  {index}. {label}")
    while True:
        value = ask("Choose a number: ").strip()
        if value.isdigit() and 1 <= int(value) <= len(labels):
            return int(value) - 1
        print(f"Choose 1–{len(labels)}.")


def download(repo, revision, filename=None, ask=input):
    info = HfApi().model_info(repo, revision=revision, files_metadata=True)
    files = {
        f.rfilename: f for f in info.siblings if f.rfilename.lower().endswith(".gguf")
    }
    if not files:
        raise ValueError("No GGUF files found in that model repository")
    if filename is None:
        options = sorted(
            name
            for name in files
            if not SHARD.fullmatch(name) or SHARD.fullmatch(name)[2] == "00001"
        )
        if not options:
            raise ValueError("No complete GGUF starting shard found")
        filename = options[choose(options, ask)]
    names = shard_files(filename, files)
    sizes = [files[name].size for name in names]
    size = (
        f"{sum(sizes) / 1024**3:.2f} GiB"
        if all(n is not None for n in sizes)
        else "size unavailable"
    )
    license_name = (info.card_data or {}).get(
        "license", "not specified — check the model card"
    )
    print(f"\n{repo}\nFiles: {', '.join(names)}\nSize: {size}\nLicense: {license_name}")
    print(f"Model card: https://huggingface.co/{repo}\nRevision: {info.sha}")
    print(f"Download cache: {HOME / 'models'}")
    if ask("Download these files? [y/N] ").strip().lower() not in {"y", "yes"}:
        return None
    paths = [
        hf_hub_download(
            repo_id=repo,
            filename=name,
            revision=info.sha,
            cache_dir=str(HOME / "models"),
        )
        for name in names
    ]
    return local_gguf(paths[0]), dict(
        repo=repo, revision=info.sha, files=names, license=license_name
    )


def register(path, name, kind, source):
    """Serialize registry changes; never replace an unrelated entry or partial download."""
    with acquire(HOME / ".model-setup"):
        registry = HOME / ("models.json" if kind == "base" else "policy-model.json")
        existing = load_models(registry, kind) if registry.exists() else []
        if any(Path(model["path"]).resolve() == path.resolve() for model in existing):
            print("This model is already configured.")
            return
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
    print(
        f"Saved {name}. {'Select it with /model.' if kind == 'base' else 'Ready for Grow.'}"
    )


def needs_setup(args):
    """Respect explicit launch configuration and saved workspace model catalogs."""
    import argparse

    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--models")
    parser.add_argument("--project")
    parser.add_argument("--workspace")
    selected, _ = parser.parse_known_args(args)
    if selected.models:
        return False
    registry = HOME / "models.json"
    models = load_models(registry) if registry.exists() else []
    folder = Path(selected.project).expanduser() if selected.project else None
    if selected.workspace:
        folder = HOME / selected.workspace
    if folder is None and (HOME / "workspaces.json").exists():
        last = json.loads((HOME / "workspaces.json").read_text()).get("last")
        folder = Path(last) if last else None
    if folder and (folder / "project.json").exists():
        models += json.loads((folder / "project.json").read_text()).get("models", [])
    return not any(Path(m.get("path", "")).is_file() for m in models)


def run(ask=input):
    print(
        "\nCarla · Model setup\nQwen presets have been used locally; character quality and speed depend on your hardware."
    )
    print(
        "Generation requires llama-server on PATH. You can skip setup to browse offline."
    )
    selected = choose(
        [
            *(p[0] for p in PRESETS),
            "Download a GGUF from Hugging Face",
            "Use a local GGUF",
            "Skip — browse without a model",
        ],
        ask,
    )
    if selected == 5:
        return
    kind = "base"
    if selected < 3:
        name, repo, revision, filename = PRESETS[selected]
        result = download(repo, revision, filename, ask)
        if result is None:
            return
        path, source = result
    else:
        if selected == 3:
            repo, revision, filename = parse_hub_source(
                ask("Hugging Face repo or GGUF URL: ")
            )
            result = download(repo, revision, filename, ask)
            if result is None:
                return
            path, source = result
        else:
            path = local_gguf(ask("Local GGUF path: ").strip())
            source = {"origin": "local"}
        print("Identify the model's training type (GGUF does not establish this):")
        role = choose(
            ["Base model — Loom and Simulator", "Instruct model — Grow selector"], ask
        )
        kind = "base" if role == 0 else "instruct"
        name = path.stem
    register(path, name, kind, source)
