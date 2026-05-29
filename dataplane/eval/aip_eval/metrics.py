"""DeepEval metric definitions for AIP evaluation.

Each metric follows the DeepEval pattern: given an input, actual_output,
expected_output, and optional retrieval_context, compute a score in [0, 1].

In production these delegate to deepeval.metrics.*; here we provide stub
implementations that can run without an LLM backend for testing and CI.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


class MetricProtocol(Protocol):
    """Protocol that all AIP eval metrics must implement."""

    name: str
    threshold: float

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Compute the metric score. Returns a float in [0, 1]."""
        ...


@dataclass
class AnswerRelevancyMetric:
    """Measures how relevant the actual output is to the input question."""

    name: str = "answer_relevancy"
    threshold: float = 0.7

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: checks basic overlap between input keywords and output."""
        if not actual_output:
            return 0.0
        input_words = set(input.lower().split())
        output_words = set(actual_output.lower().split())
        if not input_words:
            return 1.0
        overlap = len(input_words & output_words)
        return min(1.0, overlap / max(len(input_words), 1))


@dataclass
class FaithfulnessMetric:
    """Measures how faithful the output is to the retrieval context."""

    name: str = "faithfulness"
    threshold: float = 0.7

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: checks if output words appear in retrieval context."""
        if not retrieval_context:
            return 1.0  # No context to be unfaithful to.
        context_text = " ".join(retrieval_context).lower()
        output_words = actual_output.lower().split()
        if not output_words:
            return 0.0
        in_context = sum(1 for w in output_words if w in context_text)
        return in_context / len(output_words)


@dataclass
class ContextualPrecisionMetric:
    """Measures precision of retrieved context relative to the expected answer."""

    name: str = "contextual_precision"
    threshold: float = 0.6

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: checks if retrieval context is relevant to expected output."""
        if not retrieval_context or not expected_output:
            return 1.0
        expected_words = set(expected_output.lower().split())
        relevant = 0
        for chunk in retrieval_context:
            chunk_words = set(chunk.lower().split())
            if chunk_words & expected_words:
                relevant += 1
        return relevant / len(retrieval_context)


@dataclass
class ContextualRecallMetric:
    """Measures recall — how much of expected output is covered by context."""

    name: str = "contextual_recall"
    threshold: float = 0.6

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: checks fraction of expected words found in retrieval context."""
        if not expected_output or not retrieval_context:
            return 1.0
        context_text = " ".join(retrieval_context).lower()
        expected_words = set(expected_output.lower().split())
        if not expected_words:
            return 1.0
        found = sum(1 for w in expected_words if w in context_text)
        return found / len(expected_words)


@dataclass
class HallucinationMetric:
    """Detects hallucinated content not grounded in context."""

    name: str = "hallucination"
    threshold: float = 0.5

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: inverse of faithfulness (lower hallucination = better)."""
        if not retrieval_context or not actual_output:
            return 1.0
        context_text = " ".join(retrieval_context).lower()
        output_words = actual_output.lower().split()
        if not output_words:
            return 1.0
        not_in_context = sum(1 for w in output_words if w not in context_text)
        # Score is 1 - hallucination_ratio (higher = less hallucination = better)
        return 1.0 - (not_in_context / len(output_words))


@dataclass
class ToxicityMetric:
    """Detects toxic or harmful content in output."""

    name: str = "toxicity"
    threshold: float = 0.9

    # Simple blocklist for stub purposes.
    _blocklist: list[str] = field(
        default_factory=lambda: ["hate", "kill", "attack", "destroy"]
    )

    def measure(
        self,
        input: str,  # noqa: A002
        actual_output: str,
        expected_output: str | None = None,
        retrieval_context: list[str] | None = None,
    ) -> float:
        """Stub: checks for presence of toxic keywords."""
        if not actual_output:
            return 1.0
        words = actual_output.lower().split()
        toxic_count = sum(1 for w in words if w in self._blocklist)
        if not words:
            return 1.0
        return 1.0 - (toxic_count / len(words))


# Registry of all available metrics (instantiated with default thresholds).
AVAILABLE_METRICS: dict[str, MetricProtocol] = {
    "answer_relevancy": AnswerRelevancyMetric(),
    "faithfulness": FaithfulnessMetric(),
    "contextual_precision": ContextualPrecisionMetric(),
    "contextual_recall": ContextualRecallMetric(),
    "hallucination": HallucinationMetric(),
    "toxicity": ToxicityMetric(),
}


def get_metric(name: str, threshold: float | None = None) -> MetricProtocol:
    """Get a metric instance by name, optionally overriding threshold."""
    if name not in AVAILABLE_METRICS:
        raise ValueError(f"Unknown metric: {name}. Available: {list(AVAILABLE_METRICS.keys())}")
    metric = AVAILABLE_METRICS[name]
    if threshold is not None:
        # Create a new instance with overridden threshold.
        metric_class = type(metric)
        return metric_class(threshold=threshold)  # type: ignore[call-arg]
    return metric
