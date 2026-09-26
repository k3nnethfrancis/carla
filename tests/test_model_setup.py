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


def test_download_confirmation_pinning_and_failure_no_registry(local_data, monkeypatch):
    files = ["model-00001-of-00002.gguf", "model-00002-of-00002.gguf"]
    info = SimpleNamespace(
        sha="a" * 40,
        card_data={"license": "mit"},
        siblings=[SimpleNamespace(rfilename=n, size=8) for n in files],
    )
    monkeypatch.setattr(
        setup, "HfApi", lambda: SimpleNamespace(model_info=lambda *a, **kw: info)
    )
    calls = []

    def fetch(**kwargs):
        calls.append(kwargs)
        path = local_data / kwargs["filename"]
        path.write_bytes(b"GGUFtest")
        return str(path)

    monkeypatch.setattr(setup, "hf_hub_download", fetch)
    assert setup.download("owner/repo", "main", files[0], lambda _: "n") is None
    assert calls == []
    result = setup.download("owner/repo", "main", files[0], lambda _: "y")
    assert len(calls) == 2 and all(c["revision"] == info.sha for c in calls)
    assert result[1]["files"] == files
    assert not (local_data / "models.json").exists()

    def fail(**kwargs):
        raise OSError("interrupted")

    monkeypatch.setattr(setup, "hf_hub_download", fail)
    with pytest.raises(OSError):
        setup.download("owner/repo", "main", files[0], lambda _: "y")
    assert not (local_data / "models.json").exists()


def test_detection_local_wizard_and_skip(local_data):
    assert setup.needs_setup([])
    assert not setup.needs_setup(["--models", "custom.json"])
    setup.run(lambda _: "6")
    assert not (local_data / "models.json").exists()
    model = local_data / "local.gguf"
    model.write_bytes(b"GGUFtest")
    answers = iter(["5", str(model), "1"])
    setup.run(lambda _: next(answers))
    assert not setup.needs_setup([])
    registry = json.loads((local_data / "models.json").read_text())
    assert registry[0]["kind"] == "base" and registry[0]["source"] == {
        "origin": "local"
    }
    # Existing custom workspace models suppress setup even without a shared registry.
    (local_data / "models.json").unlink()
    workspace = local_data / "workspace"
    workspace.mkdir()
    (workspace / "project.json").write_text(json.dumps({"models": registry}))
    assert not setup.needs_setup(["--project", str(workspace)])
    model.unlink()
    assert setup.needs_setup(["--project", str(workspace)])
