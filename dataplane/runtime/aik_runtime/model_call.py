"""Model Call client — routes through ModelRouter and emits cost events.

Implements the ModelClient protocol defined in fsm.py. Each call:
1. Routes through the Model Router (task 11.1) to select an endpoint
2. Calls the OpenAI-compatible API at the selected endpoint
3. Emits cost events to the Cost Tracker (task 13.1) after completion

Cost formula: (tokens_in * cost_per_input_token + tokens_out * cost_per_output_token) / 1e6
"""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any, Protocol

from aik_runtime.models import ModelResponse

# --- Protocols for external dependencies ---


class ModelRouter(Protocol):
    """Protocol for the Model Router (task 11.1).

    Routes a logical model alias to a concrete endpoint.
    """

    async def route(
        self,
        alias: str,
        context: dict[str, Any] | None = None,
    ) -> RoutedEndpoint: ...


class CostTracker(Protocol):
    """Protocol for the Cost Tracker (task 13.1).

    Receives cost events after each model call.
    """

    async def emit(self, event: CostEvent) -> None: ...


# --- Data classes ---


@dataclass
class EndpointCost:
    """Per-token cost for a model endpoint (USD per 1M tokens)."""

    input_per_million: float = 0.0
    output_per_million: float = 0.0


@dataclass
class RoutedEndpoint:
    """Result from Model Router — the selected endpoint to call."""

    endpoint_name: str = ""
    base_url: str = ""
    model: str = ""
    api_key: str = ""
    region: str = ""
    cost: EndpointCost = field(default_factory=EndpointCost)
    fallback_used: bool = False


@dataclass
class CostEvent:
    """Cost event emitted to the Cost Tracker after each model call."""

    endpoint_name: str = ""
    model: str = ""
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0
    latency_ms: float = 0.0
    tenant_id: str = ""
    agent_name: str = ""
    skill_name: str = ""
    invocation_id: str = ""


# --- OpenAI-compatible API client protocol ---


class OpenAIClient(Protocol):
    """Protocol for calling an OpenAI-compatible endpoint."""

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
    ) -> ModelResponse: ...


# --- Cost computation (pure function) ---


def compute_cost(tokens_in: int, tokens_out: int, cost: EndpointCost) -> float:
    """Compute cost in USD from token counts and per-million-token pricing.

    Formula: (tokens_in * cost_per_input + tokens_out * cost_per_output) / 1e6
    """
    return (
        tokens_in * cost.input_per_million + tokens_out * cost.output_per_million
    ) / 1_000_000


# --- Model Call Client (implements ModelClient protocol) ---


class ModelCallClient:
    """Concrete ModelClient that routes through ModelRouter and tracks cost.

    This implements the ModelClient protocol from fsm.py:
        async def call(messages, tools=None, *, temperature=None, top_p=None, seed=None)
            -> ModelResponse

    Usage:
        client = ModelCallClient(
            router=router,
            openai_client=openai_client,
            cost_tracker=cost_tracker,
            model_alias="reasoner",
        )
        response = await client.call(messages, tools)
    """

    def __init__(
        self,
        router: ModelRouter,
        openai_client: OpenAIClient,
        cost_tracker: CostTracker | None = None,
        model_alias: str = "reasoner",
        route_context: dict[str, Any] | None = None,
        tenant_id: str = "",
        agent_name: str = "",
        skill_name: str = "",
        invocation_id: str = "",
    ) -> None:
        self._router = router
        self._openai = openai_client
        self._cost_tracker = cost_tracker
        self._model_alias = model_alias
        self._route_context = route_context
        self._tenant_id = tenant_id
        self._agent_name = agent_name
        self._skill_name = skill_name
        self._invocation_id = invocation_id

    async def call(
        self,
        messages: list[dict[str, str]],
        tools: list[dict[str, Any]] | None = None,
        *,
        temperature: float | None = None,
        top_p: float | None = None,
        seed: int | None = None,
    ) -> ModelResponse:
        """Call the model via router, emit cost event, return response."""
        # 1. Route to endpoint
        endpoint = await self._router.route(self._model_alias, self._route_context)

        # 2. Call OpenAI-compatible API
        start = time.perf_counter()
        response = await self._openai.chat_completion(
            base_url=endpoint.base_url,
            api_key=endpoint.api_key,
            model=endpoint.model,
            messages=messages,  # type: ignore[arg-type]
            tools=tools,
            temperature=temperature,
            top_p=top_p,
            seed=seed,
        )
        latency_ms = (time.perf_counter() - start) * 1000.0

        # 3. Emit cost event
        if self._cost_tracker is not None:
            cost_usd = compute_cost(
                response.tokens_in, response.tokens_out, endpoint.cost
            )
            event = CostEvent(
                endpoint_name=endpoint.endpoint_name,
                model=endpoint.model,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
                cost_usd=cost_usd,
                latency_ms=latency_ms,
                tenant_id=self._tenant_id,
                agent_name=self._agent_name,
                skill_name=self._skill_name,
                invocation_id=self._invocation_id,
            )
            await self._cost_tracker.emit(event)

        return response
