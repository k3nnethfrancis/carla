"""Small append-only stream records plus atomic workspace checkpoints.

A checkpoint includes the last journal sequence. On crash recovery, records at or
below that sequence are skipped, so a crash between rename and journal truncation
cannot append a token twice. A torn final JSON line is discarded; other corruption
is an error. This protects process-crash recovery, not power-loss durability.
"""

import json
import os
from pathlib import Path


def target_item(data, target):
    if "node" in target:
        return next(n for n in data["nodes"] if n["id"] == target["node"])
    run = next(r for r in data["simulation_runs"] if r["id"] == target["run"])
    return run["conversations"][target["conversation"]]["turns"][target["turn"]]


class StreamJournal:
    def __init__(self, folder: Path, data):
        self.path = folder / "stream.jsonl"
        self.sequence = data.get("journal_sequence", 0)
        self.offsets = {}
        if self.path.exists():
            with self.path.open("rb") as stream:
                for line in stream:
                    if not line.endswith(b"\n"):
                        break  # Only a final interrupted append can lack its newline.
                    record = json.loads(line)
                    if record["seq"] <= self.sequence:
                        continue
                    if record["seq"] != self.sequence + 1:
                        raise ValueError(
                            "Stream journal sequence gap; preserve the workspace for recovery"
                        )
                    item = target_item(data, record["target"])
                    item["text"] += record["text"]
                    trace = item.setdefault("trace", {})
                    trace.update(record["trace"])
                    trace.setdefault("events", []).extend(record["events"])
                    self.sequence = record["seq"]

    def append(self, target, text, trace):
        key = tuple(sorted(target.items()))
        offset = self.offsets.get(key)
        events = trace.get("events", [])
        metadata = (
            {k: v for k, v in trace.items() if k != "events"}
            if offset is None
            else {k: trace[k] for k in ("generated_tokens", "scheduling") if k in trace}
        )
        record = dict(
            seq=self.sequence + 1,
            target=target,
            text=text,
            trace=metadata,
            events=events[offset or 0 :],
        )
        with self.path.open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(record, ensure_ascii=False) + "\n")
        self.sequence += 1
        self.offsets[key] = len(events)

    def compact(self):
        # The full snapshot has already been atomically replaced by the caller.
        self.path.unlink(missing_ok=True)


class WorkspaceStore:
    """Own snapshot loading, journal replay and checkpoint ordering together.

    Project mutates `data` and decides when to checkpoint. This class owns disk
    mechanics only: the existing JSON schema and recovery sequence are unchanged.
    Never compact the journal before the snapshot replacement succeeds.
    """

    def __init__(self, folder: Path, initial):
        folder.mkdir(parents=True, exist_ok=True)
        self.path = folder / "project.json"
        self.data = json.loads(self.path.read_text()) if self.path.exists() else initial
        self._journal = StreamJournal(folder, self.data)

    def append(self, target, text, trace):
        """Record a delta after Project has applied it to its in-memory data."""
        self._journal.append(target, text, trace)

    def checkpoint(self):
        self.data["journal_sequence"] = self._journal.sequence
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self.data, ensure_ascii=False, indent=2) + "\n")
        os.replace(tmp, self.path)
        self._journal.compact()
