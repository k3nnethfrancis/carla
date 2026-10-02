"""Small data-only templates: dotted lookups, JSON values, no executable expressions.

Replacement is a single pass: braces supplied by documents/specs remain literal.
A dotted field on a list projects that field from each object, preserving order.
"""

import json
import re

TOKEN = re.compile(r"{{\s*([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)*)\s*}}")


def variables(template):
    if not isinstance(template, str):
        raise ValueError("Prompt template must be text")
    # JSON closing braces are normal prompt text; only opening {{ opts in.
    remainder = TOKEN.sub("", template)
    if "{{" in remainder:
        raise ValueError("Invalid template variable; use {{name}} or {{object.field}}")
    return {m.group(1) for m in TOKEN.finditer(template)}


def lookup(value, fields, path):
    if not fields:
        return value
    if isinstance(value, list):
        return [lookup(item, fields, path) for item in value]
    if not isinstance(value, dict) or fields[0] not in value:
        raise ValueError(f"Unknown template variable: {{{{{path}}}}}")
    return lookup(value[fields[0]], fields[1:], path)


def render(template, context):
    variables(template)

    def replace(match):
        path = match.group(1)
        value = lookup(context, path.split("."), path)
        return (
            value if isinstance(value, str) else json.dumps(value, ensure_ascii=False)
        )

    return TOKEN.sub(replace, template)


def behavior_objects(definitions):
    """Detection/acceptance and actions are deliberately not behavior semantics."""
    return [{k: b[k] for k in ("id", "name", "spec") if k in b} for b in definitions]


def assessment_context(definitions, text):
    return {"behaviors": behavior_objects(definitions), "text": text, "history": text}


def validate_assessment(template):
    refs = variables(template)
    if not refs:
        return  # Existing instruction-only templates retain their JSON envelope.
    render(
        template,
        assessment_context(
            [dict(id="example", name="Example", spec="Criterion")], "text"
        ),
    )
    if not ({"text", "history"} & refs):
        raise ValueError("Assessment template must include {{text}} or {{history}}")
    if not ({"behaviors", "behaviors.spec"} & refs):
        raise ValueError(
            "Assessment template must include {{behaviors}} or {{behaviors.spec}}"
        )


def conversation_variables(template):
    # A real legacy history field distinguishes old escaped JSON braces from
    # the new syntax. Do not reinterpret saved literal {{...}} in old prompts.
    if re.search(r"(?<!{){history}(?!})", template) or "{{" not in template:
        return set(
            re.findall(r"(?<!{){(history|anthology|visitor_brief)}(?!})", template)
        ), True
    return variables(template), False


def conversation(template, context):
    _, legacy = conversation_variables(template)
    if not legacy:
        return render(template, context)
    # Preserve old {history}/{anthology} prompts exactly until the user edits them.
    try:
        return template.format(**context)
    except (KeyError, ValueError, IndexError, AttributeError) as exc:
        raise ValueError(f"Invalid conversation template: {exc}") from exc


def validate_choice(template):
    refs = variables(template)
    render(
        template,
        dict(
            behaviors=[dict(id="example", name="Example", spec="Criterion")],
            candidates=[
                dict(node="example", parent="Seed", continuation="Continuation")
            ],
            assessments=[dict(candidate="example", results=[])],
            spec="Criteria",
        ),
    )
    if refs and "candidates" not in refs:
        raise ValueError("Choice template must include {{candidates}}")
    if refs and not ({"behaviors", "behaviors.spec", "spec"} & refs):
        raise ValueError("Choice template must include {{behaviors}} or {{spec}}")
