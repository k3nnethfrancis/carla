"""Policy activation and reusable specs change future runs, never prior evidence."""

import copy

import pytest
from test_evaluation import lab as lab_fixture
from test_evaluation_judges import config

from character_lab import evaluation_policies as evals
from character_lab import evaluation_sets
from character_lab import operational_policies as ops

lab = lab_fixture


@pytest.mark.asyncio
async def test_named_selection_roundtrip_and_legacy_sync(lab):
    initial = copy.deepcopy(lab.project.data["operational_policies"])
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            name="Alternative",
            config={
                "selection_enabled": True,
                "selection_behaviors": [
                    dict(
                        id="voice",
                        name="Voice",
                        spec="Keep a consistent voice",
                        enabled=True,
                    )
                ],
            },
        ),
        "new",
    )
    saved = lab.project.data["operational_policies"]["selection"][-1]
    assert lab.project.data.get("selection_enabled", False) is False
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="selection", id=saved["id"]),
        "activate",
    )
    assert lab.project.data["selection_enabled"] is True
    assert lab.project.data["policy_spec"] == "Keep a consistent voice"
    await lab.execute("policy.configure", {"selection_enabled": False}, "edit")
    assert saved["config"]["selection_enabled"] is False
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="selection", id=initial["selection"][0]["id"]),
        "restore",
    )
    assert (
        lab.project.data["policy_spec"]
        == initial["selection"][0]["config"]["policy_spec"]
    )
    with pytest.raises(ValueError, match="another active"):
        await lab.execute(
            "operational.policy.delete",
            dict(purpose="selection", id=initial["selection"][0]["id"]),
            "delete",
        )


@pytest.mark.asyncio
async def test_monitor_actions_single_owner_gating_and_projection(lab, monkeypatch):
    monkeypatch.setattr(ops.credentials, "openrouter_key", lambda: (None, "missing"))
    await lab.execute(
        "operational.policy.save",
        dict(purpose="monitoring", name="Remote", config={"monitor_mode": "jev"}),
        "new",
    )
    saved = lab.project.data["operational_policies"]["monitoring"][-1]
    assert "action" not in saved["config"]["monitor_dimensions"][0]
    assert saved["actions"]
    with pytest.raises(ValueError, match="API key"):
        await lab.execute(
            "operational.policy.activate",
            dict(purpose="monitoring", id=saved["id"]),
            "activate",
        )
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="monitoring",
            id=saved["id"],
            name="Remote",
            config={"monitor_mode": "diffusion"},
        ),
        "local",
    )
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="monitoring", id=saved["id"]),
        "activate",
    )
    assert (
        lab.project.data["simulator_config"]["monitor_dimensions"][0]["action"]
        == saved["actions"][saved["config"]["monitor_dimensions"][0]["id"]]["action"]
    )
    assert lab.state()["simulator_config"]["monitor_policy"]["id"] == saved["id"]


@pytest.mark.asyncio
async def test_library_copy_does_not_fan_out_and_migration_idempotent(lab):
    before = copy.deepcopy(lab.project.data)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert lab.project.data == before
    await lab.execute("behavior.save", dict(name="New", spec="A spec"), "save")
    spec = lab.project.data["behavior_library"][-1]
    judge = config()
    judge["behaviors"][0].update(
        spec=spec["spec"], source_id=spec["id"], source_revision=spec["revision"]
    )
    policy = evals.save(lab.project, dict(name="Copy", judges=[judge]))
    frozen = copy.deepcopy(policy)
    await lab.execute(
        "behavior.save", dict(id=spec["id"], name="New", spec="Changed"), "edit"
    )
    await lab.execute("behavior.delete", {"id": spec["id"]}, "delete")
    assert policy == frozen


@pytest.mark.asyncio
@pytest.mark.parametrize("explicit", [None, False])
async def test_eval_policy_training_action(lab, explicit):
    policy = evals.save(
        lab.project,
        dict(name="Training", judges=[config()], actions={"train_on_pass": True}),
    )
    node = lab.project.add("Exact coherent text")
    args = {"policy": policy["id"], "targets": [{"node": node["id"]}]}
    if explicit is not None:
        args["train_on_pass"] = explicit
    await lab.execute("evaluation.collection.run", args, "run")
    await lab.job
    assert evaluation_sets.resolve(lab.project)["items"][0]["training"] is (
        explicit is None
    )


@pytest.mark.asyncio
async def test_generation_eval_uses_frozen_training_action(lab):
    policy = evals.save(
        lab.project,
        dict(name="Training", judges=[config()], actions={"train_on_pass": True}),
    )
    plan = evaluation_sets.plan(lab, policy["id"])
    evals.save(
        lab.project, {**copy.deepcopy(policy), "actions": {"train_on_pass": False}}
    )
    node = lab.project.add("Exact coherent text")
    await evaluation_sets.after_generation(lab, plan, [{"node": node["id"]}])
    assert evaluation_sets.resolve(lab.project)["items"][0]["training"] is True


@pytest.mark.asyncio
async def test_invalid_policy_edits_atomic_and_server_ids(lab):
    saved = lab.project.data["operational_policies"]["selection"][0]
    before = copy.deepcopy(saved)
    with pytest.raises(ValueError):
        await lab.execute(
            "operational.policy.save",
            dict(
                purpose="selection",
                id=saved["id"],
                name="Invalid",
                config={"selection_enabled": "yes"},
            ),
            "invalid",
        )
    assert saved == before
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            id=saved["id"],
            name="IDs",
            config={
                "selection_behaviors": [
                    dict(name="One", spec="First", enabled=True),
                    dict(name="Two", spec="Second", enabled=False),
                ]
            },
        ),
        "save",
    )
    assert all(b["id"] for b in saved["config"]["selection_behaviors"])
    assert lab.project.data["policy_spec"] == "First"
    with pytest.raises(ValueError, match="boolean"):
        evals.save(
            lab.project,
            dict(name="Invalid", judges=[], actions={"train_on_pass": "true"}),
        )


@pytest.mark.asyncio
async def test_setup_and_reopen_reconcile_only_active_selection_model(
    lab, monkeypatch, tmp_path
):
    from character_lab import setup_service

    await lab.execute(
        "operational.policy.save", dict(purpose="selection", name="Inactive"), "new"
    )
    inactive = lab.project.data["operational_policies"]["selection"][-1]
    before = copy.deepcopy(inactive)
    path = tmp_path / "new.gguf"
    path.write_bytes(b"GGUFtest")
    model = dict(alias="new-selector", name="New selector", path=str(path))
    monkeypatch.setattr(setup_service.setup, "register", lambda *args: model)
    await lab.execute("setup.local", dict(kind="instruct", path=str(path)), "setup")
    await lab.job
    active_id = lab.project.data["active_operational_policies"]["selection"]
    active = next(
        p
        for p in lab.project.data["operational_policies"]["selection"]
        if p["id"] == active_id
    )
    assert active["config"]["model_alias"] == "new-selector"
    assert inactive == before
    revision = active["revision"]
    # A different externally configured selector on startup updates only active.
    assert ops.model_key({"alias": "alias-only"}) == "alias-only"
    lab.policy_model = {
        "alias": "external-selector",
        "name": "External",
        "path": str(path),
    }
    await lab.close()
    lab.open(lab.project.folder, lab.project.data["models"])
    active = next(
        p
        for p in lab.project.data["operational_policies"]["selection"]
        if p["id"] == active_id
    )
    assert active["config"]["model_alias"] == "external-selector"
    assert active["revision"] == revision + 1
    assert lab.project.data["operational_policies"]["selection"][-1] == before
    with pytest.raises(ValueError, match="configured selection model"):
        await lab.execute(
            "operational.policy.activate",
            dict(purpose="selection", id=inactive["id"]),
            "old",
        )


@pytest.mark.asyncio
@pytest.mark.parametrize("simulator", [False, True])
async def test_selection_override_rejects_all_disabled_before_artifacts(lab, simulator):
    saved = lab.project.data["operational_policies"]["selection"][0]
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            id=saved["id"],
            name=saved["name"],
            config={
                "selection_enabled": False,
                "selection_behaviors": [
                    dict(id="off", name="Off", spec="Old criteria", enabled=False)
                ],
            },
        ),
        "off",
    )
    node = lab.project.add("Some seed")
    before = copy.deepcopy(lab.project.data)
    args = dict(action="loom", count=2, loops=1, selection=True)
    if simulator:
        command = "simulator.run"
        args["conversations"] = 2
    else:
        command = "continue"
        args["node"] = node["id"]
    with pytest.raises(ValueError, match="Enable at least one"):
        await lab.execute(command, args, "run")
    assert lab.project.data == before
    assert lab.job is None


@pytest.mark.asyncio
async def test_inactive_off_monitor_preserves_own_provider(lab):
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="monitoring",
            name="Local off",
            config={"monitor_mode": "off", "monitor_provider": "diffusion"},
        ),
        "new",
    )
    inactive = lab.project.data["operational_policies"]["monitoring"][-1]
    lab.project.data["simulator_config"] = {
        "monitor_mode": "jev",
        "monitor_provider": "jev",
    }
    await lab.execute(
        "operational.policy.save",
        dict(purpose="monitoring", id=inactive["id"], name="Local off"),
        "edit",
    )
    assert inactive["config"]["monitor_provider"] == "diffusion"
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="monitoring", id=inactive["id"]),
        "activate",
    )
    assert lab.project.data["simulator_config"]["monitor_provider"] == "diffusion"


@pytest.mark.asyncio
async def test_selection_run_freezes_full_policy_and_spec_provenance(lab):
    from test_service import FakeRuntime

    from character_lab.exploration import explore

    saved = lab.project.data["operational_policies"]["selection"][0]
    behavior = dict(
        id="voice",
        name="Voice",
        spec="Keep voice",
        enabled=True,
        source_id="library-voice",
        source_revision=3,
    )
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            id=saved["id"],
            name="Voice",
            config={"selection_enabled": True, "selection_behaviors": [behavior]},
        ),
        "save",
    )
    frozen = copy.deepcopy(saved)

    async def batch(*args):
        return [dict(id="a", prompt="", text="A coherent continuation")]

    async def advance(*args):
        pass

    await explore(
        lab.project, 1, lab.policy_model, FakeRuntime, batch, advance, lab.emit
    )
    run = lab.project.data["policy_runs"][-1]
    assert run["policy"] == frozen
    saved["config"]["selection_behaviors"][0]["source_revision"] = 4
    assert run["policy"]["config"]["selection_behaviors"][0]["source_revision"] == 3


@pytest.mark.asyncio
async def test_key_setup_keeps_disabled_named_monitor_off(lab, monkeypatch):
    from character_lab import credentials

    monkeypatch.setattr(credentials, "save_openrouter_key", lambda key: None)
    monkeypatch.setattr(
        credentials, "openrouter_key", lambda: ("synthetic-key", "saved")
    )
    saved = lab.project.data["operational_policies"]["monitoring"][0]
    await lab.execute(
        "loom-policy.key",
        dict(
            key="synthetic-key",
            activate=False,
            policy_id=saved["id"],
            preserve_disabled=True,
        ),
        "key",
    )
    assert saved["config"]["monitor_mode"] == "off"
    assert saved["config"]["monitor_provider"] == "jev"
    assert lab.project.data["simulator_config"]["monitor_mode"] == "off"


def test_library_migration_seeds_flat_policy_behaviors_once(lab):
    data = lab.project.data
    data.pop("behavior_library", None)
    data["evaluation_policies"] = [
        {"name": "Flat policy", "behaviors": [{"name": "Voice", "spec": "Keep voice"}]}
    ]
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert sum(b["spec"] == "Keep voice" for b in data["behavior_library"]) == 1
    library = copy.deepcopy(data["behavior_library"])
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert data["behavior_library"] == library
