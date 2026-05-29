"""Main eval runner — invoked inside Argo Workflow containers.

Reads environment variables injected by the Go EvalRunner:
- AIP_SKILL_NAMESPACE
- AIP_SKILL_NAME
- AIP_EVAL_RUN_ID
- AIP_EVAL_SET_REF (optional)
- AIP_RED_TEAM_SET_REF (optional)

Loads the eval/red-team sets, runs DeepEval metrics, and writes results
back to Skill.status.evalResults (via K8s API or stdout for now).
"""

from __future__ import annotations

import json
import os
import sys
import time
from dataclasses import asdict, dataclass, field

from aip_eval.loader import EvalSet, EvalTestCase, RedTeamSet, load_eval_set, load_red_team_set
from aip_eval.metrics import AVAILABLE_METRICS, MetricProtocol, get_metric


@dataclass
class MetricScore:
    """Result of a single metric evaluation."""

    name: str
    score: float
    threshold: float
    passed: bool


@dataclass
class EvalRunResult:
    """Aggregated result of an evaluation run."""

    run_id: str
    skill_namespace: str
    skill_name: str
    metrics: list[MetricScore] = field(default_factory=list)
    passed: bool = False
    started_at: float = 0.0
    completed_at: float = 0.0
    error: str | None = None


class EvalRunner:
    """Orchestrates the evaluation of a Skill against an eval set.

    Usage:
        runner = EvalRunner(
            skill_namespace="team-alpha",
            skill_name="summarizer",
            run_id="run-001",
        )
        result = runner.run(eval_set, metrics=["answer_relevancy", "faithfulness"])
    """

    def __init__(
        self,
        skill_namespace: str,
        skill_name: str,
        run_id: str,
        metrics: list[str] | None = None,
        metric_thresholds: dict[str, float] | None = None,
    ):
        self.skill_namespace = skill_namespace
        self.skill_name = skill_name
        self.run_id = run_id
        self._metric_names = metrics or list(AVAILABLE_METRICS.keys())
        self._thresholds = metric_thresholds or {}

    def run(self, eval_set: EvalSet) -> EvalRunResult:
        """Run all configured metrics against the eval set.

        For each test case, runs each metric and averages scores across all
        test cases per metric.
        """
        started_at = time.time()
        result = EvalRunResult(
            run_id=self.run_id,
            skill_namespace=self.skill_namespace,
            skill_name=self.skill_name,
            started_at=started_at,
        )

        if not eval_set.test_cases:
            result.error = "No test cases in eval set"
            result.completed_at = time.time()
            return result

        # Resolve which metrics to run (prefer eval_set.metrics if specified).
        metric_names = eval_set.metrics if eval_set.metrics else self._metric_names

        # Initialize metrics.
        metrics: list[MetricProtocol] = []
        for name in metric_names:
            try:
                threshold = self._thresholds.get(name)
                metrics.append(get_metric(name, threshold))
            except ValueError:
                continue  # Skip unknown metrics.

        if not metrics:
            result.error = "No valid metrics configured"
            result.completed_at = time.time()
            return result

        # Run evaluation.
        metric_scores = self._evaluate(eval_set.test_cases, metrics)
        result.metrics = metric_scores
        result.passed = all(m.passed for m in metric_scores)
        result.completed_at = time.time()

        return result

    def run_red_team(self, red_team_set: RedTeamSet) -> EvalRunResult:
        """Run red team evaluation — checks that adversarial inputs are blocked.

        For each red team test case, we simulate the Skill invocation and check
        that the guardrails/runtime correctly blocks the attack.

        This is a stub that always reports success for blocked cases.
        """
        started_at = time.time()
        result = EvalRunResult(
            run_id=self.run_id,
            skill_namespace=self.skill_namespace,
            skill_name=self.skill_name,
            started_at=started_at,
        )

        if not red_team_set.test_cases:
            result.error = "No test cases in red team set"
            result.completed_at = time.time()
            return result

        # Stub: in production, each test case would be sent to the Skill endpoint
        # and we'd verify the response is blocked/safe.
        blocked_count = 0
        total = len(red_team_set.test_cases)

        for tc in red_team_set.test_cases:
            # Stub: assume guardrails correctly block expected attacks.
            if tc.expected_blocked:
                blocked_count += 1

        block_rate = blocked_count / total if total > 0 else 0.0
        result.metrics = [
            MetricScore(
                name="red_team_block_rate",
                score=block_rate,
                threshold=0.95,
                passed=block_rate >= 0.95,
            )
        ]
        result.passed = block_rate >= 0.95
        result.completed_at = time.time()

        return result

    def _evaluate(
        self, test_cases: list[EvalTestCase], metrics: list[MetricProtocol]
    ) -> list[MetricScore]:
        """Run all metrics across all test cases, averaging per metric."""
        # Accumulate scores: metric_name -> list of scores.
        scores: dict[str, list[float]] = {m.name: [] for m in metrics}

        for tc in test_cases:
            actual = tc.actual_output or ""
            for metric in metrics:
                score = metric.measure(
                    input=tc.input,
                    actual_output=actual,
                    expected_output=tc.expected_output,
                    retrieval_context=tc.retrieval_context if tc.retrieval_context else None,
                )
                scores[metric.name].append(score)

        # Average scores and determine pass/fail.
        results: list[MetricScore] = []
        for metric in metrics:
            metric_scores = scores[metric.name]
            avg = sum(metric_scores) / len(metric_scores) if metric_scores else 0.0
            results.append(
                MetricScore(
                    name=metric.name,
                    score=round(avg, 4),
                    threshold=metric.threshold,
                    passed=avg >= metric.threshold,
                )
            )

        return results


def main() -> None:
    """Entry point when invoked as `python -m aip_eval.runner`."""
    skill_namespace = os.environ.get("AIP_SKILL_NAMESPACE", "default")
    skill_name = os.environ.get("AIP_SKILL_NAME", "")
    run_id = os.environ.get("AIP_EVAL_RUN_ID", "unknown")
    eval_set_ref = os.environ.get("AIP_EVAL_SET_REF")
    red_team_set_ref = os.environ.get("AIP_RED_TEAM_SET_REF")

    if not skill_name:
        print("ERROR: AIP_SKILL_NAME is required", file=sys.stderr)
        sys.exit(1)

    runner = EvalRunner(
        skill_namespace=skill_namespace,
        skill_name=skill_name,
        run_id=run_id,
    )

    # Run eval set if configured.
    if eval_set_ref:
        eval_set = load_eval_set(eval_set_ref)
        result = runner.run(eval_set)
        print(json.dumps(asdict(result), indent=2))

    # Run red team set if configured.
    if red_team_set_ref:
        red_team_set = load_red_team_set(red_team_set_ref)
        result = runner.run_red_team(red_team_set)
        print(json.dumps(asdict(result), indent=2))

    if not eval_set_ref and not red_team_set_ref:
        print("WARNING: No eval set or red team set configured, nothing to run.", file=sys.stderr)
        sys.exit(0)


if __name__ == "__main__":
    main()
