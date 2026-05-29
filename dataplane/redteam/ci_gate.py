"""CI gate logic for red team scan results — blocks merge when attack success rate exceeds threshold."""

from __future__ import annotations

from dataclasses import dataclass

from dataplane.redteam.report import ReportMetrics


@dataclass
class GateConfig:
    """Configuration for the CI red team gate.

    Attributes:
        max_success_rate: Maximum allowed attack success rate (e.g. 0.1 = 10%).
        max_risk_score: Maximum allowed cumulative risk score.
        fail_on_critical: If True, any successful critical-severity attack fails the gate.
    """

    max_success_rate: float = 0.1
    max_risk_score: float = 50.0
    fail_on_critical: bool = True


@dataclass
class GateResult:
    """Outcome of the CI gate evaluation."""

    passed: bool
    reason: str
    metrics: ReportMetrics


def evaluate_gate(metrics: ReportMetrics, config: GateConfig) -> GateResult:
    """Evaluate red team metrics against the gate configuration.

    Returns a GateResult indicating pass/fail with an explanation.
    """
    # No attacks → gate passes (nothing to block on)
    if metrics.total_attacks == 0:
        return GateResult(passed=True, reason="No attacks executed", metrics=metrics)

    # Check critical severity
    if config.fail_on_critical and metrics.by_severity.get("critical", 0) > 0:
        count = metrics.by_severity["critical"]
        return GateResult(
            passed=False,
            reason=f"Critical severity attacks succeeded: {count}",
            metrics=metrics,
        )

    # Check success rate
    if metrics.success_rate > config.max_success_rate:
        return GateResult(
            passed=False,
            reason=(
                f"Attack success rate {metrics.success_rate:.1%} exceeds "
                f"threshold {config.max_success_rate:.1%}"
            ),
            metrics=metrics,
        )

    # Check risk score
    if metrics.risk_score > config.max_risk_score:
        return GateResult(
            passed=False,
            reason=(
                f"Risk score {metrics.risk_score:.1f} exceeds "
                f"threshold {config.max_risk_score:.1f}"
            ),
            metrics=metrics,
        )

    return GateResult(passed=True, reason="All checks passed", metrics=metrics)


def format_gate_output(result: GateResult) -> str:
    """Format gate result as GitHub Actions annotation output."""
    m = result.metrics
    lines: list[str] = []

    if result.passed:
        lines.append(f"::notice title=Red Team Gate PASSED::{result.reason}")
    else:
        lines.append(f"::error title=Red Team Gate FAILED::{result.reason}")

    lines.append(
        f"::group::Red Team Metrics\n"
        f"Total Attacks: {m.total_attacks}\n"
        f"Successful Attacks: {m.successful_attacks}\n"
        f"Success Rate: {m.success_rate:.1%}\n"
        f"Risk Score: {m.risk_score:.1f}\n"
        f"::endgroup::"
    )

    return "\n".join(lines)
