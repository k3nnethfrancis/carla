"""Bounded async work and admission against a shared, preallocated KV pool.

This is a conservative capacity heuristic, not a claim to predict GPU throughput.
Model weights load once; reservations never shrink a user's output budget.
"""

import asyncio
import time
from contextlib import asynccontextmanager

import psutil

MAX_WORKERS = 4


def memory_limit():
    memory = psutil.virtual_memory()
    reserve = max(2 * 1024**3, int(memory.total * 0.10))
    if memory.available < reserve:
        return 1
    if memory.available < reserve * 2:
        return 2
    return MAX_WORKERS


async def parallel_map(items, work):
    """Only four worker tasks exist, even for very large requested batches."""
    iterator = iter(items)

    async def worker():
        for item in iterator:
            await work(item)

    tasks = [asyncio.create_task(worker()) for _ in range(MAX_WORKERS)]
    try:
        await asyncio.gather(*tasks)
    except BaseException:
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise


class Admission:
    def __init__(self, memory=memory_limit):
        self.memory = memory
        self.condition = asyncio.Condition()
        self.active = 0
        self.tokens = 0
        self.waiting = 0

    @asynccontextmanager
    async def reserve(self, tokens, capacity, slots, trace, report=None):
        """Full prompt + output reservations prevent shared-cache overcommit."""

        if tokens < 1 or tokens > capacity or slots < 1:
            raise ValueError("Request cannot fit the available inference capacity")

        async def notify(stage):
            data = dict(
                active=self.active,
                waiting=self.waiting,
                slots=min(slots, self.memory()),
                reserved_tokens=self.tokens,
                context_capacity=capacity,
                stage=stage,
                request_tokens=tokens,
                wait_reason=(
                    "context capacity"
                    if self.tokens + tokens > capacity
                    else "memory / slots"
                    if self.active >= min(slots, self.memory())
                    else ""
                )
                if stage == "waiting for capacity"
                else "",
            )
            trace.setdefault("scheduling", []).append(data | {"time": time.time()})
            if report:
                await report(data)

        admitted = False
        async with self.condition:
            self.waiting += 1
            try:
                await notify("waiting for capacity")
                while (
                    self.active >= min(slots, self.memory())
                    or self.tokens + tokens > capacity
                ):
                    # Recheck memory even when no request completes.
                    try:
                        await asyncio.wait_for(self.condition.wait(), timeout=1)
                    except TimeoutError:
                        pass
                self.active += 1
                self.tokens += tokens
                admitted = True
            finally:
                self.waiting -= 1
            try:
                await notify("generating")
            except BaseException:
                if admitted:
                    self.active -= 1
                    self.tokens -= tokens
                    self.condition.notify_all()
                raise
        try:
            yield
        finally:
            async with self.condition:
                self.active -= 1
                self.tokens -= tokens
                self.condition.notify_all()
                await notify("capacity available")
