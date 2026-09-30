"""Stable operation names, newest first; IDs and content remain untouched.

Counters are scoped to the exact parent version/operation, survive deletion, and
are assigned before streaming. Labels describe provenance, not completion order.
"""

import json


def assign_labels(data):
    nodes = {n["id"]: n for n in data["nodes"]}
    groups = {g["id"]: g for g in data.get("document_sets", [])}
    counters = data.setdefault("name_counters", {})
    operations = data.setdefault("operation_names", {})
    visited = set()

    def allocate(record, category, parent_key, suffix=""):
        # Rebuild ancestry suffixes after an opted-in rename; retain allocated numbers.
        key = json.dumps([category, parent_key], separators=(",", ":"))
        if record.get("name_version") != 2:
            if record.get("label"):
                record.setdefault("legacy_label", record["label"])
            record["name_number"] = counters.get(key, 0) + 1
            record["name_version"] = 2
        counters[key] = max(counters.get(key, 0), record["name_number"])
        label = f"{category}-{record['name_number']}"
        record["label"] = label + ("-" + suffix if suffix else "")
        return record["label"]

    def group_name(group):
        if ("set", group["id"]) in visited:
            return group["label"]
        parent = groups.get(group.get("parent"))
        sources = [key for key in group.get("sources", []) if key in nodes]
        if parent:
            suffix, parent_key = group_name(parent), "set:" + parent["id"]
        else:
            suffix = "-and-".join(node_name(nodes[key]) for key in sources)
            parent_key = (
                sources[0]
                if len(sources) == 1
                else json.dumps(sources, separators=(",", ":"))
            )
        # Old groups have no shared invocation ID. Keep them separate rather than
        # inventing a relationship based on timestamps or adjacent list positions.
        operation_id = group.setdefault("operation_id", group["id"])
        operation = operations.setdefault(operation_id, {})
        action = group.get("action", "branch")
        label = allocate(operation, action, parent_key, suffix)
        group["operation_label"] = label
        group["label"] = (
            f"branch-{group.get('alternative', 0) + 1}-{label}"
            if action == "loom"
            else label
        )
        visited.add(("set", group["id"]))
        return group["label"]

    def node_name(node):
        if ("node", node["id"]) in visited:
            return node.get("ancestry_name") or node["label"]
        parent = nodes.get(node.get("parent"))
        group = groups.get(node.get("document_set"))
        if parent is None:
            label = allocate(node, "doc", "")
        elif group:
            label = group_name(group)
            if len(group.get("sources", [])) > 1:
                # A set operation contains separately identifiable document outputs.
                label = f"branch-{node.get('set_member', 0) + 1}-{label}"
            if node.get("label") and node.get("label") != label:
                node.setdefault("legacy_label", node["label"])
            node["label"] = label
        else:
            category = {"generated": "continue", "edit": "edit"}.get(
                node["kind"], "branch"
            )
            label = allocate(node, category, parent["id"], node_name(parent))
        visited.add(("node", node["id"]))
        return node.get("ancestry_name") or label

    for node in data["nodes"]:
        node_name(node)
    for group in groups.values():
        group_name(group)
