"""Reflection pattern for Agent Runtime.

Implements B5.1: generate → critique → refine loop.
The pattern generates an initial output, self-critiques it, and iteratively
refines until the critique passes or max iterations are reached.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from aik_runtime.patterns.plan_execute import PatternResult


# --- Data classes ---


@dataclass
class Critique:
    """Result of critiquing a generated output.

    Attributes:
        passed: Whether the output meets quality standards.
        feedback: Explanation of what needs improvement (empty if passed).
        score: Quality score from 0.0 (terrible) to 1.0 (perfect).
    """

    passed: bool = False
    feedback: str = ""
    score: float = 0.0


# --- Protocols ---


class GeneratorLLM(Protocol):
    """Protocol for LLM used to generate initial output."""

    async def generate(self, input_text: str) -> str:
        """Generate initial output from input."""
        ...


class CriticLLM(Protocol):
    """Protocol for LLM used to critique generated output."""

    async def critique(self, input_text: str, output: str) -> Critique:
        """Critique the output given the original input."""
        ...


class RefinerLLM(Protocol):
    """Protocol for LLM used to refine output based on critique."""

    async def refine(self, input_text: str, output: str, critique: Critique) -> str:
        """Refine the output based on critique feedback."""
        ...


# --- Reflection Pattern ---


class ReflectionPattern:
    """Reflection pattern: generate → critique → refine loop.

    Generates an initial output, then iteratively critiques and refines
    it until the critique passes or max_iterations is reached.

    Args:
        generator: LLM client for generating initial output.
        critic: LLM client for critiquing output.
        refiner: LLM client for refining output based on critique.
    """

    def __init__(
        self,
        generator: GeneratorLLM,
        critic: CriticLLM,
        refiner: RefinerLLM,
    ) -> None:
        self._generator = generator
        self._critic = critic
        self._refiner = refiner

    async def generate(self, input_text: str) -> str:
        """Generate initial output from input.

        Args:
            input_text: The user's input/query.

        Returns:
            Generated output string.
        """
        return await self._generator.generate(input_text)

    async def critique(self, input_text: str, output: str) -> Critique:
        """Critique the generated output.

        Args:
            input_text: The original user input.
            output: The generated output to critique.

        Returns:
            Critique with passed status, feedback, and score.
        """
        return await self._critic.critique(input_text, output)

    async def refine(self, input_text: str, output: str, critique: Critique) -> str:
        """Refine the output based on critique feedback.

        Args:
            input_text: The original user input.
            output: The current output to improve.
            critique: The critique containing feedback.

        Returns:
            Refined output string.
        """
        return await self._refiner.refine(input_text, output, critique)

    async def run(self, input_text: str, max_iterations: int = 3) -> PatternResult:
        """Orchestrate the generate → critique → refine loop.

        Generates an initial output, then iteratively critiques and refines
        until the critique passes or max_iterations is reached.

        Args:
            input_text: The user's input/query.
            max_iterations: Maximum number of critique/refine cycles.

        Returns:
            PatternResult with final output and steps_executed count.
        """
        # Phase 1: Generate initial output
        output = await self.generate(input_text)
        steps_executed = 1  # count the initial generation

        # Phase 2: Critique/Refine loop
        for _ in range(max_iterations):
            critique_result = await self.critique(input_text, output)
            steps_executed += 1  # count the critique

            if critique_result.passed:
                break

            output = await self.refine(input_text, output, critique_result)
            steps_executed += 1  # count the refinement

        return PatternResult(
            output=output,
            steps_executed=steps_executed,
            total_tokens=0,
        )
