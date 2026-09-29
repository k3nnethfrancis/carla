"""Portable, lossless content exports, independent of a training framework.

Every export is a frozen JSON bundle plus human-readable text. References needed
for ancestry live in the manifest; UI selection never changes saved content.
Credentials and unrelated workspace configuration are deliberately not exported.
"""

import copy
import hashlib
import json
from uuid import uuid4

from . import evaluation_sets
from .domain import now


def records(project, sources, args):
    """Resolve an explicit selection, or the named view's collection, atomically."""
    scope = args.get("scope", "anthology")
    data = project.data
    items = []
    if scope == "library":
        refs = args.get("refs")
        known = {
            f"{s['key']}:{p['id']}": (s, p) for s in sources for p in s["passages"]
        }
        keys = list(known) if not refs else refs
        if any(key not in known for key in keys):
            raise ValueError("A selected library passage is no longer available")
        for key in dict.fromkeys(keys):
            source, passage = known[key]
            items.append(
                dict(
                    kind="source",
                    id=key,
                    text=passage["text"],
                    source={k: v for k, v in source.items() if k != "passages"},
                    passage=copy.deepcopy(passage),
                )
            )
    elif scope in {"branches", "anthology"}:
        keys = args.get("nodes")
        available = {n["id"]: n for n in data["nodes"]}
        if keys and any(key not in available for key in keys):
            raise ValueError("A selected document is no longer available")
        nodes = (
            [available[k] for k in dict.fromkeys(keys)]
            if keys
            else [n for n in data["nodes"] if scope == "branches" or n.get("kept")]
        )
        for node in nodes:
            items.append(
                dict(
                    kind="document",
                    id=node["id"],
                    text=node["text"],
                    document=copy.deepcopy(node),
                )
            )
    elif scope == "simulator":
        runs = {r["id"]: r for r in data.get("simulation_runs", [])}
        targets = args.get("targets") or [
            dict(run=r["id"], conversation=c["index"])
            for r in runs.values()
            for c in r["conversations"]
        ]
        seen = set()
        for target in targets:
            key, index = target.get("run"), target.get("conversation")
            if key not in runs or type(index) is not int:
                raise ValueError("A selected conversation is no longer available")
            run = runs[key]
            conversation = next(
                (c for c in run["conversations"] if c["index"] == index), None
            )
            if conversation is None:
                raise ValueError("A selected conversation is no longer available")
            if (key, index) in seen:
                continue
            seen.add((key, index))
            items.append(
                dict(
                    kind="conversation",
                    id=f"{key}:{index}",
                    text="\n\n".join(
                        f"{t['role']}:\n{t['text']}" for t in conversation["turns"]
                    ),
                    conversation=copy.deepcopy(conversation),
                    run={
                        k: copy.deepcopy(v)
                        for k, v in run.items()
                        if k not in {"conversations", "revisions"}
                    },
                )
            )
    elif scope == "evaluate":
        group = evaluation_sets.resolve(project, args.get("collection"))
        ids = args.get("items")
        known = {i["id"]: i for i in group["items"]}
        if ids and any(key not in known for key in ids):
            raise ValueError("A selected evaluation item is no longer available")
        selected = [known[k] for k in dict.fromkeys(ids)] if ids else group["items"]
        for item in selected:
            items.append(
                dict(
                    kind="evaluation",
                    id=item["id"],
                    text=item["text"],
                    item=copy.deepcopy(item),
                    judgments=copy.deepcopy(evaluation_sets.records_for(project, item)),
                    collection={
                        k: copy.deepcopy(v)
                        for k, v in group.items()
                        if k not in {"items", "removed_items"}
                    },
                )
            )
    else:
        raise ValueError(
            "Choose Library, Branches, Anthology, Simulator or Evaluate to export"
        )
    if not items:
        raise ValueError("There are no saved items to export")
    return scope, items


def export(project, sources, args):
    scope, items = records(project, sources, args)
    # Preserve document ancestry needed by selected items, without adding unrelated
    # documents to the export. Conversation records already embed their seed docs.
    known = {n["id"]: n for n in project.data["nodes"]}
    ancestors = {}
    for item in items:
        parent = item.get("document", {}).get("parent")
        while parent and parent in known and parent not in ancestors:
            ancestors[parent] = copy.deepcopy(known[parent])
            parent = known[parent].get("parent")
    for item in items:
        item["sha256"] = hashlib.sha256(item["text"].encode()).hexdigest()
    groups = {g["id"]: g for g in project.data.get("document_sets", [])}
    exported_groups = {}
    for node in [i["document"] for i in items if "document" in i] + list(
        ancestors.values()
    ):
        key = node.get("document_set")
        while key in groups and key not in exported_groups:
            exported_groups[key] = copy.deepcopy(groups[key])
            key = groups[key].get("parent")
    node_ids = set(ancestors) | {i["id"] for i in items if i["kind"] == "document"}
    policies = {i.get("document", i.get("run", {})).get("policy_run") for i in items}
    manifest = dict(
        format="carla-export",
        version=1,
        created=now(),
        scope=scope,
        items=items,
        document_ancestors=list(ancestors.values()),
        document_sets=list(exported_groups.values()),
        annotations=[
            copy.deepcopy(a)
            for a in project.data.get("annotations", [])
            if a.get("node") in node_ids
        ],
        selection_policies=[
            copy.deepcopy(r)
            for r in project.data.get("policy_runs", [])
            if r["id"] in policies
        ],
    )
    folder = project.folder / "exports" / f"{scope}-{uuid4().hex[:12]}"
    folder.mkdir(parents=True)
    for i, item in enumerate(items, 1):
        (folder / f"{i:04d}.txt").write_text(item["text"], encoding="utf-8")
    # Publish the manifest last so an incomplete export is visibly incomplete.
    temporary = folder / "manifest.tmp"
    temporary.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    temporary.replace(folder / "manifest.json")
    return folder
