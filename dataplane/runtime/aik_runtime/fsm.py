"""Agent Runtime FSM — async state machines for react & tool_calling patterns.

Implements the two P0 patterns:
- react: ReAct loop (think → act → observe → repeat until final answer)
- tool_calling: Simple tool calling loop (model decides tools → call tools → return)

Other patterns (plan_execute, reflection, workflow, multi_agent) raise
UnsupportedPatternError in P0.
"""

from __future__ import annotations

import asyncio
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, Protocol

from aik_runtime.exceptions import MaxStepsExceededError, UnsupportedPatternError
from aik_runtime.models import (
    ModelResponse,
    RuntimeState,
    StepKind,
    ToolCallRequest,
    ToolCallResult,
)
from aik_runtime.step_recorder import StepRecorder


# --- Protocols for dependencies (injected by caller) ---


class ModelClient(Protocol):
    """Protocol for calling an LLM model."""

    async def call(
        self,
        messages: list[dict[str, str]],
        tools: list[dict[str, Any]] | None = None,
        *,
        temperature: float | None = None,
        top_p: float | None = None,
        seed: int | None = None,
    ) -> ModelResponse: ...


class ToolExecutor(Protocol):
    """Protocol for executing tool calls."""

    async def execute(self, request: ToolCallRequest) -> ToolCallResult: ...


# --- Configuration ---


@dataclass
class DeterminismConfig:
    """Determinism parameters passed through to LLM calls."""

    temperature: float | None = None
    top_p: float | None = None
    seed: int | None = None


@dataclass
class RuntimeConfig:
    """Configuration for the agent runtime."""

    pattern: str = "tool_calling"
    max_steps: int = 0  # 0 = unlimited
    max_tool_calls: int = 0  # 0 = unlimited
    timeout: float = 0.0  # 0 = unlimited, in seconds
    tools: list[dict[str, Any]] = field(default_factory=list)
    determinism: DeterminismConfig = field(default_factory=DeterminismConfig)


# --- Invocation result ---


@dataclass
class InvocationResult:
    """Final result of an agent invocation."""

    status: str = "success"  # success / error / timeout / blocked
    response: str = ""
    error: str | None = None
    steps: list[dict[str, Any]] = field(default_factory=list)
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    total_latency_ms: float = 0.0


# --- Supported patterns ---

_SUPPORTED_PATTERNS = {"react", "tool_calling"}

_UNSUPPORTED_PATTERNS = {
    "plan_execute",
    "reflection",
    "workflow",
    "multi_agent",
}


# --- FSM Base ---


class PatternFSM(ABC):
    """Base class for pattern-specific finite state machines."""

    def __init__(
        self,
        model_client: ModelClient,
        tool_executor: ToolExecutor,
        recorder: StepRecorder,
        config: RuntimeConfig,
    ) -> None:
        self._model = model_client
        self._tools = tool_executor
        self._recorder = recorder
        self._config = config
        self._state = RuntimeState.IDLE
        self._step_count = 0
        self._tool_call_count = 0

    @property
    def state(self) -> RuntimeState:
        return self._state

    @abstractmethod
    async def run(self, messages: list[dict[str, str]]) -> InvocationResult:
        """Execute the pattern FSM and return the result."""
        ...


# --- Tool Calling Pattern ---


class ToolCallingFSM(PatternFSM):
    """Simple tool calling loop: model decides tools → call tools → return.

    State transitions:
    IDLE → CALLING_MODEL → (if tools) CALLING_TOOL → OBSERVING → CALLING_MODEL → ... → COMPLETED
    IDLE → CALLING_MODEL → (no tools) COMPLETED
    """

    async def run(self, messages: list[dict[str, str]]) -> InvocationResult:
        self._state = RuntimeState.CALLING_MODEL
        conversation = list(messages)

        try:
            while True:
                self._step_count += 1

                # Enforce max_steps
                if (
                    self._config.max_steps > 0
                    and self._step_count > self._config.max_steps
                ):
                    raise MaxStepsExceededError(self._config.max_steps)

                # Call model
                model_response = await self._call_model(conversation)

                # If no tool calls, we're done
                if not model_response.tool_calls:
                    self._state = RuntimeState.COMPLETED
                    self._record_final(model_response.content)
                    return self._build_result(model_response.content)

                # Execute tool calls
                tool_results = await self._execute_tools(model_response.tool_calls)

                # Add tool results as observations
                for result in tool_results:
                    self._record_observation(result)
                    conversation.append({
                        "role": "tool",
                        "content": result.output or result.error or "",
                        "tool_call_id": result.tool_id,
                    })

        except MaxStepsExceededError:
            self._state = RuntimeState.COMPLETED
            self._recorder.record_step_sync(
                StepKind.FINAL, payload={"reason": "max_steps_exceeded"}
            )
            return InvocationResult(
                status="max_steps",
                response="",
                error=f"Max steps exceeded: {self._config.max_steps}",
                steps=self._recorder.to_audit_steps(),
                total_tokens_in=self._recorder.total_tokens_in,
                total_tokens_out=self._recorder.total_tokens_out,
                total_latency_ms=self._recorder.total_latency_ms,
            )
        except Exception as e:
            self._state = RuntimeState.FAILED
            self._recorder.record_step_sync(
                StepKind.FINAL, error=str(e)
            )
            return self._build_result("", error=str(e))

    async def _call_model(
        self, messages: list[dict[str, str]]
    ) -> ModelResponse:
        async with self._recorder.start_step(StepKind.MODEL_CALL) as ctx:
            response = await self._model.call(
                messages,
                tools=self._config.tools or None,
                temperature=self._config.determinism.temperature,
                top_p=self._config.determinism.top_p,
                seed=self._config.determinism.seed,
            )
            ctx.tokens_in = response.tokens_in
            ctx.tokens_out = response.tokens_out
        return response

    async def _execute_tools(
        self, tool_calls: list[ToolCallRequest]
    ) -> list[ToolCallResult]:
        results: list[ToolCallResult] = []
        for tc in tool_calls:
            self._tool_call_count += 1
            self._state = RuntimeState.CALLING_TOOL

            async with self._recorder.start_step(StepKind.TOOL_CALL) as ctx:
                ctx.payload = {"tool_name": tc.tool_name, "tool_id": tc.tool_id}
                result = await self._tools.execute(tc)
                if result.error:
                    ctx.error = result.error
            results.append(result)
        return results

    def _record_observation(self, result: ToolCallResult) -> None:
        self._state = RuntimeState.OBSERVING
        self._recorder.record_step_sync(
            StepKind.OBSERVATION,
            payload={"tool_name": result.tool_name, "tool_id": result.tool_id},
        )

    def _record_final(self, content: str) -> None:
        self._recorder.record_step_sync(
            StepKind.FINAL,
            payload={"response_length": len(content)},
        )

    def _build_result(self, response: str, error: str | None = None) -> InvocationResult:
        return InvocationResult(
            status="error" if error else "success",
            response=response,
            error=error,
            steps=self._recorder.to_audit_steps(),
            total_tokens_in=self._recorder.total_tokens_in,
            total_tokens_out=self._recorder.total_tokens_out,
            total_latency_ms=self._recorder.total_latency_ms,
        )


# --- ReAct Pattern ---


class ReactFSM(PatternFSM):
    """ReAct loop: think → act → observe → repeat until final answer.

    State transitions:
    IDLE → THINKING → (action) CALLING_TOOL → OBSERVING → THINKING → ...
    THINKING → (final answer) COMPLETED

    The model is expected to output either:
    - A thought + tool call (action)
    - A final answer (no tool calls, content is the answer)
    """

    async def run(self, messages: list[dict[str, str]]) -> InvocationResult:
        self._state = RuntimeState.THINKING
        conversation = list(messages)

        try:
            while True:
                self._step_count += 1

                # Enforce max_steps
                if (
                    self._config.max_steps > 0
                    and self._step_count > self._config.max_steps
                ):
                    raise MaxStepsExceededError(self._config.max_steps)

                # Think: call model to get thought + action or final answer
                model_response = await self._call_model_with_thought(conversation)

                # If model provides tool calls, execute them
                if model_response.tool_calls:
                    # Record thought
                    self._record_thought(model_response.content)

                    # Execute tools (act)
                    tool_results = await self._execute_tools(model_response.tool_calls)

                    # Observe: add results back
                    for result in tool_results:
                        self._record_observation(result)
                        conversation.append({
                            "role": "tool",
                            "content": result.output or result.error or "",
                            "tool_call_id": result.tool_id,
                        })

                    # Continue thinking
                    self._state = RuntimeState.THINKING
                else:
                    # Final answer — no more actions
                    self._state = RuntimeState.COMPLETED
                    self._record_final(model_response.content)
                    return self._build_result(model_response.content)

        except MaxStepsExceededError:
            self._state = RuntimeState.COMPLETED
            self._recorder.record_step_sync(
                StepKind.FINAL, payload={"reason": "max_steps_exceeded"}
            )
            return InvocationResult(
                status="max_steps",
                response="",
                error=f"Max steps exceeded: {self._config.max_steps}",
                steps=self._recorder.to_audit_steps(),
                total_tokens_in=self._recorder.total_tokens_in,
                total_tokens_out=self._recorder.total_tokens_out,
                total_latency_ms=self._recorder.total_latency_ms,
            )
        except Exception as e:
            self._state = RuntimeState.FAILED
            self._recorder.record_step_sync(
                StepKind.FINAL, error=str(e)
            )
            return self._build_result("", error=str(e))

    async def _call_model_with_thought(
        self, messages: list[dict[str, str]]
    ) -> ModelResponse:
        self._state = RuntimeState.CALLING_MODEL
        async with self._recorder.start_step(StepKind.MODEL_CALL) as ctx:
            response = await self._model.call(
                messages,
                tools=self._config.tools or None,
                temperature=self._config.determinism.temperature,
                top_p=self._config.determinism.top_p,
                seed=self._config.determinism.seed,
            )
            ctx.tokens_in = response.tokens_in
            ctx.tokens_out = response.tokens_out
        return response

    def _record_thought(self, content: str) -> None:
        self._state = RuntimeState.THINKING
        self._recorder.record_step_sync(
            StepKind.THOUGHT,
            payload={"content_length": len(content)},
        )

    async def _execute_tools(
        self, tool_calls: list[ToolCallRequest]
    ) -> list[ToolCallResult]:
        results: list[ToolCallResult] = []
        for tc in tool_calls:
            self._tool_call_count += 1
            self._state = RuntimeState.CALLING_TOOL

            async with self._recorder.start_step(StepKind.TOOL_CALL) as ctx:
                ctx.payload = {"tool_name": tc.tool_name, "tool_id": tc.tool_id}
                result = await self._tools.execute(tc)
                if result.error:
                    ctx.error = result.error
            results.append(result)
        return results

    def _record_observation(self, result: ToolCallResult) -> None:
        self._state = RuntimeState.OBSERVING
        self._recorder.record_step_sync(
            StepKind.OBSERVATION,
            payload={"tool_name": result.tool_name, "tool_id": result.tool_id},
        )

    def _record_final(self, content: str) -> None:
        self._recorder.record_step_sync(
            StepKind.FINAL,
            payload={"response_length": len(content)},
        )

    def _build_result(self, response: str, error: str | None = None) -> InvocationResult:
        return InvocationResult(
            status="error" if error else "success",
            response=response,
            error=error,
            steps=self._recorder.to_audit_steps(),
            total_tokens_in=self._recorder.total_tokens_in,
            total_tokens_out=self._recorder.total_tokens_out,
            total_latency_ms=self._recorder.total_latency_ms,
        )


# --- Main entry point ---


class AgentRuntime:
    """Top-level Agent Runtime — dispatches to pattern-specific FSM.

    Validates the requested pattern and delegates execution to the
    appropriate FSM implementation.
    """

    def __init__(
        self,
        model_client: ModelClient,
        tool_executor: ToolExecutor,
    ) -> None:
        self._model = model_client
        self._tools = tool_executor

    async def invoke(
        self,
        messages: list[dict[str, str]],
        config: RuntimeConfig,
        invocation_id: str = "",
    ) -> InvocationResult:
        """Invoke the agent runtime with the given messages and config.

        Args:
            messages: Conversation messages.
            config: Runtime configuration including pattern.
            invocation_id: Unique invocation ID for audit trail.

        Returns:
            InvocationResult with steps, tokens, and response.

        Raises:
            UnsupportedPatternError: If pattern is not react or tool_calling.
        """
        pattern = config.pattern.lower()

        if pattern not in _SUPPORTED_PATTERNS:
            raise UnsupportedPatternError(pattern)

        recorder = StepRecorder(invocation_id=invocation_id)

        fsm: PatternFSM
        if pattern == "tool_calling":
            fsm = ToolCallingFSM(self._model, self._tools, recorder, config)
        elif pattern == "react":
            fsm = ReactFSM(self._model, self._tools, recorder, config)
        else:
            raise UnsupportedPatternError(pattern)

        return await fsm.run(messages)
