"""Stable human names must never change frozen conversation evidence."""

import copy

import pytest
from test_service import session as session_fixture
from test_simulator import Runtime
from test_simulator import setup as simulator_setup

from character_lab import simulator, simulator_actions, simulator_names
from character_lab.domain import Project

setup = simulator_setup
session = session_fixture


@pytest.mark.asyncio
async def test_continue_fork_and_reload_keep_names(setup):
    project, config, emit, _ = setup
    config.update(action="loom", alternatives=2, turns=1)
    original = (await simulator.generate_alternatives(project, config, Runtime, emit))[
        0
    ]
    assert original["label"] == "loom-1"
    assert [c["label"] for c in original["conversations"]] == [
        "convo-1-loom-1",
        "convo-2-loom-1",
    ]
    original_names = [c["label"] for c in original["conversations"]]
    seed = simulator_actions.resolve_seed(project, dict(run=original["id"]))
    config.update(action="continue")
    await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    assert [c["label"] for c in original["conversations"]] == original_names
    seed = simulator_actions.resolve_seed(
        project, dict(run=original["id"], conversation=1)
    )
    fork = simulator.fork_sets(project, seed, {})[0]
    assert fork["label"] == "branch-1-convo-2-loom-1"
    assert fork["conversations"][0]["label"] == fork["label"]
    # Names are not reused when the prior branch is deleted.
    project.data["simulation_runs"].remove(fork)
    project.save()
    fork = simulator.fork_sets(project, seed, {})[0]
    assert fork["label"] == "branch-2-convo-2-loom-1"
    restored = Project(project.folder)
    assert restored.data["simulation_runs"][-1]["label"] == fork["label"]


@pytest.mark.asyncio
async def test_split_set_and_nested_sets_are_named_once(setup):
    project, config, emit, _ = setup
    config.update(action="loom", alternatives=2, turns=1)
    root = (await simulator.generate_alternatives(project, config, Runtime, emit))[0]
    seed = simulator_actions.resolve_seed(project, dict(run=root["id"]))
    split = await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    assert {r["operation_label"] for r in split} == {"loom-2-loom-1"}
    assert [r["label"] for r in split] == [
        "branch-1-loom-2-loom-1",
        "branch-2-loom-2-loom-1",
    ]
    assert split[0]["conversations"][1]["label"] == "convo-2-branch-1-loom-2-loom-1"
    scope = dict(
        kind="set",
        id=split[0]["alternative_group"],
        children=[r["alternative_scope"] for r in split],
    )
    seed = simulator_actions.resolve_seed(project, dict(scope=scope))
    nested = await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    assert len(nested) == 4
    assert {r["operation_label"] for r in nested} == {"loom-3-loom-2-loom-1"}
    labels = [c["label"] for r in nested for c in r["conversations"]]
    assert len(labels) == len(set(labels))
    before = copy.deepcopy(project.data)
    simulator_names.assign_labels(project.data)
    assert project.data == before
    # Parent propagation works regardless of serialized run order.
    project.data["simulation_runs"].reverse()
    simulator_names.rename(
        project.data, dict(run=root["id"], title="paths", update_children=True)
    )
    assert {r["operation_label"] for r in split} == {"loom-2-paths"}
    assert {r["operation_label"] for r in nested} == {"loom-3-loom-2-paths"}
    again = copy.deepcopy(project.data)
    simulator_names.assign_labels(project.data)
    assert project.data == again


@pytest.mark.asyncio
async def test_conversation_split_and_custom_titles(setup):
    project, config, emit, _ = setup
    config.update(action="loom", alternatives=2, turns=1)
    root = (await simulator.generate_alternatives(project, config, Runtime, emit))[0]
    simulator_names.rename(
        project.data,
        dict(run=root["id"], conversation=0, title="favourite", update_children=True),
    )
    seed = simulator_actions.resolve_seed(project, dict(run=root["id"], conversation=0))
    split = await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    assert {r["operation_label"] for r in split} == {"loom-2-favourite"}
    assert [r["conversations"][0]["label"] for r in split] == [
        "branch-1-loom-2-favourite",
        "branch-2-loom-2-favourite",
    ]
    before = copy.deepcopy(root["conversations"][0]["turns"])
    simulator_names.rename(
        project.data,
        dict(
            group=split[0]["alternative_group"],
            title="exploration",
            update_children=True,
        ),
    )
    assert split[0]["operation_title"] == "exploration"
    assert split[0]["conversations"][0]["label"] == "branch-1-exploration"
    assert root["conversations"][0]["turns"] == before
    simulator_names.rename(
        project.data,
        dict(run=root["id"], conversation=0, title="", update_children=True),
    )
    assert root["conversations"][0]["label"] == "convo-1-loom-1"


def test_missing_parent_legacy_migration_is_idempotent():
    data = {
        "simulation_runs": [
            dict(
                id="old",
                parent=dict(run="deleted", conversation=0),
                conversations=[dict(index=0, turns=[dict(text="exact text")])],
            )
        ]
    }
    simulator_names.assign_labels(data)
    before = copy.deepcopy(data)
    simulator_names.assign_labels(data)
    assert data == before
    assert data["simulation_runs"][0]["label"] == "loom-1"
    assert data["simulation_runs"][0]["conversations"][0]["turns"] == [
        dict(text="exact text")
    ]


@pytest.mark.asyncio
async def test_rename_command_emits_updated_names(session):
    from test_simulator_actions_service import configure

    await configure(session)
    await session.execute("simulator.run", {"action": "loom", "count": 2}, "run")
    await session.job
    run = session.project.data["simulation_runs"][0]
    await session.execute(
        "simulator.rename",
        {"run": run["id"], "title": "favourite", "update_children": True},
        "rename",
    )
    assert run["title"] == "favourite"
    assert run["conversations"][1]["label"] == "convo-2-favourite"
    events = [
        (kind, data) for kind, data, request in session.events if request == "rename"
    ]
    assert any(kind == "state" for kind, _ in events)
    views = [data for kind, data in events if kind == "simulation"]
    assert views[0]["title"] == "favourite"
    assert views[0]["conversations"][1]["label"] == "convo-2-favourite"
    assert not any(kind == "error" for kind, _ in events)


@pytest.mark.asyncio
async def test_subgroup_and_nested_scope_renames(setup):
    project, config, emit, _ = setup
    config.update(action="loom", alternatives=2, turns=1)
    root = (await simulator.generate_alternatives(project, config, Runtime, emit))[0]
    seed = simulator_actions.resolve_seed(project, dict(run=root["id"]))
    split = await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    simulator_names.rename(
        project.data,
        dict(
            group=split[0]["alternative_group"],
            alternative=0,
            title="chosen",
            update_children=True,
        ),
    )
    assert split[0]["alternative_scope"]["title"] == "chosen"
    assert split[0]["conversations"][0]["label"] == "convo-1-chosen"
    project.save()
    restored = Project(project.folder)
    assert restored.data["simulation_runs"][1]["alternative_scope"]["title"] == "chosen"
