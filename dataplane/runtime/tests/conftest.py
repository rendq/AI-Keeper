"""Shared test fixtures for Agent Runtime tests."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

import pytest

from aik_runtime.models import ModelResponse, ToolCallRequest, ToolCallResult


class MockModelClient:
    """Mock model client that returns predefined responses."""

    def __init__(self, responses: list[ModelResponse] | None = None) -> None:
        self._responses = list(responses or [])
        self._call_count = 0
        self.calls: list[dict[str, Any]] = []

    def add_response(self, response: ModelResponse) -> None:
        self._responses.append(response)

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
        if self._call_count < len(self._responses):
            resp = self._responses[self._call_count]
            self._call_count += 1
            return resp
        # Default: return empty response (final answer)
        return ModelResponse(content="default response", tokens_in=10, tokens_out=5)


class MockToolExecutor:
    """Mock tool executor that returns predefined results."""

    def __init__(self, results: dict[str, ToolCallResult] | None = None) -> None:
        self._results = results or {}
        self.calls: list[ToolCallRequest] = []

    def set_result(self, tool_name: str, result: ToolCallResult) -> None:
        self._results[tool_name] = result

    async def execute(self, request: ToolCallRequest) -> ToolCallResult:
        self.calls.append(request)
        if request.tool_name in self._results:
            return self._results[request.tool_name]
        return ToolCallResult(
            tool_name=request.tool_name,
            tool_id=request.tool_id,
            output=f"result of {request.tool_name}",
        )


@pytest.fixture
def model_client() -> MockModelClient:
    return MockModelClient()


@pytest.fixture
def tool_executor() -> MockToolExecutor:
    return MockToolExecutor()
