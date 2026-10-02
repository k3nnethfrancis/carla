"""Independent local judge identities persist without loading model weights."""

import json

import pytest
from test_service import FakeRuntime

from character_lab import model_setup
from character_lab.models import available_judges, load_models
from character_lab.service import Session
from character_lab.workspaces import Workspaces


def test_register_multiple_judges_preserves_existing_and_deduplicates(local_data):
    first = local_data / "first.gguf"
    second = local_data / "second.gguf"
    first.touch()
    second.touch()
    a = model_setup.register(first, "First", "instruct", {})
    # Existing installs used a single object. Adding a judge preserves its identity.
    (local_data / "policy-model.json").write_text(json.dumps(a))
    b = model_setup.register(second, "Second", "instruct", {})
    assert a["port"] != b["port"]
    assert model_setup.register(first, "Renamed", "instruct", {}) == a
    assert available_judges() == [a, b]
    assert load_models(local_data / "policy-model.json", "instruct") == [a, b]


@pytest.mark.asyncio
async def test_named_selection_model_survives_restart_and_missing_alias_fails(
    local_data, tmp_path
):
    catalog = []
    for name in ("first", "second"):
        path = local_data / (name + ".gguf")
        path.touch()
        catalog.append(model_setup.register(path, name, "instruct", {}))

    async def emit(*args):
        pass

    def open_session():
        return Session(
            tmp_path / "workspace",
            emit,
            workspaces=Workspaces(tmp_path / "workspaces"),
            runtime_factory=FakeRuntime,
        )

    session = open_session()
    try:
        named = session.project.data["operational_policies"]["selection"][0]
        await session.execute(
            "operational.policy.save",
            dict(
                purpose="selection",
                id=named["id"],
                name=named["name"],
                config={"model_alias": catalog[1]["alias"]},
            ),
            "select",
        )
        assert session.selection_model() == catalog[1]
        assert session.state()["selector_models"] == catalog
        with pytest.raises(ValueError, match="unavailable"):
            session.judge_model("missing")
    finally:
        await session.close()
    reopened = open_session()
    try:
        assert reopened.selection_model() == catalog[1]
        assert reopened.judge_model(catalog[0]["alias"]) == catalog[0]
    finally:
        await reopened.close()


@pytest.mark.asyncio
async def test_adding_eval_model_preserves_configured_selection(local_data, tmp_path):
    first = local_data / "first.gguf"
    second = local_data / "second.gguf"
    first.write_bytes(b"GGUFtest")
    second.write_bytes(b"GGUFtest")
    a = model_setup.register(first, "First", "instruct", {})

    async def emit(*args):
        pass

    session = Session(
        tmp_path / "workspace",
        emit,
        workspaces=Workspaces(tmp_path / "workspaces"),
        runtime_factory=FakeRuntime,
    )
    try:
        from copy import deepcopy

        before = deepcopy(session.project.data["operational_policies"]["selection"])
        await session.execute(
            "setup.local", dict(kind="instruct", path=str(second)), "add"
        )
        await session.job
        assert len(session.judge_models) == 2
        assert session.selection_model() == a
        assert session.project.data["operational_policies"]["selection"] == before
    finally:
        await session.close()
