"""Tests use original synthetic documents and never read the user's library."""

import json

import pytest


@pytest.fixture(autouse=True)
def local_data(tmp_path, monkeypatch):
    home = tmp_path / "app-data"
    home.mkdir()
    for module in ("models", "library", "workspaces", "model_setup"):
        monkeypatch.setattr(f"character_lab.{module}.HOME", home)
    sources = [
        {
            "key": "gunkel",
            "title": "Test paths",
            "passages": [{"id": "table", "text": "A fictional path for testing."}],
        },
        {
            "key": "meditations",
            "title": "Test numbered document",
            "passages": [
                {
                    "id": f"{book}.{number}",
                    "book": book,
                    "number": number,
                    "text": f"Original test passage {book}.{number}.",
                }
                for book in range(1, 5)
                for number in range(1, 4)
            ],
        },
        {
            "key": "tractatus",
            "title": "Test propositions",
            "passages": [{"id": "1", "text": "A distinct fictional proposition."}],
        },
    ]
    library = home / "library"
    library.mkdir()
    for i, source in enumerate(sources):
        (library / f"{i}.json").write_text(json.dumps(source))
    return home
