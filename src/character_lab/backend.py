"""Private localhost NDJSON transport. One launched TUI owns one service process.

Stdout carries one discovery record. All subsequent traffic uses a loopback TCP
socket and a per-launch secret; stderr is reserved for diagnostics. No web stack.
"""

import argparse
import asyncio
import json
import secrets
from pathlib import Path

from .local_judge import managed
from .models import load_models
from .service import Session
from .workspaces import Workspaces


async def write_event(writer, message):
    """A closed UI cancels work; it is not an inference failure.

    Translate only this event socket's errors. Model/provider connections retain
    their normal error handling and must still be reported as failures.
    """
    try:
        writer.write((json.dumps(message, ensure_ascii=False) + "\n").encode())
        await writer.drain()
    except ConnectionError:
        raise asyncio.CancelledError("Carla terminal disconnected") from None


async def serve(args):
    workspaces = Workspaces()
    folder = args.project
    if args.workspace:
        if folder:
            raise ValueError("Use --workspace separately from --project")
        folder = workspaces.named(args.workspace)
    if folder is None:
        last = workspaces.read().get("last")
        folder = Path(last) if last else workspaces.home / "default"
    folder = Path(folder).expanduser().resolve()
    models = load_models(args.models) if args.models else None
    policy_model = (
        load_models(args.policy_model, "instruct")[0] if args.policy_model else None
    )
    secret = secrets.token_urlsafe(32)
    done = asyncio.Event()
    connected = False

    async def handle(reader, writer):
        nonlocal connected
        session = None
        authenticated = False
        sequence = 0
        write_lock = asyncio.Lock()

        async def emit(kind, data, request_id=None):
            nonlocal sequence
            async with write_lock:
                sequence += 1
                message = dict(v=1, type=kind, seq=sequence, id=request_id, data=data)
                await write_event(writer, message)

        try:
            hello = json.loads(await asyncio.wait_for(reader.readline(), 10))
            if (
                connected
                or hello.get("v") != 1
                or not secrets.compare_digest(str(hello.get("token", "")), secret)
            ):
                return
            connected = authenticated = True
            session = Session(folder, emit, models, policy_model, workspaces)
            await emit("library", session.sources)
            await session.snapshot()
            if args.setup_model or (
                models is None
                and not any(
                    Path(m.get("path", "")).is_file()
                    for m in session.project.data["models"]
                )
            ):
                await session.execute("setup.open", {}, None)
            while line := await reader.readline():
                request_id = None
                try:
                    request = json.loads(line)
                    request_id = request["id"]
                    if request.get("v") != 1:
                        raise ValueError("Unsupported protocol version")
                    command = request["command"]
                    arguments = request.get("args", {})
                    if not isinstance(arguments, dict):
                        raise ValueError("Command args must be an object")
                    await session.execute(command, arguments, request_id)
                    if command == "quit":
                        break
                except (ValueError, KeyError, TypeError, StopIteration) as exc:
                    await emit("error", dict(message=str(exc)), request_id)
        except (ConnectionError, asyncio.IncompleteReadError):
            pass
        except Exception as exc:
            try:
                await emit("error", dict(message=str(exc)))
            except ConnectionError:
                pass
        finally:
            if session:
                try:
                    await session.close()
                except ConnectionError:
                    pass
            if authenticated:
                await managed.close()
            writer.close()
            if authenticated:
                done.set()

    server = await asyncio.start_server(handle, "127.0.0.1", 0, limit=16 * 1024 * 1024)
    port = server.sockets[0].getsockname()[1]
    print(json.dumps(dict(v=1, port=port, token=secret)), flush=True)
    async with server:
        # If the launcher dies before connecting, do not leave an orphan service.
        for _ in range(300):
            if connected:
                break
            await asyncio.sleep(0.1)
        if connected:
            await done.wait()


def main():
    parser = argparse.ArgumentParser(description="Carla local event service")
    parser.add_argument("--project", type=Path)
    parser.add_argument("--workspace")
    parser.add_argument("--setup-model", action="store_true")
    parser.add_argument("--models", type=Path)
    parser.add_argument("--policy-model", type=Path)
    args = parser.parse_args()
    try:
        asyncio.run(serve(args))
    except (ValueError, OSError) as exc:
        parser.exit(1, str(exc) + "\n")


if __name__ == "__main__":
    main()
