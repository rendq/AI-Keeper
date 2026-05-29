"""The @skill decorator — core of the Python Skill SDK."""

from __future__ import annotations

import asyncio
import functools
import inspect
from typing import Any, Callable, Coroutine

from dataplane.skillruntime.schema import SchemaValidationError, validate, validate_schema_itself


class SkillDefinition:
    """Metadata and callable for a decorated skill function."""

    def __init__(
        self,
        func: Callable[..., Any],
        input_schema: dict[str, Any],
        output_schema: dict[str, Any],
    ) -> None:
        self.func = func
        self.name = func.__name__
        self.input_schema = input_schema
        self.output_schema = output_schema
        self._is_async = inspect.iscoroutinefunction(func)

        # Validate schemas at registration time
        validate_schema_itself(input_schema)
        validate_schema_itself(output_schema)

    async def invoke(self, input_data: dict[str, Any]) -> dict[str, Any]:
        """Validate input, call the skill function, validate output."""
        # Validate input
        validate(input_data, self.input_schema, label="input")

        # Call the function
        if self._is_async:
            result = await self.func(input_data)
        else:
            result = self.func(input_data)

        # Validate output
        validate(result, self.output_schema, label="output")

        return result


# Global registry of skill definitions
_skill_registry: dict[str, SkillDefinition] = {}


def skill(
    input_schema: dict[str, Any],
    output_schema: dict[str, Any],
) -> Callable[[Callable[..., Any]], SkillDefinition]:
    """Decorator that registers a function as a gRPC-served skill.

    Args:
        input_schema: JSON Schema dict for input validation.
        output_schema: JSON Schema dict for output validation.

    Example:
        @skill(
            input_schema={"type": "object", "properties": {"text": {"type": "string"}}},
            output_schema={"type": "object", "properties": {"result": {"type": "string"}}},
        )
        async def my_skill(input: dict) -> dict:
            return {"result": input["text"].upper()}
    """

    def decorator(func: Callable[..., Any]) -> SkillDefinition:
        defn = SkillDefinition(func, input_schema, output_schema)
        _skill_registry[defn.name] = defn
        return defn

    return decorator


def get_registry() -> dict[str, SkillDefinition]:
    """Return the global skill registry."""
    return _skill_registry


def clear_registry() -> None:
    """Clear the global skill registry (for testing)."""
    _skill_registry.clear()
