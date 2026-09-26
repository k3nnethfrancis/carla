"""Model setup commands. Network work emits data; the Go client owns each screen."""

import asyncio
import json
import sys
from pathlib import Path

from . import model_setup as setup
from .models import available_models


async def dispatch(session, command, args, request_id):
    async def emit(data):
        await session.emit("setup", data, request_id)

    if command == "setup.open":
        await emit(dict(stage="home", presets=[p[0] for p in setup.PRESETS]))
        return
    if command not in {"setup.plan", "setup.local", "setup.download"}:
        raise ValueError("Unknown setup command")
    kind = args.get("kind", "base")
    if kind not in {"base", "instruct"}:
        raise ValueError("Choose Base or Instruct")
    plan = None
    if command == "setup.download":
        index = args.get("index")
        if type(index) is not int or not 0 <= index < len(session.setup_plans):
            raise ValueError("Select a model before downloading")
        plan = session.setup_plans[index]

    async def work():
        process = None
        try:
            if command == "setup.plan":
                if "preset" in args:
                    index = args["preset"]
                    if type(index) is not int or not 0 <= index < len(setup.PRESETS):
                        raise ValueError("Unknown preset")
                    _, repo, revision, filename = setup.PRESETS[index]
                else:
                    repo, revision, filename = setup.parse_hub_source(args["source"])
                session.setup_plans = await asyncio.to_thread(
                    setup.plans, repo, revision, filename
                )
                await emit(dict(stage="plans", plans=session.setup_plans))
                return
            if command == "setup.local":
                path = setup.local_gguf(args["path"])
                source = {"origin": "local"}
            else:
                # A child owns all download threads; terminating it leaves Hub's
                # partial cache reusable and prevents a cancelled job registering.
                process = await asyncio.create_subprocess_exec(
                    sys.executable,
                    "-m",
                    "character_lab.model_setup",
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.DEVNULL,
                )
                process.stdin.write((json.dumps(plan) + "\n").encode())
                await process.stdin.drain()
                process.stdin.close()
                path = None
                failure = "Download failed"
                while line := await process.stdout.readline():
                    data = json.loads(line)
                    if data["stage"] == "downloaded":
                        path = Path(data["path"])
                    elif data["stage"] == "error":
                        failure = data["message"]
                    else:
                        await emit(data)
                if await process.wait() or path is None:
                    raise ValueError(failure)
                source = plan
            model = setup.register(path, path.stem, kind, source)
            if kind == "base":
                session.project.data["models"] = available_models(
                    session.project.data["models"]
                )
                session.project.data["model_alias"] = model["alias"]
                session.runtime.close()
                session.runtime = session.runtime_factory(session.project.folder, model)
                session.project.save()
            else:
                session.policy_model = model
            await emit(dict(stage="complete", name=model["name"]))
        except asyncio.CancelledError:
            await emit(dict(stage="cancelled"))
            raise
        except Exception as exc:
            await emit(dict(stage="error", message=str(exc)))
        finally:
            if process and process.returncode is None:
                process.kill()
                await process.wait()
            session.job = None
            await session.snapshot()

    session.job_id = request_id
    session.job = asyncio.create_task(work())
