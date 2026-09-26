import asyncio
import sys

import pytest

from character_lab.service import Session
from character_lab.workspaces import Workspaces


@pytest.fixture
async def session(local_data):
    events = []

    async def emit(kind, data, request_id=None):
        events.append((kind, data))

    s = Session(local_data / "workspace", emit, workspaces=Workspaces(local_data))
    s.events = events
    yield s
    await s.close()


async def test_local_model_registration_selects_it_without_loading(session, local_data):
    model = local_data / "m.gguf"
    model.write_bytes(b"GGUFtest")
    await session.execute("setup.local", {"path": str(model), "kind": "base"}, "setup")
    await session.job
    assert session.runtime.model["path"] == str(model)
    assert session.runtime.process is None
    assert any(
        kind == "setup" and data["stage"] == "complete" for kind, data in session.events
    )


async def test_download_cancel_kills_child_without_registering(
    session, local_data, monkeypatch
):
    session.setup_plans = [
        {"repo": "owner/repo", "revision": "a" * 40, "files": ["m.gguf"]}
    ]
    original = asyncio.create_subprocess_exec
    processes = []

    async def fake(*args, **kwargs):
        p = await original(
            sys.executable,
            "-c",
            'import time; print(\'{"stage":"progress","downloaded":1,"total":10}\',flush=True); time.sleep(60)',
            **kwargs,
        )
        processes.append(p)
        return p

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake)
    await session.execute("setup.download", {"index": 0, "kind": "base"}, "download")

    async def started():
        while not any(
            k == "setup" and d["stage"] == "progress" for k, d in session.events
        ):
            await asyncio.sleep(0.001)

    await asyncio.wait_for(started(), 3)
    await session.execute("cancel", {}, "stop")
    assert processes[0].returncode is not None
    assert not (local_data / "models.json").exists()
    assert not session.busy
