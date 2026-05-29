"""Tests for red team CI gate logic."""

from __future__ import annotations

from dataplane.redteam.ci_gate import GateConfig, evaluate_gate, format_gate_output
from dataplane.redteam.report import ReportMetrics


class TestEvaluateGate:
    """Tests for evaluate_gate function."""

    def test_passes_when_success_rate_below_threshold(self) -> None:
        metrics = ReportMetrics(
            total_attacks=100,
            successful_attacks=5,
            success_rate=0.05,
            risk_score=20.0,
        )
        config = GateConfig(max_success_rate=0.1)
        result = evaluate_gate(metrics, config)
        assert result.passed is True
        assert result.reason == "All checks passed"

    def test_fails_when_success_rate_above_threshold(self) -> None:
        metrics = ReportMetrics(
            total_attacks=100,
            successful_attacks=20,
            success_rate=0.20,
            risk_score=30.0,
        )
        config = GateConfig(max_success_rate=0.1)
        result = evaluate_gate(metrics, config)
        assert result.passed is False
        assert "exceeds" in result.reason
        assert "20.0%" in result.reason

    def test_fails_on_critical_severity_when_configured(self) -> None:
        metrics = ReportMetrics(
            total_attacks=50,
            successful_attacks=2,
            success_rate=0.04,
            by_severity={"critical": 2},
            risk_score=20.0,
        )
        config = GateConfig(fail_on_critical=True)
        result = evaluate_gate(metrics, config)
        assert result.passed is False
        assert "Critical severity" in result.reason

    def test_passes_on_critical_severity_when_not_configured(self) -> None:
        metrics = ReportMetrics(
            total_attacks=50,
            successful_attacks=2,
            success_rate=0.04,
            by_severity={"critical": 2},
            risk_score=20.0,
        )
        config = GateConfig(fail_on_critical=False)
        result = evaluate_gate(metrics, config)
        assert result.passed is True

    def test_passes_with_no_attacks(self) -> None:
        metrics = ReportMetrics(total_attacks=0)
        config = GateConfig()
        result = evaluate_gate(metrics, config)
        assert result.passed is True
        assert result.reason == "No attacks executed"

    def test_fails_when_risk_score_exceeds_threshold(self) -> None:
        metrics = ReportMetrics(
            total_attacks=100,
            successful_attacks=5,
            success_rate=0.05,
            risk_score=60.0,
        )
        config = GateConfig(max_risk_score=50.0)
        result = evaluate_gate(metrics, config)
        assert result.passed is False
        assert "Risk score" in result.reason


class TestFormatGateOutput:
    """Tests for format_gate_output function."""

    def test_format_passed_uses_notice_annotation(self) -> None:
        metrics = ReportMetrics(total_attacks=10, successful_attacks=0, success_rate=0.0)
        from dataplane.redteam.ci_gate import GateResult

        result = GateResult(passed=True, reason="All checks passed", metrics=metrics)
        output = format_gate_output(result)
        assert "::notice title=Red Team Gate PASSED::" in output
        assert "All checks passed" in output

    def test_format_failed_uses_error_annotation(self) -> None:
        metrics = ReportMetrics(
            total_attacks=100, successful_attacks=20, success_rate=0.2, risk_score=30.0
        )
        from dataplane.redteam.ci_gate import GateResult

        result = GateResult(passed=False, reason="Threshold exceeded", metrics=metrics)
        output = format_gate_output(result)
        assert "::error title=Red Team Gate FAILED::" in output
        assert "Threshold exceeded" in output

    def test_format_includes_metrics_group(self) -> None:
        metrics = ReportMetrics(
            total_attacks=50, successful_attacks=5, success_rate=0.1, risk_score=15.0
        )
        from dataplane.redteam.ci_gate import GateResult

        result = GateResult(passed=True, reason="All checks passed", metrics=metrics)
        output = format_gate_output(result)
        assert "::group::Red Team Metrics" in output
        assert "::endgroup::" in output
        assert "Total Attacks: 50" in output
        assert "Success Rate: 10.0%" in output
