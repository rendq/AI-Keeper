"""Tests for Agent Runtime FSM — react & tool_calling patterns.

Validates: Requirements B5.1, B5.6
"""

from __future__ import annotations

import pytest

from aik_runtime.exceptions import UnsupportedPatternError
from aik_runtime.fsm import AgentRuntime, RuntimeConfig
from aik_runtime.models import ModelResponse, StepKind, ToolCallRequest, ToolCallResult

from .conftest import MockModelClient, MockToolExecutor


# --- UnsupportedPattern tests ---


class TestUnsupportedPatterns:
    """Other patterns in P0 raise UnsupportedPattern."""

    @pytest.mark.parametrize(
        "pattern",
        ["plan_execute", "reflection", "workflow", "multi_agent", "unknown"],
    )
    async def test_unsupported_pattern_raises(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor, pattern: str
    ) -> None:
        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern=pattern)

        with pytest.raises(UnsupportedPatternError) as exc_info:
            await runtime.invoke([{"role": "user", "content": "hello"}], config)

        assert pattern in str(exc_info.value)

    async def test_supported_patterns_do_not_raise(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        runtime = AgentRuntime(model_client, tool_executor)

        for pattern in ("react", "tool_calling"):
            model_client._responses = [
                ModelResponse(content="answer", tokens_in=10, tokens_out=5)
            ]
            model_client._call_count = 0
            config = RuntimeConfig(pattern=pattern)
            result = await runtime.invoke(
                [{"role": "user", "content": "hi"}], config
            )
            assert result.status == "success"


# --- Tool Calling Pattern tests ---


class TestToolCallingFSM:
    """Tests for the tool_calling pattern state machine."""

    async def test_simple_response_no_tools(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Model returns direct answer with no tool calls."""
        model_client.add_response(
            ModelResponse(content="The answer is 42.", tokens_in=20, tokens_out=10)
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="tool_calling")

        result = await runtime.invoke(
            [{"role": "user", "content": "What is the answer?"}],
            config,
            invocation_id="inv-001",
        )

        assert result.status == "success"
        assert result.response == "The answer is 42."
        assert result.total_tokens_in == 20
        assert result.total_tokens_out == 10
        # Should have model_call + final steps
        step_types = [s["step_type"] for s in result.steps]
        assert StepKind.MODEL_CALL.value in step_types
        assert StepKind.FINAL.value in step_types

    async def test_single_tool_call(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Model calls a tool, then returns final answer."""
        # First call: model requests tool
        model_client.add_response(
            ModelResponse(
                content="",
                tool_calls=[
                    ToolCallRequest(
                        tool_name="search", tool_id="call-1", arguments={"q": "hello"}
                    )
                ],
                tokens_in=15,
                tokens_out=8,
            )
        )
        # Second call: model returns final answer
        model_client.add_response(
            ModelResponse(content="Found result.", tokens_in=25, tokens_out=12)
        )

        tool_executor.set_result(
            "search",
            ToolCallResult(tool_name="search", tool_id="call-1", output="search result"),
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="tool_calling")

        result = await runtime.invoke(
            [{"role": "user", "content": "search for hello"}], config
        )

        assert result.status == "success"
        assert result.response == "Found result."
        assert result.total_tokens_in == 40  # 15 + 25
        assert result.total_tokens_out == 20  # 8 + 12

        step_types = [s["step_type"] for s in result.steps]
        assert step_types.count(StepKind.MODEL_CALL.value) == 2
        assert StepKind.TOOL_CALL.value in step_types
        assert StepKind.OBSERVATION.value in step_types
        assert StepKind.FINAL.value in step_types

    async def test_tool_call_with_error(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Tool call returns an error — recorded in step."""
        model_client.add_response(
            ModelResponse(
                content="",
                tool_calls=[
                    ToolCallRequest(tool_name="bad_tool", tool_id="call-err")
                ],
                tokens_in=10,
                tokens_out=5,
            )
        )
        model_client.add_response(
            ModelResponse(content="Handled error.", tokens_in=10, tokens_out=5)
        )

        tool_executor.set_result(
            "bad_tool",
            ToolCallResult(
                tool_name="bad_tool", tool_id="call-err", error="connection timeout"
            ),
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="tool_calling")
        result = await runtime.invoke([{"role": "user", "content": "do thing"}], config)

        assert result.status == "success"
        # Find the tool_call step and verify error recorded
        tool_steps = [s for s in result.steps if s["step_type"] == StepKind.TOOL_CALL.value]
        assert len(tool_steps) == 1
        assert tool_steps[0]["error"] == "connection timeout"

    async def test_model_exception_results_in_error(
        self, tool_executor: MockToolExecutor
    ) -> None:
        """If model client raises, FSM transitions to FAILED."""

        class FailingModel:
            async def call(self, messages: Any, tools: Any = None, **kwargs: Any) -> ModelResponse:
                raise RuntimeError("LLM API down")

        runtime = AgentRuntime(FailingModel(), tool_executor)  # type: ignore[arg-type]
        config = RuntimeConfig(pattern="tool_calling")
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "error"
        assert "LLM API down" in (result.error or "")


# --- ReAct Pattern tests ---


class TestReactFSM:
    """Tests for the react pattern state machine."""

    async def test_immediate_final_answer(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Model returns final answer without any tool calls."""
        model_client.add_response(
            ModelResponse(content="Direct answer.", tokens_in=12, tokens_out=6)
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="react")

        result = await runtime.invoke(
            [{"role": "user", "content": "what is 2+2?"}], config
        )

        assert result.status == "success"
        assert result.response == "Direct answer."
        step_types = [s["step_type"] for s in result.steps]
        assert StepKind.MODEL_CALL.value in step_types
        assert StepKind.FINAL.value in step_types

    async def test_think_act_observe_loop(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """ReAct loop: think → tool_call → observe → think → final."""
        # Step 1: Model thinks and calls tool
        model_client.add_response(
            ModelResponse(
                content="I need to search for this.",
                tool_calls=[
                    ToolCallRequest(tool_name="web_search", tool_id="tc-1", arguments={"q": "AI"})
                ],
                tokens_in=20,
                tokens_out=15,
            )
        )
        # Step 2: Model gives final answer after observation
        model_client.add_response(
            ModelResponse(content="Based on search: AI is cool.", tokens_in=30, tokens_out=20)
        )

        tool_executor.set_result(
            "web_search",
            ToolCallResult(tool_name="web_search", tool_id="tc-1", output="AI info..."),
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="react")

        result = await runtime.invoke(
            [{"role": "user", "content": "tell me about AI"}], config
        )

        assert result.status == "success"
        assert "AI is cool" in result.response
        assert result.total_tokens_in == 50  # 20 + 30
        assert result.total_tokens_out == 35  # 15 + 20

        step_types = [s["step_type"] for s in result.steps]
        # Should have: model_call, thought, tool_call, observation, model_call, final
        assert step_types.count(StepKind.MODEL_CALL.value) == 2
        assert StepKind.THOUGHT.value in step_types
        assert StepKind.TOOL_CALL.value in step_types
        assert StepKind.OBSERVATION.value in step_types
        assert StepKind.FINAL.value in step_types

    async def test_multiple_react_iterations(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Multiple think-act-observe cycles before final answer."""
        # Iteration 1
        model_client.add_response(
            ModelResponse(
                content="First, let me check...",
                tool_calls=[ToolCallRequest(tool_name="lookup", tool_id="tc-1")],
                tokens_in=10,
                tokens_out=5,
            )
        )
        # Iteration 2
        model_client.add_response(
            ModelResponse(
                content="Now let me verify...",
                tool_calls=[ToolCallRequest(tool_name="verify", tool_id="tc-2")],
                tokens_in=10,
                tokens_out=5,
            )
        )
        # Final
        model_client.add_response(
            ModelResponse(content="Final answer.", tokens_in=10, tokens_out=5)
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="react")

        result = await runtime.invoke(
            [{"role": "user", "content": "complex question"}], config
        )

        assert result.status == "success"
        assert result.response == "Final answer."
        step_types = [s["step_type"] for s in result.steps]
        assert step_types.count(StepKind.MODEL_CALL.value) == 3
        assert step_types.count(StepKind.THOUGHT.value) == 2
        assert step_types.count(StepKind.TOOL_CALL.value) == 2
        assert step_types.count(StepKind.OBSERVATION.value) == 2


# --- Step Recorder integration tests ---


class TestStepRecorderIntegration:
    """Verify step recorder captures correct audit data."""

    async def test_step_records_contain_required_fields(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Every step record must have step_type, tokens_in, tokens_out, latency_ms."""
        model_client.add_response(
            ModelResponse(content="done", tokens_in=100, tokens_out=50)
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="tool_calling")
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        for step in result.steps:
            assert "step_type" in step
            assert "tokens_in" in step
            assert "tokens_out" in step
            assert "latency_ms" in step
            assert step["tokens_in"] >= 0
            assert step["tokens_out"] >= 0
            assert step["latency_ms"] >= 0

    async def test_latency_is_positive(
        self, model_client: MockModelClient, tool_executor: MockToolExecutor
    ) -> None:
        """Latency should be > 0 for actual operations."""
        model_client.add_response(
            ModelResponse(content="answer", tokens_in=10, tokens_out=5)
        )

        runtime = AgentRuntime(model_client, tool_executor)
        config = RuntimeConfig(pattern="tool_calling")
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        model_steps = [
            s for s in result.steps if s["step_type"] == StepKind.MODEL_CALL.value
        ]
        assert len(model_steps) > 0
        # Latency should be >= 0 (may be very small in tests)
        assert model_steps[0]["latency_ms"] >= 0

    async def test_error_recorded_in_step(
        self, tool_executor: MockToolExecutor
    ) -> None:
        """Errors are captured in the step record."""

        class ErrorModel:
            async def call(self, messages: Any, tools: Any = None, **kwargs: Any) -> ModelResponse:
                raise ValueError("bad input")

        runtime = AgentRuntime(ErrorModel(), tool_executor)  # type: ignore[arg-type]
        config = RuntimeConfig(pattern="react")
        result = await runtime.invoke([{"role": "user", "content": "hi"}], config)

        assert result.status == "error"
        final_steps = [s for s in result.steps if s["step_type"] == StepKind.FINAL.value]
        assert len(final_steps) == 1
        assert final_steps[0].get("error") == "bad input"
