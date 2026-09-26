"""Bounded in-flight policy checks over immutable prefixes of a character reply."""

import asyncio

from . import monitor
from .simulator import view


class TurnMonitor:
    def __init__(self, config, project, run, conversation, turn, emit):
        self.config, self.project, self.run = config, project, run
        self.conversation, self.turn, self.emit = conversation, turn, emit
        self.enabled = config["monitor_mode"] == "jev" and turn["role"] == "character"
        self.interval = config.get("monitor_interval_tokens", 512)
        self.next_tokens = self.interval
        self.task = None

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        if self.task:
            if not self.task.done():
                self.task.cancel()
            await asyncio.gather(self.task, return_exceptions=True)
        self.project.save()

    @property
    def stopped(self):
        return self.conversation["status"] == "policy_stopped"

    def checkpoint(self):
        """Use llama.cpp's actual output-token counter, never characters or chunks."""
        tokens = self.turn["trace"].get("generated_tokens", 0)
        if not self.enabled or not self.interval or self.stopped:
            return
        if self.task is not None and self.task.done():
            self.task.result()  # Surface programmer errors before replacing a finished task.
        if self.task is not None and not self.task.done():
            return  # Coalesce growth while the provider is busy; never queue prefixes.
        if tokens >= self.next_tokens:
            self.next_tokens = tokens + self.interval
            self.task = asyncio.create_task(
                self.check("partial", tokens, self.snapshot())
            )

    def snapshot(self):
        return {
            "index": self.conversation["index"],
            "turns": [
                {"role": t["role"], "text": t["text"]}
                for t in self.conversation["turns"]
            ],
        }

    async def check(self, phase, tokens, snapshot=None):
        snapshot = snapshot if snapshot is not None else self.snapshot()
        captured = snapshot["turns"][-1]
        captured["status"] = "generating" if phase == "partial" else "complete"
        checks = self.turn.setdefault("monitor_checks", [])
        record = dict(
            status="checking",
            phase=phase,
            tokens=tokens,
            characters=len(captured["text"]),
            check=len(checks),
        )
        checks.append(record)
        self.turn["monitor"] = record
        self.project.save()
        await self.emit("simulation", view(self.run))
        try:
            await monitor.scan(self.config, snapshot, captured)
            record.update(captured["monitor"])
            for detection in record.get("detections", []):
                if detection["action"] == "stop":
                    self.conversation["status"] = "policy_stopped"
                await self.emit(
                    "policy.detection",
                    dict(
                        run=self.run["id"],
                        conversation=self.conversation["index"],
                        turn=len(self.conversation["turns"]) - 1,
                        check=record["check"],
                        **detection,
                    ),
                )
            self.project.save()
            await self.emit("simulation", view(self.run))
        except asyncio.CancelledError:
            record["status"] = "cancelled"
            raise

    async def finish(self):
        if not self.enabled:
            return
        if self.task:
            await self.task
        if self.stopped:
            return
        checks = self.turn.get("monitor_checks", [])
        if checks and checks[-1]["characters"] == len(self.turn["text"]):
            checks[-1]["end_of_turn"] = True
            return
        await self.check("complete", self.turn["trace"].get("generated_tokens", 0))
