"""Explicit action containment, separate from ancestry and terminal focus.

Executors may enumerate leaves for scheduling, but persist the tree so a set of
sets never becomes an indistinguishable flat selection.
"""

from copy import deepcopy


def validate(scope, leaf_kind):
    """Return an owned tree after validating structure (owners validate IDs)."""
    if not isinstance(scope, dict):
        raise ValueError("An action scope must be a document/conversation or a set")
    kind = scope.get("kind")
    if kind == "set":
        children = scope.get("children")
        if not isinstance(children, list) or not children:
            raise ValueError("An action set must contain at least one item")
        if "id" in scope and not isinstance(scope["id"], str):
            raise ValueError("A set identity must be text")
        return {
            **deepcopy(scope),
            "children": [validate(c, leaf_kind) for c in children],
        }
    if kind != leaf_kind:
        raise ValueError(f"This action requires {leaf_kind} targets")
    if "children" in scope:
        raise ValueError("Only sets can contain children")
    return deepcopy(scope)


def leaves(scope):
    """Yield exact leaf targets in user-visible containment order."""
    if scope["kind"] == "set":
        for child in scope["children"]:
            yield from leaves(child)
    else:
        yield scope


def map_leaves(scope, transform):
    """Replace leaves without changing the containment tree."""
    if scope["kind"] == "set":
        return {
            **deepcopy(scope),
            "children": [map_leaves(c, transform) for c in scope["children"]],
        }
    return transform(deepcopy(scope))
