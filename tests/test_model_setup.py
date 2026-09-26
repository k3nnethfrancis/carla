"""Setup tests download only mocked bytes and never alter the user's catalog."""

import json
from types import SimpleNamespace

import pytest

from character_lab import model_setup as setup
from character_lab.models import available_models, load_models
from character_lab.runtime import DEFAULT_MODEL


def test_hub_url_parsing():
    assert setup.parse_hub_source("owner/repo") == ("owner/repo", "main", None)
    assert setup.parse_hub_source(
        "https://huggingface.co/owner/repo/blob/main/folder/a.gguf?download=true"
    ) == ("owner/repo", "main", "folder/a.gguf")
    with pytest.raises(ValueError):
        setup.parse_hub_source("https://unrelated.invalid/model.gguf")


def test_split_gguf_requires_all_parts_and_preserves_cache_symlink(local_data):
    blob = local_data / "blobhash"
    blob.write_bytes(b"GGUFtest")
    first = local_data / "model-00001-of-00002.gguf"
    first.symlink_to(blob)
    with pytest.raises(ValueError):
        setup.local_gguf(str(first))
    (local_data / "model-00002-of-00002.gguf").symlink_to(blob)
    assert setup.local_gguf(str(first)) == first
    setup.register(first, "Test", "base", {})
    assert load_models(local_data / "models.json")[0]["path"] == str(first)
    setup.register(first, "Same model", "base", {})
    assert len(load_models(local_data / "models.json")) == 1
    # Previously browsing without weights must not retain the empty placeholder.
    assert len(available_models([DEFAULT_MODEL])) == 1


def test_download_plan_pins_revision_without_fetching(local_data, monkeypatch):
    files = ["model-00001-of-00002.gguf", "model-00002-of-00002.gguf"]
    info = SimpleNamespace(
        sha="a" * 40,
        card_data={"license": "mit"},
        siblings=[SimpleNamespace(rfilename=n, size=8) for n in files],
    )
    monkeypatch.setattr(
        setup, "HfApi", lambda: SimpleNamespace(model_info=lambda *a, **kw: info)
    )

    def forbidden(**kwargs):
        raise AssertionError("Metadata inspection must never download weights")

    monkeypatch.setattr(setup, "hf_hub_download", forbidden)
    result = setup.plans("owner/repo", "main")
    assert len(result) == 1 and result[0]["files"] == files
    assert result[0]["revision"] == info.sha and result[0]["size"] == 16
    assert not (local_data / "models.json").exists()


def test_worker_downloads_only_confirmed_shards(local_data, monkeypatch, capsys):
    import io
    import sys

    plan = {
        "repo": "owner/repo",
        "revision": "a" * 40,
        "files": ["m-00001-of-00002.gguf", "m-00002-of-00002.gguf"],
    }
    monkeypatch.setattr(sys, "stdin", io.StringIO(json.dumps(plan) + "\n"))
    calls = []

    def fetch(**kwargs):
        calls.append(kwargs)
        path = local_data / kwargs["filename"]
        path.write_bytes(b"GGUFtest")
        return str(path)

    monkeypatch.setattr(setup, "hf_hub_download", fetch)
    setup.download_worker()
    events = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    assert events[-1]["stage"] == "downloaded"
    assert len(calls) == 2 and all(c["revision"] == plan["revision"] for c in calls)
    assert not (local_data / "models.json").exists()
