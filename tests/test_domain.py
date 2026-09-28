import pytest

from character_lab.domain import Project, library, search


def corpus():
    return next(source for source in library() if source["key"] == "meditations")


def test_library_reads_local_numbered_documents():
    c = corpus()
    assert len(c["passages"]) == 12
    assert search(c["passages"], "1.1")[0]["text"] == "Original test passage 1.1."


def test_selection_filters_and_source_order(tmp_path):
    c = corpus()
    p = Project(tmp_path)
    p.toggle("4.3")
    p.toggle("2.1")
    text, refs = p.seed(c)
    assert refs == ["2.1", "4.3"]
    assert (
        text
        == search(c["passages"], "2.1")[0]["text"]
        + "\n\n"
        + search(c["passages"], "4.3")[0]["text"]
    )
    assert search(c["passages"], "4:3")[0]["id"] == "4.3"
    assert search(c["passages"], "", 4, p.selected)[0]["id"] == "4.3"
    assert Project(tmp_path).selected == ["4.3", "2.1"]


def test_non_destructive_edits_and_snapshot(tmp_path):
    p = Project(tmp_path)
    p.toggle("2.1")
    root = p.root(corpus())
    edited = p.edit(root["id"], "Changed text")
    assert root["text"] != "Changed text" and edited["parent"] == root["id"]
    p.keep(edited["id"])
    snap = p.snapshot()
    assert list(snap.glob("*.txt"))[0].read_text() == "Changed text"
    frozen = (snap / "manifest.json").read_text()
    p.edit(edited["id"], "Another edit")
    assert (snap / "manifest.json").read_text() == frozen


@pytest.mark.asyncio
async def test_context_overflow_never_sends_truncated_prompt(tmp_path, monkeypatch):
    import httpx

    from character_lab.runtime import Runtime

    client = httpx.AsyncClient
    calls = []

    def handler(request):
        calls.append(request.url.path)
        if request.url.path == "/tokenize":
            return httpx.Response(200, json={"tokens": list(range(8100))})
        if request.url.path == "/props":
            return httpx.Response(
                200, json={"default_generation_settings": {"n_ctx": 8192}}
            )
        raise AssertionError("An overflowing prompt must never reach generation")

    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: client(transport=httpx.MockTransport(handler), **kw),
    )
    runtime = Runtime(tmp_path)

    async def ready():
        pass

    runtime.ensure = ready
    with pytest.raises(ValueError, match="Nothing was truncated"):
        async for _ in runtime.stream(
            "unaltered input",
            {"n_predict": 256, "temperature": 1.0, "top_p": 0.98, "seed": 1},
            {},
        ):
            pass
    assert calls == ["/tokenize", "/props"]


def test_origins_follow_nested_branches_edits_and_reload(tmp_path):
    from character_lab.ancestry import origin_labels

    p = Project(tmp_path)
    root = p.add("Original α\ntext.", kind="source")
    first = p.add(
        root["text"] + " AI words.",
        parent=root["id"],
        prompt=root["text"],
        fork_offset=len(root["text"]),
    )
    prefix = first["text"][:-3]
    second = p.add(
        prefix + " new words.",
        parent=first["id"],
        prompt=prefix,
        fork_offset=len(prefix),
    )
    labels = origin_labels(second["text"], p.origins(second["id"]))
    assert labels[: len(root["text"])] == ["source"] * len(root["text"])
    assert labels[len(root["text"]) :] == ["ai"] * (
        len(second["text"]) - len(root["text"])
    )
    edited = p.edit(second["id"], "My " + second["text"])
    labels = origin_labels(edited["text"], p.origins(edited["id"]))
    assert labels[:3] == ["edited"] * 3
    assert labels[3 : 3 + len(root["text"])] == ["source"] * len(root["text"])
    assert Project(tmp_path).origins(edited["id"]) == p.origins(edited["id"])
    # Forking inside the source cannot retain AI attribution from a discarded suffix.
    fork = p.add("Orig fresh", parent=second["id"], prompt="Orig", fork_offset=4)
    assert (
        origin_labels(fork["text"], p.origins(fork["id"]))
        == ["source"] * 4 + ["ai"] * 6
    )


def test_numbered_labels_preserve_lineage_and_survive_restart_and_deletion(tmp_path):
    from character_lab.domain import display_title

    p = Project(tmp_path)
    root = p.add("Source title", kind="source")
    gen = p.add("Source title continuation", parent=root["id"])
    edit = p.edit(gen["id"], "Edited text")
    fork = p.add(edit["text"], parent=edit["id"], kind="fork")
    assert [display_title(n) for n in p.data["nodes"]] == [
        "doc-branch-0001",
        "doc-gen-0001",
        "doc-edit-0001",
        "doc-branch-0002",
    ]
    p.delete_nodes([fork["id"]], [fork["id"]])
    p = Project(tmp_path)
    assert display_title(p.add("another fork", kind="fork")) == "doc-branch-0003"
    assert p.node(edit["id"])["parent"] == gen["id"]
    p.node(gen["id"])["title"] = "My own name"
    assert display_title(p.node(gen["id"])) == "My own name"


def test_legacy_labels_do_not_use_text_or_change_content():
    from character_lab.domain import assign_labels, display_title

    data = {
        "nodes": [dict(id="a", kind="generated", text="First AI line", parent=None)]
    }
    assign_labels(data)
    assign_labels(data)
    assert display_title(data["nodes"][0]) == "doc-gen-0001"
    assert data["nodes"][0]["text"] == "First AI line"
    assert data["label_counters"] == {"Gen": 1}


def test_preview_offset_tracks_this_version_not_inherited_ai(tmp_path):
    p = Project(tmp_path)
    root = p.add("Seed 日本語\n", kind="source")
    first = p.add(root["text"] + "Old AI\n", parent=root["id"], prompt=root["text"])
    second = p.add(first["text"] + "New AI", parent=first["id"], prompt=first["text"])
    assert p.change_offset(second["id"]) == len(first["text"])
    edit = p.edit(second["id"], second["text"].replace("New", "Changed"))
    assert p.change_offset(edit["id"]) == len(first["text"])
    deletion = p.edit(second["id"], second["text"].replace("Old AI\n", ""))
    assert p.change_offset(deletion["id"]) == len(root["text"])
    fork = p.add(second["text"], parent=second["id"], kind="fork")
    assert p.change_offset(fork["id"]) == len(first["text"])
    assert p.change_offset(root["id"]) == 0


def test_source_labels_inherit_and_migrate_existing_numbers(tmp_path):
    from character_lab.domain import assign_labels, display_title

    p = Project(tmp_path)
    root = p.add("Seed", kind="source", source_documents=[{"key": "gunkel"}])
    gen = p.add("Seed output", parent=root["id"])
    assert display_title(root) == "paths-branch-0001"
    assert display_title(gen) == "paths-gen-0001"
    gen.pop("label_number")
    gen["label"] = "Gen 7"
    assign_labels(p.data)
    assert display_title(gen) == "paths-gen-0007"
    assert display_title(p.add("Next", parent=gen["id"])) == "paths-gen-0008"
