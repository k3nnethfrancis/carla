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
    assert (
        lab.project.data["policy_spec"]
        == "Voice (expected present): Keep a consistent voice"
    )
    await lab.execute("policy.configure", {"selection_enabled": False}, "edit")
    assert saved["config"]["selection_enabled"] is False
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="selection", id=initial["selection"][0]["id"]),
        "restore",
    )
    assert (
        lab.project.data["policy_spec"]
        == "Selection criteria (expected present): "
        + initial["selection"][0]["config"]["policy_spec"]
    )
    await lab.execute(
        "operational.policy.delete",
        dict(purpose="selection", id=initial["selection"][0]["id"]),
        "delete",
    )
    assert lab.project.data["selection_enabled"] is False
    assert lab.project.data["active_operational_policies"]["selection"] == saved["id"]


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
    assert lab.project.data["policy_spec"] == "One (expected present): First"
    with pytest.raises(ValueError, match="boolean"):
        evals.save(
            lab.project,
            dict(name="Invalid", judges=[], actions={"train_on_pass": "true"}),
        )


@pytest.mark.asyncio
async def test_setup_and_reopen_preserve_existing_named_selection_model(
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
    assert active["config"]["model_alias"] == before["config"]["model_alias"]
    assert inactive == before
    revision = active["revision"]
    # Reopening with another default model must not rewrite a named judge.
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
    assert active["config"]["model_alias"] == before["config"]["model_alias"]
    assert active["revision"] == revision
    assert lab.project.data["operational_policies"]["selection"][-1] == before
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="selection", id=inactive["id"]),
        "old",
    )
    assert lab.selection_model()["alias"] == before["config"]["model_alias"]


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


@pytest.mark.asyncio
@pytest.mark.parametrize("purpose", ["monitoring", "selection"])
async def test_on_selects_runtime_exclusively_new_policy_off_and_all_off(lab, purpose):
    items = lab.project.data["operational_policies"][purpose]
    first = items[0]
    on = (
        {"monitor_mode": "diffusion"}
        if purpose == "monitoring"
        else {"selection_enabled": True}
    )
    off = (
        {"monitor_mode": "off"}
        if purpose == "monitoring"
        else {"selection_enabled": False}
    )

    async def save(item, config, name=None):
        await lab.execute(
            "operational.policy.save",
            dict(
                purpose=purpose, id=item["id"], name=name or item["name"], config=config
            ),
            "save",
        )

    await save(first, on)
    assert ops.enabled(
        purpose,
        ops.current(lab.project, purpose, lab.runtime.model["alias"], lab.policy_model),
    )
    await lab.execute(
        "operational.policy.save",
        dict(purpose=purpose, name="Second", config=on),
        "new",
    )
    second = items[-1]
    assert not ops.enabled(purpose, second["config"])
    assert ops.enabled(purpose, first["config"])
    await save(second, on)
    assert lab.project.data["active_operational_policies"][purpose] == second["id"]
    assert not ops.enabled(purpose, first["config"])
    assert ops.enabled(purpose, second["config"])
    if purpose == "monitoring":
        assert first["config"]["monitor_provider"] == "diffusion"
    await save(second, {}, "Renamed")
    assert ops.enabled(purpose, second["config"])
    await save(second, off)
    assert not any(ops.enabled(purpose, p["config"]) for p in items)
    assert not ops.enabled(
        purpose,
        ops.current(lab.project, purpose, lab.runtime.model["alias"], lab.policy_model),
    )
    await save(first, {}, "Still off")
    assert lab.project.data["active_operational_policies"][purpose] == second["id"]
    await lab.execute(
        "operational.policy.delete", dict(purpose=purpose, id=second["id"]), "delete"
    )
    assert lab.project.data["active_operational_policies"][purpose] == first["id"]
    await lab.execute(
        "operational.policy.delete",
        dict(purpose=purpose, id=first["id"]),
        "delete-last",
    )
    assert not items and not ops.enabled(
        purpose,
        ops.current(lab.project, purpose, lab.runtime.model["alias"], lab.policy_model),
    )


@pytest.mark.asyncio
async def test_failed_monitor_enable_keeps_previous_on(lab, monkeypatch):
    first = lab.project.data["operational_policies"]["monitoring"][0]
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="monitoring",
            id=first["id"],
            name=first["name"],
            config={"monitor_mode": "diffusion"},
        ),
        "local",
    )
    await lab.execute(
        "operational.policy.save",
        dict(purpose="monitoring", name="Remote", config={"monitor_mode": "jev"}),
        "new",
    )
    second = lab.project.data["operational_policies"]["monitoring"][-1]
    monkeypatch.setattr(ops.credentials, "openrouter_key", lambda: (None, "missing"))
    before = copy.deepcopy(lab.project.data)
    with pytest.raises(ValueError, match="API key"):
        await lab.execute(
            "operational.policy.save",
            dict(
                purpose="monitoring",
                id=second["id"],
                name=second["name"],
                config={"monitor_mode": "jev"},
            ),
            "enable",
        )
    assert lab.project.data == before


@pytest.mark.asyncio
async def test_migrate_nonselected_on_off_and_legacy_sync_single_on(lab):
    first = lab.project.data["operational_policies"]["monitoring"][0]
    second = copy.deepcopy(first)
    second.update(id="old-inactive", name="Old inactive")
    second["config"].update(monitor_mode="diffusion", monitor_provider="diffusion")
    lab.project.data["operational_policies"]["monitoring"].append(second)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert second["config"]["monitor_mode"] == "off"
    assert second["config"]["monitor_provider"] == "diffusion"
    before = copy.deepcopy(lab.project.data)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert lab.project.data == before
    second["config"]["monitor_mode"] = "diffusion"
    await lab.execute("simulator.configure", {"monitor_mode": "diffusion"}, "legacy")
    assert first["config"]["monitor_mode"] == "diffusion"
    assert second["config"]["monitor_mode"] == "off"


@pytest.mark.asyncio
@pytest.mark.parametrize("purpose", ["monitoring", "selection"])
async def test_delete_last_create_first_then_legacy_toggle_has_named_owner(
    lab, purpose
):
    first = lab.project.data["operational_policies"][purpose][0]
    await lab.execute(
        "operational.policy.delete", dict(purpose=purpose, id=first["id"]), "delete"
    )
    await lab.execute(
        "operational.policy.save", dict(purpose=purpose, name="Replacement"), "create"
    )
    replacement = lab.project.data["operational_policies"][purpose][0]
    assert lab.project.data["active_operational_policies"][purpose] == replacement["id"]
    command, args = (
        ("simulator.configure", {"monitor_mode": "diffusion"})
        if purpose == "monitoring"
        else ("policy.configure", {"selection_enabled": True})
    )
    await lab.execute(command, args, "legacy-enable")
    assert ops.enabled(purpose, replacement["config"])
    assert ops.enabled(
        purpose,
        ops.current(lab.project, purpose, lab.runtime.model["alias"], lab.policy_model),
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("preserve", [False, True])
async def test_key_completion_switches_on_only_when_requested(
    lab, monkeypatch, preserve
):
    first = lab.project.data["operational_policies"]["monitoring"][0]
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="monitoring",
            id=first["id"],
            name=first["name"],
            config={"monitor_mode": "diffusion"},
        ),
        "local",
    )
    await lab.execute(
        "operational.policy.save", dict(purpose="monitoring", name="Remote"), "new"
    )
    remote = lab.project.data["operational_policies"]["monitoring"][-1]
    saved_keys = []
    monkeypatch.setattr(ops.credentials, "save_openrouter_key", saved_keys.append)
    monkeypatch.setattr(
        ops.credentials,
        "openrouter_key",
        lambda: ("test-key", "saved") if saved_keys else (None, "missing"),
    )
    await lab.execute(
        "loom-policy.key",
        dict(
            key="test-key",
            activate=False,
            policy_id=remote["id"],
            preserve_disabled=preserve,
        ),
        "key",
    )
    assert ops.enabled("monitoring", remote["config"]) is (not preserve)
    assert ops.enabled("monitoring", first["config"]) is preserve
    assert remote["config"]["monitor_provider"] == "jev"
    assert lab.project.data["simulator_config"]["monitor_mode"] == (
        "diffusion" if preserve else "jev"
    )


@pytest.mark.asyncio
async def test_missing_legacy_route_cannot_leave_unrepresented_monitor_on(lab):
    first = lab.project.data["operational_policies"]["monitoring"][0]
    first["config"]["monitor_mode"] = "diffusion"
    lab.project.data["simulator_config"] = {"monitor_mode": "diffusion"}
    lab.project.data["active_operational_policies"]["monitoring"] = "deleted"
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert first["config"]["monitor_mode"] == "off"
    assert lab.project.data["simulator_config"]["monitor_mode"] == "off"
    assert lab.project.data["active_operational_policies"]["monitoring"] == first["id"]


@pytest.mark.asyncio
async def test_default_policy_name_migration_preserves_identity_config_and_history(lab):
    catalog = lab.project.data["operational_policies"]
    assert all(items[0]["name"] == "Default policy" for items in catalog.values())
    original = catalog["monitoring"][0]
    original["name"] = "Default"
    prior = copy.deepcopy(original)
    lab.project.data["policy_runs"] = [
        {"id": "historic", "policy": copy.deepcopy(original)}
    ]
    history = copy.deepcopy(lab.project.data["policy_runs"])
    custom = catalog["selection"][0]
    custom["name"] = "My custom policy"
    custom_before = copy.deepcopy(custom)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert original == {
        **prior,
        "name": "Default policy",
        "revision": prior["revision"] + 1,
    }
    assert custom == custom_before
    assert lab.project.data["policy_runs"] == history
    frozen = copy.deepcopy(lab.project.data)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert lab.project.data == frozen


@pytest.mark.asyncio
async def test_default_policy_name_collision_keeps_both_saved_entries(lab):
    items = lab.project.data["operational_policies"]["monitoring"]
    original = items[0]
    legacy = copy.deepcopy(original)
    legacy.update(id="legacy", name="Default")
    items.append(legacy)
    before = copy.deepcopy(items)
    ops.migrate(lab.project, lab.runtime.model["alias"], lab.policy_model)
    assert items == before


@pytest.mark.asyncio
async def test_legacy_sync_recreates_default_policy_name_after_empty_catalog(lab):
    items = lab.project.data["operational_policies"]["selection"]
    await lab.execute(
        "operational.policy.delete",
        dict(purpose="selection", id=items[0]["id"]),
        "delete",
    )
    await lab.execute("policy.configure", {"selection_enabled": True}, "legacy")
    assert len(items) == 1 and items[0]["name"] == "Default policy"
    assert items[0]["config"]["selection_enabled"] is True


@pytest.mark.asyncio
async def test_selection_assessment_settings_are_independent_of_choice_template(lab):
    saved = lab.project.data["operational_policies"]["selection"][0]
    chooser = saved["config"]["policy_prompt"]
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            id=saved["id"],
            name=saved["name"],
            config={
                "selection_assessment_prompt": "Observe the behavior across the full input.",
                "selection_call_mode": "bundled",
                "selection_behaviors": [
                    dict(
                        id="harm",
                        name="Harm",
                        spec="Harmful speech",
                        expected="absent",
                        enabled=True,
                    )
                ],
            },
        ),
        "save",
    )
    assert saved["config"]["policy_prompt"] == chooser
    assert saved["config"]["selection_call_mode"] == "bundled"
    assert saved["config"]["selection_behaviors"][0]["expected"] == "absent"
    assert (
        lab.project.data["selection_assessment_prompt"]
        == saved["config"]["selection_assessment_prompt"]
    )
    before = copy.deepcopy(saved)
    with pytest.raises(ValueError, match="Separate or Bundled"):
        await lab.execute(
            "operational.policy.save",
            dict(
                purpose="selection",
                id=saved["id"],
                name=saved["name"],
                config={"selection_call_mode": "random"},
            ),
            "bad",
        )
    assert saved == before


@pytest.mark.asyncio
async def test_legacy_selection_projection_exposes_defaults_without_revision_change(
    lab,
):
    from character_lab.assessments import DEFAULT_PROMPT

    item = lab.project.data["operational_policies"]["selection"][0]
    item["config"].pop("selection_assessment_prompt")
    item["config"].pop("selection_call_mode")
    before = copy.deepcopy(item)
    for _ in range(2):
        config = ops.summaries(lab.project)["selection"][0]["config"]
        assert config["selection_assessment_prompt"] == DEFAULT_PROMPT
        assert config["selection_call_mode"] == "separate"
        assert item == before


@pytest.mark.asyncio
async def test_delete_custom_selection_resets_legacy_fallback_assessment_defaults(lab):
    from character_lab.assessments import DEFAULT_PROMPT

    items = lab.project.data["operational_policies"]["selection"]
    fallback = items[0]
    fallback["config"].pop("selection_assessment_prompt")
    fallback["config"].pop("selection_call_mode")
    await lab.execute(
        "operational.policy.save",
        dict(
            purpose="selection",
            name="Custom",
            config={
                "selection_assessment_prompt": "Custom observation instructions",
                "selection_call_mode": "bundled",
            },
        ),
        "create",
    )
    custom = items[-1]
    await lab.execute(
        "operational.policy.activate",
        dict(purpose="selection", id=custom["id"]),
        "activate",
    )
    assert lab.project.data["selection_call_mode"] == "bundled"
    await lab.execute(
        "operational.policy.delete",
        dict(purpose="selection", id=custom["id"]),
        "delete",
    )
    assert (
        lab.project.data["active_operational_policies"]["selection"] == fallback["id"]
    )
    assert lab.project.data["selection_assessment_prompt"] == DEFAULT_PROMPT
    assert lab.project.data["selection_call_mode"] == "separate"
    assert lab.project.data["selection_enabled"] is False
