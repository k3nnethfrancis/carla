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
    journal = p.journal.path.read_bytes()
    p.save()
    # Crash between snapshot rename and journal removal: old records are skipped.
    p.journal.path.write_bytes(journal + b'{"text":"\xf0\x9f')
    recovered = Project(tmp_path)
    assert [n["text"] for n in recovered.data["nodes"]] == ["seed024", "seed135"]
    assert not recovered.journal.path.exists()


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
