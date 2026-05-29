"""Tests for the multi_agent pattern.

Covers:
- Single coordinator delegates to workers
- Multiple rounds of delegation
- max_rounds enforcement
- Worker results are aggregated
- Parallel execution of worker tasks (B5.7)
"""

from __future__ import annotations

import pytest

from aik_runtime.patterns.multi_agent import (
    AgentRole,
    Delegation,
    MultiAgentPattern,
)
from aik_runtime.patterns.plan_execute import PatternResult


# --- Mock implementations ---


class MockCoordinatorLLM:
    """Mock coordinator LLM that delegates tasks to workers based on config."""

    def __init__(
        self,
        delegations_per_round: list[list[Delegation]] | None = None,
    ) -> None:
        # Each entry is the delegations returned for that round
        self._delegations_per_round = delegations_per_round or []
        self._round_idx = 0
        self.delegate_call_count = 0
        self.synthesize_call_count = 0
        self._synthesis_result = "synthesized"

    def set_synthesis_result(self, result: str) -> None:
        self._synthesis_result = result

    async def delegate(
        self, input_text: str, workers: list[str], history: list[Delegation]
    ) -> list[Delegation]:
        self.delegate_call_count += 1
        if self._round_idx >= len(self._delegations_per_round):
            return []
        delegations = self._delegations_per_round[self._round_idx]
        self._round_idx += 1
        return delegations

    async def synthesize(
        self, input_text: str, delegations: list[Delegation]
    ) -> str:
        self.synthesize_call_count += 1
        # Combine all delegation results
        parts = [d.result for d in delegations if d.result]
        return f"{self._synthesis_result}: {'; '.join(parts)}"


class MockWorkerLLM:
    """Mock worker LLM that returns a predefined result."""

    def __init__(self, result: str = "done") -> None:
        self._result = result
        self.call_count = 0

    async def execute(self, task: str) -> str:
        self.call_count += 1
        return f"{self._result}({task})"


# --- Tests ---


class TestSingleDelegation:
    """Test coordinator delegates to workers in a single round."""

    @pytest.mark.asyncio
    async def test_coordinator_delegates_to_single_worker(self) -> None:
        coordinator = AgentRole(name="coordinator", system_prompt="You coordinate.")
        worker = AgentRole(name="researcher", system_prompt="You research.", tools=["search"])

        delegations = [
            [Delegation(from_agent="coordinator", to_agent="researcher", task="find info")]
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        worker_llm = MockWorkerLLM(result="found")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            coordinator_llm=coord_llm,
            worker_llms={"researcher": worker_llm},
        )

        result = await pattern.run("Find some information")

        assert isinstance(result, PatternResult)
        assert result.steps_executed == 1
        assert "found(find info)" in result.output
        assert worker_llm.call_count == 1
        assert coord_llm.synthesize_call_count == 1

    @pytest.mark.asyncio
    async def test_coordinator_delegates_to_multiple_workers(self) -> None:
        coordinator = AgentRole(name="coordinator")
        workers = [
            AgentRole(name="researcher", tools=["search"]),
            AgentRole(name="writer", tools=["write"]),
        ]

        delegations = [
            [
                Delegation(from_agent="coordinator", to_agent="researcher", task="research topic"),
                Delegation(from_agent="coordinator", to_agent="writer", task="write draft"),
            ]
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        researcher_llm = MockWorkerLLM(result="researched")
        writer_llm = MockWorkerLLM(result="written")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=workers,
            coordinator_llm=coord_llm,
            worker_llms={"researcher": researcher_llm, "writer": writer_llm},
        )

        result = await pattern.run("Research and write about AI")

        assert result.steps_executed == 2
        assert "researched(research topic)" in result.output
        assert "written(write draft)" in result.output

    @pytest.mark.asyncio
    async def test_no_delegations_returns_empty_result(self) -> None:
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="worker")

        coord_llm = MockCoordinatorLLM(delegations_per_round=[])
        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            coordinator_llm=coord_llm,
            worker_llms={},
        )

        result = await pattern.run("Nothing to do")

        assert result.steps_executed == 0
        assert result.output == ""


class TestMultipleRounds:
    """Test multiple rounds of delegation."""

    @pytest.mark.asyncio
    async def test_two_rounds_of_delegation(self) -> None:
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="researcher")

        delegations = [
            [Delegation(from_agent="coordinator", to_agent="researcher", task="round1 task")],
            [Delegation(from_agent="coordinator", to_agent="researcher", task="round2 task")],
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        worker_llm = MockWorkerLLM(result="done")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            max_rounds=3,
            coordinator_llm=coord_llm,
            worker_llms={"researcher": worker_llm},
        )

        result = await pattern.run("Multi-round research")

        assert result.steps_executed == 2
        assert worker_llm.call_count == 2
        assert "done(round1 task)" in result.output
        assert "done(round2 task)" in result.output

    @pytest.mark.asyncio
    async def test_coordinator_stops_early_when_no_more_delegations(self) -> None:
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="worker")

        # Only one round of delegations, then empty (stop)
        delegations = [
            [Delegation(from_agent="coordinator", to_agent="worker", task="only task")],
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        worker_llm = MockWorkerLLM(result="result")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            max_rounds=5,
            coordinator_llm=coord_llm,
            worker_llms={"worker": worker_llm},
        )

        result = await pattern.run("Do something")

        # Should only do 1 round despite max_rounds=5
        assert result.steps_executed == 1
        assert coord_llm.delegate_call_count == 2  # 1st returns tasks, 2nd returns empty


class TestMaxRoundsEnforcement:
    """Test max_rounds enforcement."""

    @pytest.mark.asyncio
    async def test_max_rounds_limits_delegation_rounds(self) -> None:
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="worker")

        # Provide more rounds than max_rounds allows
        delegations = [
            [Delegation(from_agent="coordinator", to_agent="worker", task=f"task{i}")]
            for i in range(10)
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        worker_llm = MockWorkerLLM(result="done")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            max_rounds=2,
            coordinator_llm=coord_llm,
            worker_llms={"worker": worker_llm},
        )

        result = await pattern.run("Lots of work")

        # Only 2 rounds executed
        assert result.steps_executed == 2
        assert worker_llm.call_count == 2

    @pytest.mark.asyncio
    async def test_max_rounds_minimum_is_one(self) -> None:
        """max_rounds=0 should be treated as 1."""
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="worker")

        delegations = [
            [Delegation(from_agent="coordinator", to_agent="worker", task="task")],
            [Delegation(from_agent="coordinator", to_agent="worker", task="task2")],
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        worker_llm = MockWorkerLLM(result="done")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            max_rounds=0,  # Should be clamped to 1
            coordinator_llm=coord_llm,
            worker_llms={"worker": worker_llm},
        )

        result = await pattern.run("Work")

        assert result.steps_executed == 1


class TestResultAggregation:
    """Test that worker results are properly aggregated."""

    @pytest.mark.asyncio
    async def test_results_from_all_workers_are_aggregated(self) -> None:
        coordinator = AgentRole(name="coordinator")
        workers = [
            AgentRole(name="analyzer"),
            AgentRole(name="reviewer"),
            AgentRole(name="editor"),
        ]

        delegations = [
            [
                Delegation(from_agent="coordinator", to_agent="analyzer", task="analyze"),
                Delegation(from_agent="coordinator", to_agent="reviewer", task="review"),
                Delegation(from_agent="coordinator", to_agent="editor", task="edit"),
            ]
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)
        coord_llm.set_synthesis_result("final")

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=workers,
            coordinator_llm=coord_llm,
            worker_llms={
                "analyzer": MockWorkerLLM(result="analyzed"),
                "reviewer": MockWorkerLLM(result="reviewed"),
                "editor": MockWorkerLLM(result="edited"),
            },
        )

        result = await pattern.run("Process document")

        assert result.steps_executed == 3
        assert "analyzed(analyze)" in result.output
        assert "reviewed(review)" in result.output
        assert "edited(edit)" in result.output

    @pytest.mark.asyncio
    async def test_worker_with_missing_llm_returns_error(self) -> None:
        coordinator = AgentRole(name="coordinator")
        worker = AgentRole(name="ghost_worker")

        delegations = [
            [Delegation(from_agent="coordinator", to_agent="ghost_worker", task="do something")]
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=[worker],
            coordinator_llm=coord_llm,
            worker_llms={},  # No LLM configured for ghost_worker
        )

        result = await pattern.run("Test missing worker")

        assert result.steps_executed == 1
        assert "error:" in result.output

    @pytest.mark.asyncio
    async def test_parallel_worker_execution(self) -> None:
        """Workers should execute in parallel when max_parallel > 1 (B5.7)."""
        coordinator = AgentRole(name="coordinator")
        workers = [
            AgentRole(name="w1"),
            AgentRole(name="w2"),
            AgentRole(name="w3"),
        ]

        delegations = [
            [
                Delegation(from_agent="coordinator", to_agent="w1", task="task1"),
                Delegation(from_agent="coordinator", to_agent="w2", task="task2"),
                Delegation(from_agent="coordinator", to_agent="w3", task="task3"),
            ]
        ]
        coord_llm = MockCoordinatorLLM(delegations_per_round=delegations)

        pattern = MultiAgentPattern(
            coordinator=coordinator,
            workers=workers,
            coordinator_llm=coord_llm,
            worker_llms={
                "w1": MockWorkerLLM(result="r1"),
                "w2": MockWorkerLLM(result="r2"),
                "w3": MockWorkerLLM(result="r3"),
            },
            max_parallel=3,
        )

        result = await pattern.run("Parallel work")

        assert result.steps_executed == 3
        assert "r1(task1)" in result.output
        assert "r2(task2)" in result.output
        assert "r3(task3)" in result.output
