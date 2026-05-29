"""Tests for aik_runtime.observability module."""

import time

import pytest


def test_get_metrics_singleton():
    """get_metrics() returns the same instance."""
    from aik_runtime.observability import get_metrics

    m1 = get_metrics()
    m2 = get_metrics()
    assert m1 is m2


def test_observe_step_duration_no_crash():
    """observe_step_duration works without prometheus_client installed or with it."""
    from aik_runtime.observability import RuntimeMetrics

    m = RuntimeMetrics()
    # Should not raise regardless of prometheus_client presence.
    m.observe_step_duration("tool_call", 0.5)
    m.observe_step_duration("model_call", 1.2)
    m.observe_step_duration("thought", 0.01)


def test_inc_invocation_no_crash():
    """inc_invocation works without crashing."""
    from aik_runtime.observability import RuntimeMetrics

    m = RuntimeMetrics()
    m.inc_invocation("react", "success")
    m.inc_invocation("tool_calling", "error")
    m.inc_invocation("react", "timeout")


def test_step_timer_records_duration():
    """StepTimer context manager measures elapsed time."""
    from aik_runtime.observability import StepTimer

    with StepTimer("test_step") as timer:
        time.sleep(0.05)

    # Should have measured at least 50ms.
    assert timer.duration >= 0.04
    assert timer.duration < 1.0


def test_init_observability_returns_provider():
    """init_observability returns a configured provider."""
    from aik_runtime.observability import init_observability

    provider = init_observability(
        service_name="test-runtime",
        service_version="0.1.0",
        otlp_endpoint="",  # No OTel endpoint — just metrics.
        metrics_port=19092,
    )

    assert provider.config.service_name == "test-runtime"
    assert provider.config.service_version == "0.1.0"
    assert provider.metrics is not None


def test_metrics_with_prometheus_client():
    """If prometheus_client is available, metrics are recorded."""
    try:
        import prometheus_client  # noqa: F401
    except ImportError:
        pytest.skip("prometheus_client not installed")

    from aik_runtime.observability import RuntimeMetrics

    m = RuntimeMetrics()
    m.observe_step_duration("tool_call", 0.123)
    m.inc_invocation("react", "success")

    # Verify metrics are registered (no exception means success).
    assert m._step_duration is not None
    assert m._invocation_total is not None
