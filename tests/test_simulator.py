"""Exercise raw sampling and durable evidence without loading GPU weights."""

import asyncio
import copy
import json

import pytest

from character_lab import simulator
from character_lab.domain import Project
from character_lab.service import Session


class Runtime:
    instances = []
    output = " A new path."
    final = {}

    def __init__(self, folder, model):
        assert all(r.closed for r in self.instances)
        self.instances.append(self)
        self.process = object()
        self.closed = False

    async def preflight(self, prompt, settings):
        return {}

    async def stream(self, prompt, settings, trace):
        trace.update(request={"prompt": prompt, **settings}, events=[self.final])
        yield self.output

    def close(self):
        self.closed = True


@pytest.fixture
def setup(tmp_path):
    Runtime.instances = []
    Runtime.output = " A new path."
    Runtime.final = {}
    project = Project(tmp_path)
    project.data["models"] = [
        dict(alias="base", name="Base", kind="base"),
        dict(alias="other", name="Other", kind="base"),
    ]
    node = project.add("Paths without destinations.", kind="source")
    node["kept"] = True
    project.data["policy_spec"] = "SECRET CRITERIA"
    config = simulator.defaults("base")
    config.update(documents=[node["id"]], turns=2)
    events = []

    async def emit(kind, data):
        events.append((kind, copy.deepcopy(data)))

    return project, config, emit, events


@pytest.mark.asyncio
async def test_alternation_provenance_and_model_handoff(setup):
    project, config, emit, events = setup
    config["visitor_alias"] = "other"
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "complete"
    turns = run["conversations"][0]["turns"]
    assert [t["role"] for t in turns] == ["user", "character", "visitor", "character"]
    assert [t["model"]["alias"] for t in turns[1:]] == ["base", "other", "base"]
    assert len(Runtime.instances) == 3
    assert all(r.closed for r in Runtime.instances)
    assert "Paths without destinations." in turns[1]["prompt"]
    assert "Paths without destinations." not in turns[2]["prompt"]
    assert "**Model C:**  A new path." in turns[2]["prompt"]
    assert all("SECRET CRITERIA" not in t["prompt"] for t in turns[1:])
    assert turns[1]["trace"]["request"]["stop"] == ["\n\n**User:**"]
    assert turns[1]["text"] == " A new path."
    config["opening"] = "changed"
    project.data["nodes"][0]["text"] = "edited"
    assert run["config"]["opening"] != "changed"
    assert run["documents"][0]["text"] == "Paths without destinations."
    assert "trace" not in simulator.view(run)["conversations"][0]["turns"][1]
    assert [k for k, _ in events].count("simulation.token") == 3
    assert (
        json.loads(project.path.read_text())["simulation_runs"][0]["status"]
        == "complete"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "output,final,flag",
    [
        ("", {}, "empty"),
        ("raw **User:** surprise", {}, "unexpected_role_boundary"),
        ("partial", {"stopped_limit": True}, "token_limit"),
    ],
)
async def test_generation_observations_never_stop_conversation(
    setup, output, final, flag
):
    project, config, emit, _ = setup
    Runtime.output, Runtime.final = output, final
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "complete"
    turns = run["conversations"][0]["turns"]
    assert len(turns) == 4
    assert turns[1]["text"] == output
    assert flag in turns[1]["flags"]


@pytest.mark.asyncio
async def test_cancel_keeps_partial_output_and_prompt(setup):
    project, config, emit, _ = setup
    started = asyncio.Event()

    class Slow(Runtime):
        async def stream(self, prompt, settings, trace):
            yield "partial"
            started.set()
            await asyncio.sleep(60)

    task = asyncio.create_task(simulator.generate(project, config, Slow, emit))
    await started.wait()
    task.cancel()
    run = await task
    assert run["status"] == "stopped"
    turn = run["conversations"][0]["turns"][-1]
    assert turn["text"] == "partial" and turn["status"] == "stopped"
    assert turn["prompt"].startswith("Paths")
    assert Runtime.instances[-1].closed


def test_validation_and_recovery(setup):
    project, config, _, _ = setup
    simulator.validate(config, project, Session.validate_settings)
    for update in [
        dict(documents=[{}]),
        dict(turns=0),
        dict(character_template="{unknown}"),
        dict(visitor_alias="missing"),
        dict(documents=["missing"]),
    ]:
        with pytest.raises(ValueError):
            simulator.validate({**config, **update}, project, Session.validate_settings)
    project.data["simulation_runs"] = [
        dict(
            status="running",
            conversations=[
                dict(
                    status="running", turns=[dict(status="generating", text="partial")]
                )
            ],
        )
    ]
    project.save()
    restored = Project(project.folder).data["simulation_runs"][0]
    assert restored["status"] == "interrupted"
    assert restored["conversations"][0]["turns"][0]["status"] == "interrupted"


@pytest.mark.asyncio
async def test_generated_opening_context_provenance_and_handoff(setup):
    project, config, emit, _ = setup
    config.update(opening_mode="generated", visitor_alias="other", conversations=2)
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "complete"
    for conversation in run["conversations"]:
        opening, character, visitor, _ = conversation["turns"]
        assert opening["origin"] == "generated_opening"
        assert opening["model"]["alias"] == "other"
        assert opening["prompt"] == config["opening_prompt"]
        assert opening["trace"]["request"]["prompt"] == config["opening_prompt"]
        assert opening["settings"]["n_predict"] == 128
        assert "**User:** " + opening["text"] in character["prompt"]
        assert "Paths without destinations." not in opening["prompt"]
        assert opening["role"] == "user"
    assert all(r.closed for r in Runtime.instances)


@pytest.mark.asyncio
async def test_empty_opening_is_preserved_without_stopping(setup):
    project, config, emit, _ = setup
    config["opening_mode"] = "generated"
    Runtime.output = ""
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "complete"
    assert len(run["conversations"][0]["turns"]) == 4
    assert run["conversations"][0]["turns"][0]["flags"] == ["empty"]


@pytest.mark.asyncio
async def test_preview_only_generates_openings(setup):
    project, config, emit, _ = setup
    config.update(
        preview=True,
        opening_mode="generated",
        conversations=3,
        documents=[],
        opening_alias="other",
    )
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["preview"] and run["documents"] == []
    assert len(run["conversations"]) == 3
    assert all(len(c["turns"]) == 1 for c in run["conversations"])
    assert all(c["turns"][0]["model"]["alias"] == "other" for c in run["conversations"])
    assert simulator.summary(run)["models_label"].startswith("Opening preview")


def test_opening_validation(setup):
    project, config, _, _ = setup
    for update in [
        dict(opening_mode="random"),
        dict(opening_alias="missing"),
        dict(opening_prompt=""),
        dict(opening_settings={"temperature": -1}),
    ]:
        with pytest.raises(ValueError):
            simulator.validate({**config, **update}, project, Session.validate_settings)


@pytest.mark.asyncio
async def test_conversations_overlap_but_turns_and_model_loads_do_not(setup):
    project, config, emit, _ = setup
    config.update(conversations=6, turns=2, visitor_alias="other")
    active = peak = 0

    class Concurrent(Runtime):
        async def stream(self, prompt, settings, trace):
            nonlocal active, peak
            active += 1
            peak = max(peak, active)
            try:
                await asyncio.sleep(0.01)
                async for chunk in super().stream(prompt, settings, trace):
                    yield chunk
            finally:
                active -= 1

        def close(self):
            assert active == 0
            super().close()

    run = await simulator.generate(project, config, Concurrent, emit)
    assert run["status"] == "complete"
    assert peak == 4
    assert (
        len(Runtime.instances) == 3
    )  # character -> visitor -> character, not per conversation
    for c in run["conversations"]:
        assert [t["role"] for t in c["turns"]] == [
            "user",
            "character",
            "visitor",
            "character",
        ]
        assert c["turns"][1]["text"] in c["turns"][2]["prompt"]


@pytest.mark.asyncio
async def test_cancel_parallel_conversations_preserves_every_partial(setup):
    project, config, emit, _ = setup
    config.update(conversations=6, turns=2)
    started = asyncio.Event()
    count = 0

    class SlowBatch(Runtime):
        async def stream(self, prompt, settings, trace):
            nonlocal count
            count += 1
            yield "partial"
            if count == 4:
                started.set()
            await asyncio.sleep(60)

    task = asyncio.create_task(simulator.generate(project, config, SlowBatch, emit))
    await started.wait()
    task.cancel()
    run = await task
    assert run["status"] == "stopped"
    assert all(c["status"] == "stopped" for c in run["conversations"])
    for c in run["conversations"][:4]:
        assert c["turns"][-1]["text"] == "partial"
        assert c["turns"][-1]["status"] == "stopped"
    assert all(r.closed for r in Runtime.instances)


@pytest.mark.asyncio
async def test_flagged_conversation_does_not_block_other_conversations(setup):
    project, config, emit, _ = setup
    config.update(conversations=2, turns=2)
    calls = 0

    class OneEmpty(Runtime):
        async def stream(self, prompt, settings, trace):
            nonlocal calls
            calls += 1
            text = "" if calls == 1 else "A path."
            trace.update(request={"prompt": prompt}, events=[])
            yield text

    run = await simulator.generate(project, config, OneEmpty, emit)
    assert run["status"] == "complete"
    first, second = run["conversations"]
    assert first["status"] == "complete" and len(first["turns"]) == 4
    assert second["status"] == "complete" and len(second["turns"]) == 4


@pytest.mark.asyncio
async def test_high_monitor_scores_never_stop_next_turn(setup, monkeypatch):
    from character_lab import monitor

    project, config, emit, _ = setup
    config.update(monitor_mode="jev", turns=3, conversations=2)
    lengths = []

    async def high_score(config, conversation, turn):
        lengths.append(len(conversation["turns"]))
        turn["monitor"] = {"status": "complete", "scores": {"looping": 1.0}}

    monkeypatch.setattr(monitor, "scan", high_score)
    run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "complete"
    assert all(len(c["turns"]) == 6 for c in run["conversations"])
    assert sorted(lengths) == [2, 2, 4, 4, 6, 6]


@pytest.mark.asyncio
@pytest.mark.parametrize("action", ["warn", "stop"])
@pytest.mark.parametrize("provider", ["jev", "diffusion"])
async def test_policy_applies_only_to_matching_conversation(
    setup, monkeypatch, action, provider
):
    from character_lab import monitor

    project, config, emit, events = setup
    config.update(
        conversations=2,
        monitor_mode=provider,
        monitor_dimensions=monitor.dimensions({}),
    )
    config["monitor_dimensions"][0]["action"] = action

    async def scan(config, conversation, turn):
        scores = {"looping": 0.95 if conversation["index"] == 0 else 0.1}
        turn["monitor"] = dict(
            status="complete",
            scores=scores,
            detections=monitor.detections(config, scores),
        )

    monkeypatch.setattr(monitor, "scan", scan)
    run = await simulator.generate(project, config, Runtime, emit)
    a, b = run["conversations"]
    assert run["status"] == "complete"
    assert len(b["turns"]) == 4 and b["status"] == "complete"
    assert len(a["turns"]) == (2 if action == "stop" else 4)
    assert a["status"] == ("policy_stopped" if action == "stop" else "complete")
    hits = [data for kind, data in events if kind == "policy.detection"]
    assert len(hits) == (1 if action == "stop" else 2)
    assert all(h["conversation"] == 0 and h["run"] == run["id"] for h in hits)


@pytest.mark.asyncio
async def test_conversation_fork_edit_resume_preserves_ancestors(setup):
    project, config, emit, events = setup
    original = await simulator.generate(project, config, Runtime, emit)
    before = copy.deepcopy(original)
    seed = simulator.conversation_seed(project, original["id"], 0)
    fork = simulator.fork_conversation(
        project, seed, {"turn": 1, "text": "A human revision."}
    )
    assert len(fork["conversations"][0]["turns"]) == 2
    edited = fork["conversations"][0]["turns"][-1]
    assert (
        edited["origin"] == "human_edit"
        and "trace" not in edited
        and "monitor" not in edited
    )
    assert original == before
    seed = simulator.conversation_seed(project, fork["id"], 0)
    visitor = simulator.fork_conversation(
        project, seed, {"visitor": True, "text": "What follows?"}
    )
    seed = simulator.conversation_seed(project, visitor["id"], 0)
    project.data["nodes"][0]["text"] = "CHANGED SOURCE MUST NOT LEAK"
    config.update(turns=2, conversations=3, opening_mode="generated")
    resumed = await simulator.generate(project, config, Runtime, emit, seed)
    assert resumed["parent"] == {"run": visitor["id"], "conversation": 0}
    assert len(resumed["conversations"]) == 3
    for c in resumed["conversations"]:
        assert [t["role"] for t in c["turns"]] == [
            "user",
            "character",
            "visitor",
            "character",
            "visitor",
            "character",
        ]
        assert "A human revision." in c["turns"][3]["prompt"]
        assert "What follows?" in c["turns"][3]["prompt"]
        assert "Paths without destinations." in c["turns"][3]["prompt"]
        assert "CHANGED SOURCE MUST NOT LEAK" not in c["turns"][3]["prompt"]
    resumed["conversations"][0]["turns"][0]["text"] = "changed"
    assert resumed["conversations"][1]["turns"][0]["text"] != "changed"
    assert original == before


@pytest.mark.asyncio
async def test_resume_after_character_generates_visitor_first(setup):
    project, config, emit, events = setup
    config["turns"] = 1
    original = await simulator.generate(project, config, Runtime, emit)
    seed = simulator.conversation_seed(project, original["id"], 0)
    resumed = await simulator.generate(project, config, Runtime, emit, seed)
    assert [t["role"] for t in resumed["conversations"][0]["turns"]] == [
        "user",
        "character",
        "visitor",
        "character",
    ]
    turns = resumed["conversations"][0]["turns"]
    assert turns[:2] == seed["turns"]
    assert turns[2]["prompt"] == simulator.conversation_prompt(
        config, "visitor", seed["documents"], seed["turns"]
    )
    assert "**Model C:**  A new path." in turns[2]["prompt"]
    assert turns[2]["origin"] == "generated"
    assert simulator.summary(resumed)["parent"]["run"] == original["id"]


@pytest.mark.asyncio
@pytest.mark.parametrize("action", ["warn", "stop"])
async def test_midturn_monitor_is_nonblocking_bounded_and_preserves_prefix(
    setup, monkeypatch, action
):
    from character_lab import monitor

    project, config, emit, events = setup
    config.update(turns=1, monitor_mode="jev", monitor_interval_tokens=2)
    config["monitor_dimensions"][0]["action"] = action
    active = peak = 0
    prefixes = []
    closed = []

    class Streaming(Runtime):
        async def stream(self, prompt, settings, trace):
            try:
                trace["events"] = []
                for i in range(12):
                    await asyncio.sleep(0.005)
                    trace["generated_tokens"] = i + 1
                    yield "x"
            finally:
                closed.append(True)

    async def scan(cfg, conversation, turn):
        nonlocal active, peak
        active += 1
        peak = max(peak, active)
        before = turn["text"]
        await asyncio.sleep(0.016)
        assert turn["text"] == before  # New tokens cannot mutate the sampled prefix.
        prefixes.append(before)
        scores = {"looping": 0.99}
        turn["monitor"] = dict(
            status="complete",
            scores=scores,
            request={"sampled_text": before},
            detections=monitor.detections(cfg, scores),
        )
        active -= 1

    monkeypatch.setattr(monitor, "scan", scan)
    run = await simulator.generate(project, config, Streaming, emit)
    turn = run["conversations"][0]["turns"][-1]
    assert peak == 1 and active == 0 and closed
    assert prefixes[0] == "xx"
    assert turn["monitor_checks"][0]["tokens"] == 2
    assert turn["monitor_checks"][0]["request"]["sampled_text"] == "xx"
    assert turn["monitor_checks"][0]["phase"] == "partial"
    if action == "stop":
        assert (
            2 < len(turn["text"]) < 12
        )  # Generation progresses while Jev is in flight.
        assert run["conversations"][0]["status"] == "policy_stopped"
        assert turn["status"] == "policy_stopped"
    else:
        assert turn["text"] == "x" * 12
        assert prefixes[-1] == turn["text"]
        assert run["status"] == "complete"
    visible = simulator.view(run)["conversations"][0]["turns"][-1]
    assert all("request" not in c for c in visible["monitor_checks"])
    assert len([v for k, v in events if k == "policy.detection"]) == len(prefixes)


@pytest.mark.asyncio
async def test_cancel_during_midturn_scan_drains_task(setup, monkeypatch):
    from character_lab import monitor

    project, config, emit, events = setup
    config.update(turns=1, monitor_mode="jev", monitor_interval_tokens=1)
    started, cancelled, closed = asyncio.Event(), asyncio.Event(), asyncio.Event()

    class Streaming(Runtime):
        async def stream(self, prompt, settings, trace):
            try:
                trace["generated_tokens"] = 1
                yield "x"
                await asyncio.Event().wait()
            finally:
                closed.set()

    async def classify(request, record):
        record["request"] = request
        started.set()
        try:
            await asyncio.Event().wait()
        finally:
            cancelled.set()

    monkeypatch.setattr(monitor, "classify", classify)
    task = asyncio.create_task(simulator.generate(project, config, Streaming, emit))
    await asyncio.wait_for(started.wait(), 1)
    task.cancel()
    run = await task
    assert cancelled.is_set() and closed.is_set()
    assert run["status"] == "stopped"
    assert (
        run["conversations"][0]["turns"][-1]["monitor_checks"][0]["status"]
        == "cancelled"
    )

    check = run["conversations"][0]["turns"][-1]["monitor_checks"][0]
    assert check["calls"][0]["status"] == "cancelled"
    assert check["calls"][0]["request"]["state"]["history"][-1]["text"] == "x"


@pytest.mark.asyncio
async def test_same_model_fast_conversation_advances_while_other_is_blocked(setup):
    project, config, emit, events = setup
    config.update(conversations=2, turns=2)
    blocked, release = asyncio.Event(), asyncio.Event()
    calls = 0

    class Independent(Runtime):
        async def stream(self, prompt, settings, trace):
            nonlocal calls
            calls += 1
            if calls == 1:
                blocked.set()
                await release.wait()
            yield "independent reply"

    task = asyncio.create_task(simulator.generate(project, config, Independent, emit))
    try:
        await asyncio.wait_for(blocked.wait(), 1)

        async def completed():
            while (
                project.data["simulation_runs"][0]["conversations"][1]["status"]
                != "complete"
            ):
                await asyncio.sleep(0)

        await asyncio.wait_for(completed(), 1)
        assert len(project.data["simulation_runs"][0]["conversations"][1]["turns"]) == 4
        assert not task.done()
    finally:
        release.set()
        await task
    assert len(Runtime.instances) == 1


@pytest.mark.asyncio
async def test_slow_monitor_does_not_block_sibling_progress(setup, monkeypatch):
    from character_lab import monitor

    project, config, emit, events = setup
    config.update(conversations=2, turns=2, monitor_mode="jev")
    entered, release = asyncio.Event(), asyncio.Event()

    async def scan(config, conversation, turn):
        if conversation["index"] == 0:
            entered.set()
            await release.wait()
        turn["monitor"] = {"status": "complete", "scores": {}}

    monkeypatch.setattr(monitor, "scan", scan)
    task = asyncio.create_task(simulator.generate(project, config, Runtime, emit))
    try:
        await asyncio.wait_for(entered.wait(), 1)

        async def sibling_complete():
            while (
                project.data["simulation_runs"][0]["conversations"][1]["status"]
                != "complete"
            ):
                await asyncio.sleep(0)

        await asyncio.wait_for(sibling_complete(), 1)
        assert not task.done()
    finally:
        release.set()
        await task


@pytest.mark.asyncio
async def test_policy_stop_interrupts_stalled_read_without_stopping_sibling(
    setup, monkeypatch
):
    from character_lab import monitor

    project, config, emit, events = setup
    config.update(
        conversations=2, turns=2, monitor_mode="jev", monitor_interval_tokens=1
    )
    config["monitor_dimensions"][0]["action"] = "stop"
    stalled, closed = asyncio.Event(), asyncio.Event()
    calls = 0

    class Streaming(Runtime):
        async def stream(self, prompt, settings, trace):
            nonlocal calls
            calls += 1
            first = calls == 1
            try:
                trace["generated_tokens"] = 1
                yield "partial" if first else "sibling reply"
                if first:
                    stalled.set()
                    await asyncio.Event().wait()
            finally:
                if first:
                    closed.set()

    async def scan(cfg, conversation, turn):
        if conversation["index"] == 0:
            await stalled.wait()
        scores = {"looping": 0.99 if conversation["index"] == 0 else 0}
        turn["monitor"] = dict(
            status="complete", scores=scores, detections=monitor.detections(cfg, scores)
        )

    monkeypatch.setattr(monitor, "scan", scan)
    run = await asyncio.wait_for(
        simulator.generate(project, config, Streaming, emit), 1
    )
    assert closed.is_set()
    stopped, sibling = run["conversations"]
    assert run["status"] == "complete"
    assert stopped["status"] == "policy_stopped"
    assert len(stopped["turns"]) == 2
    assert stopped["turns"][-1]["text"] == "partial"
    assert stopped["turns"][-1]["status"] == "policy_stopped"
    assert sibling["status"] == "complete"
    assert len(sibling["turns"]) == 4
    assert (
        Project(project.folder).data["simulation_runs"][0]["conversations"][0]["turns"][
            -1
        ]["text"]
        == "partial"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("persistent_disconnect", [False, True])
async def test_terminal_disconnect_stops_incomplete_conversations(
    setup, persistent_disconnect
):
    from character_lab.backend import write_event

    project, config, _, _ = setup
    config.update(conversations=2, turns=3)

    class ClosedTerminal:
        def write(self, data):
            pass

        async def drain(self):
            raise ConnectionResetError("Connection lost")

    disconnected = False

    async def emit(kind, data):
        nonlocal disconnected
        if kind == "simulation.token" or (persistent_disconnect and disconnected):
            disconnected = True
            await write_event(ClosedTerminal(), {"type": kind, "data": data})

    if persistent_disconnect:
        with pytest.raises(asyncio.CancelledError):
            await simulator.generate(project, config, Runtime, emit)
        run = project.data["simulation_runs"][-1]
    else:
        run = await simulator.generate(project, config, Runtime, emit)
    assert run["status"] == "stopped"
    assert "error" not in run
    assert all(c["status"] == "stopped" for c in run["conversations"])
    assert any(
        t.get("text") == " A new path."
        for c in run["conversations"]
        for t in c["turns"]
    )
    assert all(
        t["status"] in {"complete", "stopped"}
        for c in run["conversations"]
        for t in c["turns"]
    )
    assert all(r.closed for r in Runtime.instances)
    assert (
        json.loads(project.path.read_text())["simulation_runs"][-1]["status"]
        == "stopped"
    )


@pytest.mark.asyncio
async def test_inference_connection_failure_is_still_a_failure(setup):
    project, config, emit, _ = setup

    class BrokenModel(Runtime):
        async def stream(self, prompt, settings, trace):
            raise ConnectionResetError("Model connection lost")
            yield  # Keep the runtime's asynchronous stream interface.

    run = await simulator.generate(project, config, BrokenModel, emit)
    assert run["status"] == "failed"
    assert run["error"] == "Model connection lost"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "after,during", [(True, True), (True, False), (False, True), (False, False)]
)
async def test_monitor_timing_switches_are_independent(
    setup, monkeypatch, after, during
):
    from character_lab import monitor

    project, config, emit, _ = setup
    config.update(
        turns=1,
        monitor_mode="jev",
        monitor_interval_tokens=2,
        monitor_after_reply=after,
        monitor_during_reply=during,
    )

    class Tokens(Runtime):
        async def stream(self, prompt, settings, trace):
            for count in range(1, 4):
                trace["generated_tokens"] = count
                yield "x"
                await asyncio.sleep(0)

    async def scan(config, conversation, turn):
        turn["monitor"] = {"status": "complete", "scores": {}, "detections": []}

    monkeypatch.setattr(monitor, "scan", scan)
    run = await simulator.generate(project, config, Tokens, emit)
    turn = run["conversations"][0]["turns"][-1]
    checks = turn.get("monitor_checks", [])
    assert any(c["phase"] == "partial" for c in checks) == during
    assert (
        any(c["phase"] == "complete" or c.get("end_of_turn") for c in checks) == after
    )
    assert turn["text"] == "xxx" and run["status"] == "complete"


def test_monitor_timing_defaults_and_validation(setup):
    project, config, _, _ = setup
    project.data["simulator_config"] = {"monitor_interval_tokens": 0}
    restored = simulator.configuration(project, "base")
    assert restored["monitor_interval_tokens"] == 0
    assert restored["monitor_after_reply"] is True
    for key in ("monitor_after_reply", "monitor_during_reply"):
        with pytest.raises(ValueError, match="boolean"):
            simulator.validate(
                {**config, key: "false"}, project, Session.validate_settings
            )


@pytest.mark.asyncio
@pytest.mark.parametrize("indices", [None, [0, 2]])
async def test_continue_selected_siblings_keeps_each_history(setup, indices):
    project, config, emit, events = setup
    config.update(conversations=4, turns=1)
    original = await simulator.generate(project, config, Runtime, emit)
    for c in original["conversations"]:
        c["turns"][-1]["text"] = f"Unique sibling {c['index']}"
    before = copy.deepcopy(original)
    seed = simulator.batch_seed(project, original["id"], indices)
    config.update(conversations=len(seed["conversations"]), turns=2)
    continued = await simulator.generate(project, config, Runtime, emit, seed)
    assert original == before
    assert continued["parent"] == dict(run=original["id"], conversation=-1)
    for c, index in zip(continued["conversations"], indices or range(4)):
        assert c["parent"] == dict(run=original["id"], conversation=index)
        assert c["turns"][:2] == before["conversations"][index]["turns"]
        assert [t["role"] for t in c["turns"][2:]] == [
            "visitor",
            "character",
            "visitor",
            "character",
        ]
        assert f"Unique sibling {index}" in c["turns"][2]["prompt"]
        assert all(
            f"Unique sibling {other}" not in c["turns"][2]["prompt"]
            for other in range(4)
            if other != index
        )


@pytest.mark.asyncio
async def test_continue_batch_with_different_final_speakers(setup):
    project, config, emit, _ = setup
    config.update(conversations=2, turns=1, visitor_alias="other")
    original = await simulator.generate(project, config, Runtime, emit)
    original["conversations"][1]["turns"].pop()
    continued = await simulator.generate(
        project, config, Runtime, emit, simulator.batch_seed(project, original["id"])
    )
    assert [t["role"] for t in continued["conversations"][0]["turns"]] == [
        "user",
        "character",
        "visitor",
        "character",
    ]
    assert [t["role"] for t in continued["conversations"][1]["turns"]] == [
        "user",
        "character",
    ]
    for indices in ([], [0, 0], [True], [999]):
        with pytest.raises(ValueError):
            simulator.batch_seed(project, original["id"], indices)


@pytest.mark.asyncio
async def test_continue_keeps_identity_and_archives_exact_prior_version(setup):
    from character_lab import simulator_actions

    project, config, emit, _ = setup
    config.update(conversations=4, turns=1)
    original = await simulator.generate(project, config, Runtime, emit)
    before = copy.deepcopy(original)
    seed = simulator.batch_seed(project, original["id"], [1, 3])
    config.update(action="continue", turns=2)
    results = await simulator.generate_alternatives(
        project, config, Runtime, emit, seed
    )
    assert len(results) == len(project.data["simulation_runs"]) == 1
    assert results[0] is original
    assert original["revisions"] == [before]
    assert "revisions" not in simulator.view(original)
    assert original["conversations"][0] == before["conversations"][0]
    assert original["conversations"][2] == before["conversations"][2]
    assert [len(c["turns"]) for c in original["conversations"]] == [2, 6, 2, 6]
    assert simulator_actions.affected_targets(results) == [
        dict(run=original["id"], conversation=1),
        dict(run=original["id"], conversation=3),
    ]
    await simulator.generate_alternatives(
        project,
        config,
        Runtime,
        emit,
        simulator.conversation_seed(project, original["id"], 1),
    )
    assert original["revisions"][0] == before
    assert len(original["revisions"]) == 2
    assert "revisions" not in original["revisions"][1]
    restored = Project(project.folder).data["simulation_runs"][0]
    assert restored["revisions"][0] == before
    assert restored["id"] == before["id"]


@pytest.mark.asyncio
async def test_loom_group_preserves_sets_with_one_shared_runtime(setup):
    from character_lab import simulator_actions

    project, config, emit, _ = setup
    config.update(conversations=4, turns=1)
    original = await simulator.generate(project, config, Runtime, emit)
    before = copy.deepcopy(original)
    instances = len(Runtime.instances)
    config.update(action="loom", alternatives=3)
    runs = await simulator.generate_alternatives(
        project, config, Runtime, emit, simulator.batch_seed(project, original["id"])
    )
    assert len(runs) == 3
    assert len(Runtime.instances) == instances + 1
    assert original == before
    assert len({run["alternative_group"] for run in runs}) == 1
    assert [run["alternative_index"] for run in runs] == [0, 1, 2]
    for run in runs:
        assert len(run["conversations"]) == 4
        assert simulator.summary(run)["alternative_group"] == run["alternative_group"]
        for index, conversation in enumerate(run["conversations"]):
            assert conversation["parent"] == dict(
                run=original["id"], conversation=index
            )
            assert conversation["turns"][:2] == before["conversations"][index]["turns"]
    assert len(simulator_actions.group_candidates(runs)) == 3
    chosen = simulator_actions.selected_seed(project, runs, "1")
    assert len(chosen["conversations"]) == 4
    assert chosen["parent"]["run"] == runs[1]["id"]


@pytest.mark.asyncio
async def test_explicit_visitor_is_atomic_and_responded_to_without_extra_visitor(setup):
    from character_lab import simulator_actions

    project, config, emit, _ = setup
    config.update(conversations=2, turns=1)
    original = await simulator.generate(project, config, Runtime, emit)
    before = copy.deepcopy(original)
    seed = simulator.batch_seed(project, original["id"])
    supplied = simulator_actions.add_visitor(seed, "My concrete question")
    assert seed["conversations"][0]["turns"] == before["conversations"][0]["turns"]
    config.update(action="continue")
    await simulator.generate_alternatives(project, config, Runtime, emit, supplied)
    for c in original["conversations"]:
        assert [t["role"] for t in c["turns"]] == [
            "user",
            "character",
            "visitor",
            "character",
        ]
        assert c["turns"][2]["origin"] == "human"
        assert "My concrete question" in c["turns"][3]["prompt"]
    original["conversations"][1]["turns"].pop()
    before = copy.deepcopy(original)
    with pytest.raises(ValueError, match="unanswered visitor"):
        simulator_actions.add_visitor(
            simulator.batch_seed(project, original["id"]), "Another"
        )
    assert original == before


@pytest.mark.asyncio
async def test_multiple_source_sets_preserve_shape_and_select_as_whole_alternative(
    setup,
):
    from character_lab import simulator_actions

    project, config, emit, _ = setup
    config.update(conversations=3, turns=1)
    first = await simulator.generate(project, config, Runtime, emit)
    second = await simulator.generate(project, config, Runtime, emit)
    targets = [dict(run=first["id"], conversation=i) for i in [0, 2]] + [
        dict(run=second["id"], conversation=1)
    ]
    seed = simulator_actions.resolve_seed(project, dict(targets=targets))
    config.update(action="loom", alternatives=2)
    runs = await simulator.generate_alternatives(project, config, Runtime, emit, seed)
    assert [len(r["conversations"]) for r in runs] == [2, 1, 2, 1]
    assert [r["alternative_index"] for r in runs] == [0, 0, 1, 1]
    candidates = simulator_actions.group_candidates(runs)
    assert len(candidates) == 2
    selected = simulator_actions.selected_seed(project, runs, "1")
    assert len(selected["sets"]) == 2
    assert [len(s["conversations"]) for s in selected["sets"]] == [2, 1]
    branched = simulator.fork_sets(project, selected, {})
    assert [len(r["conversations"]) for r in branched] == [2, 1]
    assert all(r["status"] == "draft" for r in branched)
    before = copy.deepcopy([first, second])
    config.update(action="continue")
    advanced = await simulator.generate_alternatives(
        project, config, Runtime, emit, seed
    )
    assert [r["id"] for r in advanced] == [first["id"], second["id"]]
    assert first["conversations"][1] == before[0]["conversations"][1]
    assert second["conversations"][0] == before[1]["conversations"][0]


@pytest.mark.asyncio
async def test_continue_cancellation_preserves_archived_head_and_new_partial(setup):
    project, config, emit, _ = setup
    config.update(conversations=2, turns=1)
    run = await simulator.generate(project, config, Runtime, emit)
    before = copy.deepcopy(run)
    started = asyncio.Event()

    class Slow(Runtime):
        async def stream(self, prompt, settings, trace):
            yield "new partial"
            started.set()
            await asyncio.Event().wait()

    config.update(action="continue")
    task = asyncio.create_task(
        simulator.generate_alternatives(
            project, config, Slow, emit, simulator.batch_seed(project, run["id"], [1])
        )
    )
    await started.wait()
    task.cancel()
    await task
    assert run["status"] == "stopped"
    assert run["revisions"][0] == before
    assert run["conversations"][0] == before["conversations"][0]
    assert run["conversations"][1]["turns"][-1]["text"] == "new partial"
    assert run["conversations"][1]["turns"][-1]["status"] == "stopped"


@pytest.mark.asyncio
async def test_action_dispatch_validates_before_start_and_records_overrides(setup):
    from types import SimpleNamespace

    from character_lab import simulator_commands

    project, config, emit, _ = setup
    config.update(conversations=2, turns=1)
    original = await simulator.generate(project, config, Runtime, emit)
    project.data["simulator_config"] = copy.deepcopy(config)
    captured = []

    async def start(*args):
        captured.append(args)

    session = SimpleNamespace(
        project=project,
        runtime=SimpleNamespace(model={"alias": "base"}),
        validate_settings=Session.validate_settings,
        start_simulation=start,
        policy_model={},
        selection_model=lambda: {},
    )
    before = copy.deepcopy(project.data)
    for update, match in [
        (dict(action="continue", count=2), "count belongs"),
        (dict(action="continue", model="missing"), "configured local base"),
        (dict(action="continue", visitor_model="missing"), "configured local base"),
        (dict(action="loom", count=0), "positive integer"),
        (dict(action="loom", visitor=""), "nonempty"),
    ]:
        with pytest.raises(ValueError, match=match):
            await simulator_commands.dispatch(
                session, "simulator.run", dict(run=original["id"], **update), "request"
            )
    assert not captured
    assert project.data == before
    await simulator_commands.dispatch(
        session,
        "simulator.run",
        dict(
            action="continue",
            run=original["id"],
            conversations=[1],
            visitor="My question",
            model="other",
            visitor_model="base",
        ),
        "request",
    )
    effective, seed, request, evaluation = captured[0]
    assert effective["action"] == "continue"
    assert effective["character_alias"] == "other"
    assert effective["visitor_alias"] == "base"
    assert seed["conversations"][0]["turns"][-1]["text"] == "My question"
    assert project.data == before
    assert request == "request" and evaluation is None


@pytest.mark.asyncio
async def test_stale_continuation_seed_cannot_overwrite_a_newer_head(setup):
    project, config, emit, _ = setup
    config.update(conversations=1, turns=1)
    run = await simulator.generate(project, config, Runtime, emit)
    stale = simulator.conversation_seed(project, run["id"], 0)
    config.update(action="continue")
    await simulator.generate_alternatives(project, config, Runtime, emit, stale)
    before = copy.deepcopy(project.data)
    with pytest.raises(ValueError, match="changed"):
        await simulator.generate_alternatives(project, config, Runtime, emit, stale)
    assert project.data == before
