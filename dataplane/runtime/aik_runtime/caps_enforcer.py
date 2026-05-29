"""Caps Enforcer — enforces maxSteps, maxToolCalls, timeout, and determinism.

Wraps the AgentRuntime.invoke() to enforce runtime caps:
- max_steps: enforced inside the FSM (step count check per iteration)
- max_tool_calls: enforced via CappedToolExecutor wrapper
- timeout: cancels in-flight model/tool calls on timeout, returns status=timeout
- determinism: passed through by the FSM to model calls via RuntimeConfig.determinism
"""

from __future__ import annotations

import asyncio
from typing import Any

from aik_runtime.exceptions import MaxToolCallsExceededError
from aik_runtime.fsm import (
    AgentRuntime,
    InvocationResult,
    ModelClient,
    RuntimeConfig,
    ToolExecutor,
)
from aik_runtime.models import ModelResponse, ToolCallRequest, ToolCallResult


class CappedToolExecutor:
    """Wraps a ToolExecutor to enforce max_tool_calls."""

    def __init__(self, inner: ToolExecutor, max_tool_calls: int) -> None:
        self._inner = inner
        self._max_tool_calls = max_tool_calls
        self._call_count = 0

    @property
    def call_count(self) -> int:
        return self._call_count

    async def execute(self, request: ToolCallRequest) -> ToolCallResult:
        if self._max_tool_calls > 0 and self._call_count >= self._max_tool_calls:
            raise MaxToolCallsExceededError(self._max_tool_calls)
        self._call_count += 1
        return await self._inner.execute(request)


class CapsEnforcedRuntime:
    """Agent Runtime with caps enforcement.

    Wraps the base AgentRuntime to enforce:
    - max_steps: delegated to FSM internals
    - max_tool_calls: refuses new tool calls at limit via CappedToolExecutor
    - timeout: cancels in-flight operations on timeout
    - determinism: delegated to FSM internals (passes params to model calls)
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
        """Invoke with caps enforcement."""
        # Wrap tool executor with max_tool_calls enforcement
        capped_tools = CappedToolExecutor(self._tools, config.max_tool_calls)

        # Create runtime with capped tool executor
        runtime = AgentRuntime(self._model, capped_tools)

        # Apply timeout if configured
        timeout = config.timeout if config.timeout > 0 else None

        try:
            if timeout:
                result = await asyncio.wait_for(
                    runtime.invoke(messages, config, invocation_id),
                    timeout=timeout,
                )
            else:
                result = await runtime.invoke(messages, config, invocation_id)
        except asyncio.TimeoutError:
            return InvocationResult(
                status="timeout",
                response="",
                error=f"Execution timed out after {config.timeout}s",
                total_latency_ms=config.timeout * 1000,
            )
        except MaxToolCallsExceededError as e:
            return InvocationResult(
                status="error",
                response="",
                error=str(e),
            )

        return result
