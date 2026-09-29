"""Carla's application boundary: commands in, domain events out. No terminal code.

A session owns one workspace and at most one inference task. The transport awaits
emission, so a slow frontend applies backpressure rather than dropping tokens.
"""

import asyncio
import json
import math
import random
from contextlib import aclosing
from pathlib import Path

from . import (
    credentials,
    document_actions,
    evaluation,
    evaluation_sets,
    exports,
    simulator,
)
from .domain import Project, display_title, generation_status, library, now
from .exploration import explore, require_selector
from .model_metadata import native_context
from .models import available_models
from .policy import DEFAULT_PROMPT, DEFAULT_SPEC, default_model
from .runtime import Runtime
from .scheduling import parallel_map
from .stream_monitor import DocumentMonitor
from .workspaces import Workspaces, acquire

DEFAULT_SETTINGS = dict(count=3, n_predict=-1, temperature=1.0, top_p=0.98, rounds=1)


class Session:
    def __init__(
        self,
        folder,
        emit,
        models=None,
        policy_model=None,
        workspaces=None,
        runtime_factory=Runtime,
    ):
        self.emit = emit
        self.workspaces = workspaces or Workspaces()
        self.runtime_factory = runtime_factory
        self.policy_model = policy_model or default_model()
        self.sources = library()
        self.job = None
        self.job_id = None
        self.setup_plans = []
        self.lock = None
        self.view_node = None
        self.active_node = None
        self.open(folder, models)

    def open(self, folder, models=None):
        self.view_node = None
        self.active_node = None
        folder = Path(folder).expanduser().resolve()
        lock = acquire(folder)
        try:
            project = Project(folder)
            catalog = (
                models
                if models is not None
                else available_models(project.data.get("models", []))
            )
            if any(
                m.get("kind") != "base"
                or not m.get("url", "").startswith("http://127.0.0.1:")
                for m in catalog
            ):
                raise ValueError(
                    "Generator models must be identified local base models"
                )
            alias = project.data.get("model_alias", catalog[0]["alias"])
            model = next((m for m in catalog if m["alias"] == alias), catalog[0])
            project.data.pop("show_keys", None)
            project.data["models"] = catalog
            project.data["model_alias"] = model["alias"]
            project.data["selected"] = []
            evaluation_sets.migrate(project)
            project.save()
        except Exception:
            lock.close()
            raise
        if self.lock:
            self.runtime.close()
            self.lock.close()
        self.lock, self.project = lock, project
        self.runtime = self.runtime_factory(folder, model)
        self.workspaces.remember(folder)

    def read_bindings(self):
        path = self.workspaces.home / "keybindings.json"
        return json.loads(path.read_text()) if path.exists() else {}

    @property
    def busy(self):
        return self.job is not None and not self.job.done()

    def current(self):
        key = self.view_node or self.project.data["current"]
        if not key:
            raise ValueError("Select seed passages or open a saved branch first")
        return self.project.node(key)

    def state(self):
        p = self.project
        view = self.view_node if self.busy and self.view_node else p.data["current"]
        current = p.node(view) if view else None
        # Stream traces stay on disk; inspect explicitly rather than retransmitting
        # every provider event whenever the user moves to a different branch.
        node = {k: v for k, v in current.items() if k != "trace"} if current else None
        if node:
            node["origins"] = p.origins(node["id"])
            node["status"] = generation_status(current)
            node["title"] = display_title(node)
            node["change_offset"] = p.change_offset(node["id"])
            ancestor = current
            while not ancestor.get("trace", {}).get("model") and ancestor.get("parent"):
                ancestor = p.node(ancestor["parent"])
            node["model"] = ancestor.get("trace", {}).get("model", {}).get("name", "")
        return dict(
            workspace=dict(name=p.folder.name, path=str(p.folder)),
            workspaces=self.workspaces.list(p.folder),
            selected=p.selected,
            nodes=[
                {
                    k: n.get(k)
                    for k in (
                        "id",
                        "parent",
                        "kind",
                        "kept",
                        "status",
                        "title",
                        "label",
                        "created",
                        "document_id",
                        "revision_of",
                        "document_set",
                        "set_member",
                    )
                }
                | {
                    "status": generation_status(n),
                    "preview": n["text"][len(n.get("prompt", "")) :][:100],
                    "title": display_title(n),
                    "model": n.get("trace", {}).get("model", {}).get("name", ""),
                }
                for n in p.data["nodes"]
            ],
            document_sets=p.data.get("document_sets", []),
            document_set_heads=p.data.get("document_set_heads", {}),
            document_heads=p.data.get("document_heads", {}),
            current=node,
            models=p.data["models"],
            model_alias=self.runtime.model["alias"],
            settings={**DEFAULT_SETTINGS, **p.data.get("settings", {})},
            model_context=self.runtime.model["context"],
            native_context=native_context(self.runtime.model),
            evaluators=evaluation.definitions(p),
            evaluations=evaluation.summaries(p),
            evaluation_sets=evaluation_sets.summaries(p),
            active_evaluation=p.data.get("active_evaluation", ""),
            evaluation_prompt=evaluation.DEFAULT_PROMPT,
            policy_spec=p.data.get("policy_spec", DEFAULT_SPEC),
            policy_prompt=p.data.get("policy_prompt", DEFAULT_PROMPT),
            policy_model=self.policy_model["name"],
            annotations=p.data.get("annotations", []),
            policy_runs=[
                {k: r.get(k) for k in ("id", "status", "selected", "created")}
                for r in p.data.get("policy_runs", [])
            ],
            simulator_config=simulator.configuration(p, self.runtime.model["alias"]),
            monitor_key_source=credentials.openrouter_key()[1],
            simulation_runs=[
                simulator.summary(r) for r in p.data.get("simulation_runs", [])
            ],
            grow_settings={
                **DEFAULT_SETTINGS,
                **p.data.get("settings", {}),
                **p.data.get("grow_settings", {}),
            },
            selector_models=[self.policy_model],
            bindings=self.read_bindings(),
            busy=self.busy,
            active_node=self.active_node,
        )

    async def snapshot(self, request_id=None):
        await self.emit("state", self.state(), request_id)

    async def execute(self, command, args, request_id):
        """Short mutations are serialized by the socket reader; jobs run separately."""
        if self.busy and command not in {
            "cancel",
            "inspect",
            "quit",
            "bindings.save",
            "seed.clear",
            "node.open",
            "simulator.open",
            "simulator.inspect",
            "evaluation.open",
            "evaluation.item.open",
        }:
            raise ValueError(
                "Stop the active operation before changing the workspace or document"
            )
        if command.startswith("evaluation."):
            await evaluation.dispatch(self, command, args, request_id)
            return
        if command.startswith("setup."):
            from .setup_service import dispatch

            await dispatch(self, command, args, request_id)
            return
        if command == "library.import":
            from .library import import_document

            source = import_document(
                args.get("path", ""),
                args.get("title", ""),
                args.get("author", ""),
                args.get("url", ""),
            )
            self.sources = library()
            await self.emit("library", self.sources, request_id)
            await self.snapshot(request_id)
            await self.emit(
                "library.imported",
                {"key": source["key"], "title": source["title"]},
                request_id,
            )
            return
        p = self.project
        if "token_range" in args:
            raise ValueError("Token ranges were removed; use a single maximum")
        if "n_predict" in args and (
            type(args["n_predict"]) is not int
            or (args["n_predict"] != -1 and args["n_predict"] < 1)
        ):
            raise ValueError("Output tokens must be a positive integer or Max (-1)")
        if command.startswith("simulator.") or command.startswith("loom-policy."):
            from .simulator_commands import dispatch

            await dispatch(self, command, args, request_id)
            return
        eval_plan = (
            evaluation_sets.plan(self, args["eval"]) if args.get("eval") else None
        )
        if command == "grow.selector":
            if args["alias"] != self.policy_model["alias"]:
                raise ValueError("Selector model is not configured")
            await self.snapshot(request_id)
            return
        if command == "grow.configure":
            settings = {
                **DEFAULT_SETTINGS,
                **p.data.get("settings", {}),
                **p.data.get("grow_settings", {}),
                **args.get("settings", {}),
            }
            self.validate_settings(settings)
            p.data["grow_settings"] = settings
            p.save()
            await self.snapshot(request_id)
            return
        if command == "cancel":
            if self.busy:
                job = self.job
                job.cancel()
                try:
                    await job
                except asyncio.CancelledError:
                    evaluation.interrupt_pending(self.project)
                    # Cancellation can arrive before the task's first instruction.
                    self.job = None
                    await self.snapshot(request_id)
                    await self.emit("operation", {"stage": "stopped"}, self.job_id)
            return
        if command == "quit":
            if args.get("text") is not None:
                self.edit(args)
            await self.close()
            await self.emit("bye", {})
            return
        if command == "workspace.open":
            folder = (
                self.workspaces.named(args["name"])
                if "name" in args
                else Path(args["path"])
            )
            if args.get("create") and folder.exists():
                raise ValueError("Workspace already exists; open it instead")
            if folder.resolve() != p.folder:
                self.open(folder)
        elif command in {"seed.toggle", "seed.add", "seed.remove"}:
            refs = args.get("refs", [args.get("ref")])
            valid = {
                s["key"] + ":" + x["id"] for s in self.sources for x in s["passages"]
            }
            if not refs or any(ref not in valid for ref in refs):
                raise ValueError("Unknown source passage")
            for ref in refs:
                if (
                    command == "seed.toggle"
                    or (command == "seed.add" and ref not in p.selected)
                    or (command == "seed.remove" and ref in p.selected)
                ):
                    p.toggle(ref)
            if not p.source_root(self.sources):
                p.data["current"] = None
                p.save()
        elif command == "seed.open":
            if not p.selected:
                raise ValueError("Select source passages with Space first")
            node = next(
                (
                    n
                    for n in reversed(p.data["nodes"])
                    if n["kind"] == "source"
                    and set(n.get("passage_ids", [])) == set(p.selected)
                ),
                None,
            )
            if node is None:
                node = p.source_root(self.sources)
            p.data["current"] = node["id"]
            p.save()
        elif command == "seed.clear":
            p.data["selected"] = []
            p.save()
        elif command == "node.open":
            self.view_node = args["node"] if self.busy else None
            p.data["current"] = p.node(args["node"])["id"]
            p.save()
        elif command == "node.fork" and ("nodes" in args or "set" in args):
            offsets = args.get("offsets")
            if "offset" in args:
                if len(args.get("nodes", [])) != 1 or "offsets" in args:
                    raise ValueError("A cursor position requires one selected document")
                offsets = {args["nodes"][0]: args["offset"]}
            plan = document_actions.plan(
                p, "branch", args.get("nodes"), set_id=args.get("set"), offsets=offsets
            )
            document_actions.branch(p, plan)
        elif command == "node.fork":
            original = p.node(args["node"])
            parent = (
                p.edit(original["id"], args["text"]) if "text" in args else original
            )
            p.add(
                parent["text"],
                parent=parent["id"],
                kind="fork",
                origins=p.origins(parent["id"]),
            )
        elif command == "node.rename":
            title = args.get("title", "").strip()
            if not title:
                raise ValueError("Enter a document title")
            p.node(args["node"])["title"] = title
            p.save()
        elif command == "node.edit":
            self.edit(args)
        elif command == "note.update":
            note = args.get("note", "").strip()
            if not note:
                raise ValueError("Write a note first")
            annotation = next(
                (
                    a
                    for a in p.data.get("annotations", [])
                    if a["id"] == args["id"] and a["node"] == args["node"]
                ),
                None,
            )
            if annotation is None:
                raise ValueError("Note not found in this document")
            annotation.update(note=note, updated=now())
            p.save()
        elif command == "note.add":
            original = p.node(args["node"])
            text = args.get("text", original["text"])
            offset = args.get("offset", 0)
            note = args.get("note", "").strip()
            if not note:
                raise ValueError("Write a note first")
            if type(offset) is not int or not 0 <= offset <= len(text):
                raise ValueError("Note position lies outside the document")
            node = p.edit(original["id"], text)
            p.data["current"] = node["id"]
            p.annotate(node["id"], offset, offset, "unreviewed", note)
        elif command == "node.keep":
            ids = args.get("nodes")
            if ids is not None:
                if not ids:
                    raise ValueError("Select branches to keep")
                nodes = [p.node(key) for key in ids]
            else:
                nodes = [p.node(args["node"]) if args.get("node") else self.current()]
            for node in nodes:
                node["kept"] = args.get("kept", True)
            p.save()
        elif command == "node.delete":
            p.delete_nodes(args["nodes"], args["expected"])
        elif command == "annotate":
            if args.get("text") is not None:
                self.edit(args)
            node = self.current()
            p.annotate(
                node["id"],
                int(args.get("start", 0)),
                int(args.get("end", len(node["text"]))),
                args["verdict"],
                args["note"],
            )
        elif command == "export":
            path = exports.export(p, self.sources, args)
            await self.emit("result", {"path": str(path)}, request_id)
        elif command == "snapshot":
            await self.emit("result", {"path": str(p.snapshot())}, request_id)
        elif command == "configure":
            settings = {
                **DEFAULT_SETTINGS,
                **p.data.get("settings", {}),
                **args.get("settings", {}),
            }
            self.validate_settings(settings)
            alias = args.get("model_alias", self.runtime.model["alias"])
            model = next((m for m in p.data["models"] if m["alias"] == alias), None)
            if model is None:
                raise ValueError("Unknown base model")
            context = args.get("model_context", model["context"])
            if type(context) is not int or context < 0:
                raise ValueError(
                    "Context must be a nonnegative integer; 0 uses the model default"
                )
            if alias != self.runtime.model["alias"] or context != model["context"]:
                self.runtime.close()
                model["context"] = context
                self.runtime = self.runtime_factory(p.folder, model)
            p.data.update(settings=settings, model_alias=alias)
            for field in ("policy_spec", "policy_prompt"):
                if field in args:
                    if not args[field].strip():
                        raise ValueError("Policy instructions cannot be empty")
                    p.data[field] = args[field]
            p.save()
        elif command == "bindings.save":
            bindings = args["bindings"]
            if not isinstance(bindings, dict) or any(
                not isinstance(k, str) or not isinstance(v, str)
                for k, v in bindings.items()
            ):
                raise ValueError("Bindings must map actions to key names")
            path = self.workspaces.home / "keybindings.json"
            tmp = path.with_suffix(".tmp")
            tmp.write_text(json.dumps(bindings, indent=2) + "\n")
            tmp.replace(path)
        elif command == "inspect":
            if args.get("run"):
                record = next(
                    r for r in p.data["policy_runs"] if r["id"] == args["run"]
                )
            else:
                node = self.current()
                ancestor = node
                while not ancestor.get("trace") and ancestor.get("parent"):
                    ancestor = p.node(ancestor["parent"])
                record = dict(
                    node=node,
                    generation=ancestor,
                    origins=p.origins(node["id"]),
                    selection_runs=[
                        r
                        for r in p.data.get("policy_runs", [])
                        if any(
                            node["id"] in step.get("candidates", [])
                            for step in r.get("steps", [])
                        )
                    ],
                    annotations=[
                        a
                        for a in p.data.get("annotations", [])
                        if a["node"] == node["id"]
                    ],
                )
            await self.emit("inspection", record, request_id)
            return
        elif command in {"continue", "grow"} and "action" in args:
            from .document_generation import start

            await start(self, args, request_id, eval_plan)
        elif command in {"continue", "grow"}:
            loops = args.get(
                "loops",
                p.data.get("grow_settings", {}).get(
                    "rounds", p.data.get("settings", {}).get("rounds", 1)
                )
                if command == "grow"
                else 1,
            )
            if type(loops) is not int or loops < 1:
                raise ValueError("Loops must be a positive integer")
            if loops > 1:
                require_selector(self.policy_model)
            override = args.get("count")
            if override is not None and (type(override) is not int or override < 1):
                raise ValueError("Generation count must be a positive integer")
            if "refs" in args:
                valid = {
                    s["key"] + ":" + x["id"]
                    for s in self.sources
                    for x in s["passages"]
                }
                if not args["refs"] or any(ref not in valid for ref in args["refs"]):
                    raise ValueError("Unknown source passage")
                p.source_root(self.sources, args["refs"])
            elif args.get("node") and args.get("text") is None:
                p.data["current"] = p.node(args["node"])["id"]
                p.save()
            if args.get("text") is not None:
                self.edit(args)
            node = self.current()
            end = int(args.get("offset", len(node["text"])))
            if not 0 <= end <= len(node["text"]):
                raise ValueError("Branch position lies outside the document")
            settings = {**DEFAULT_SETTINGS, **p.data.get("settings", {})}
            if command == "grow":
                settings.update(p.data.get("grow_settings", {}))
            self.validate_settings(settings)
            count = (
                override
                if override is not None
                else (
                    settings["count"] if args.get("branch") or command == "grow" else 1
                )
            )
            settings["count"] = count
            if "n_predict" in args:
                settings["n_predict"] = args["n_predict"]
            self.validate_settings(settings)
            self.job_id = request_id
            self.job = asyncio.create_task(
                self.document_loom(
                    command,
                    node["id"],
                    node["text"][:end],
                    settings,
                    count,
                    loops,
                    eval_plan,
                )
            )
        else:
            raise ValueError("Unknown command: " + command)
        await self.snapshot(request_id)

    @staticmethod
    def validate_settings(s):
        for key in ("count", "rounds"):
            if type(s[key]) is not int or s[key] < 1:
                raise ValueError(f"{key} must be a positive integer")
        if type(s["n_predict"]) is not int or (
            s["n_predict"] != -1 and s["n_predict"] < 1
        ):
            raise ValueError(
                "Output tokens must be positive, or -1 for available context"
            )
        for key in ("temperature", "top_p"):
            if type(s[key]) not in (int, float) or not math.isfinite(s[key]):
                raise ValueError(f"{key} must be finite")
        if s["temperature"] < 0 or not 0 <= s["top_p"] <= 1:
            raise ValueError(
                "Temperature must be nonnegative; top-p must be between 0 and 1"
            )

    def edit(self, args):
        node = self.current()
        if args.get("node", node["id"]) != node["id"]:
            raise ValueError("Document changed before the edit was saved")
        self.project.edit(node["id"], args["text"])

    async def generate(
        self, command, parent, prefix, settings, count, *, nested=False, candidates=None
    ):
        p = self.project
        self.view_node = None
        job_id = self.job_id
        node = None
        branches = []
        status = "complete"
        empty_count = 0
        output_chars = 0
        message = ""
        try:
            await self.emit("operation", dict(stage="loading", command=command), job_id)
            await self.emit("loom.start", dict(count=count), job_id)
            branches = []
            # Reserve every member before awaiting inference. A cancelled queue
            # still has the complete selected set, with honest stopped members.
            if candidates is not None:
                for i, candidate in enumerate(candidates):
                    item = document_actions.create_revision(
                        p,
                        candidate,
                        prompt=candidate.target.prefix,
                        settings={**settings, "seed": random.randrange(2**31)},
                        trace={},
                        status="queued",
                        loom_index=i,
                    )
                    branches.append(item)
                await self.snapshot()

            async def schedule(data):
                await self.emit("capacity", data, job_id)

            self.runtime.on_schedule = schedule

            async def sample(i):
                nonlocal empty_count, output_chars
                branch_settings = {**settings, "seed": random.randrange(2**31)}
                candidate = candidates[i] if candidates is not None else None
                sample_prefix = candidate.target.prefix if candidate else prefix
                if candidate:
                    node = branches[i]
                else:
                    node = p.add(
                        sample_prefix,
                        parent=parent,
                        fork_offset=len(sample_prefix),
                        prompt=sample_prefix,
                        settings=branch_settings,
                        trace={},
                    )
                if candidate is None:
                    branches.append(node)
                node["loom_index"] = i
                node["status"] = "generating"
                self.active_node = node["id"]
                p.save()
                await self.emit(
                    "loom.branch",
                    dict(
                        index=i,
                        id=node["id"],
                        title=f"Branch {i + 1} · {node['id'][:6]}",
                        text="",
                        status="generating",
                    ),
                    job_id,
                )
                await self.snapshot()
                await self.emit(
                    "operation",
                    dict(stage="generating", index=i + 1, count=count),
                    job_id,
                )
                try:
                    config = simulator.configuration(p, self.runtime.model["alias"])

                    async def monitor_event(kind, data):
                        await self.emit(kind, data, job_id)

                    async with DocumentMonitor(
                        config, p, node, monitor_event
                    ) as watcher:
                        async with aclosing(
                            self.runtime.stream(
                                sample_prefix, node["settings"], node["trace"]
                            )
                        ) as stream:
                            while True:
                                try:
                                    chunk = await watcher.next_chunk(stream)
                                except StopAsyncIteration:
                                    break
                                node["text"] += chunk
                                watcher.turn["text"] += chunk
                                p.stream_delta(
                                    {"node": node["id"]}, chunk, node["trace"]
                                )
                                await self.emit(
                                    "token",
                                    dict(node=node["id"], text=chunk),
                                    job_id,
                                )
                                watcher.checkpoint()
                        await watcher.finish()
                        node["policy_stopped"] = watcher.stopped
                except asyncio.CancelledError:
                    node.update(status="stopped", finished=now())
                    p.save()
                    raise
                except Exception as exc:
                    node.update(status="failed", error=str(exc), finished=now())
                    p.save()
                    raise
                node["status"] = "complete"
                node["status"] = (
                    "policy_stopped"
                    if node.get("policy_stopped")
                    else generation_status(node)
                )
                empty_count += node["status"] == "empty"
                output_chars += len(node["text"]) - len(sample_prefix)
                node["finished"] = now()
                p.save()
                await self.emit(
                    "loom.branch",
                    dict(
                        index=i,
                        id=node["id"],
                        title=f"Branch {i + 1} · {node['id'][:6]}",
                        text=node["text"][len(node.get("prompt", prefix)) :],
                        status=node["status"],
                    ),
                    job_id,
                )

            await parallel_map(range(count), sample)
            node = branches[-1]
            if empty_count == count:
                events = node.get("trace", {}).get("events", [])
                eos = count == 1 and events and events[-1].get("stop_type") == "eos"
                message = (
                    "No new text — model ended immediately (EOS)"
                    if eos
                    else "No new text — all continuations were empty"
                )
            else:
                message = f"Added {output_chars} characters across {count - empty_count} continuation(s)"
                if empty_count:
                    message += f"; {empty_count} empty"
        except asyncio.CancelledError:
            status = "stopped"
        except Exception as exc:
            status = "failed"
            if node:
                node["error"] = str(exc)
            await self.emit("error", dict(message=str(exc)), job_id)
        finally:
            for branch in branches:
                if branch["status"] in {"generating", "queued"}:
                    branch.update(status=status, finished=now())
            if branches:
                p.save()
                for branch in branches:
                    i = branch["loom_index"]
                    await self.emit(
                        "loom.branch",
                        dict(
                            index=i,
                            id=branch["id"],
                            title=f"Branch {i + 1} · {branch['id'][:6]}",
                            text=branch["text"][len(branch.get("prompt", prefix)) :],
                            status=branch["status"],
                        ),
                        job_id,
                    )
            if command != "grow":
                await self.emit("loom.end", dict(status=status), job_id)
            self.runtime.on_schedule = None
            if node and node["status"] == "generating":
                node["status"] = status
                node["finished"] = now()
                p.save()
            if status != "complete":
                self.runtime.close()
            # Clear ownership before the final snapshot so controls become available.
            if not nested:
                self.job = None
            if self.view_node:
                p.data["current"] = self.view_node
                self.view_node = None
                p.save()
            await self.snapshot()
            await self.emit(
                "operation",
                dict(stage=status, command=command, message=message),
                job_id,
            )

        if nested and status == "stopped":
            raise asyncio.CancelledError
        if nested and status == "failed":
            raise ValueError("Loom batch failed; partial outputs retained")
        return branches

    async def document_loom(
        self, command, parent, prefix, settings, count, loops, eval_plan=None
    ):
        if loops == 1 and not eval_plan:
            return await self.generate("continue", parent, prefix, settings, count)
        before = {n["id"] for n in self.project.data["nodes"]}

        async def emit(kind, data):
            await self.emit(kind, data, self.job_id)

        async def batch(run_id, index):
            candidates = await self.generate(
                "continue", parent, prefix, settings, count, nested=True
            )
            # A selector cannot compete with an externally owned model server.
            if self.runtime.process is None:
                raise ValueError(
                    "Stop the externally managed generator before switching to selection"
                )
            self.runtime.close()
            for candidate in candidates:
                candidate.update(policy_run=run_id, loop=index + 1)
            self.project.save()
            return [c for c in candidates if c["status"] == "complete"]

        async def advance(key):
            nonlocal parent, prefix
            node = self.project.node(key)
            parent, prefix = key, node["text"]

        try:
            if loops == 1:
                await self.generate(
                    "continue", parent, prefix, settings, count, nested=True
                )
            else:
                await explore(
                    self.project,
                    loops,
                    self.policy_model,
                    self.runtime_factory,
                    batch,
                    advance,
                    emit,
                )
            if eval_plan and not asyncio.current_task().cancelling():
                targets = [
                    {"node": n["id"]}
                    for n in self.project.data["nodes"]
                    if n["id"] not in before and n.get("status") == "complete"
                ]
                await evaluation_sets.after_generation(self, eval_plan, targets)
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            await emit("error", dict(message=str(exc)))
        finally:
            self.job = None
            await self.snapshot()

    async def close(self):
        try:
            if self.busy:
                job = self.job
                job.cancel()
                try:
                    await job
                except asyncio.CancelledError:
                    evaluation.interrupt_pending(self.project)
                    self.job = None
        finally:
            self.runtime.close()
            if self.lock:
                self.lock.close()
                self.lock = None

    async def start_simulation(self, config, seed, request_id, eval_plan=None):
        """Take ownership of a validated run before returning to the command reader."""
        self.active_node = None
        self.runtime.close()
        self.job_id = request_id
        self.job = asyncio.create_task(self.simulate(config, seed, eval_plan))
        await self.snapshot(request_id)

    async def simulate(self, config, seed=None, eval_plan=None):
        p = self.project
        if config.get("action"):
            return await self.simulate_action(config, seed, eval_plan)
        before = {r["id"] for r in p.data.get("simulation_runs", [])}

        async def emit(kind, data):
            await self.emit(kind, data, self.job_id)

        try:
            loops = config.get("loops", 1)
            if loops == 1:
                await simulator.generate(
                    self.project, config, self.runtime_factory, emit, seed
                )
            else:
                current_seed = seed
                latest = None

                def transcript(turns):
                    return "".join(f"{t['role']}: {t['text']}\n\n" for t in turns)

                async def batch(run_id, index):
                    nonlocal latest
                    latest = await simulator.generate(
                        self.project, config, self.runtime_factory, emit, current_seed
                    )
                    latest.update(policy_run=run_id, loop=index + 1)
                    p.save()
                    if latest["status"] == "stopped":
                        raise asyncio.CancelledError
                    if latest["status"] != "complete":
                        raise ValueError(
                            latest.get("error", "Conversation batch failed")
                        )
                    prefix = (
                        transcript(current_seed["turns"])
                        if current_seed and "turns" in current_seed
                        else ""
                    )
                    return [
                        dict(
                            id=str(c["index"]),
                            prompt=(
                                transcript(
                                    current_seed["conversations"][c["index"]]["turns"]
                                )
                                if current_seed and "conversations" in current_seed
                                else prefix
                            ),
                            text=transcript(c["turns"]),
                        )
                        for c in latest["conversations"]
                        if c["status"] == "complete"
                    ]

                async def advance(key):
                    nonlocal current_seed
                    current_seed = simulator.conversation_seed(
                        p, latest["id"], int(key)
                    )

                await explore(
                    p,
                    loops,
                    self.policy_model,
                    self.runtime_factory,
                    batch,
                    advance,
                    emit,
                )
            if eval_plan and not asyncio.current_task().cancelling():
                targets = [
                    {"run": r["id"], "conversation": c["index"]}
                    for r in p.data.get("simulation_runs", [])
                    if r["id"] not in before
                    for c in r["conversations"]
                    if c["status"] == "complete"
                ]
                await evaluation_sets.after_generation(self, eval_plan, targets)
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            await emit("error", dict(message=str(exc)))
        finally:
            self.job = None
            if self.view_node:
                p.data["current"] = self.view_node
                self.view_node = None
                p.save()
            await self.snapshot()

    async def simulate_action(self, config, seed=None, eval_plan=None):
        """Run one action over a preserved item/set shape, then judge exact outputs."""
        from . import simulator_actions

        p = self.project
        latest = []
        generated_targets = []
        current_seed = seed

        async def emit(kind, data):
            await self.emit(kind, data, self.job_id)

        async def batch(policy_id=None, index=0):
            nonlocal latest
            latest = await simulator.generate_alternatives(
                p, config, self.runtime_factory, emit, current_seed
            )
            generated_targets.extend(simulator_actions.affected_targets(latest))
            if policy_id:
                for run in latest:
                    run.update(policy_run=policy_id, loop=index + 1)
                p.save()
            if any(r["status"] == "stopped" for r in latest):
                raise asyncio.CancelledError
            if any(r["status"] != "complete" for r in latest):
                raise ValueError(
                    "Conversation generation incomplete; partial outputs retained"
                )
            return simulator_actions.group_candidates(latest)

        async def advance(key):
            nonlocal current_seed
            current_seed = simulator_actions.selected_seed(p, latest, key)

        try:
            if config.get("loops", 1) == 1:
                await batch()
            else:
                await explore(
                    p,
                    config["loops"],
                    self.policy_model,
                    self.runtime_factory,
                    batch,
                    advance,
                    emit,
                )
            if eval_plan and not asyncio.current_task().cancelling():
                await evaluation_sets.after_generation(
                    self, eval_plan, generated_targets
                )
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            await emit("error", dict(message=str(exc)))
        finally:
            self.job = None
            if self.view_node:
                p.data["current"] = self.view_node
                self.view_node = None
                p.save()
            await self.snapshot()
