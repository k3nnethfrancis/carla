import json

import pytest

from character_lab.library import load_library


def test_empty_library_has_bundled_starters(tmp_path):
    sources = load_library(tmp_path)
    assert [s["key"] for s in sources] == ["gunkel", "meditations", "tractatus"]
    assert [len(s["passages"]) for s in sources] == [1, 487, 7]
    assert all(s["url"] and s["source_sha256"] for s in sources)


def test_local_document_overrides_starter_without_hiding_others(tmp_path):
    source = {"key": "gunkel", "title": "My paths", "passages": []}
    (tmp_path / "custom.json").write_text(json.dumps(source))
    sources = load_library(tmp_path)
    assert sources[0] == source
    assert len(sources) == 3


def test_duplicate_library_keys_fail(tmp_path):
    source = {"key": "duplicate", "title": "Example", "passages": []}
    for i in range(2):
        (tmp_path / f"{i}.json").write_text(json.dumps(source))
    with pytest.raises(ValueError, match="duplicate"):
        load_library(tmp_path)
