"""Persistent Simulator operation names; inference records and IDs stay unchanged.

Full names record ancestry. Short labels are a presentation convenience for a
nested tree. Counters survive deletion, and renames only affect descendants when
an explicit ancestry alias is set. Historical revisions remain frozen evidence.
"""

import json

NAME_FIELDS = ("label", "short_label", "title", "ancestry_name")


def assign_labels(data):
    runs = {r["id"]: r for r in data.get("simulation_runs", []) if "id" in r}
    state = data.setdefault("simulation_names", {})
    counters = state.setdefault("counters", {})
    operations = state.setdefault("operations", {})
    scopes = state.setdefault("scopes", {})
    visiting, done, named_groups = set(), set(), set()

    def allocate(record, kind, parent_key, suffix=""):
        key = json.dumps([kind, parent_key], separators=(",", ":"))
        if "name_number" not in record:
            record["name_number"] = counters.get(key, 0) + 1
        counters[key] = max(counters.get(key, 0), record["name_number"])
        suffix = suffix or record.get("name_parent", "")
        record["name_parent"] = suffix
        short = f"{kind}-{record['name_number']}"
        record.update(short_label=short, label=short + ("-" + suffix if suffix else ""))
        return record.get("ancestry_name") or record["label"]

    def parent_name(parent):
        if not parent or parent.get("run") not in runs:
            return ""
        run = runs[parent["run"]]
        if run["id"] in visiting:
            return ""  # Damaged legacy ancestry must not prevent workspace loading.
        name_run(run)
        index = parent.get("conversation", -1)
        record = next((c for c in run["conversations"] if c["index"] == index), run)
        return record.get("ancestry_name") or record["label"]

    def source_name(scope, parent):
        if scope:
            # Resolve every ancestor before consulting stored group names. This
            # makes renamed ancestry independent of saved list order.
            def resolve(node):
                if node.get("kind") == "conversation":
                    parent_name(node)
                else:
                    for child in node.get("children", []):
                        resolve(child)

            resolve(scope)
            key = scope.get("id", "")
            if key.startswith("simulation-group:"):
                key = key.removeprefix("simulation-group:")
            if key in operations:
                rec = operations[key]
                return rec.get("ancestry_name") or rec.get("label", "")
            if key in scopes:
                rec = scopes[key]
                return rec.get("ancestry_name") or rec.get("label", "")
            if scope.get("kind") == "conversation":
                return parent_name(scope)
            if key in runs:
                return parent_name(dict(run=key, conversation=-1))
        return parent_name(parent)

    def scope_names(tree, full, short, owner=None):
        if tree.get("kind") != "set":
            return
        record = scopes.setdefault(tree["id"], {}) if owner is None else owner
        scopes[tree["id"]] = record
        record.update(label=full, short_label=short)
        for key in NAME_FIELDS:
            tree.pop(key, None)
        tree.update({k: record[k] for k in NAME_FIELDS if k in record})
        prefix = record.get("ancestry_name") or full
        for i, child in enumerate(tree.get("children", [])):
            if child.get("kind") == "set":
                local = f"set-{i + 1}"
                scope_names(child, local + "-" + prefix, local)
            else:
                run = runs.get(child.get("run"))
                if run:
                    conversation = next(
                        (
                            c
                            for c in run["conversations"]
                            if c["index"] == child.get("conversation")
                        ),
                        None,
                    )
                    if conversation is not None:
                        local = f"convo-{conversation['index'] + 1}"
                        conversation.update(
                            short_label=local, label=local + "-" + prefix
                        )

    def name_run(run):
        rid = run["id"]
        if rid in done:
            return
        visiting.add(rid)
        suffix = source_name(run.get("source_scope"), run.get("parent"))
        parent_key = json.dumps(run.get("parent"), sort_keys=True)
        group = run.get("alternative_group")
        fork = run.get("protocol") == "conversation-fork-v1"
        if group:
            operation = operations.setdefault(group, {})
            if group not in named_groups:
                # Every peer shares one operation. Freeze its naming parent once;
                # unrelated source sets must not rewrite it as peers are visited.
                operation.setdefault("parent", run.get("parent"))
                operation.setdefault("source_scope", run.get("source_scope"))
                group_suffix = source_name(
                    operation["source_scope"], operation["parent"]
                )
                allocate(
                    operation,
                    "branch" if fork else "loom",
                    json.dumps(operation["parent"], sort_keys=True) if fork else "",
                    group_suffix,
                )
                named_groups.add(group)
            full = operation.get("ancestry_name") or operation["label"]
            run.update(
                operation_label=operation["label"],
                operation_short_label=operation["short_label"],
            )
            if operation.get("title"):
                run["operation_title"] = operation["title"]
            else:
                run.pop("operation_title", None)
            short = f"branch-{run.get('alternative_index', 0) + 1}"
            alternate_key = f"{group}/{run.get('alternative_index', 0)}"
            alt = scopes.setdefault(alternate_key, {})
            alt.update(label=short + "-" + full, short_label=short)
            run.update(alternative_label=alt["label"], alternative_short_label=short)
            if alt.get("title"):
                run["alternative_title"] = alt["title"]
            else:
                run.pop("alternative_title", None)
            base = alt.get("ancestry_name") or alt["label"]
            run.update(label=base, short_label=short)
            if (
                "source_set" in run
                and sum(
                    r.get("alternative_group") == group
                    and r.get("alternative_index") == run.get("alternative_index")
                    for r in runs.values()
                )
                > 1
            ):
                short = f"set-{run['source_set'] + 1}"
                run.update(label=short + "-" + base, short_label=short)
        else:
            allocate(
                run, "branch" if fork else "loom", parent_key if fork else "", suffix
            )
        base = run.get("ancestry_name") or run["label"]
        for conversation in run["conversations"]:
            short = f"convo-{conversation['index'] + 1}"
            # A one-conversation explicit fork is itself the branch, not another
            # newly invented conversation nested below that branch.
            single_fork = (
                fork
                or (group and run.get("source_scope", {}).get("kind") == "conversation")
            ) and len(run["conversations"]) == 1
            conversation.update(
                short_label=run["short_label"] if single_fork else short,
                label=run["label"] if single_fork else short + "-" + base,
            )
        tree = run.get("alternative_scope")
        if tree and tree.get("kind") == "set":
            scope_names(
                tree,
                alt["label"] if group else run["label"],
                run.get("alternative_short_label", run["short_label"]),
                alt if group else run,
            )
        visiting.remove(rid)
        done.add(rid)

    for run in runs.values():
        name_run(run)


def rename(data, args):
    """Rename a run, conversation or named alternative group; content is untouched."""
    assign_labels(data)
    if "scope" in args:
        record = data["simulation_names"]["scopes"].get(args["scope"])
    elif "group" in args:
        key = args["group"]
        collection = "operations"
        if "alternative" in args:
            key += "/" + str(args["alternative"])
            collection = "scopes"
        record = data["simulation_names"][collection].get(key)
    else:
        record = next(
            (r for r in data.get("simulation_runs", []) if r["id"] == args.get("run")),
            None,
        )
        if record is not None and "conversation" in args:
            record = next(
                (
                    c
                    for c in record["conversations"]
                    if c["index"] == args["conversation"]
                ),
                None,
            )
    if record is None:
        raise ValueError("Select a saved conversation or Loom to rename")
    title = args.get("title", "")
    if not isinstance(title, str):
        raise ValueError("Name must be text")
    title = title.strip()
    if "\n" in title or "\r" in title:
        raise ValueError("Use a single-line name")
    record["title"] = title
    if args.get("update_children", False):
        if title:
            record["ancestry_name"] = title
        else:
            record.pop("ancestry_name", None)
    assign_labels(data)
