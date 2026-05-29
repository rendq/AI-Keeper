"""Plan-Execute pattern FSM for Agent Runtime.

Implements B5.1 (6 patterns) and B5.7 (parallelism in plan_execute):
1. Plan phase: call LLM to generate a list of PlanStep from user input.
2. Execute phase: execute steps sequentially or in parallel (based on depends_on).

Independent steps (no unresolved dependencies) can run concurrently when
parallelism > 1.
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from typing import Any, Protocol


# --- Data classes ---


@dataclass
class PlanStep:
    """A single step in an execution plan.

    Attributes:
        description: Human-readable description of what this step does.
        tool_name: Name of the tool to call for this step.
        tool_args: Arguments to pass to the tool.
        depends_on: Indices of steps that must complete before this one.
    """

    description: str = ""
    tool_name: str = ""
    tool_args: dict[str, Any] = field(default_factory=dict)
    depends_on: list[int] = field(default_factory=list)


@dataclass
class PatternResult:
    """Result of a plan_execute pattern invocation.

    Attributes:
        output: Final output string from the execution.
        steps_executed: Number of steps that were actually executed.
        total_tokens: Total tokens consumed (input + output) across all LLM calls.
    """

    output: str = ""
    steps_executed: int = 0
    total_tokens: int = 0


# --- Protocols ---


class PlannerLLM(Protocol):
    """Protocol for the LLM used during the planning phase."""

    async def generate_plan(self, input_text: str) -> list[PlanStep]:
        """Generate an execution plan from user input."""
        ...


class StepExecutor(Protocol):
    """Protocol for executing a single plan step (tool call)."""

    async def execute_step(self, step: PlanStep) -> str:
        """Execute a plan step and return the output string."""
        ...


# --- Plan-Execute Pattern ---


class PlanExecutePattern:
    """Plan-Execute pattern: plan steps first, then execute each.

    Supports parallel execution of independent steps (steps whose
    dependencies have all been satisfied) when max_parallel > 1.

    Args:
        planner: LLM client for generating execution plans.
        executor: Executor for running individual plan steps.
        max_steps: Maximum number of steps allowed (0 = unlimited).
        max_parallel: Maximum concurrent step executions (1 = sequential).
    """

    def __init__(
        self,
        planner: PlannerLLM,
        executor: StepExecutor,
        *,
        max_steps: int = 0,
        max_parallel: int = 1,
    ) -> None:
        self._planner = planner
        self._executor = executor
        self._max_steps = max_steps
        self._max_parallel = max(1, max_parallel)
        self._total_tokens = 0

    async def plan(self, input_text: str) -> list[PlanStep]:
        """Generate an execution plan by calling the planner LLM.

        Args:
            input_text: The user's input/query.

        Returns:
            A list of PlanStep describing the execution plan.
        """
        steps = await self._planner.generate_plan(input_text)

        # Enforce max_steps on the plan itself
        if self._max_steps > 0 and len(steps) > self._max_steps:
            steps = steps[: self._max_steps]

        return steps

    async def execute(self, steps: list[PlanStep]) -> str:
        """Execute plan steps, respecting dependencies and parallelism.

        Steps with no unresolved dependencies are executed concurrently
        (up to max_parallel). Steps that depend on others wait until
        their dependencies complete.

        Args:
            steps: The execution plan (list of PlanStep).

        Returns:
            Combined output from all executed steps.
        """
        if not steps:
            return ""

        # Enforce max_steps on execution
        effective_steps = steps
        if self._max_steps > 0:
            effective_steps = steps[: self._max_steps]

        num_steps = len(effective_steps)
        completed: set[int] = set()
        results: dict[int, str] = {}

        while len(completed) < num_steps:
            # Find steps that are ready (all deps satisfied)
            ready = [
                i
                for i in range(num_steps)
                if i not in completed
                and all(d in completed for d in effective_steps[i].depends_on)
            ]

            if not ready:
                # No ready steps but not all done — circular dependency
                break

            # Limit concurrency
            batch = ready[: self._max_parallel]

            # Execute batch
            if self._max_parallel == 1:
                # Sequential execution
                for idx in batch:
                    result = await self._executor.execute_step(effective_steps[idx])
                    results[idx] = result
                    completed.add(idx)
            else:
                # Parallel execution
                tasks = [
                    self._executor.execute_step(effective_steps[idx])
                    for idx in batch
                ]
                outputs = await asyncio.gather(*tasks)
                for idx, output in zip(batch, outputs):
                    results[idx] = output
                    completed.add(idx)

        # Combine results in execution order
        combined_output = "\n".join(
            results[i] for i in sorted(results.keys()) if results[i]
        )
        return combined_output

    async def run(self, input_text: str) -> PatternResult:
        """Orchestrate the full plan + execute cycle.

        Args:
            input_text: The user's input/query.

        Returns:
            PatternResult with output, steps_executed, and total_tokens.
        """
        # Phase 1: Plan
        steps = await self.plan(input_text)

        if not steps:
            return PatternResult(output="", steps_executed=0, total_tokens=0)

        # Phase 2: Execute
        output = await self.execute(steps)

        # Enforce max_steps: count only what was actually run
        steps_executed = min(len(steps), self._max_steps) if self._max_steps > 0 else len(steps)

        return PatternResult(
            output=output,
            steps_executed=steps_executed,
            total_tokens=self._total_tokens,
        )
