"""Multi-Agent pattern for Agent Runtime.

Implements B5.1 (6 patterns) and B5.7 (parallelism in multi_agent):
- A coordinator agent delegates tasks to worker agents.
- Workers execute their assigned tasks and return results.
- The coordinator synthesizes all worker results into a final answer.
- Supports multiple rounds of delegation up to max_rounds.
- Independent worker tasks can run concurrently (B5.7).
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from typing import Protocol

from aik_runtime.patterns.plan_execute import PatternResult


# --- Data classes ---


@dataclass
class AgentRole:
    """Defines a role (coordinator or worker) in a multi-agent system.

    Attributes:
        name: Unique identifier for this agent role.
        system_prompt: System prompt describing the agent's persona/instructions.
        tools: List of tool names this agent is allowed to use.
    """

    name: str
    system_prompt: str = ""
    tools: list[str] = field(default_factory=list)


@dataclass
class Delegation:
    """Records a delegation event between agents.

    Attributes:
        from_agent: Name of the agent that delegated the task.
        to_agent: Name of the agent that received the delegation.
        task: Description of the delegated task.
        result: Result returned by the delegated agent (empty until completed).
    """

    from_agent: str
    to_agent: str
    task: str
    result: str = ""


# --- Protocols ---


class CoordinatorLLM(Protocol):
    """Protocol for the coordinator's LLM that generates delegations."""

    async def delegate(
        self, input_text: str, workers: list[str], history: list[Delegation]
    ) -> list[Delegation]:
        """Decide which tasks to delegate to which workers.

        Args:
            input_text: The original user input or current synthesis context.
            workers: Available worker names.
            history: Previous delegation history for context.

        Returns:
            List of Delegation objects (with task filled, result empty).
            An empty list signals that no more delegation is needed.
        """
        ...

    async def synthesize(
        self, input_text: str, delegations: list[Delegation]
    ) -> str:
        """Synthesize final answer from all delegation results.

        Args:
            input_text: The original user input.
            delegations: All completed delegations with results.

        Returns:
            Final synthesized output string.
        """
        ...


class WorkerLLM(Protocol):
    """Protocol for a worker agent's LLM that executes delegated tasks."""

    async def execute(self, task: str) -> str:
        """Execute a delegated task and return the result.

        Args:
            task: The task description to execute.

        Returns:
            Result string from executing the task.
        """
        ...


# --- Multi-Agent Pattern ---


class MultiAgentPattern:
    """Multi-Agent pattern: coordinator delegates to workers, collects and synthesizes.

    The coordinator decides which workers to delegate tasks to, workers execute
    their tasks (potentially in parallel), and the coordinator synthesizes
    the final answer from all results.

    Supports multiple rounds: the coordinator can issue further delegations
    based on previous results, up to max_rounds.

    Args:
        coordinator: The coordinator agent role definition.
        workers: List of worker agent role definitions.
        max_rounds: Maximum number of delegation rounds (1 = single round).
        coordinator_llm: LLM backing the coordinator's decisions.
        worker_llms: Mapping of worker name -> LLM for that worker.
        max_parallel: Maximum concurrent worker executions (1 = sequential).
    """

    def __init__(
        self,
        coordinator: AgentRole,
        workers: list[AgentRole],
        *,
        max_rounds: int = 1,
        coordinator_llm: CoordinatorLLM,
        worker_llms: dict[str, WorkerLLM] | None = None,
        max_parallel: int = 1,
    ) -> None:
        self._coordinator = coordinator
        self._workers = {w.name: w for w in workers}
        self._max_rounds = max(1, max_rounds)
        self._coordinator_llm = coordinator_llm
        self._worker_llms = worker_llms or {}
        self._max_parallel = max(1, max_parallel)

    @property
    def worker_names(self) -> list[str]:
        """List of available worker agent names."""
        return list(self._workers.keys())

    async def _execute_delegation(self, delegation: Delegation) -> Delegation:
        """Execute a single delegation by calling the appropriate worker LLM."""
        worker_llm = self._worker_llms.get(delegation.to_agent)
        if worker_llm is None:
            delegation.result = f"error: no LLM configured for worker '{delegation.to_agent}'"
            return delegation

        result = await worker_llm.execute(delegation.task)
        delegation.result = result
        return delegation

    async def _execute_delegations(
        self, delegations: list[Delegation]
    ) -> list[Delegation]:
        """Execute a batch of delegations, respecting max_parallel."""
        if not delegations:
            return []

        if self._max_parallel == 1:
            # Sequential execution
            for d in delegations:
                await self._execute_delegation(d)
            return delegations

        # Parallel execution in batches
        results: list[Delegation] = []
        for i in range(0, len(delegations), self._max_parallel):
            batch = delegations[i : i + self._max_parallel]
            tasks = [self._execute_delegation(d) for d in batch]
            completed = await asyncio.gather(*tasks)
            results.extend(completed)

        return results

    async def run(self, input_text: str) -> PatternResult:
        """Run the multi-agent coordination cycle.

        1. Coordinator decides delegations for available workers.
        2. Workers execute their delegated tasks (possibly in parallel).
        3. Repeat up to max_rounds if coordinator issues more delegations.
        4. Coordinator synthesizes all results into final output.

        Args:
            input_text: The user's input/query.

        Returns:
            PatternResult with the synthesized output and execution metadata.
        """
        all_delegations: list[Delegation] = []
        rounds_executed = 0

        for _round in range(self._max_rounds):
            # Coordinator decides what to delegate
            new_delegations = await self._coordinator_llm.delegate(
                input_text, self.worker_names, all_delegations
            )

            if not new_delegations:
                # No more work to delegate
                break

            # Execute delegations
            completed = await self._execute_delegations(new_delegations)
            all_delegations.extend(completed)
            rounds_executed += 1

        # Synthesize final result
        if not all_delegations:
            return PatternResult(output="", steps_executed=0, total_tokens=0)

        output = await self._coordinator_llm.synthesize(input_text, all_delegations)

        return PatternResult(
            output=output,
            steps_executed=len(all_delegations),
            total_tokens=0,
        )
