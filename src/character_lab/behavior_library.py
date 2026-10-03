"""Explicit publication of policy specs; library edits never fan out to policies."""

import uuid

from . import evaluation_policies, operational_policies


def publish(project, args):
    selection = args.get("purpose") == "selection"
    if selection:
        policy = next(
            (
                p
                for p in project.data["operational_policies"]["selection"]
                if p["id"] == args.get("policy")
            ),
            None,
        )
        if policy is None:
            raise ValueError("Selection policy no longer exists")
        behaviors = policy["config"]["selection_behaviors"]
    else:
        policy = evaluation_policies.resolve(project, args.get("policy"))
        behaviors = policy["behaviors"]
    behavior = next((b for b in behaviors if b["id"] == args.get("behavior")), None)
    if behavior is None:
        raise ValueError("Behavior no longer exists")
    items = project.data["behavior_library"]
    source = next((b for b in items if b["id"] == behavior.get("source_id")), None)
    action = args.get("action", "save")
    if action not in {"save", "update", "refresh", "copy"}:
        raise ValueError("Unknown library action")
    if action in {"update", "refresh"}:
        if source is None or source["revision"] != args.get("revision"):
            raise ValueError("Library spec changed; reopen it before updating")
    before = dict(behavior)
    if action == "refresh":
        behavior.update(name=source["name"], spec=source["spec"])
    elif action == "update":
        if any(source[k] != behavior[k] for k in ("name", "spec")):
            source.update(
                name=behavior["name"],
                spec=behavior["spec"],
                revision=source["revision"] + 1,
            )
    else:
        # Repeated saves link to the same definition, including pre-link legacy copies.
        source = (
            next(
                (
                    b
                    for b in items
                    if all(b[k] == behavior[k] for k in ("name", "spec"))
                ),
                None,
            )
            if action == "save"
            else None
        )
        if source is None:
            source = dict(
                id=uuid.uuid4().hex[:12],
                name=behavior["name"],
                spec=behavior["spec"],
                revision=1,
            )
            items.append(source)
    behavior.update(source_id=source["id"], source_revision=source["revision"])
    if behavior != before:
        behavior["revision"] = behavior.get("revision", 0) + 1
        policy["revision"] += 1

    if selection:
        config = policy["config"]
        config["policy_spec"] = (
            "\n\n".join(
                f"{b['name']} (expected {b.get('expected', 'present')}): {b['spec']}"
                for b in behaviors
                if b.get("enabled", True)
            )
            or config["policy_spec"]
        )
        if project.data["active_operational_policies"].get("selection") == policy["id"]:
            operational_policies.project_values(project, "selection", config)
