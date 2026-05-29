"""Tool Loop — tool calling cycle with OBO token injection.

Implements the ToolExecutor protocol from fsm.py, adding:
1. OBO token injection before each tool call (for oauth2_obo tools)
2. Integration with ShortTermMemory for conversation context

The ToolLoop sits between the FSM and the actual tool backends
(MCP client, OpenAPI adapter, etc.).
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol

from aik_runtime.models import ToolCallRequest, ToolCallResult


class TokenExchanger(Protocol):
    """Protocol for OBO token exchange (RFC 8693).

    Exchanges the current user's ID token for a downstream OBO token.
    """

    async def exchange(
        self,
        user_token: str,
        target_audience: str,
    ) -> str:
        """Exchange user token for OBO token.

        Args:
            user_token: The end-user's ID token.
            target_audience: The target tool/service audience.

        Returns:
            The OBO access token for the downstream tool.
        """
        ...


class ToolBackend(Protocol):
    """Protocol for actual tool execution backends (MCP, OpenAPI, etc.)."""

    async def invoke(
        self,
        tool_name: str,
        arguments: dict[str, Any],
        headers: dict[str, str] | None = None,
    ) -> str:
        """Invoke a tool with optional auth headers.

        Returns the tool output as a string.
        """
        ...


@dataclass
class ToolSpec:
    """Specification for a registered tool."""

    name: str
    auth_mode: str = "none"  # none | oauth2_obo | service_account
    target_audience: str = ""  # Used for OBO token exchange


@dataclass
class ToolLoopConfig:
    """Configuration for the ToolLoop."""

    # Tool specifications (name -> spec)
    tool_specs: dict[str, ToolSpec] = field(default_factory=dict)
    # User token for OBO exchange (from session context)
    user_token: str = ""
    # Session ID for memory operations
    session_id: str = ""
    # Tenant ID
    tenant_id: str = ""


class ToolLoop:
    """Tool calling loop with OBO token injection.

    Implements the ToolExecutor protocol so it can be used by the FSM.
    Before calling each tool:
    - If tool requires oauth2_obo mode, exchanges user token for OBO token
    - Injects the OBO token as Authorization header
    - Delegates to the appropriate tool backend
    """

    def __init__(
        self,
        backend: ToolBackend,
        token_exchanger: TokenExchanger,
        config: ToolLoopConfig,
    ) -> None:
        self._backend = backend
        self._token_exchanger = token_exchanger
        self._config = config

    async def execute(self, request: ToolCallRequest) -> ToolCallResult:
        """Execute a tool call with OBO token injection if needed.

        This method implements the ToolExecutor protocol from fsm.py.
        """
        headers: dict[str, str] = {}

        # Look up tool spec to determine auth mode
        tool_spec = self._config.tool_specs.get(request.tool_name)

        if tool_spec and tool_spec.auth_mode == "oauth2_obo":
            # OBO token injection required
            if not self._config.user_token:
                return ToolCallResult(
                    tool_name=request.tool_name,
                    tool_id=request.tool_id,
                    output="",
                    error="OBO token required but no user token available (401)",
                )

            try:
                obo_token = await self._token_exchanger.exchange(
                    user_token=self._config.user_token,
                    target_audience=tool_spec.target_audience or request.tool_name,
                )
                headers["Authorization"] = f"Bearer {obo_token}"
            except Exception as e:
                return ToolCallResult(
                    tool_name=request.tool_name,
                    tool_id=request.tool_id,
                    output="",
                    error=f"OBO token exchange failed: {e}",
                )

        # Execute the tool via the backend
        try:
            output = await self._backend.invoke(
                tool_name=request.tool_name,
                arguments=request.arguments,
                headers=headers if headers else None,
            )
            return ToolCallResult(
                tool_name=request.tool_name,
                tool_id=request.tool_id,
                output=output,
            )
        except Exception as e:
            return ToolCallResult(
                tool_name=request.tool_name,
                tool_id=request.tool_id,
                output="",
                error=f"Tool execution failed: {e}",
            )
