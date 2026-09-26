import json

import pytest

from character_lab.library import load_library


def test_empty_library_has_original_offline_example(tmp_path):
    assert load_library(tmp_path)[0]["key"] == "paths"


def test_duplicate_library_keys_fail(tmp_path):
    source = {"key": "duplicate", "title": "Example", "passages": []}
    for i in range(2):
        (tmp_path / f"{i}.json").write_text(json.dumps(source))
    with pytest.raises(ValueError, match="duplicate"):
        load_library(tmp_path)
