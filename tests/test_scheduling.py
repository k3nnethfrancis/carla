import asyncio

import pytest

from character_lab.scheduling import Admission, parallel_map


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "budget,slots,memory,expected",
    [(20, 4, 4, 4), (60, 4, 4, 1), (20, 4, 1, 1), (20, 1, 4, 1)],
)
async def test_admission_bounds_context_slots_and_pressure(
    budget, slots, memory, expected
):
    gate = Admission(memory=lambda: memory)
    peak = 0

    async def work(_):
        nonlocal peak
        async with gate.reserve(budget, 100, slots, {}):
            peak = max(peak, gate.active)
            assert gate.tokens <= 100
            await asyncio.sleep(0.01)

    await parallel_map(range(8), work)
    assert peak == expected
    assert gate.active == gate.tokens == gate.waiting == 0


@pytest.mark.asyncio
async def test_pressure_change_reduces_new_admissions():
    limit = 4
    gate = Admission(memory=lambda: limit)
    entered = asyncio.Event()
    release = asyncio.Event()

    async def hold():
        async with gate.reserve(20, 100, 4, {}):
            entered.set()
            await release.wait()

    task = asyncio.create_task(hold())
    await entered.wait()
    limit = 1
    second = asyncio.create_task(hold())
    await asyncio.sleep(0.01)
    assert gate.active == 1 and gate.waiting == 1
    release.set()
    await asyncio.gather(task, second)
    assert gate.active == gate.tokens == gate.waiting == 0


@pytest.mark.asyncio
async def test_cancel_drains_running_and_waiting_tasks():
    gate = Admission(memory=lambda: 4)

    async def work(_):
        async with gate.reserve(60, 100, 4, {}):
            await asyncio.sleep(60)

    task = asyncio.create_task(parallel_map(range(100), work))
    await asyncio.sleep(0.01)
    assert gate.active == 1 and gate.waiting == 3
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await asyncio.wait_for(task, timeout=5)
    assert gate.active == gate.waiting == gate.tokens == 0


@pytest.mark.asyncio
async def test_worker_failure_drains_other_tasks():
    gate = Admission(memory=lambda: 4)

    async def work(i):
        async with gate.reserve(20, 100, 4, {}):
            await asyncio.sleep(0.01)
            if i == 0:
                raise ValueError("provider failed")
            await asyncio.sleep(60)

    with pytest.raises(ValueError, match="provider failed"):
        await asyncio.wait_for(parallel_map(range(8), work), timeout=5)
    assert gate.active == gate.waiting == gate.tokens == 0
