"""Tests for ModelCallClient — routing through ModelRouter + cost tracking.

Validates:
- ModelCallClient implements the ModelClient protocol (fsm.py)
- Calls are routed through the Model Router
- Cost events are emitted to the Cost Tracker
- Cost formula: (tokens_in * input_rate + tokens_out * output_rate) / 1e6
- Works with and without cost tracker
- Passes determinism params (temperature, top_p, seed) to the API
"""

from __future__ import annotations

from typing import Any

import pytest
from aik_runtime.model_call import (
    CostEvent,
    EndpointCost,
    ModelCallClient,
    RoutedEndpoint,
    compute_cost,
)
from aik_runtime.models import ModelResponse, ToolCallRequest

# --- Mock Router ---


class MockRouter:
    """Mock Model Router that returns a predefined endpoint."""

    def __init__(self, endpoint: RoutedEndpoint | None = None) -> None:
        self.endpoint = endpoint or RoutedEndpoint(
            endpoint_name="gpt-4o-eu",
            base_url="https://api.openai.com/v1",
            model="gpt-4o",
            api_key="sk-test",
            region="eu-west-1",
            cost=EndpointCost(input_per_million=5.0, output_per_million=15.0),
        )
        self.calls: list[dict[str, Any]] = []

    async def route(
        self, alias: str, context: dict[str, Any] | None = None
    ) -> RoutedEndpoint:
        self.calls.append({"alias": alias, "context": context})
        return self.endpoint


# --- Mock OpenAI Client ---


class MockOpenAIClient:
    """Mock OpenAI-compatible client."""

    def __init__(self, response: ModelResponse | None = None) -> None:
        self.response = response or ModelResponse(
            content="Hello!",
            tokens_in=100,
            tokens_out=50,
            finish_reason="stop",
        )
        self.calls: list[dict[str, Any]] = []

    async def chat_completion(
        self,
        base_url: str,
        api_key: str,
        model: str,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]] | None = None,
        *,
        temperature: float | None = None,
        top_p: float | None = None,
        seed: int | None = None,
    ) -> ModelResponse:
        self.calls.append({
            "base_url": base_url,
            "api_key": api_key,
            "model": model,
            "messages": messages,
            "tools": tools,
            "temperature": temperature,
            "top_p": top_p,
            "seed": seed,
        })
        return self.response


# --- Mock Cost Tracker ---


class MockCostTracker:
    """Mock Cost Tracker that records emitted events."""

    def __init__(self) -> None:
        self.events: list[CostEvent] = []

    async def emit(self, event: CostEvent) -> None:
        self.events.append(event)


# --- Tests ---


class TestComputeCost:
    """Tests for the pure compute_cost function."""

    def test_basic_cost(self) -> None:
        cost = compute_cost(
            tokens_in=1000,
            tokens_out=500,
            cost=EndpointCost(input_per_million=5.0, output_per_million=15.0),
        )
        # (1000 * 5 + 500 * 15) / 1_000_000 = (5000 + 7500) / 1_000_000 = 0.0125
        assert cost == pytest.approx(0.0125)

    def test_zero_tokens(self) -> None:
        cost = compute_cost(
            tokens_in=0,
            tokens_out=0,
            cost=EndpointCost(input_per_million=5.0, output_per_million=15.0),
        )
        assert cost == 0.0

    def test_zero_cost_rates(self) -> None:
        cost = compute_cost(
            tokens_in=1000,
            tokens_out=500,
            cost=EndpointCost(input_per_million=0.0, output_per_million=0.0),
        )
        assert cost == 0.0

    def test_cost_is_non_negative(self) -> None:
        cost = compute_cost(
            tokens_in=1,
            tokens_out=1,
            cost=EndpointCost(input_per_million=0.01, output_per_million=0.01),
        )
        assert cost >= 0.0

    def test_large_token_count(self) -> None:
        cost = compute_cost(
            tokens_in=1_000_000,
            tokens_out=1_000_000,
            cost=EndpointCost(input_per_million=5.0, output_per_million=15.0),
        )
        # (1M * 5 + 1M * 15) / 1M = 20.0
        assert cost == pytest.approx(20.0)


class TestModelCallClient:
    """Tests for ModelCallClient integration."""

    @pytest.fixture
    def router(self) -> MockRouter:
        return MockRouter()

    @pytest.fixture
    def openai_client(self) -> MockOpenAIClient:
        return MockOpenAIClient()

    @pytest.fixture
    def cost_tracker(self) -> MockCostTracker:
        return MockCostTracker()

    @pytest.fixture
    def client(
        self,
        router: MockRouter,
        openai_client: MockOpenAIClient,
        cost_tracker: MockCostTracker,
    ) -> ModelCallClient:
        return ModelCallClient(
            router=router,
            openai_client=openai_client,
            cost_tracker=cost_tracker,
            model_alias="reasoner",
            tenant_id="tenant-1",
            agent_name="legal-copilot",
            skill_name="contract-review",
            invocation_id="inv-123",
        )

    async def test_routes_through_model_router(
        self, client: ModelCallClient, router: MockRouter
    ) -> None:
        """Call should route through the Model Router with the alias."""
        await client.call([{"role": "user", "content": "hello"}])
        assert len(router.calls) == 1
        assert router.calls[0]["alias"] == "reasoner"

    async def test_calls_openai_with_routed_endpoint(
        self, client: ModelCallClient, openai_client: MockOpenAIClient
    ) -> None:
        """Call should use endpoint details from router."""
        await client.call([{"role": "user", "content": "hello"}])
        assert len(openai_client.calls) == 1
        call = openai_client.calls[0]
        assert call["base_url"] == "https://api.openai.com/v1"
        assert call["api_key"] == "sk-test"
        assert call["model"] == "gpt-4o"
        assert call["messages"] == [{"role": "user", "content": "hello"}]

    async def test_passes_tools(
        self, client: ModelCallClient, openai_client: MockOpenAIClient
    ) -> None:
        """Tools should be passed through to the OpenAI client."""
        tools = [{"type": "function", "function": {"name": "search"}}]
        await client.call([{"role": "user", "content": "hello"}], tools=tools)
        assert openai_client.calls[0]["tools"] == tools

    async def test_passes_determinism_params(
        self, client: ModelCallClient, openai_client: MockOpenAIClient
    ) -> None:
        """Temperature, top_p, seed should be passed through."""
        await client.call(
            [{"role": "user", "content": "hello"}],
            temperature=0.7,
            top_p=0.9,
            seed=42,
        )
        call = openai_client.calls[0]
        assert call["temperature"] == 0.7
        assert call["top_p"] == 0.9
        assert call["seed"] == 42

    async def test_emits_cost_event(
        self, client: ModelCallClient, cost_tracker: MockCostTracker
    ) -> None:
        """Cost event should be emitted after successful call."""
        await client.call([{"role": "user", "content": "hello"}])
        assert len(cost_tracker.events) == 1
        event = cost_tracker.events[0]
        assert event.endpoint_name == "gpt-4o-eu"
        assert event.model == "gpt-4o"
        assert event.tokens_in == 100
        assert event.tokens_out == 50
        # (100 * 5 + 50 * 15) / 1_000_000 = (500 + 750) / 1_000_000 = 0.00125
        assert event.cost_usd == pytest.approx(0.00125)
        assert event.tenant_id == "tenant-1"
        assert event.agent_name == "legal-copilot"
        assert event.skill_name == "contract-review"
        assert event.invocation_id == "inv-123"

    async def test_cost_event_has_latency(
        self, client: ModelCallClient, cost_tracker: MockCostTracker
    ) -> None:
        """Cost event latency should be positive."""
        await client.call([{"role": "user", "content": "hello"}])
        assert cost_tracker.events[0].latency_ms > 0

    async def test_returns_model_response(self, client: ModelCallClient) -> None:
        """Should return the ModelResponse from the OpenAI client."""
        response = await client.call([{"role": "user", "content": "hello"}])
        assert response.content == "Hello!"
        assert response.tokens_in == 100
        assert response.tokens_out == 50
        assert response.finish_reason == "stop"

    async def test_returns_tool_calls(
        self,
        router: MockRouter,
        cost_tracker: MockCostTracker,
    ) -> None:
        """Should return tool calls from the model response."""
        tool_calls = [
            ToolCallRequest(tool_name="search", tool_id="tc-1", arguments={"q": "test"})
        ]
        openai = MockOpenAIClient(
            ModelResponse(
                content="",
                tool_calls=tool_calls,
                tokens_in=50,
                tokens_out=30,
                finish_reason="tool_calls",
            )
        )
        client = ModelCallClient(
            router=router,
            openai_client=openai,
            cost_tracker=cost_tracker,
            model_alias="reasoner",
        )
        response = await client.call([{"role": "user", "content": "search for X"}])
        assert len(response.tool_calls) == 1
        assert response.tool_calls[0].tool_name == "search"

    async def test_works_without_cost_tracker(
        self, router: MockRouter, openai_client: MockOpenAIClient
    ) -> None:
        """Should work fine without a cost tracker (no error)."""
        client = ModelCallClient(
            router=router,
            openai_client=openai_client,
            cost_tracker=None,
            model_alias="reasoner",
        )
        response = await client.call([{"role": "user", "content": "hello"}])
        assert response.content == "Hello!"

    async def test_route_context_passed(self, openai_client: MockOpenAIClient) -> None:
        """Route context should be passed to the router."""
        router = MockRouter()
        ctx = {"user.country": "CN", "classification": "internal"}
        client = ModelCallClient(
            router=router,
            openai_client=openai_client,
            cost_tracker=None,
            model_alias="reasoner",
            route_context=ctx,
        )
        await client.call([{"role": "user", "content": "hello"}])
        assert router.calls[0]["context"] == ctx

    async def test_implements_model_client_protocol(
        self, client: ModelCallClient
    ) -> None:
        """ModelCallClient should satisfy the ModelClient protocol from fsm.py."""
        from aik_runtime.fsm import ModelClient

        # Structural subtyping — if this doesn't raise, it conforms
        def accepts_model_client(mc: ModelClient) -> None:
            pass

        # This is a static check; at runtime we just verify the method exists
        assert hasattr(client, "call")
        assert callable(client.call)
