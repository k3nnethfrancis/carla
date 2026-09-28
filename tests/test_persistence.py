"""Journal recovery must retain exact interleaved output without duplication."""

import copy
import json

import pytest

from character_lab.domain import Project
from character_lab.persistence import StreamJournal


def test_interleaved_recovery_checkpoint_and_torn_tail(tmp_path):
    p = Project(tmp_path)
    nodes = [p.add("seed", kind="generated") for _ in range(2)]
    for i in range(6):
        n = nodes[i % 2]
        trace = n.setdefault("trace", {"request": {"prompt": "seed"}, "events": []})
        trace["events"].append({"content": str(i)})
        trace["generated_tokens"] = i
        n["text"] += str(i)
        p.stream_delta({"node": n["id"]}, str(i), trace)
    # Recovery starts from the last snapshot, not mutated in-memory objects.
    data = json.loads(p.path.read_text())
    StreamJournal(tmp_path, data)
    assert [n["text"] for n in data["nodes"]] == ["seed024", "seed135"]
    assert data["nodes"][0]["trace"]["events"] == nodes[0]["trace"]["events"]
    assert data["nodes"][1]["trace"]["request"] == {"prompt": "seed"}
    journal = (tmp_path / "stream.jsonl").read_bytes()
    p.save()
    # Crash between snapshot rename and journal removal: old records are skipped.
    (tmp_path / "stream.jsonl").write_bytes(journal + b'{"text":"\xf0\x9f')
    recovered = Project(tmp_path)
    assert [n["text"] for n in recovered.data["nodes"]] == ["seed024", "seed135"]
    assert not (tmp_path / "stream.jsonl").exists()


def test_corrupt_complete_record_and_sequence_gap_fail_loudly(tmp_path):
    data = {"nodes": [], "journal_sequence": 0}
    path = tmp_path / "stream.jsonl"
    path.write_text("broken\n")
    with pytest.raises(json.JSONDecodeError):
        StreamJournal(tmp_path, copy.deepcopy(data))
    path.write_text('{"seq":2}\n')
    with pytest.raises(ValueError, match="sequence gap"):
        StreamJournal(tmp_path, copy.deepcopy(data))


def test_checkpoint_midstream_then_replay_only_new_events(tmp_path):
    p = Project(tmp_path)
    node = p.add("seed")
    node["trace"] = {"events": []}
    for text in ("a", "b"):
        node["text"] += text
        node["trace"]["events"].append({"content": text})
        p.stream_delta({"node": node["id"]}, text, node["trace"])
        if text == "a":
            p.save()  # A sibling's turn or monitor can checkpoint another live stream.
    recovered = Project(tmp_path).node(node["id"])
    assert recovered["text"] == "seedab"
    assert recovered["trace"]["events"] == [{"content": "a"}, {"content": "b"}]


def test_failed_snapshot_replacement_keeps_replayable_journal(tmp_path, monkeypatch):
    from character_lab import persistence

    store = persistence.WorkspaceStore(
        tmp_path, {"nodes": [{"id": "a", "text": "seed"}]}
    )
    store.checkpoint()
    original = store.path.read_bytes()
    store.data["nodes"][0]["text"] += "🙂"
    store.append({"node": "a"}, "🙂", {"events": [{"content": "🙂"}]})

    def fail_replace(*args):
        raise OSError("disk full")

    with monkeypatch.context() as patch:
        patch.setattr(persistence.os, "replace", fail_replace)
        with pytest.raises(OSError, match="disk full"):
            store.checkpoint()
    assert store.path.read_bytes() == original
    assert (tmp_path / "stream.jsonl").exists()
    recovered = persistence.WorkspaceStore(tmp_path, {})
    assert recovered.data["nodes"][0]["text"] == "seed🙂"
    recovered.checkpoint()
    assert not (tmp_path / "stream.jsonl").exists()


def test_failed_compaction_does_not_duplicate_checkpointed_output(
    tmp_path, monkeypatch
):
    from pathlib import Path

    from character_lab.persistence import WorkspaceStore

    store = WorkspaceStore(tmp_path, {"nodes": [{"id": "a", "text": "seed"}]})
    store.checkpoint()
    store.data["nodes"][0]["text"] += "x"
    store.data["nodes"][0]["trace"] = {"events": [{"content": "x"}]}
    store.append({"node": "a"}, "x", store.data["nodes"][0]["trace"])
    unlink = Path.unlink

    def fail_journal_unlink(path, **kwargs):
        if path.name == "stream.jsonl":
            raise OSError("interrupted after rename")
        return unlink(path, **kwargs)

    with monkeypatch.context() as patch:
        patch.setattr(Path, "unlink", fail_journal_unlink)
        with pytest.raises(OSError, match="after rename"):
            store.checkpoint()
    recovered = WorkspaceStore(tmp_path, {})
    assert recovered.data["nodes"][0]["text"] == "seedx"
    assert recovered.data["nodes"][0]["trace"]["events"] == [{"content": "x"}]
    recovered.checkpoint()
    assert not (tmp_path / "stream.jsonl").exists()
