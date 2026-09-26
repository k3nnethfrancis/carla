"""Source selection and durable, non-destructive document branches."""

from __future__ import annotations

import hashlib
import json
import os
import re
import uuid
from datetime import datetime, timezone
from difflib import SequenceMatcher
from pathlib import Path

from .persistence import StreamJournal


def now():
    return datetime.now(timezone.utc).isoformat()


def generation_status(node):
    status = node.get("status", "complete")
    if (
        status == "complete"
        and node.get("kind") == "generated"
        and "prompt" in node
        and not node["text"][len(node["prompt"]) :].strip()
    ):
        return "empty"
    return status


def display_title(node):
    """Name a version by its new text when available; never rewrite its content."""
    if node.get("title"):
        return node["title"]
    text = node["text"]
    continuation = text[len(node.get("prompt", "")) :].strip()
    return next(
        (
            line.strip()[:90]
            for line in (continuation or text).splitlines()
            if line.strip()
        ),
        node["kind"],
    )


def library():
    from .library import load_library

    return load_library()


def search(passages, query="", book=0, selected=None):
    terms = query.casefold().strip().split()
    ref = re.fullmatch(r"(\d+)[.:](\d+)", query.strip())
    return [
        p
        for p in passages
        if (not book or p["book"] == book)
        and (selected is None or p["id"] in selected)
        and (
            p["id"] == f"{int(ref[1])}.{int(ref[2])}"
            if ref
            else all(term in p["text"].casefold() or term == p["id"] for term in terms)
        )
    ]


def pack_origins(labels):
    spans = []
    for i, kind in enumerate(labels):
        if spans and spans[-1]["kind"] == kind:
            spans[-1]["end"] = i + 1
        else:
            spans.append(dict(start=i, end=i + 1, kind=kind))
    return spans


def origin_labels(text, spans):
    labels = ["edited"] * len(text)
    for span in spans:
        start, end = max(0, span["start"]), min(len(text), span["end"])
        labels[start:end] = [span["kind"]] * (end - start)
    return labels


def remap_origins(before, after, spans):
    """Preserve unchanged text attribution; inserted/replaced text is an edit."""
    if before == after:
        return spans
    old = origin_labels(before, spans)
    labels = ["edited"] * len(after)
    for match in SequenceMatcher(
        None, before, after, autojunk=False
    ).get_matching_blocks():
        labels[match.b : match.b + match.size] = old[match.a : match.a + match.size]
    return pack_origins(labels)


class Project:
    def __init__(self, folder: Path):
        self.folder = folder
        folder.mkdir(parents=True, exist_ok=True)
        self.path = folder / "project.json"
        if self.path.exists():
            self.data = json.loads(self.path.read_text())
        else:
            self.data = dict(
                version=1,
                title=folder.name,
                selected=[],
                nodes=[],
                current=None,
                snapshots=[],
                created=now(),
            )
        self.journal = StreamJournal(folder, self.data)
        # Interrupted continuations must never masquerade as completed output.
        for node in self.data["nodes"]:
            if node.get("status") in {"generating", "queued"}:
                node["status"] = "interrupted"
        for run in self.data.get("policy_runs", []):
            if run["status"] == "running":
                run["status"] = "interrupted"
                if run.get("steps") and run["steps"][-1]["status"] in {
                    "generating",
                    "queued",
                    "selecting",
                }:
                    run["steps"][-1]["status"] = "interrupted"
        for run in self.data.get("simulation_runs", []):
            if run["status"] == "running":
                run["status"] = "interrupted"
                for conversation in run["conversations"]:
                    if conversation["status"] in {"running", "queued"}:
                        conversation["status"] = "interrupted"
                    for turn in conversation["turns"]:
                        if turn["status"] == "generating":
                            turn["status"] = "interrupted"
                        for check in [
                            turn.get("monitor", {}),
                            *turn.get("monitor_checks", []),
                        ]:
                            if check.get("status") == "checking":
                                check["status"] = "interrupted"
        self.save()

    def stream_delta(self, target, text, trace):
        """Persist only new text/provider events; in-memory state is already updated."""
        self.journal.append(target, text, trace)

    def save(self):
        self.data["journal_sequence"] = self.journal.sequence
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self.data, ensure_ascii=False, indent=2) + "\n")
        os.replace(tmp, self.path)
        self.journal.compact()

    @property
    def selected(self):
        return self.data["selected"]

    def toggle(self, ref):
        if ref in self.selected:
            self.selected.remove(ref)
        else:
            self.selected.append(ref)
        self.save()

    def seed(self, source):
        selected = [p for p in source["passages"] if p["id"] in self.selected]
        return "\n\n".join(p["text"] for p in selected), [p["id"] for p in selected]

    def node(self, node_id):
        return next(n for n in self.data["nodes"] if n["id"] == node_id)

    def add(self, text, parent=None, fork_offset=None, kind="generated", **extra):
        node = dict(
            id=uuid.uuid4().hex[:12],
            text=text,
            parent=parent,
            fork_offset=fork_offset,
            kind=kind,
            kept=False,
            created=now(),
            status="complete",
            **extra,
        )
        self.data["nodes"].append(node)
        self.data["current"] = node["id"]
        self.save()
        return node

    def root(self, source):
        text, refs = self.seed(source)
        if not text:
            raise ValueError("Select at least one passage first.")
        return self.add(
            text,
            kind="source",
            source={k: v for k, v in source.items() if k != "passages"},
            passage_ids=refs,
        )

    def source_root(self, sources, refs=None):
        chosen = []
        for source in sources:
            for passage in source["passages"]:
                key = source["key"] + ":" + passage["id"]
                if key in (self.selected if refs is None else refs):
                    chosen.append((source, passage))
        if not chosen:
            return None
        return self.add(
            "\n\n".join(p["text"] for _, p in chosen),
            kind="source",
            source_documents=[
                {k: v for k, v in source.items() if k != "passages"}
                for source in sources
                if any(s["key"] == source["key"] for s, _ in chosen)
            ],
            passage_ids=[source["key"] + ":" + p["id"] for source, p in chosen],
        )

    def origins(self, node_id):
        node = self.node(node_id)
        if "origins" in node:
            return node["origins"]
        text = node["text"]
        if node["kind"] == "source":
            return [dict(start=0, end=len(text), kind="source")] if text else []
        if node["parent"]:
            parent = self.node(node["parent"])
            inherited = self.origins(parent["id"])
            if node["kind"] == "generated":
                prefix = node.get(
                    "prompt", parent["text"][: node.get("fork_offset") or 0]
                )
                labels = (
                    origin_labels(parent["text"], inherited)[: len(prefix)]
                    if parent["text"].startswith(prefix)
                    else origin_labels(
                        prefix, remap_origins(parent["text"], prefix, inherited)
                    )
                )
                labels += ["ai"] * max(0, len(text) - len(prefix))
                return pack_origins(labels[: len(text)])
            return remap_origins(parent["text"], text, inherited)
        return [dict(start=0, end=len(text), kind="edited")] if text else []

    def edit(self, node_id, text):
        original = self.node(node_id)
        if text == original["text"]:
            return original
        return self.add(
            text,
            parent=node_id,
            kind="edit",
            edited_from=node_id,
            origins=remap_origins(original["text"], text, self.origins(node_id)),
        )

    def keep(self, node_id):
        node = self.node(node_id)
        node["kept"] = not node["kept"]
        self.save()

    def delete_nodes(self, ids, expected):
        """Delete a reviewed subtree, retaining a complete recovery snapshot."""
        known = {n["id"] for n in self.data["nodes"]}
        removed = set(ids)
        if not removed or not removed <= known:
            raise ValueError("Select existing branches to delete")
        while True:
            expanded = removed | {
                n["id"] for n in self.data["nodes"] if n.get("parent") in removed
            }
            if expanded == removed:
                break
            removed = expanded
        if removed != set(expected):
            raise ValueError("Branch tree changed; review the deletion again")
        archive = self.folder / "deleted"
        archive.mkdir(exist_ok=True)
        (archive / (uuid.uuid4().hex + ".json")).write_text(
            json.dumps(self.data, ensure_ascii=False, indent=2) + "\n"
        )

        def references(value):
            if isinstance(value, str):
                return value in removed
            if isinstance(value, dict):
                return any(references(v) for v in value.values())
            if isinstance(value, list):
                return any(references(v) for v in value)
            return False

        self.data["nodes"] = [n for n in self.data["nodes"] if n["id"] not in removed]
        self.data["annotations"] = [
            a for a in self.data.get("annotations", []) if a["node"] not in removed
        ]
        self.data["policy_runs"] = [
            r for r in self.data.get("policy_runs", []) if not references(r)
        ]
        if self.data["current"] in removed:
            self.data["current"] = (
                self.data["nodes"][-1]["id"] if self.data["nodes"] else None
            )
        self.save()
        return len(removed)

    def annotate(self, node_id, start, end, verdict, note):
        text = self.node(node_id)["text"]
        if not 0 <= start <= end <= len(text):
            raise ValueError("Selection is outside the saved document")
        if verdict not in {"unreviewed", "promising", "pass"}:
            raise ValueError("Unknown review decision")
        annotation = dict(
            id=uuid.uuid4().hex[:12],
            node=node_id,
            start=start,
            end=end,
            quote=text[start:end],
            verdict=verdict,
            note=note.strip(),
            text_sha256=hashlib.sha256(text.encode()).hexdigest(),
            created=now(),
        )
        self.data.setdefault("annotations", []).append(annotation)
        self.save()
        return annotation

    def import_run(self, folder, label):
        """Copy a frozen run without opening or mutating the original Project."""
        source = (Path(folder) / "project.json").resolve()
        imports = self.data.setdefault("imports", [])
        if any(item["path"] == str(source) for item in imports):
            return
        raw = source.read_bytes()
        incoming = json.loads(raw)
        existing = {n["id"] for n in self.data["nodes"]}
        if existing.intersection(n["id"] for n in incoming["nodes"]):
            raise ValueError("Imported run has colliding node IDs")
        for node in incoming["nodes"]:
            node["imported_from"] = str(source)
            node["run_label"] = label
        self.data["nodes"].extend(incoming["nodes"])
        imports.append(
            dict(
                path=str(source),
                label=label,
                sha256=hashlib.sha256(raw).hexdigest(),
                imported=now(),
            )
        )
        self.save()

    def snapshot(self):
        nodes = [n for n in self.data["nodes"] if n["kept"]]
        if not nodes:
            raise ValueError("Keep a document in the Loom before saving a snapshot.")
        name = datetime.now().strftime("%Y%m%d-%H%M%S") + "-" + uuid.uuid4().hex[:6]
        folder = self.folder / "snapshots" / name
        folder.mkdir(parents=True)
        for i, node in enumerate(nodes, 1):
            (folder / f"{i:02d}-{node['id']}.txt").write_text(node["text"])
        manifest = json.loads(json.dumps(self.data))
        manifest["snapshot_created"] = now()
        manifest["kept_documents"] = [
            {"node": n["id"], "sha256": hashlib.sha256(n["text"].encode()).hexdigest()}
            for n in nodes
        ]
        (folder / "manifest.json").write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"
        )
        self.data["snapshots"].append(str(folder))
        self.save()
        return folder
