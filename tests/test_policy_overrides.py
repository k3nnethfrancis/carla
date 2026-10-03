"""Per-operation switches are validated before work and never rewrite defaults."""

import copy

import pytest
from test_service import session as session_fixture
from test_simulator_actions_service import configure

from character_lab import credentials, monitor, policy_overrides

session = session_fixture


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
async def test_monitor_override_applies_then_reverts(session, monkeypatch, simulator):
    await configure(session)
    calls = []

    async def scan(config, conversation, turn):
        calls.append(config["monitor_mode"])
        turn["monitor"] = dict(status="complete", detections=[])

    monkeypatch.setattr(monitor, "scan", scan)
    monkeypatch.setattr(
        credentials, "openrouter_key", lambda: ("test-key", "environment")
    )
    p = session.project
    p.data["simulator_config"].update(monitor_mode="jev", monitor_during_reply=False)
    saved = copy.deepcopy(p.data["simulator_config"])
    node = p.data["nodes"][0]
    command = "simulator.run" if simulator else "continue"
    args = (
        dict(action="loom") if simulator else dict(action="continue", node=node["id"])
    )
    await session.execute(command, {**args, "monitoring": False}, "off")
    await session.job
    assert not calls
    record = (
        p.data["simulation_runs"][-1]["config"]
        if simulator
        else session.current()["policy_config"]
    )
    assert record["monitor_mode"] == "off"
    await session.execute(command, args, "default")
    await session.job
    assert calls and set(calls) == {"jev"}
    assert p.data["simulator_config"] == saved
    assert not [e for e in session.events if e[0] == "error"]


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
async def test_monitor_on_restores_provider_without_saving_override(
    session, monkeypatch, simulator
):
    await configure(session)
    calls = []

    async def scan(config, conversation, turn):
        calls.append(config["monitor_mode"])
        turn["monitor"] = dict(status="complete", detections=[])

    monkeypatch.setattr(monitor, "scan", scan)
    p = session.project
    p.data["simulator_config"].update(
        monitor_mode="off", monitor_provider="diffusion", monitor_during_reply=False
    )
    command = "simulator.run" if simulator else "continue"
    args = (
        dict(action="loom")
        if simulator
        else dict(action="continue", node=p.data["nodes"][0]["id"])
    )
    await session.execute(command, {**args, "monitoring": True}, "on")
    await session.job
    assert calls == ["diffusion"]
    assert p.data["simulator_config"]["monitor_mode"] == "off"
    await session.execute(command, args, "default")
    await session.job
    assert calls == ["diffusion"]


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
@pytest.mark.parametrize("selection", [True, False])
async def test_one_loop_selection_records_winner_or_bypasses(
    session, simulator, selection
):
    await configure(session)
    p = session.project
    p.data["selection_enabled"] = not selection
    command = "simulator.run" if simulator else "continue"
    args = dict(action="loom", count=2, loops=1, selection=selection)
    if not simulator:
        args["node"] = p.data["nodes"][0]["id"]
    await session.execute(command, args, "run")
    await session.job
    policies = p.data.get("policy_runs", [])
    assert len(policies) == int(selection)
    if selection:
        assert len(policies[0]["steps"]) == 1
        assert policies[0]["selected"] is not None
    assert p.data["selection_enabled"] is not selection
    assert not [e for e in session.events if e[0] == "error"]


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
async def test_invalid_overrides_fail_before_creating_outputs(session, simulator):
    await configure(session)
    p = session.project
    node = p.data["nodes"][0]
    command = "simulator.run" if simulator else "continue"
    target = {} if simulator else dict(node=node["id"])
    before = copy.deepcopy(p.data)
    for args, message in [
        (dict(action="loom", count=2, selection=True), "--loops"),
        (dict(action="loom", count=1, loops=1, selection=True), "at least 2"),
        (dict(action="loom", monitoring=True), "Choose a monitoring provider"),
    ]:
        with pytest.raises(ValueError, match=message):
            await session.execute(command, {**target, **args}, "invalid")
        assert p.data == before
        assert not session.busy


def test_provider_memory_and_off_skips_credentials(monkeypatch):
    previous = dict(monitor_mode="jev")
    config = dict(monitor_mode="off")
    policy_overrides.remember_provider(config, previous)
    assert config["monitor_provider"] == "jev"
    monkeypatch.setattr(credentials, "openrouter_key", lambda: ("", "none"))
    with pytest.raises(ValueError, match="API key"):
        policy_overrides.monitoring(config, dict(monitoring=True))
    assert (
        policy_overrides.monitoring(previous, dict(monitoring=False))["monitor_mode"]
        == "off"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
async def test_saved_selection_requires_explicit_loop_option(session, simulator):
    await configure(session)
    p = session.project
    p.data["selection_enabled"] = True
    command = "simulator.run" if simulator else "continue"
    args = dict(action="loom", count=2)
    if not simulator:
        args["node"] = p.data["nodes"][0]["id"]
    await session.execute(command, args, "ordinary")
    await session.job
    assert not p.data.get("policy_runs")
    await session.execute(command, {**args, "loops": 1}, "judged")
    await session.job
    assert len(p.data["policy_runs"]) == 1
    assert len(p.data["policy_runs"][0]["steps"]) == 1
    assert not [e for e in session.events if e[0] == "error"]


@pytest.mark.asyncio
async def test_disabling_saved_monitor_remembers_explicit_provider(
    session, monkeypatch
):
    await configure(session)
    monkeypatch.setattr(
        credentials, "openrouter_key", lambda: ("test-key", "environment")
    )
    await session.execute("simulator.configure", {"monitor_mode": "jev"}, "provider")
    await session.execute("simulator.configure", {"monitor_mode": "off"}, "disable")
    config = session.project.data["simulator_config"]
    assert config["monitor_mode"] == "off"
    assert config["monitor_provider"] == "jev"
    assert (
        policy_overrides.monitoring(config, {"monitoring": True})["monitor_mode"]
        == "jev"
    )


def test_explicit_selection_does_not_apply_to_continue():
    with pytest.raises(ValueError, match="at least 2"):
        policy_overrides.selection(
            dict(selection=True, loops=1), False, "continue", 1, {}
        )
