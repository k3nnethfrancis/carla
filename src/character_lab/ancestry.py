"""Pure text provenance for immutable document versions.

Offsets are Unicode code points, matching Python slices and frontend offsets.
`origins` reconstructs source/AI/human spans from a node index; `first_change`
locates this version's first difference for preview. Neither mutates nodes or
performs I/O. `remap_origins` carries spans across a human edit.
"""

from difflib import SequenceMatcher


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


def first_change(nodes, node_id):
    """First change in this version, in Unicode code points for the preview."""
    node = nodes[node_id]
    if node["kind"] == "generated":
        return min(len(node["text"]), len(node.get("prompt", "")))
    if not node.get("parent"):
        return 0
    parent = nodes[node["parent"]]
    if node["text"] == parent["text"]:
        return first_change(nodes, parent["id"])
    # Includes pure deletions, which have no inserted origin span to reveal.
    for i, (before, after) in enumerate(zip(parent["text"], node["text"])):
        if before != after:
            return i
    return min(len(parent["text"]), len(node["text"]))


def origins(nodes, node_id):
    node = nodes[node_id]
    if "origins" in node:
        return node["origins"]
    text = node["text"]
    if node["kind"] == "source":
        return [dict(start=0, end=len(text), kind="source")] if text else []
    if node["parent"]:
        parent = nodes[node["parent"]]
        inherited = origins(nodes, parent["id"])
        if node["kind"] == "generated":
            prefix = node.get("prompt", parent["text"][: node.get("fork_offset") or 0])
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
