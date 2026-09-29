"""Document action identity and grouping, independent of inference and the UI.

Node IDs remain immutable version references used by ancestry, anthology and
evaluation. ``document_id`` identifies the evolving document, and its head points
to the latest saved version. Sets follow the same pattern: a set revision stores
ordered node references; advancing it never rewrites previous membership.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass

from .domain import now


@dataclass(frozen=True)
class DocumentTarget:
    node_id: str
    prefix: str


@dataclass(frozen=True)
class DocumentPlan:
    action: str
    targets: tuple[DocumentTarget, ...]
    count: int
    source_set: str | None = None


@dataclass(frozen=True)
class DocumentCandidate:
    target: DocumentTarget
    action: str
    group_id: str
    index: int
    member_index: int
    advance_identity: bool


def group(project, set_id):
    """Look up an exact set revision, never silently substitute its latest head."""
    found = next(
        (s for s in project.data.get("document_sets", []) if s["id"] == set_id),
        None,
    )
    if found is None:
        raise ValueError("The selected document set no longer exists")
    return found


def plan(project, action, node_ids=None, *, count=1, offsets=None, set_id=None):
    """Validate and freeze targets; ancestry never expands into implicit scope.

    Offsets are Unicode code points. A selected historical version or prefix is
    valid, but Continue will branch rather than replace its existing future.
    Library passage composition must happen before calling this function.
    """
    if action not in {"continue", "loom", "branch"}:
        raise ValueError("Unknown document action")
    if type(count) is not int or count < 1:
        raise ValueError("Alternatives must be a positive integer")
    if action != "loom" and count != 1:
        raise ValueError("Only Loom accepts a number of alternatives")
    if set_id is not None:
        if node_ids:
            raise ValueError("Select document versions or a document set, not both")
        selected_set = group(project, set_id)
        node_ids = selected_set["members"]
        if len(node_ids) != len(selected_set["sources"]):
            raise ValueError(
                "This document set did not finish starting; select its saved members instead"
            )
    if not isinstance(node_ids, (list, tuple)) or not node_ids:
        raise ValueError("Select at least one document")
    if any(not isinstance(key, str) for key in node_ids):
        raise ValueError("Document targets must be saved version IDs")
    if len(node_ids) != len(set(node_ids)):
        raise ValueError("A document cannot appear twice in the same selection")
    offsets = {} if offsets is None else offsets
    if not isinstance(offsets, dict) or set(offsets) - set(node_ids):
        raise ValueError("Document positions must refer to selected versions")
    targets = []
    identities = set()
    for key in node_ids:
        try:
            node = project.node(key)
        except StopIteration:
            raise ValueError("A selected document version no longer exists") from None
        if node.get("status") in {"queued", "generating"}:
            raise ValueError("Wait for the selected document to finish generating")
        end = offsets.get(key, len(node["text"]))
        if type(end) is not int or not 0 <= end <= len(node["text"]):
            raise ValueError("Branch position lies outside the document")
        identity = node.get("document_id", key)
        if action == "continue" and identity in identities:
            raise ValueError("Select one version per document to continue")
        identities.add(identity)
        targets.append(DocumentTarget(key, node["text"][:end]))
    return DocumentPlan(action, tuple(targets), count, set_id)


def _can_advance(project, target):
    node = project.node(target.node_id)
    identity = node.get("document_id", node["id"])
    return (
        node.get("kind") != "source"
        and project.data.get("document_heads", {}).get(identity, node["id"])
        == node["id"]
        and target.prefix == node["text"]
    )


def begin(project, action_plan):
    """Create result set revisions and return leaves in alternative/member order.

    Streaming may finish leaves out of order. ``member_index`` ensures the saved
    shape and member order follow the input set rather than completion timing.
    """
    source = group(project, action_plan.source_set) if action_plan.source_set else None
    heads = project.data.setdefault("document_set_heads", {})
    advance_set = (
        action_plan.action == "continue"
        and source is not None
        and heads.get(source["set_id"], source["id"]) == source["id"]
        and all(_can_advance(project, target) for target in action_plan.targets)
    )
    candidates = []
    for alternative in range(action_plan.count):
        key = uuid.uuid4().hex[:12]
        result = dict(
            id=key,
            set_id=source["set_id"] if advance_set else key,
            parent=source["id"] if source else None,
            previous_revision=source["id"] if advance_set else None,
            action=action_plan.action,
            alternative=alternative,
            sources=[target.node_id for target in action_plan.targets],
            members=[],
            created=now(),
        )
        project.data.setdefault("document_sets", []).append(result)
        heads[result["set_id"]] = key
        for member, target in enumerate(action_plan.targets):
            candidates.append(
                DocumentCandidate(
                    target,
                    action_plan.action,
                    key,
                    len(candidates),
                    member,
                    action_plan.action == "continue"
                    and (source is None or advance_set),
                )
            )
    project.save()
    return candidates


def create_revision(project, candidate, **extra):
    """Allocate a saved output version before streaming or complete a plain fork.

    A Continue from the latest saved document retains logical identity. A
    source, historical revision or earlier cursor is copied to a new identity.
    Neither old text nor existing kept/evaluated references are overwritten.
    """
    target = candidate.target
    original = project.node(target.node_id)
    result = group(project, candidate.group_id)
    if any(
        project.node(key).get("set_member") == candidate.member_index
        for key in result["members"]
    ):
        raise ValueError("This document candidate already has a saved result")
    advance = candidate.advance_identity and _can_advance(project, target)
    fields = dict(
        document_set=candidate.group_id,
        set_member=candidate.member_index,
        action="branch"
        if candidate.action == "continue" and not advance
        else candidate.action,
    )
    if advance:
        fields.update(
            document_id=original.get("document_id", original["id"]),
            revision_of=original["id"],
        )
    if candidate.action == "continue" and not advance:
        fields["branch_reason"] = (
            "source" if original.get("kind") == "source" else "historical_prefix"
        )
    # Identity/provenance are owned here; caller supplies sampling/trace metadata.
    if (
        fields.keys() & extra.keys()
        or {"parent", "fork_offset", "document_id", "revision_of", "kept", "id"}
        & extra.keys()
    ):
        raise ValueError("Document identity fields cannot be overridden")
    kind = "fork" if candidate.action == "branch" else "generated"
    node = project.add(
        target.prefix,
        parent=original["id"],
        fork_offset=len(target.prefix),
        kind=kind,
        checkpoint=False,
        **fields,
        **extra,
    )
    result["members"].append(node["id"])
    result["members"].sort(key=lambda key: project.node(key)["set_member"])
    project.save()
    return node


def branch(project, action_plan):
    """Fork an ordered selection without calling a model."""
    if action_plan.action != "branch":
        raise ValueError("Branch requires a branch plan")
    return [create_revision(project, item) for item in begin(project, action_plan)]
