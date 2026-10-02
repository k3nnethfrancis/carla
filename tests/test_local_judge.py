"""Process ownership and automatic transport tests, without model downloads."""

import asyncio
from unittest.mock import AsyncMock

import httpx
import pytest

from character_lab import local_judge, monitor


async def test_auto_classify_uses_managed_endpoint_and_records_it(monkeypatch):
    ensure = AsyncMock(return_value="http://127.0.0.1:43219")
    monkeypatch.setattr(monitor.managed, "ensure", ensure)
    real = httpx.AsyncClient

    def handle(request):
        assert str(request.url) == "http://127.0.0.1:43219/v1/systemone"
        assert "authorization" not in request.headers
        return httpx.Response(200, json={"answers": {"ok": {"noul": 0.9}}})

    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(transport=httpx.MockTransport(handle), **kw),
    )
    record = {}
    await monitor.classify({"questions": {"ok": {}}}, record, endpoint="auto")
    assert record["status"] == "complete"
    assert record["endpoint"] == "http://127.0.0.1:43219/v1/systemone"
    ensure.assert_awaited_once()


async def test_missing_install_is_unavailable_never_remote(monkeypatch):
    monkeypatch.setattr(
        monitor.managed,
        "ensure",
        AsyncMock(side_effect=local_judge.LocalJudgeError("Install local judge first")),
    )
    monkeypatch.setattr(
        httpx, "AsyncClient", lambda **kw: pytest.fail("No HTTP request allowed")
    )
    record = {}
    await monitor.classify({"questions": {"ok": {}}}, record, endpoint="auto")
    assert record["status"] == "unavailable"
    assert record["error"] == "Install local judge first"


async def test_parallel_start_reuses_one_process_and_shutdown_owns_it(monkeypatch):
    class Process:
        pid = 987654
        returncode = None
        stdout = asyncio.StreamReader()

        async def wait(self):
            self.returncode = 0

    process = Process()
    process.stdout.feed_data(b"CARLA_JUDGE_PORT=43219\n")
    spawn = AsyncMock(return_value=process)
    monkeypatch.setattr(asyncio, "create_subprocess_exec", spawn)
    monkeypatch.setattr(local_judge.platform, "system", lambda: "Darwin")
    monkeypatch.setattr(local_judge.platform, "machine", lambda: "arm64")
    monkeypatch.setattr(local_judge.shutil, "which", lambda _: "/bin/uv")
    signals = []
    monkeypatch.setattr(
        local_judge.os, "killpg", lambda pid, sig: signals.append((pid, sig))
    )
    real = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(
            transport=httpx.MockTransport(
                lambda _: httpx.Response(200, json={"status": "ok"})
            ),
            **kw,
        ),
    )
    manager = local_judge.LocalJudge()
    assert (
        await asyncio.gather(manager.ensure(), manager.ensure())
        == ["http://127.0.0.1:43219"] * 2
    )
    spawn.assert_awaited_once()
    assert spawn.call_args.kwargs["env"]["UV_OFFLINE"] == "1"
    assert spawn.call_args.kwargs["start_new_session"] is True
    await manager.close()
    await manager.close()
    assert len(signals) == 1 and signals[0][0] == process.pid


async def test_start_failure_cleans_up_owned_process(monkeypatch):
    class Process:
        pid = 987655
        returncode = 1
        stdout = asyncio.StreamReader()

    process = Process()
    process.stdout.feed_eof()

    async def spawn(*args, **kwargs):
        kwargs["stderr"].write(b"Synthetic missing dependency\n")
        return process

    monkeypatch.setattr(asyncio, "create_subprocess_exec", spawn)
    monkeypatch.setattr(local_judge.platform, "system", lambda: "Darwin")
    monkeypatch.setattr(local_judge.platform, "machine", lambda: "arm64")
    monkeypatch.setattr(local_judge.shutil, "which", lambda _: "/bin/uv")
    manager = local_judge.LocalJudge()
    with pytest.raises(local_judge.LocalJudgeError, match="install") as error:
        await manager.ensure()
    diagnostic = local_judge.Path(str(error.value).split("Startup log: ", 1)[1])
    assert diagnostic.read_text() == "Synthetic missing dependency\n"
    assert diagnostic.stat().st_mode & 0o077 == 0
    diagnostic.unlink()
    assert manager.process is None and manager.log is None
