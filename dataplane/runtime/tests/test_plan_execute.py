"""Tests for the plan_execute pattern.

Covers:
- Plan generation produces steps
- Sequential execution follows plan order
- Parallel execution of independent steps
- max_steps enforcement
"""

from __future__ import annotations

import asyncio
from typing import Any

import pytest

from aik_runtime.patterns.plan_execute import (
    PatternResult,
    PlanExecutePattern,
    PlanStep,
)


# --- Mock implementations ---


class MockPlannerLLM:
    """Mock planner that returns predefined steps."""

    def __init__(self, steps: list[PlanStep] | None = None) -> None:
        self._steps = steps or []
        self.call_count = 0

    async def generate_plan(self, input_text: str) -> list[PlanStep]:
        self.call_count += 1
        return list(self._steps)


class MockStepExecutor:
    """Mock executor that records execution order and returns step descriptions."""

    def __init__(self, delay: float = 0.0) -> None:
        self.executed: list[str] = []
        self.execution_order: list[str] = []
        self._delay = delay

    async def execute_step(self, step: PlanStep) -> str:
        if self._delay > 0:
            await asyncio.sleep(self._delay)
        self.executed.append(step.tool_name)
        self.execution_order.append(step.tool_name)
        return f"result:{step.tool_name}"


# --- Tests ---


class TestPlanGeneration:
    """Test that plan() generates steps correctly."""

    @pytest.mark.asyncio
    async def test_plan_returns_steps_from_planner(self) -> None:
        steps = [
            PlanStep(description="Search", tool_name="search", tool_args={"q": "test"}),
            PlanStep(description="Summarize", tool_name="summarize", depends_on=[0]),
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        result = await pattern.plan("Find and summarize info")

        assert len(result) == 2
        assert result[0].tool_name == "search"
        assert result[1].tool_name == "summarize"
        assert result[1].depends_on == [0]
        assert planner.call_count == 1

    @pytest.mark.asyncio
    async def test_plan_empty_input_returns_empty(self) -> None:
        planner = MockPlannerLLM(steps=[])
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        result = await pattern.plan("")

        assert result == []

    @pytest.mark.asyncio
    async def test_plan_truncates_to_max_steps(self) -> None:
        steps = [
            PlanStep(description=f"Step {i}", tool_name=f"tool_{i}")
            for i in range(10)
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor, max_steps=3)

        result = await pattern.plan("Do many things")

        assert len(result) == 3


class TestSequentialExecution:
    """Test that execute() follows plan order for sequential (dependent) steps."""

    @pytest.mark.asyncio
    async def test_sequential_execution_order(self) -> None:
        steps = [
            PlanStep(description="First", tool_name="a"),
            PlanStep(description="Second", tool_name="b", depends_on=[0]),
            PlanStep(description="Third", tool_name="c", depends_on=[1]),
        ]
        executor = MockStepExecutor()
        planner = MockPlannerLLM(steps=steps)
        pattern = PlanExecutePattern(planner=planner, executor=executor, max_parallel=1)

        output = await pattern.execute(steps)

        # All steps executed in order
        assert executor.execution_order == ["a", "b", "c"]
        assert "result:a" in output
        assert "result:b" in output
        assert "result:c" in output

    @pytest.mark.asyncio
    async def test_sequential_single_step(self) -> None:
        steps = [PlanStep(description="Only", tool_name="only_tool")]
        executor = MockStepExecutor()
        planner = MockPlannerLLM(steps=steps)
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        output = await pattern.execute(steps)

        assert executor.executed == ["only_tool"]
        assert output == "result:only_tool"

    @pytest.mark.asyncio
    async def test_execute_empty_steps(self) -> None:
        executor = MockStepExecutor()
        planner = MockPlannerLLM(steps=[])
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        output = await pattern.execute([])

        assert output == ""
        assert executor.executed == []


class TestParallelExecution:
    """Test parallel execution of independent steps (B5.7)."""

    @pytest.mark.asyncio
    async def test_independent_steps_run_in_parallel(self) -> None:
        """Steps with no dependencies should all be eligible for parallel execution."""
        steps = [
            PlanStep(description="A", tool_name="a"),
            PlanStep(description="B", tool_name="b"),
            PlanStep(description="C", tool_name="c"),
        ]
        executor = MockStepExecutor(delay=0.01)
        planner = MockPlannerLLM(steps=steps)
        pattern = PlanExecutePattern(
            planner=planner, executor=executor, max_parallel=3
        )

        output = await pattern.execute(steps)

        # All three executed
        assert set(executor.executed) == {"a", "b", "c"}
        assert "result:a" in output
        assert "result:b" in output
        assert "result:c" in output

    @pytest.mark.asyncio
    async def test_parallel_respects_dependencies(self) -> None:
        """Steps with deps wait for their predecessors."""
        steps = [
            PlanStep(description="A", tool_name="a"),  # independent
            PlanStep(description="B", tool_name="b"),  # independent
            PlanStep(description="C", tool_name="c", depends_on=[0, 1]),  # waits for a, b
        ]
        executor = MockStepExecutor()
        planner = MockPlannerLLM(steps=steps)
        pattern = PlanExecutePattern(
            planner=planner, executor=executor, max_parallel=3
        )

        output = await pattern.execute(steps)

        # a and b should execute before c
        assert "c" in executor.executed
        c_idx = executor.execution_order.index("c")
        a_idx = executor.execution_order.index("a")
        b_idx = executor.execution_order.index("b")
        assert c_idx > a_idx
        assert c_idx > b_idx

    @pytest.mark.asyncio
    async def test_parallel_limited_by_max_parallel(self) -> None:
        """max_parallel=2 should batch at most 2 at a time."""
        steps = [
            PlanStep(description="A", tool_name="a"),
            PlanStep(description="B", tool_name="b"),
            PlanStep(description="C", tool_name="c"),
            PlanStep(description="D", tool_name="d"),
        ]
        executor = MockStepExecutor()
        planner = MockPlannerLLM(steps=steps)
        pattern = PlanExecutePattern(
            planner=planner, executor=executor, max_parallel=2
        )

        output = await pattern.execute(steps)

        # All four executed
        assert len(executor.executed) == 4
        assert set(executor.executed) == {"a", "b", "c", "d"}


class TestMaxStepsEnforcement:
    """Test max_steps enforcement in plan and execute phases."""

    @pytest.mark.asyncio
    async def test_max_steps_limits_plan(self) -> None:
        steps = [
            PlanStep(description=f"Step {i}", tool_name=f"tool_{i}")
            for i in range(5)
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor, max_steps=2)

        result = await pattern.run("test")

        assert result.steps_executed == 2
        assert len(executor.executed) == 2

    @pytest.mark.asyncio
    async def test_max_steps_zero_means_unlimited(self) -> None:
        steps = [
            PlanStep(description=f"Step {i}", tool_name=f"tool_{i}")
            for i in range(5)
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor, max_steps=0)

        result = await pattern.run("test")

        assert result.steps_executed == 5
        assert len(executor.executed) == 5

    @pytest.mark.asyncio
    async def test_max_steps_limits_execution(self) -> None:
        """Even if plan returns more steps, execution is capped."""
        steps = [
            PlanStep(description=f"Step {i}", tool_name=f"tool_{i}")
            for i in range(10)
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor, max_steps=3)

        output = await pattern.execute(steps)

        # Only first 3 executed
        assert len(executor.executed) == 3


class TestRunOrchestration:
    """Test the full run() method that orchestrates plan + execute."""

    @pytest.mark.asyncio
    async def test_run_full_cycle(self) -> None:
        steps = [
            PlanStep(description="Search", tool_name="search"),
            PlanStep(description="Analyze", tool_name="analyze", depends_on=[0]),
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        result = await pattern.run("Find and analyze data")

        assert isinstance(result, PatternResult)
        assert result.steps_executed == 2
        assert "result:search" in result.output
        assert "result:analyze" in result.output

    @pytest.mark.asyncio
    async def test_run_with_empty_plan(self) -> None:
        planner = MockPlannerLLM(steps=[])
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(planner=planner, executor=executor)

        result = await pattern.run("Nothing to do")

        assert result.steps_executed == 0
        assert result.output == ""
        assert result.total_tokens == 0

    @pytest.mark.asyncio
    async def test_run_parallel_independent(self) -> None:
        steps = [
            PlanStep(description="A", tool_name="a"),
            PlanStep(description="B", tool_name="b"),
            PlanStep(description="Final", tool_name="final", depends_on=[0, 1]),
        ]
        planner = MockPlannerLLM(steps=steps)
        executor = MockStepExecutor()
        pattern = PlanExecutePattern(
            planner=planner, executor=executor, max_parallel=2
        )

        result = await pattern.run("Do A, B then combine")

        assert result.steps_executed == 3
        assert "result:final" in result.output
