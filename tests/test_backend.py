"""The authenticated frontend owns local resources; probes do not."""

import asyncio
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

from character_lab import backend


async def test_unauthenticated_connection_cannot_stop_active_judge(
    monkeypatch, tmp_path, capsys
):
    closed = AsyncMock()
    monkeypatch.setattr(backend.managed, "close", closed)
    monkeypatch.setattr(backend, "load_models", lambda _: [])
    monkeypatch.setattr(backend, "Workspaces", lambda: object())

    class Session:
        sources = []

        def __init__(self, *args):
            pass

        async def snapshot(self):
            pass

        async def execute(self, *args):
            pass

        async def close(self):
            pass

    monkeypatch.setattr(backend, "Session", Session)
    args = SimpleNamespace(
        project=tmp_path,
        workspace=None,
        models="test",
        policy_model=None,
        setup_model=False,
    )
    task = asyncio.create_task(backend.serve(args))
    writer = probe = None
    try:
        # The server prints exactly one startup record before accepting clients.
        async with asyncio.timeout(2):
            output = ""
            while not output:
                await asyncio.sleep(0.01)
                output = capsys.readouterr().out
            discovery = json.loads(output)
            reader, writer = await asyncio.open_connection(
                "127.0.0.1", discovery["port"]
            )
            writer.write(
                (json.dumps({"v": 1, "token": discovery["token"]}) + "\n").encode()
            )
            await writer.drain()
            await reader.readline()  # authenticated library event
            probe_reader, probe = await asyncio.open_connection(
                "127.0.0.1", discovery["port"]
            )
            probe.write(b'{"v":1,"token":"incorrect"}\n')
            await probe.drain()
            assert await probe_reader.read() == b""
            closed.assert_not_awaited()
            writer.write(b'{"v":1,"id":1,"command":"quit","args":{}}\n')
            await writer.drain()
            await task
            closed.assert_awaited_once()
    finally:
        for connection in (writer, probe):
            if connection:
                connection.close()
                await connection.wait_closed()
        if not task.done():
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass
