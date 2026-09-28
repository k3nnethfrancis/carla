import json

import pytest

from character_lab.library import load_library


def test_empty_library_has_bundled_starters(tmp_path):
    sources = load_library(tmp_path)
    assert [s["key"] for s in sources] == ["meditations", "tractatus"]
    assert [len(s["passages"]) for s in sources] == [487, 7]
    assert all(s["url"] and s["source_sha256"] for s in sources)


def test_local_document_overrides_starter_without_hiding_others(tmp_path):
    source = {"key": "meditations", "title": "My passages", "passages": []}
    (tmp_path / "custom.json").write_text(json.dumps(source))
    sources = load_library(tmp_path)
    assert sources[0] == source
    assert len(sources) == 2


def test_duplicate_library_keys_fail(tmp_path):
    source = {"key": "duplicate", "title": "Example", "passages": []}
    for i in range(2):
        (tmp_path / f"{i}.json").write_text(json.dumps(source))
    with pytest.raises(ValueError, match="duplicate"):
        load_library(tmp_path)


def test_import_preserves_text_and_allocates_unique_keys(tmp_path):
    import hashlib

    from character_lab.library import import_document

    path = tmp_path / "My seed.md"
    raw = "# A seed\r\n\r\n日本語 🙂\n".encode()
    path.write_bytes(raw)
    first = import_document(path, author="Author", source_url="https://example.org")
    second = import_document(path)
    assert first["title"] == "My seed"
    assert first["key"] != second["key"]
    assert first["passages"][0]["text"] == raw.decode()
    assert first["source_sha256"] == hashlib.sha256(raw).hexdigest()
    assert any(s == first for s in load_library())
    with pytest.raises(ValueError, match="already exists"):
        import_document(path, key=first["key"])


def test_invalid_import_does_not_add_document(tmp_path):
    from character_lab.library import import_document

    before = load_library()
    for content in [b"", b"\xff", b"binary\x00data"]:
        path = tmp_path / "bad.txt"
        path.write_bytes(content)
        with pytest.raises(ValueError):
            import_document(path)
    with pytest.raises(ValueError, match="existing"):
        import_document(tmp_path / "missing")
    assert load_library() == before
