"""Pure lineage behavior without a workspace, files, or an inference engine."""

from copy import deepcopy

from character_lab.ancestry import first_change, origin_labels, origins, remap_origins


def test_unicode_draft_prefix_and_generated_suffix_keep_distinct_origins():
    source = "A🙂e\u0301Z"
    draft = "A🙂!e\u0301Z"
    nodes = {
        "source": dict(id="source", kind="source", text=source, parent=None),
        "generated": dict(
            id="generated",
            kind="generated",
            parent="source",
            prompt=draft,
            text=draft + "Ω",
            fork_offset=len(source),
        ),
    }
    before = deepcopy(nodes)
    spans = origins(nodes, "generated")
    assert origin_labels(draft + "Ω", spans) == [
        "source",
        "source",
        "edited",
        "source",
        "source",
        "source",
        "ai",
    ]
    assert first_change(nodes, "generated") == len(draft)
    assert nodes == before


def test_pure_deletion_and_unchanged_fork_reveal_the_same_change():
    nodes = {
        "source": dict(id="source", kind="source", text="a🙂bc", parent=None),
        "edit": dict(id="edit", kind="edit", text="a🙂c", parent="source"),
        "fork": dict(id="fork", kind="fork", text="a🙂c", parent="edit"),
    }
    assert first_change(nodes, "edit") == first_change(nodes, "fork") == 2
    spans = origins(nodes, "fork")
    assert origin_labels("a🙂c", spans) == ["source"] * 3
    assert remap_origins("a🙂c", "", spans) == []
