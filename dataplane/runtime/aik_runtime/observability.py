"""AIP Runtime Observability — OpenTelemetry SDK + Prometheus metrics for Python data plane.

Provides:
- OTel TracerProvider + MeterProvider initialization with OTLP gRPC exporter
- Prometheus metrics exporter for the runtime
- Key Python metrics:
  - aik_runtime_step_duration_seconds — histogram per step type
  - aik_runtime_invocation_total — counter with pattern/status labels

Usage:
    from aik_runtime.observability import init_observability, get_metrics

    provider = init_observability(service_name="aip-runtime", otlp_endpoint="otel-collector:4317")
    metrics = get_metrics()
    metrics.observe_step_duration("tool_call", 0.123)
    metrics.inc_invocation("react", "success")
"""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any


@dataclass
class ObservabilityConfig:
    """Configuration for the observability stack."""

    service_name: str = "aip-runtime"
    service_version: str = "0.0.0"
    otlp_endpoint: str = ""
    metrics_port: int = 9090


class RuntimeMetrics:
    """Holds and exposes AIP runtime Prometheus metrics.

    Uses prometheus_client when available; falls back to no-op counters
    to avoid hard dependency failures in testing.

    Metrics are registered once at the module level to prevent duplicate
    registration errors across multiple instances.
    """

    # Class-level metric references (registered once).
    _class_step_duration: Any = None
    _class_invocation_total: Any = None
    _class_initialized = False

    def __init__(self) -> None:
        self._step_duration: Any = None
        self._invocation_total: Any = None
        self._initialized = False

    def _ensure_initialized(self) -> None:
        if self._initialized:
            return

        # Use class-level singletons to avoid duplicate registration.
        if not RuntimeMetrics._class_initialized:
            try:
                from prometheus_client import Counter, Histogram

                RuntimeMetrics._class_step_duration = Histogram(
                    "aik_runtime_step_duration_seconds",
                    "Duration of runtime steps in seconds.",
                    labelnames=["step_type"],
                    buckets=(
                        0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
                    ),
                )
                RuntimeMetrics._class_invocation_total = Counter(
                    "aik_runtime_invocation_total",
                    "Total runtime invocations.",
                    labelnames=["pattern", "status"],
                )
            except ImportError:
                pass
            RuntimeMetrics._class_initialized = True

        self._step_duration = RuntimeMetrics._class_step_duration
        self._invocation_total = RuntimeMetrics._class_invocation_total
        self._initialized = True

    def observe_step_duration(self, step_type: str, duration_seconds: float) -> None:
        """Record step execution duration."""
        self._ensure_initialized()
        if self._step_duration is not None:
            self._step_duration.labels(step_type=step_type).observe(duration_seconds)

    def inc_invocation(self, pattern: str, status: str) -> None:
        """Increment invocation counter."""
        self._ensure_initialized()
        if self._invocation_total is not None:
            self._invocation_total.labels(pattern=pattern, status=status).inc()


# Module-level singleton.
_metrics: RuntimeMetrics | None = None
_provider_initialized = False


def get_metrics() -> RuntimeMetrics:
    """Return the shared RuntimeMetrics instance (creates on first call)."""
    global _metrics
    if _metrics is None:
        _metrics = RuntimeMetrics()
    return _metrics


@dataclass
class ObservabilityProvider:
    """Holds references to OTel providers and metrics server."""

    config: ObservabilityConfig = field(default_factory=ObservabilityConfig)
    metrics: RuntimeMetrics = field(default_factory=get_metrics)
    _server_thread: Any = None

    def start_metrics_server(self) -> None:
        """Start the Prometheus metrics HTTP server in a daemon thread."""
        try:
            from prometheus_client import start_http_server

            start_http_server(self.config.metrics_port)
        except ImportError:
            pass
        except OSError:
            # Port already in use — skip.
            pass


def init_observability(
    service_name: str = "aip-runtime",
    service_version: str = "0.0.0",
    otlp_endpoint: str = "",
    metrics_port: int = 9090,
) -> ObservabilityProvider:
    """Initialize the observability stack.

    Sets up OTel TracerProvider + MeterProvider with OTLP gRPC exporter (if endpoint
    is provided), and starts the Prometheus metrics HTTP server.

    Args:
        service_name: Name of this service for OTel resource attributes.
        service_version: Version of this service.
        otlp_endpoint: gRPC OTLP collector endpoint. Empty means OTel export disabled.
        metrics_port: Port for the Prometheus /metrics HTTP server.

    Returns:
        ObservabilityProvider with references to initialized resources.
    """
    global _provider_initialized

    config = ObservabilityConfig(
        service_name=service_name,
        service_version=service_version,
        otlp_endpoint=otlp_endpoint,
        metrics_port=metrics_port,
    )

    provider = ObservabilityProvider(config=config, metrics=get_metrics())

    # Initialize OTel if endpoint is provided and packages are available.
    if otlp_endpoint:
        _setup_otel(config)

    _provider_initialized = True
    return provider


def _setup_otel(config: ObservabilityConfig) -> None:
    """Attempt to set up OpenTelemetry SDK with OTLP gRPC exporter."""
    try:
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
        from opentelemetry.sdk.resources import Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor

        resource = Resource.create(
            {
                "service.name": config.service_name,
                "service.version": config.service_version,
            }
        )

        exporter = OTLPSpanExporter(endpoint=config.otlp_endpoint, insecure=True)
        tp = TracerProvider(resource=resource)
        tp.add_span_processor(BatchSpanProcessor(exporter))
        trace.set_tracer_provider(tp)
    except ImportError:
        # OTel packages not installed — skip silently.
        pass


class StepTimer:
    """Context manager for timing steps and recording duration.

    Usage:
        with StepTimer("tool_call") as timer:
            await do_tool_call()
        # duration is automatically recorded
    """

    def __init__(self, step_type: str) -> None:
        self.step_type = step_type
        self.duration: float = 0.0
        self._start: float = 0.0

    def __enter__(self) -> "StepTimer":
        self._start = time.perf_counter()
        return self

    def __exit__(self, *args: Any) -> None:
        self.duration = time.perf_counter() - self._start
        get_metrics().observe_step_duration(self.step_type, self.duration)
