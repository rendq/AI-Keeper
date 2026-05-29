"""Step Recorder — records each execution step for AuditEvent.spec.steps[].

Each step (thought / tool_call / model_call / observation / final) is recorded
with tokensIn, tokensOut, latencyMs, and optional error information.
"""

from __future__ import annotations

import uuid
from typing import Any

from aik_runtime.models import StepKind, StepRecord, Timer


class StepRecorder:
    """Records execution steps for audit trail.

    Usage:
        recorder = StepRecorder(invocation_id="inv-123")
        async with recorder.record_step(StepKind.MODEL_CALL) as step:
            # ... perform model call ...
            step.tokens_in = 100
            step.tokens_out = 50
    """

    def __init__(self, invocation_id: str = "") -> None:
        self._invocation_id = invocation_id
        self._steps: list[StepRecord] = []

    @property
    def invocation_id(self) -> str:
        return self._invocation_id

    @property
    def steps(self) -> list[StepRecord]:
        """Return all recorded steps (immutable view)."""
        return list(self._steps)

    def record_step_sync(
        self,
        step_type: StepKind,
        *,
        tokens_in: int = 0,
        tokens_out: int = 0,
        latency_ms: float = 0.0,
        error: str | None = None,
        payload: dict[str, Any] | None = None,
    ) -> StepRecord:
        """Record a step synchronously (used after completion)."""
        record = StepRecord(
            step_type=step_type,
            tokens_in=tokens_in,
            tokens_out=tokens_out,
            latency_ms=latency_ms,
            error=error,
            step_id=self._generate_step_id(),
            payload=payload or {},
        )
        self._steps.append(record)
        return record

    def start_step(self, step_type: StepKind) -> _StepContext:
        """Start recording a step, returns a context that must be finished."""
        return _StepContext(self, step_type)

    def to_audit_steps(self) -> list[dict[str, Any]]:
        """Serialize all steps to audit-compatible dicts."""
        return [step.to_audit_dict() for step in self._steps]

    @property
    def total_tokens_in(self) -> int:
        return sum(s.tokens_in for s in self._steps)

    @property
    def total_tokens_out(self) -> int:
        return sum(s.tokens_out for s in self._steps)

    @property
    def total_latency_ms(self) -> float:
        return sum(s.latency_ms for s in self._steps)

    def _generate_step_id(self) -> str:
        return f"step-{uuid.uuid4().hex[:8]}"

    def _append(self, record: StepRecord) -> None:
        self._steps.append(record)


class _StepContext:
    """Context manager for timing a step."""

    def __init__(self, recorder: StepRecorder, step_type: StepKind) -> None:
        self._recorder = recorder
        self._step_type = step_type
        self._timer = Timer()
        self.tokens_in: int = 0
        self.tokens_out: int = 0
        self.error: str | None = None
        self.payload: dict[str, Any] = {}

    def __enter__(self) -> _StepContext:
        self._timer.start()
        return self

    def __exit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        latency_ms = self._timer.elapsed_ms()
        error = self.error
        if exc_val is not None and error is None:
            error = str(exc_val)
        self._recorder.record_step_sync(
            self._step_type,
            tokens_in=self.tokens_in,
            tokens_out=self.tokens_out,
            latency_ms=latency_ms,
            error=error,
            payload=self.payload if self.payload else None,
        )

    async def __aenter__(self) -> _StepContext:
        self._timer.start()
        return self

    async def __aexit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        self.__exit__(exc_type, exc_val, exc_tb)
