"""Tests for Caps Enforcer — maxSteps, maxToolCalls, timeout, determinism.

Validates Requirements: B5.2, B5.3, B5.4, B5.5
"""

from __future__ import annotations

import asyncio
from typing import Any

import pytest

from aik_runtime.caps_enforcer import CapsEnforcedRuntime
from aik_runtime.fsm import AgentRuntime, DeterminismConfig, RuntimeConfig
from aik_runtime.models import ModelResponse, ToolCallRequest, ToolCallResult

from .conftest import MockModelClient, MockToolExecutor


# --- Helpers ---


class SlowModelClient:
    """Model client that introduces a delay to test timeout."""

    def __init__(self, delay: float, response: ModelResponse | None = None) -> None:
        self._delay = delay
        self._response = response or ModelResponse(
            content="slow response", tokens_in=10, tokens_out=5
        )
        self.calls: list[dict[str, Any]] = []
        self.was_cancelled = False

    async def call(
        self,
        messages: list[dict[str, str]],
        tools: list[dict[str, Any]] | None = None,
        *,
        temperature: float | None = None,
        top_p: float | None = None,
        seed: int | None = None,
    ) -> ModelResponse:
        self.calls.append({
            "messages": messages,
            "tools": tools,
            "temperature": temperature,
            "top_p": top_p,
            "seed": seed,
        })
        try:
            await asyncio.sleep(self._delay)
        except asyncio.CancelledError:
            self.was_cancelled = True
            raise
        return self._response


class SlowToolExecutor:
    """Tool executor that introduces a delay to test timeout."""

    def __init__(self, delay: float) -> None:
        self._delay = delay
        self.was_cancelled = False

    async def execute(self, request: ToolCallRequest) -> ToolCallResult:
        try:
            await asyncio.sleep(self._delay)
        except asyncio.CancelledError:
            self.was_cancelled = True
            raise
        return ToolCallResult(
            tool_name=request.tool_name,
            tool_id=request.tool_id,
            output="done",
        )


# --- B5.2: maxSteps enforcement ---


class TestMaxSteps:
    """Tests for runtime.maxSteps enforcement (B5.2)."""

    async def test_max_steps_unlimited_completes_normally(self) -> None:
        """When max_steps=0 (unlimited), execution completes normally."""
        model = MockModelClient([
            ModelResponse(
                content="thinking",
                tool_calls=[ToolCallRequest(tool_name="search", tool_id="t1")],
                tokens_in=10,
                tokens_out=5,
            ),
            ModelResponse(content="final answer", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_steps=0)

        runtime = AgentRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "success"
        assert result.response == "final answer"

    async def test_max_steps_reached_forces_end(self) -> None:
        """When step count exceeds max_steps, execution is forced to end."""
        # Model always returns tool calls, never a final answer
        responses = [
            ModelResponse(
                content="step",
                tool_calls=[ToolCallRequest(tool_name="search", tool_id=f"t{i}")],
                tokens_in=10,
                tokens_out=5,
            )
            for i in range(20)
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_steps=3)

        runtime = AgentRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "max_steps"
        assert "Max steps exceeded" in (result.error or "")

    async def test_max_steps_react_pattern(self) -> None:
        """max_steps enforcement works in react pattern too."""
        responses = [
            ModelResponse(
                content="thought",
                tool_calls=[ToolCallRequest(tool_name="act", tool_id=f"t{i}")],
                tokens_in=10,
                tokens_out=5,
            )
            for i in range(20)
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="react", max_steps=2)

        runtime = AgentRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "max_steps"

    async def test_max_steps_exactly_at_limit_still_executes(self) -> None:
        """Execution runs for exactly max_steps iterations before stopping."""
        responses = [
            ModelResponse(
                content="step",
                tool_calls=[ToolCallRequest(tool_name="s", tool_id=f"t{i}")],
                tokens_in=10,
                tokens_out=5,
            )
            for i in range(10)
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_steps=3)

        runtime = AgentRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        # Should have executed 3 steps, then stopped on 4th attempt
        assert result.status == "max_steps"
        # Model was called 3 times (steps 1, 2, 3 executed, step 4 refused)
        assert len(model.calls) == 3


# --- B5.3: maxToolCalls enforcement ---


class TestMaxToolCalls:
    """Tests for runtime.maxToolCalls enforcement (B5.3)."""

    async def test_max_tool_calls_unlimited(self) -> None:
        """When max_tool_calls=0, tool calls are unlimited."""
        responses = [
            ModelResponse(
                content="call tools",
                tool_calls=[
                    ToolCallRequest(tool_name="t1", tool_id="id1"),
                    ToolCallRequest(tool_name="t2", tool_id="id2"),
                    ToolCallRequest(tool_name="t3", tool_id="id3"),
                ],
                tokens_in=10,
                tokens_out=5,
            ),
            ModelResponse(content="done", tokens_in=10, tokens_out=5),
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_tool_calls=0)

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "success"
        assert len(tools.calls) == 3

    async def test_max_tool_calls_reached_refuses(self) -> None:
        """When tool call count reaches max_tool_calls, refuses new calls."""
        responses = [
            ModelResponse(
                content="call tools",
                tool_calls=[
                    ToolCallRequest(tool_name="t1", tool_id="id1"),
                    ToolCallRequest(tool_name="t2", tool_id="id2"),
                    ToolCallRequest(tool_name="t3", tool_id="id3"),
                ],
                tokens_in=10,
                tokens_out=5,
            ),
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_tool_calls=2)

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "error"
        assert "Max tool calls exceeded" in (result.error or "")

    async def test_max_tool_calls_allows_exact_limit(self) -> None:
        """Exactly max_tool_calls tool calls are allowed."""
        responses = [
            ModelResponse(
                content="call tools",
                tool_calls=[
                    ToolCallRequest(tool_name="t1", tool_id="id1"),
                    ToolCallRequest(tool_name="t2", tool_id="id2"),
                ],
                tokens_in=10,
                tokens_out=5,
            ),
            ModelResponse(content="done", tokens_in=10, tokens_out=5),
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", max_tool_calls=2)

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "success"
        assert result.response == "done"


# --- B5.4: timeout enforcement ---


class TestTimeout:
    """Tests for runtime.timeout enforcement (B5.4)."""

    async def test_no_timeout_completes_normally(self) -> None:
        """Without timeout, execution completes normally."""
        model = MockModelClient([
            ModelResponse(content="answer", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", timeout=0.0)

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "success"

    async def test_timeout_cancels_slow_model_call(self) -> None:
        """When timeout exceeded during model call, returns status=timeout."""
        model = SlowModelClient(delay=5.0)  # 5s delay
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", timeout=0.1)  # 100ms timeout

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "timeout"
        assert "timed out" in (result.error or "")

    async def test_timeout_cancels_slow_tool_call(self) -> None:
        """When timeout exceeded during tool call, returns status=timeout."""
        model = MockModelClient([
            ModelResponse(
                content="use tool",
                tool_calls=[ToolCallRequest(tool_name="slow", tool_id="t1")],
                tokens_in=10,
                tokens_out=5,
            ),
        ])
        slow_tools = SlowToolExecutor(delay=5.0)
        config = RuntimeConfig(pattern="tool_calling", timeout=0.1)

        runtime = CapsEnforcedRuntime(model, slow_tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "timeout"

    async def test_timeout_records_latency(self) -> None:
        """Timeout result includes total_latency_ms approximately equal to timeout."""
        model = SlowModelClient(delay=5.0)
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling", timeout=0.05)

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "timeout"
        # Latency should be approximately timeout * 1000
        assert result.total_latency_ms == pytest.approx(50.0, abs=10.0)


# --- B5.5: determinism pass-through ---


class TestDeterminism:
    """Tests for runtime.determinism pass-through (B5.5)."""

    async def test_determinism_params_passed_to_model(self) -> None:
        """temperature, topP, seed are passed through to every LLM call."""
        model = MockModelClient([
            ModelResponse(content="answer", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(
            pattern="tool_calling",
            determinism=DeterminismConfig(temperature=0.0, top_p=0.9, seed=42),
        )

        runtime = AgentRuntime(model, tools)
        await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert len(model.calls) == 1
        assert model.calls[0]["temperature"] == 0.0
        assert model.calls[0]["top_p"] == 0.9
        assert model.calls[0]["seed"] == 42

    async def test_determinism_none_by_default(self) -> None:
        """Without determinism config, params are None (not sent)."""
        model = MockModelClient([
            ModelResponse(content="answer", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(pattern="tool_calling")

        runtime = AgentRuntime(model, tools)
        await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert model.calls[0]["temperature"] is None
        assert model.calls[0]["top_p"] is None
        assert model.calls[0]["seed"] is None

    async def test_determinism_passed_in_react_pattern(self) -> None:
        """Determinism params are passed through in react pattern too."""
        model = MockModelClient([
            ModelResponse(
                content="thought",
                tool_calls=[ToolCallRequest(tool_name="act", tool_id="t1")],
                tokens_in=10,
                tokens_out=5,
            ),
            ModelResponse(content="final", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(
            pattern="react",
            determinism=DeterminismConfig(temperature=0.5, top_p=0.95, seed=123),
        )

        runtime = AgentRuntime(model, tools)
        await runtime.invoke([{"role": "user", "content": "hi"}], config)

        # Both model calls should have determinism params
        for call in model.calls:
            assert call["temperature"] == 0.5
            assert call["top_p"] == 0.95
            assert call["seed"] == 123

    async def test_determinism_partial_params(self) -> None:
        """Only specified determinism params are sent."""
        model = MockModelClient([
            ModelResponse(content="answer", tokens_in=10, tokens_out=5),
        ])
        tools = MockToolExecutor()
        config = RuntimeConfig(
            pattern="tool_calling",
            determinism=DeterminismConfig(temperature=0.7),  # only temperature
        )

        runtime = AgentRuntime(model, tools)
        await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert model.calls[0]["temperature"] == 0.7
        assert model.calls[0]["top_p"] is None
        assert model.calls[0]["seed"] is None


# --- Combined enforcement ---


class TestCombinedEnforcement:
    """Tests for combined caps enforcement."""

    async def test_max_steps_and_max_tool_calls_together(self) -> None:
        """Both max_steps and max_tool_calls are enforced simultaneously."""
        responses = [
            ModelResponse(
                content="step",
                tool_calls=[ToolCallRequest(tool_name="s", tool_id=f"t{i}")],
                tokens_in=10,
                tokens_out=5,
            )
            for i in range(10)
        ]
        model = MockModelClient(responses)
        tools = MockToolExecutor()
        # max_steps=5 will trigger before max_tool_calls=10
        config = RuntimeConfig(
            pattern="tool_calling", max_steps=5, max_tool_calls=10
        )

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "max_steps"

    async def test_timeout_with_determinism(self) -> None:
        """Timeout and determinism work together."""
        model = SlowModelClient(delay=5.0)
        tools = MockToolExecutor()
        config = RuntimeConfig(
            pattern="tool_calling",
            timeout=0.05,
            determinism=DeterminismConfig(temperature=0.0, seed=42),
        )

        runtime = CapsEnforcedRuntime(model, tools)
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "timeout"
        # Model was called with determinism params
        assert len(model.calls) == 1
        assert model.calls[0]["temperature"] == 0.0
        assert model.calls[0]["seed"] == 42
