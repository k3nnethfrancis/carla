"""Document action identity and grouping, independent of inference and the UI.

Node IDs remain immutable version references used by ancestry, anthology and
evaluation. ``document_id`` identifies the evolving document, and its head points
to the latest saved version. Sets follow the same pattern: a set revision stores
ordered node references; advancing it never rewrites previous membership.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass

from . import action_scope
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
    scope: dict | None = None
    fork: bool = False


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


def current_members(project, selected_set):
    """Resolve operational heads without rewriting saved set provenance."""
    existing = {node["id"] for node in project.data["nodes"]}
    if any(key not in existing for key in selected_set["members"]):
        raise ValueError("A selected document version no longer exists")
    if (
        project.data.get("document_set_heads", {}).get(
            selected_set["set_id"], selected_set["id"]
        )
        != selected_set["id"]
    ):
        return list(selected_set["members"])
    return [
        project.data.get("document_heads", {}).get(
            project.node(key).get("document_id", key), key
        )
        for key in selected_set["members"]
    ]


def plan(
    project,
    action,
    node_ids=None,
    *,
    count=1,
    offsets=None,
    set_id=None,
    scope=None,
    fork=False,
):
    """Validate and freeze targets; ancestry never expands into implicit scope.

    Offsets are Unicode code points. A selected historical version or prefix is
    valid, but Continue will branch rather than replace its existing future.
    Library passage composition must happen before calling this function.
    """
    if not isinstance(fork, bool):
        raise ValueError("Document fork choice must be enabled or disabled")
    if action not in {"continue", "loom", "branch"}:
        raise ValueError("Unknown document action")
    if type(count) is not int or count < 1:
        raise ValueError("Alternatives must be a positive integer")
    if action != "loom" and count != 1:
        raise ValueError("Only Loom accepts a number of alternatives")
    if action == "loom" and count == 1:
        action = "continue"
    if scope is not None:
        if node_ids or set_id is not None:
            raise ValueError("Supply one explicit action scope")
        scope = action_scope.validate(scope, "document")
        node_ids = [leaf.get("node") for leaf in action_scope.leaves(scope)]
        # Saved set identity is useful only when the explicit leaves still match
        # its operational membership; it never overrides the requested scope.
        if scope.get("kind") == "set" and any(
            s["id"] == scope.get("id") for s in project.data.get("document_sets", [])
        ):
            selected_set = group(project, scope["id"])
            if node_ids != current_members(project, selected_set):
                raise ValueError("The selected set changed; select it again")
            set_id = scope["id"]
            node_ids = None
    if set_id is not None:
        if node_ids:
            raise ValueError("Select document versions or a document set, not both")
        selected_set = group(project, set_id)
        node_ids = current_members(project, selected_set)
        saved_scope = selected_set.get("scope")
        if saved_scope:
            heads = dict(zip(selected_set["members"], node_ids))
            scope = action_scope.map_leaves(
                saved_scope,
                lambda leaf: {**leaf, "node": heads.get(leaf["node"], leaf["node"])},
            )
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
    if scope is None:
        children = [{"kind": "document", "node": t.node_id} for t in targets]
        scope = (
            children[0] if len(children) == 1 else {"kind": "set", "children": children}
        )
    return DocumentPlan(action, tuple(targets), count, set_id, scope, fork)


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
        and not action_plan.fork
        and source is not None
        and heads.get(source["set_id"], source["id"]) == source["id"]
        and all(_can_advance(project, target) for target in action_plan.targets)
    )
    candidates = []
    operation_id = uuid.uuid4().hex[:12]
    for alternative in range(action_plan.count):
        key = uuid.uuid4().hex[:12]
        result = dict(
            id=key,
            operation_id=operation_id,
            set_id=source["set_id"] if advance_set else key,
            parent=source["id"] if source else None,
            previous_revision=source["id"] if advance_set else None,
            action=action_plan.action,
            alternative=alternative,
            sources=[target.node_id for target in action_plan.targets],
            members=[],
            source_scope=action_plan.scope,
            scope=action_plan.scope,
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
                    and not action_plan.fork
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
    replacements = {
        result["sources"][project.node(key)["set_member"]]: key
        for key in result["members"]
    }
    result["scope"] = action_scope.map_leaves(
        result["source_scope"],
        lambda leaf: {**leaf, "node": replacements.get(leaf["node"], leaf["node"])},
    )

    def clear_ids(scope):
        if scope["kind"] == "set":
            scope.pop("id", None)
            for child in scope["children"]:
                clear_ids(child)

    clear_ids(result["scope"])
    if result["scope"]["kind"] == "set":
        result["scope"]["id"] = result["id"]
    project.save()
    return node


def branch(project, action_plan):
    """Fork an ordered selection without calling a model."""
    if action_plan.action != "branch":
        raise ValueError("Branch requires a branch plan")
    return [create_revision(project, item) for item in begin(project, action_plan)]
