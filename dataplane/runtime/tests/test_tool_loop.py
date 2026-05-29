"""Tests for ToolLoop (OBO injection) and ShortTermMemory (Redis LIST + TTL).

Validates:
- Requirements B5.1: Tool calling loop with OBO token injection
- Requirements B10 (shortTerm only): Session memory via Redis per_session
"""

from __future__ import annotations

from typing import Any

import pytest
from aik_runtime.memory import ShortTermMemory
from aik_runtime.models import ToolCallRequest
from aik_runtime.tool_loop import ToolLoop, ToolLoopConfig, ToolSpec

# ---------- Mock Redis Client ----------


class MockRedisClient:
    """In-memory mock Redis client for testing."""

    def __init__(self) -> None:
        self._store: dict[str, list[str]] = {}
        self._ttls: dict[str, int] = {}
        self.commands: list[tuple[str, ...]] = []

    async def lpush(self, key: str, *values: str) -> int:
        self.commands.append(("LPUSH", key, *values))
        if key not in self._store:
            self._store[key] = []
        for v in values:
            self._store[key].insert(0, v)
        return len(self._store[key])

    async def lrange(self, key: str, start: int, stop: int) -> list[str]:
        self.commands.append(("LRANGE", key, str(start), str(stop)))
        if key not in self._store:
            return []
        data = self._store[key]
        # Redis LRANGE is inclusive on both ends
        if stop < 0:
            stop = len(data) + stop + 1
        else:
            stop = stop + 1
        return data[start:stop]

    async def ltrim(self, key: str, start: int, stop: int) -> None:
        self.commands.append(("LTRIM", key, str(start), str(stop)))
        if key not in self._store:
            return
        data = self._store[key]
        if stop < 0:
            stop = len(data) + stop + 1
        else:
            stop = stop + 1
        self._store[key] = data[start:stop]

    async def expire(self, key: str, seconds: int) -> None:
        self.commands.append(("EXPIRE", key, str(seconds)))
        self._ttls[key] = seconds

    async def delete(self, key: str) -> None:
        self.commands.append(("DELETE", key))
        self._store.pop(key, None)
        self._ttls.pop(key, None)

    def get_ttl(self, key: str) -> int | None:
        return self._ttls.get(key)


# ---------- Mock Token Exchanger ----------


class MockTokenExchanger:
    """Mock OBO token exchanger."""

    def __init__(self, token_prefix: str = "obo-") -> None:  # noqa: S107
        self._token_prefix = token_prefix
        self.exchanges: list[tuple[str, str]] = []
        self._should_fail = False
        self._fail_error: str = "exchange error"

    def set_fail(self, should_fail: bool, error: str = "exchange error") -> None:
        self._should_fail = should_fail
        self._fail_error = error

    async def exchange(self, user_token: str, target_audience: str) -> str:
        self.exchanges.append((user_token, target_audience))
        if self._should_fail:
            raise RuntimeError(self._fail_error)
        return f"{self._token_prefix}{target_audience}-{user_token[:8]}"


# ---------- Mock Tool Backend ----------


class MockToolBackend:
    """Mock tool backend."""

    def __init__(self) -> None:
        self.invocations: list[dict[str, Any]] = []
        self._results: dict[str, str] = {}
        self._should_fail: dict[str, str] = {}

    def set_result(self, tool_name: str, output: str) -> None:
        self._results[tool_name] = output

    def set_fail(self, tool_name: str, error: str) -> None:
        self._should_fail[tool_name] = error

    async def invoke(
        self,
        tool_name: str,
        arguments: dict[str, Any],
        headers: dict[str, str] | None = None,
    ) -> str:
        self.invocations.append({
            "tool_name": tool_name,
            "arguments": arguments,
            "headers": headers,
        })
        if tool_name in self._should_fail:
            raise RuntimeError(self._should_fail[tool_name])
        return self._results.get(tool_name, f"output of {tool_name}")


# ---------- Fixtures ----------


@pytest.fixture
def redis() -> MockRedisClient:
    return MockRedisClient()


@pytest.fixture
def token_exchanger() -> MockTokenExchanger:
    return MockTokenExchanger()


@pytest.fixture
def tool_backend() -> MockToolBackend:
    return MockToolBackend()


# ========== ShortTermMemory Tests ==========


class TestShortTermMemory:
    """Tests for ShortTermMemory Redis-backed conversation memory."""

    async def test_write_and_read_single_message(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=3600)

        await mem.write_message("s1", {"role": "user", "content": "hello"})
        history = await mem.read_history("s1")

        assert len(history) == 1
        assert history[0] == {"role": "user", "content": "hello"}

    async def test_read_empty_session(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=3600)

        history = await mem.read_history("nonexistent")
        assert history == []

    async def test_messages_in_chronological_order(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=3600)

        await mem.write_message("s1", {"role": "user", "content": "first"})
        await mem.write_message("s1", {"role": "assistant", "content": "second"})
        await mem.write_message("s1", {"role": "user", "content": "third"})

        history = await mem.read_history("s1")
        assert len(history) == 3
        assert history[0]["content"] == "first"
        assert history[1]["content"] == "second"
        assert history[2]["content"] == "third"

    async def test_window_cap_trims_old_messages(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=3, ttl_seconds=3600)

        for i in range(5):
            await mem.write_message("s1", {"role": "user", "content": f"msg-{i}"})

        history = await mem.read_history("s1")
        assert len(history) == 3
        # Should keep the 3 most recent messages
        assert history[0]["content"] == "msg-2"
        assert history[1]["content"] == "msg-3"
        assert history[2]["content"] == "msg-4"

    async def test_ttl_set_on_write(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=7200)

        await mem.write_message("s1", {"role": "user", "content": "hi"})

        key = "mem:t1:s1:msgs"
        assert redis.get_ttl(key) == 7200

    async def test_per_session_isolation(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=3600)

        await mem.write_message("session-a", {"role": "user", "content": "msg-a"})
        await mem.write_message("session-b", {"role": "user", "content": "msg-b"})

        history_a = await mem.read_history("session-a")
        history_b = await mem.read_history("session-b")

        assert len(history_a) == 1
        assert history_a[0]["content"] == "msg-a"
        assert len(history_b) == 1
        assert history_b[0]["content"] == "msg-b"

    async def test_clear_removes_session(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="t1", window=20, ttl_seconds=3600)

        await mem.write_message("s1", {"role": "user", "content": "hello"})
        await mem.clear("s1")

        history = await mem.read_history("s1")
        assert history == []

    async def test_key_includes_tenant_and_session(self, redis: MockRedisClient) -> None:
        mem = ShortTermMemory(redis, tenant_id="acme-corp", window=20, ttl_seconds=3600)

        await mem.write_message("session-42", {"role": "user", "content": "hi"})

        # Verify key format matches design: mem:{tenant}:{session_id}:msgs
        expected_key = "mem:acme-corp:session-42:msgs"
        assert expected_key in redis._store


# ========== ToolLoop Tests ==========


class TestToolLoop:
    """Tests for ToolLoop with OBO token injection."""

    async def test_tool_call_no_auth(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """Tool with auth_mode=none should not inject any token."""
        config = ToolLoopConfig(
            tool_specs={"search": ToolSpec(name="search", auth_mode="none")},
            user_token="user-jwt-abc",
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(tool_name="search", tool_id="tc1", arguments={"q": "test"})
        result = await loop.execute(request)

        assert result.error is None
        assert result.output == "output of search"
        # No token exchange should have happened
        assert len(token_exchanger.exchanges) == 0
        # No auth header
        assert tool_backend.invocations[0]["headers"] is None

    async def test_tool_call_obo_injects_token(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """Tool with auth_mode=oauth2_obo should get OBO token injected."""
        config = ToolLoopConfig(
            tool_specs={
                "docusign": ToolSpec(
                    name="docusign",
                    auth_mode="oauth2_obo",
                    target_audience="https://docusign.example.com",
                )
            },
            user_token="user-jwt-token-xyz",
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(
            tool_name="docusign", tool_id="tc2", arguments={"doc": "contract.pdf"}
        )
        result = await loop.execute(request)

        assert result.error is None
        # Verify token exchange happened
        assert len(token_exchanger.exchanges) == 1
        assert token_exchanger.exchanges[0] == (
            "user-jwt-token-xyz",
            "https://docusign.example.com",
        )
        # Verify Authorization header was injected
        headers = tool_backend.invocations[0]["headers"]
        assert headers is not None
        assert headers["Authorization"].startswith("Bearer obo-")

    async def test_tool_call_obo_missing_user_token(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """OBO tool without user token should return 401 error."""
        config = ToolLoopConfig(
            tool_specs={
                "docusign": ToolSpec(name="docusign", auth_mode="oauth2_obo")
            },
            user_token="",  # No user token
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(tool_name="docusign", tool_id="tc3", arguments={})
        result = await loop.execute(request)

        assert result.error is not None
        assert "401" in result.error
        # Backend should not have been called
        assert len(tool_backend.invocations) == 0

    async def test_tool_call_obo_exchange_failure(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """OBO token exchange failure should return error."""
        token_exchanger.set_fail(True, "IdP unreachable")
        config = ToolLoopConfig(
            tool_specs={
                "api-tool": ToolSpec(name="api-tool", auth_mode="oauth2_obo")
            },
            user_token="valid-jwt",
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(tool_name="api-tool", tool_id="tc4", arguments={})
        result = await loop.execute(request)

        assert result.error is not None
        assert "OBO token exchange failed" in result.error
        assert "IdP unreachable" in result.error
        # Backend should not have been called
        assert len(tool_backend.invocations) == 0

    async def test_tool_call_backend_failure(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """Backend execution failure should return error result."""
        tool_backend.set_fail("broken-tool", "connection refused")
        config = ToolLoopConfig(
            tool_specs={"broken-tool": ToolSpec(name="broken-tool", auth_mode="none")},
            user_token="",
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(tool_name="broken-tool", tool_id="tc5", arguments={})
        result = await loop.execute(request)

        assert result.error is not None
        assert "Tool execution failed" in result.error

    async def test_tool_call_unknown_tool_defaults_to_no_auth(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """Unknown tool (not in specs) should proceed without OBO."""
        config = ToolLoopConfig(tool_specs={}, user_token="some-jwt")
        loop = ToolLoop(tool_backend, token_exchanger, config)

        request = ToolCallRequest(
            tool_name="unknown-tool", tool_id="tc6", arguments={"x": 1}
        )
        result = await loop.execute(request)

        assert result.error is None
        assert result.output == "output of unknown-tool"
        assert len(token_exchanger.exchanges) == 0

    async def test_tool_loop_implements_executor_protocol(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """ToolLoop should be usable as a ToolExecutor (duck typing with FSM)."""
        config = ToolLoopConfig(tool_specs={})
        loop = ToolLoop(tool_backend, token_exchanger, config)

        # Verify it has the execute method with right signature
        assert hasattr(loop, "execute")
        request = ToolCallRequest(tool_name="test", tool_id="id1", arguments={})
        result = await loop.execute(request)
        assert result.tool_name == "test"

    async def test_multiple_tools_mixed_auth(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """Multiple tools with different auth modes in same config."""
        config = ToolLoopConfig(
            tool_specs={
                "search": ToolSpec(name="search", auth_mode="none"),
                "docusign": ToolSpec(
                    name="docusign",
                    auth_mode="oauth2_obo",
                    target_audience="docusign-api",
                ),
            },
            user_token="my-jwt-token",
        )
        loop = ToolLoop(tool_backend, token_exchanger, config)

        # Call search (no auth)
        r1 = await loop.execute(
            ToolCallRequest(tool_name="search", tool_id="t1", arguments={})
        )
        assert r1.error is None
        assert tool_backend.invocations[-1]["headers"] is None

        # Call docusign (OBO)
        r2 = await loop.execute(
            ToolCallRequest(tool_name="docusign", tool_id="t2", arguments={})
        )
        assert r2.error is None
        assert tool_backend.invocations[-1]["headers"] is not None
        assert "Bearer" in tool_backend.invocations[-1]["headers"]["Authorization"]

        # Only one exchange (for docusign)
        assert len(token_exchanger.exchanges) == 1


# ========== Integration: ToolLoop + FSM ==========


class TestToolLoopFSMIntegration:
    """Test that ToolLoop integrates with the FSM as a ToolExecutor."""

    async def test_fsm_with_tool_loop(
        self,
        tool_backend: MockToolBackend,
        token_exchanger: MockTokenExchanger,
    ) -> None:
        """FSM should be able to use ToolLoop as its ToolExecutor."""
        from aik_runtime.fsm import AgentRuntime, RuntimeConfig
        from aik_runtime.models import ModelResponse
        from aik_runtime.models import ToolCallRequest as TCR

        tool_backend.set_result("calculator", "42")

        config = ToolLoopConfig(
            tool_specs={"calculator": ToolSpec(name="calculator", auth_mode="none")},
        )
        tool_loop = ToolLoop(tool_backend, token_exchanger, config)

        # Mock model that calls a tool then returns final answer
        class FakeModel:
            def __init__(self) -> None:
                self._call_count = 0

            async def call(
                self,
                messages: list[dict[str, str]],
                tools: list[dict[str, Any]] | None = None,
                **kwargs: Any,
            ) -> ModelResponse:
                self._call_count += 1
                if self._call_count == 1:
                    return ModelResponse(
                        content="",
                        tool_calls=[
                            TCR(tool_name="calculator", tool_id="c1", arguments={"expr": "6*7"})
                        ],
                        tokens_in=10,
                        tokens_out=20,
                    )
                return ModelResponse(content="The answer is 42", tokens_in=5, tokens_out=10)

        runtime = AgentRuntime(FakeModel(), tool_loop)
        rt_config = RuntimeConfig(pattern="tool_calling", tools=[{"name": "calculator"}])

        result = await runtime.invoke(
            [{"role": "user", "content": "what is 6*7?"}],
            rt_config,
            invocation_id="inv-1",
        )

        assert result.status == "success"
        assert result.response == "The answer is 42"
        # Tool backend was called
        assert len(tool_backend.invocations) == 1
        assert tool_backend.invocations[0]["tool_name"] == "calculator"
