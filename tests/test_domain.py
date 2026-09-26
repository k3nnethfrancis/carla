import pytest

from character_lab.domain import Project, library, search


def corpus():
    return library()[1]


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
    from character_lab.domain import origin_labels

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
