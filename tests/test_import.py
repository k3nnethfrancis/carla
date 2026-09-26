import hashlib

from character_lab.domain import Project


def test_import_preserves_original_and_is_idempotent(tmp_path):
    source = Project(tmp_path / "original")
    root = source.add("Source", kind="source")
    child = source.add(
        "Source and continuation",
        parent=root["id"],
        prompt="Source",
        settings={"seed": 2},
        trace={"request": {"prompt": "Source"}},
    )
    source.keep(child["id"])
    before = source.path.read_bytes()
    review = Project(tmp_path / "review")
    review.import_run(source.folder, "Original")
    review.import_run(source.folder, "Original")
    assert len(review.data["nodes"]) == 2
    assert review.node(child["id"])["trace"] == child["trace"]
    assert review.data["imports"][0]["sha256"] == hashlib.sha256(before).hexdigest()
    edited = review.edit(child["id"], "Revised")
    assert source.path.read_bytes() == before
    assert review.node(child["id"])["text"] == child["text"]
    assert edited["parent"] == child["id"]
