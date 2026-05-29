"""Short-term conversation memory backed by Redis.

Implements per_session isolation using Redis LIST + TTL.
longTerm and Forget APIs are P1 (out of scope).

Redis key pattern: mem:{tenant_id}:{session_id}:msgs  (LIST)
Each entry is a JSON-serialized message dict.
"""

from __future__ import annotations

import json
from typing import Any, Protocol


class RedisClient(Protocol):
    """Protocol for async Redis operations used by ShortTermMemory."""

    async def lpush(self, key: str, *values: str) -> int: ...
    async def lrange(self, key: str, start: int, stop: int) -> list[str]: ...
    async def ltrim(self, key: str, start: int, stop: int) -> None: ...
    async def expire(self, key: str, seconds: int) -> None: ...
    async def delete(self, key: str) -> None: ...


class ShortTermMemory:
    """Short-term conversation memory using Redis LIST per session.

    - Messages stored as JSON in a Redis LIST (LPUSH for newest first)
    - Window cap: keeps only the most recent N messages
    - TTL: session auto-expires after configured seconds
    - Isolation: per_session (key includes tenant + session_id)

    longTerm and Forget are P1 — not implemented here.
    """

    def __init__(
        self,
        redis: RedisClient,
        tenant_id: str,
        window: int = 20,
        ttl_seconds: int = 3600,
    ) -> None:
        self._redis = redis
        self._tenant_id = tenant_id
        self._window = window
        self._ttl_seconds = ttl_seconds

    def _key(self, session_id: str) -> str:
        """Redis key for a session's message list."""
        return f"mem:{self._tenant_id}:{session_id}:msgs"

    async def read_history(self, session_id: str) -> list[dict[str, Any]]:
        """Read conversation history for a session.

        Returns messages in chronological order (oldest first).
        """
        key = self._key(session_id)
        # lrange 0 -1 gets all elements; list is newest-first (LPUSH)
        raw = await self._redis.lrange(key, 0, self._window - 1)
        # Reverse to get chronological order (oldest first)
        messages = [json.loads(item) for item in reversed(raw)]
        return messages

    async def write_message(self, session_id: str, message: dict[str, Any]) -> None:
        """Write a message to the session history.

        Maintains the rolling window (trims to window size) and refreshes TTL.
        """
        key = self._key(session_id)
        serialized = json.dumps(message, ensure_ascii=False)
        await self._redis.lpush(key, serialized)
        # Trim to window size (keep indices 0..window-1)
        await self._redis.ltrim(key, 0, self._window - 1)
        # Refresh TTL on every write
        await self._redis.expire(key, self._ttl_seconds)

    async def clear(self, session_id: str) -> None:
        """Clear all messages for a session."""
        key = self._key(session_id)
        await self._redis.delete(key)
