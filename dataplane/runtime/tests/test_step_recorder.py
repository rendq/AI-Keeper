"""Tests for StepRecorder — unit tests for step recording logic.

Validates: Requirements B5.6
"""

from __future__ import annotations

import asyncio
from typing import Any

import pytest

from aik_runtime.models import StepKind
from aik_runtime.step_recorder import StepRecorder


class TestStepRecorderSync:
    """Tests for synchronous step recording."""

    def test_record_step_basic(self) -> None:
        recorder = StepRecorder(invocation_id="inv-100")
        recorder.record_step_sync(
            StepKind.MODEL_CALL,
            tokens_in=50,
            tokens_out=25,
            latency_ms=123.45,
        )

        assert len(recorder.steps) == 1
        step = recorder.steps[0]
        assert step.step_type == StepKind.MODEL_CALL
        assert step.tokens_in == 50
        assert step.tokens_out == 25
        assert step.latency_ms == 123.45
        assert step.error is None

    def test_record_step_with_error(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(
            StepKind.TOOL_CALL,
            tokens_in=0,
            tokens_out=0,
            latency_ms=50.0,
            error="connection refused",
        )

        step = recorder.steps[0]
        assert step.error == "connection refused"

    def test_multiple_steps(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(StepKind.THOUGHT, tokens_in=10, tokens_out=5)
        recorder.record_step_sync(StepKind.TOOL_CALL, tokens_in=0, tokens_out=0)
        recorder.record_step_sync(StepKind.OBSERVATION, tokens_in=0, tokens_out=0)
        recorder.record_step_sync(StepKind.FINAL, tokens_in=20, tokens_out=15)

        assert len(recorder.steps) == 4
        types = [s.step_type for s in recorder.steps]
        assert types == [StepKind.THOUGHT, StepKind.TOOL_CALL, StepKind.OBSERVATION, StepKind.FINAL]

    def test_total_tokens(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(StepKind.MODEL_CALL, tokens_in=100, tokens_out=50)
        recorder.record_step_sync(StepKind.MODEL_CALL, tokens_in=200, tokens_out=100)

        assert recorder.total_tokens_in == 300
        assert recorder.total_tokens_out == 150

    def test_to_audit_steps(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(
            StepKind.MODEL_CALL,
            tokens_in=10,
            tokens_out=5,
            latency_ms=42.567,
        )

        audit_steps = recorder.to_audit_steps()
        assert len(audit_steps) == 1
        step = audit_steps[0]
        assert step["step_type"] == "model_call"
        assert step["tokens_in"] == 10
        assert step["tokens_out"] == 5
        assert step["latency_ms"] == 42.57  # rounded to 2 decimals
        assert "error" not in step

    def test_to_audit_steps_with_error(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(
            StepKind.FINAL,
            error="timeout exceeded",
            latency_ms=5000.0,
        )

        audit_steps = recorder.to_audit_steps()
        assert audit_steps[0]["error"] == "timeout exceeded"

    def test_step_ids_are_unique(self) -> None:
        recorder = StepRecorder()
        recorder.record_step_sync(StepKind.MODEL_CALL)
        recorder.record_step_sync(StepKind.MODEL_CALL)
        recorder.record_step_sync(StepKind.MODEL_CALL)

        ids = [s.step_id for s in recorder.steps]
        assert len(set(ids)) == 3  # all unique


class TestStepRecorderContext:
    """Tests for context manager based step recording."""

    async def test_async_context_records_latency(self) -> None:
        recorder = StepRecorder()

        async with recorder.start_step(StepKind.MODEL_CALL) as ctx:
            await asyncio.sleep(0.01)  # 10ms
            ctx.tokens_in = 30
            ctx.tokens_out = 15

        assert len(recorder.steps) == 1
        step = recorder.steps[0]
        assert step.tokens_in == 30
        assert step.tokens_out == 15
        assert step.latency_ms >= 5  # at least some measurable time

    async def test_async_context_captures_exception(self) -> None:
        recorder = StepRecorder()

        with pytest.raises(ValueError):
            async with recorder.start_step(StepKind.TOOL_CALL) as ctx:
                raise ValueError("tool failed")

        assert len(recorder.steps) == 1
        assert recorder.steps[0].error == "tool failed"

    def test_sync_context_records_latency(self) -> None:
        recorder = StepRecorder()

        import time

        with recorder.start_step(StepKind.THOUGHT) as ctx:
            time.sleep(0.005)
            ctx.tokens_in = 5

        assert len(recorder.steps) == 1
        assert recorder.steps[0].latency_ms >= 1


class TestStepRecorderInvocationId:
    """Test invocation ID tracking."""

    def test_invocation_id_stored(self) -> None:
        recorder = StepRecorder(invocation_id="test-inv-123")
        assert recorder.invocation_id == "test-inv-123"

    def test_default_invocation_id_empty(self) -> None:
        recorder = StepRecorder()
        assert recorder.invocation_id == ""
