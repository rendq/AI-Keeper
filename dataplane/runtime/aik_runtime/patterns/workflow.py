"""Workflow pattern for Agent Runtime.

Implements B5.1: predefined DAG of steps with conditional branching.

A workflow is a directed acyclic graph (DAG) of steps. Each step has a handler
(callable), a list of possible next steps, and an optional condition (callable)
that determines whether a particular next step should be followed.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable


@dataclass
class PatternResult:
    """Result of a workflow pattern invocation.

    Attributes:
        output: Final output string from the execution.
        steps_executed: Number of steps that were actually executed.
        trace: Ordered list of step IDs that were executed.
    """

    output: str = ""
    steps_executed: int = 0
    trace: list[str] = field(default_factory=list)


@dataclass
class WorkflowStep:
    """A single step in a workflow DAG.

    Attributes:
        id: Unique identifier for this step.
        handler: Callable that takes a context dict and returns a string result.
        next_steps: List of candidate next step IDs to transition to.
        condition: Optional callable (context -> bool) that determines whether
            this step's next_steps should be evaluated. If None, always proceeds.
    """

    id: str = ""
    handler: Callable[[dict[str, Any]], str] = field(default_factory=lambda: lambda ctx: "")
    next_steps: list[str] = field(default_factory=list)
    condition: Callable[[dict[str, Any]], bool] | None = None


@dataclass
class WorkflowDefinition:
    """Definition of a workflow DAG.

    Attributes:
        steps: Mapping from step ID to WorkflowStep.
        entry_point: The ID of the first step to execute.
    """

    steps: dict[str, WorkflowStep] = field(default_factory=dict)
    entry_point: str = ""


class WorkflowPattern:
    """Workflow pattern: execute a predefined DAG of steps with conditional branching.

    Starting from the entry_point, executes each step's handler and then
    evaluates next_steps to determine which step to proceed to. If a step
    has multiple next_steps, the first one whose condition evaluates to True
    (or has no condition) is selected.
    """

    def run(self, definition: WorkflowDefinition, context: dict[str, Any]) -> PatternResult:
        """Execute a workflow DAG from the entry point.

        Args:
            definition: The workflow definition containing steps and entry_point.
            context: Shared context dict passed to each step handler and condition.

        Returns:
            PatternResult with combined output, steps executed count, and trace.
        """
        if not definition.entry_point or definition.entry_point not in definition.steps:
            return PatternResult(output="", steps_executed=0, trace=[])

        outputs: list[str] = []
        trace: list[str] = []
        current_step_id: str | None = definition.entry_point
        visited: set[str] = set()

        while current_step_id and current_step_id not in visited:
            step = definition.steps.get(current_step_id)
            if step is None:
                break

            visited.add(current_step_id)
            trace.append(current_step_id)

            # Execute the step handler
            result = step.handler(context)
            if result:
                outputs.append(result)

            # Determine next step
            current_step_id = self._resolve_next(step, definition, context)

        return PatternResult(
            output="\n".join(outputs),
            steps_executed=len(trace),
            trace=trace,
        )

    def _resolve_next(
        self,
        step: WorkflowStep,
        definition: WorkflowDefinition,
        context: dict[str, Any],
    ) -> str | None:
        """Resolve which next step to transition to.

        Iterates through next_steps and returns the first step whose condition
        is None (unconditional) or evaluates to True. Returns None if no
        next step qualifies.
        """
        for next_id in step.next_steps:
            next_step = definition.steps.get(next_id)
            if next_step is None:
                continue

            # If the next step has a condition, evaluate it
            if next_step.condition is None or next_step.condition(context):
                return next_id

        return None
