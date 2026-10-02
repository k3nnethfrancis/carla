"""Prompt placement, literal data and compatibility at the inference boundary."""

import copy
import json

import pytest

from character_lab import assessments, simulator, templates
from character_lab import operational_policies as ops
from character_lab.domain import Project


def test_object_projection_and_single_pass_literals():
    context = templates.assessment_context(
        [
            dict(
                id="a",
                name="Looping",
                spec="Literal {{history}}",
                action="stop",
                threshold=0.8,
            ),
            dict(id="b", name="Voice", spec="Distinct voice"),
        ],
        "Human text {{behaviors.name}} / 漢字",
    )
    assert json.loads(templates.render("{{behaviors.name}}", context)) == [
        "Looping",
        "Voice",
    ]
    objects = json.loads(templates.render("{{behaviors}}", context))
    assert objects[0] == dict(id="a", name="Looping", spec="Literal {{history}}")
    assert templates.render("{{ text }}", context) == context["text"]
    assert templates.render('JSON {"ok":true}\n{{behaviors.spec}}', context).endswith(
        '["Literal {{history}}", "Distinct voice"]'
    )


@pytest.mark.parametrize(
    "template",
    [
        "{{unknown}}",
        "{{behaviors.missing}}",
        "{{__class__}}",
        "{{text.upper()}}",
        "{{text",
        "{{}}",
    ],
)
def test_unsupported_expressions_and_unknown_fields_fail(template):
    with pytest.raises(ValueError):
        templates.render(
            template, templates.assessment_context([dict(name="B", spec="S")], "text")
        )


@pytest.mark.parametrize(
    "template", ["{{text}}", "{{behaviors}}", "{{behaviors.name}} {{text}}"]
)
def test_assessment_requires_input_and_specs(template):
    with pytest.raises(ValueError):
        templates.validate_assessment(template)


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["separate", "bundled"])
async def test_judge_template_controls_exact_placement_without_duplicate_envelope(mode):
    records = [
        dict(
            text="Exact {{history}} document",
            definition=dict(
                id="j:b",
                name="Coherence",
                spec="Coherent text",
                kind="llm",
                expected="present",
                prompt="Read {{text}}\nThen assess {{behaviors}}",
                call_mode=mode,
            ),
        )
    ]

    class Judge:
        async def judge(self, messages, trace):
            assert (
                messages[1]["content"]
                == 'Read Exact {{history}} document\nThen assess [{"id": "j:b", "name": "Coherence", "spec": "Coherent text"}]'
            )
            assert len(messages) == 2
            assert "observation only" in messages[0]["content"]
            assert trace["template_context"]["text"] == records[0]["text"]
            result = dict(passed=True, reason="Coherent", evidence="Exact")
            return dict(results={"j:b": result}) if mode == "bundled" else result

    assert (await assessments.assess_group(records, Judge()))[0]["passed"]
    assert records[0]["trace"]["template"] == records[0]["definition"]["prompt"]


def test_conversation_old_and_new_templates_produce_same_exact_prompt():
    context = dict(
        anthology="Seed {braces}",
        history="**User:** {{anthology}}",
        visitor_brief="Visitor",
    )
    old = "{anthology}\n\nFull conversation with Model C:\n\n{history}\n\n**Model C:**"
    assert templates.conversation(old, context) == templates.conversation(
        simulator.CHARACTER_TEMPLATE, context
    )
    assert "{{anthology}}" in templates.conversation(
        simulator.CHARACTER_TEMPLATE, context
    )


def test_monitor_detection_and_actions_migrate_without_changing_execution(tmp_path):
    p = Project(tmp_path / "workspace")
    alias = "base"
    config = {
        k: v for k, v in simulator.defaults(alias).items() if k.startswith("monitor_")
    }
    config["monitor_dimensions"][0].update(
        decision="threshold", threshold=0.93, action="stop", color="coral"
    )
    old = dict(id="m", name="Policy", revision=8, config=config)
    # Recreate old storage: actions already belonged to the policy, detection did not.
    old["actions"] = {
        b["id"]: {k: b.pop(k) for k in ("action", "color")}
        for b in config["monitor_dimensions"]
    }
    config["monitor_detection"] = {
        b["id"]: {k: b.pop(k) for k in ("decision", "threshold")}
        for b in config["monitor_dimensions"]
    }
    p.data["operational_policies"] = {"monitoring": [old], "selection": []}
    p.data["active_operational_policies"] = {"monitoring": "m", "selection": ""}
    before = ops.expanded(old)
    p.data["policy_runs"] = [dict(config=copy.deepcopy(before))]
    ops.migrate(p, alias, dict(alias="judge"))
    assert ops.expanded(old) == before
    assert "monitor_detection" not in old["config"]
    assert old["config"]["monitor_dimensions"][0]["decision"] == "threshold"
    assert old["config"]["monitor_dimensions"][0]["threshold"] == 0.93
    assert old["actions"]["looping"] == dict(action="stop", color="coral")
    assert p.data["policy_runs"][0]["config"] == before
    frozen = copy.deepcopy(p.data)
    ops.migrate(p, alias, dict(alias="judge"))
    assert p.data == frozen


def test_legacy_conversation_escaped_json_remains_literal():
    old = '{anthology}\n{{"label": "example"}}\nLiteral {{history}}; actual {history}'
    context = dict(anthology="Seed", history="User: Hi", visitor_brief="Visitor")
    assert templates.conversation(old, context) == old.format(**context)
    assert templates.conversation_variables(old)[0] == {"anthology", "history"}
