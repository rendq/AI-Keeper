"""Tests for the reflection pattern.

Covers:
- Generate produces output
- Critique evaluates output
- Refine improves output
- Run terminates when critique passes
- Run respects max_iterations
- Run with immediate pass (no refinement needed)
"""

from __future__ import annotations

import pytest

from aik_runtime.patterns.plan_execute import PatternResult
from aik_runtime.patterns.reflection import (
    Critique,
    ReflectionPattern,
)


# --- Mock implementations ---


class MockGeneratorLLM:
    """Mock generator that returns a predefined output."""

    def __init__(self, output: str = "initial output") -> None:
        self._output = output
        self.call_count = 0

    async def generate(self, input_text: str) -> str:
        self.call_count += 1
        return self._output


class MockCriticLLM:
    """Mock critic that returns predefined critiques in sequence."""

    def __init__(self, critiques: list[Critique] | None = None) -> None:
        self._critiques = critiques or [Critique(passed=True, feedback="", score=1.0)]
        self._index = 0
        self.call_count = 0

    async def critique(self, input_text: str, output: str) -> Critique:
        self.call_count += 1
        if self._index < len(self._critiques):
            result = self._critiques[self._index]
            self._index += 1
            return result
        # Default: pass after exhausting critiques
        return Critique(passed=True, feedback="", score=1.0)


class MockRefinerLLM:
    """Mock refiner that appends refinement info to output."""

    def __init__(self) -> None:
        self.call_count = 0

    async def refine(self, input_text: str, output: str, critique: Critique) -> str:
        self.call_count += 1
        return f"{output} [refined: {critique.feedback}]"


# --- Tests ---


class TestGenerate:
    """Test that generate produces output."""

    @pytest.mark.asyncio
    async def test_generate_produces_output(self) -> None:
        generator = MockGeneratorLLM(output="hello world")
        critic = MockCriticLLM()
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.generate("say hello")

        assert result == "hello world"
        assert generator.call_count == 1

    @pytest.mark.asyncio
    async def test_generate_with_empty_input(self) -> None:
        generator = MockGeneratorLLM(output="default response")
        critic = MockCriticLLM()
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.generate("")

        assert result == "default response"


class TestCritique:
    """Test that critique evaluates output."""

    @pytest.mark.asyncio
    async def test_critique_returns_feedback(self) -> None:
        generator = MockGeneratorLLM()
        critic = MockCriticLLM(
            critiques=[Critique(passed=False, feedback="too short", score=0.3)]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.critique("write essay", "short text")

        assert result.passed is False
        assert result.feedback == "too short"
        assert result.score == 0.3
        assert critic.call_count == 1

    @pytest.mark.asyncio
    async def test_critique_passes_good_output(self) -> None:
        generator = MockGeneratorLLM()
        critic = MockCriticLLM(
            critiques=[Critique(passed=True, feedback="", score=0.95)]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.critique("write essay", "excellent essay content")

        assert result.passed is True
        assert result.score == 0.95


class TestRefine:
    """Test that refine improves output."""

    @pytest.mark.asyncio
    async def test_refine_incorporates_feedback(self) -> None:
        generator = MockGeneratorLLM()
        critic = MockCriticLLM()
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        critique_result = Critique(passed=False, feedback="add more detail", score=0.4)
        result = await pattern.refine("write essay", "draft text", critique_result)

        assert "refined" in result
        assert "add more detail" in result
        assert refiner.call_count == 1


class TestRunTermination:
    """Test that run terminates when critique passes."""

    @pytest.mark.asyncio
    async def test_run_stops_when_critique_passes(self) -> None:
        """After one failed critique + refine, second critique passes."""
        generator = MockGeneratorLLM(output="draft")
        critic = MockCriticLLM(
            critiques=[
                Critique(passed=False, feedback="needs work", score=0.4),
                Critique(passed=True, feedback="", score=0.9),
            ]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.run("write something", max_iterations=5)

        assert isinstance(result, PatternResult)
        # 1 generate + 1 critique (fail) + 1 refine + 1 critique (pass) = 4
        assert result.steps_executed == 4
        assert generator.call_count == 1
        assert critic.call_count == 2
        assert refiner.call_count == 1
        assert "refined" in result.output


class TestRunMaxIterations:
    """Test that run respects max_iterations."""

    @pytest.mark.asyncio
    async def test_run_respects_max_iterations(self) -> None:
        """Should stop after max_iterations even if critique never passes."""
        generator = MockGeneratorLLM(output="draft")
        critic = MockCriticLLM(
            critiques=[
                Critique(passed=False, feedback="bad1", score=0.2),
                Critique(passed=False, feedback="bad2", score=0.3),
                Critique(passed=False, feedback="bad3", score=0.4),
            ]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.run("write something", max_iterations=2)

        # 1 generate + (1 critique + 1 refine) * 2 = 5
        assert result.steps_executed == 5
        assert generator.call_count == 1
        assert critic.call_count == 2
        assert refiner.call_count == 2

    @pytest.mark.asyncio
    async def test_run_max_iterations_one(self) -> None:
        """With max_iterations=1, only one critique/refine cycle."""
        generator = MockGeneratorLLM(output="draft")
        critic = MockCriticLLM(
            critiques=[Critique(passed=False, feedback="not great", score=0.5)]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.run("test", max_iterations=1)

        # 1 generate + 1 critique + 1 refine = 3
        assert result.steps_executed == 3
        assert refiner.call_count == 1


class TestRunImmediatePass:
    """Test run with immediate pass (no refinement needed)."""

    @pytest.mark.asyncio
    async def test_run_immediate_pass_no_refinement(self) -> None:
        """When first critique passes, no refinement occurs."""
        generator = MockGeneratorLLM(output="perfect output")
        critic = MockCriticLLM(
            critiques=[Critique(passed=True, feedback="", score=1.0)]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.run("write something perfect")

        assert isinstance(result, PatternResult)
        # 1 generate + 1 critique (pass) = 2
        assert result.steps_executed == 2
        assert result.output == "perfect output"
        assert generator.call_count == 1
        assert critic.call_count == 1
        assert refiner.call_count == 0

    @pytest.mark.asyncio
    async def test_run_returns_pattern_result(self) -> None:
        generator = MockGeneratorLLM(output="output")
        critic = MockCriticLLM(
            critiques=[Critique(passed=True, feedback="", score=0.9)]
        )
        refiner = MockRefinerLLM()
        pattern = ReflectionPattern(generator=generator, critic=critic, refiner=refiner)

        result = await pattern.run("test")

        assert isinstance(result, PatternResult)
        assert result.output == "output"
        assert result.total_tokens == 0
