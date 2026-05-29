"""Tests for the @skill decorator and SkillDefinition."""

from __future__ import annotations

import pytest

from dataplane.skillruntime.decorator import SkillDefinition, clear_registry, get_registry, skill
from dataplane.skillruntime.schema import SchemaValidationError


@pytest.fixture(autouse=True)
def _clean_registry():
    """Ensure each test starts with a clean skill registry."""
    clear_registry()
    yield
    clear_registry()


INPUT_SCHEMA = {
    "type": "object",
    "properties": {"text": {"type": "string"}},
    "required": ["text"],
}

OUTPUT_SCHEMA = {
    "type": "object",
    "properties": {"result": {"type": "string"}},
    "required": ["result"],
}


class TestSkillDecorator:
    """Tests for @skill decorator registration and validation."""

    def test_registers_skill_in_global_registry(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def echo(input: dict) -> dict:
            return {"result": input["text"]}

        registry = get_registry()
        assert "echo" in registry
        assert registry["echo"].name == "echo"

    def test_decorator_returns_skill_definition(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def my_skill(input: dict) -> dict:
            return {"result": "ok"}

        assert isinstance(my_skill, SkillDefinition)
        assert my_skill.input_schema == INPUT_SCHEMA
        assert my_skill.output_schema == OUTPUT_SCHEMA

    def test_rejects_invalid_input_schema(self):
        with pytest.raises(SchemaValidationError, match="Invalid JSON Schema"):

            @skill(
                input_schema={"type": "not-a-valid-type"},
                output_schema=OUTPUT_SCHEMA,
            )
            async def bad_input(input: dict) -> dict:
                return {"result": "x"}

    def test_rejects_invalid_output_schema(self):
        with pytest.raises(SchemaValidationError, match="Invalid JSON Schema"):

            @skill(
                input_schema=INPUT_SCHEMA,
                output_schema={"type": "bogus"},
            )
            async def bad_output(input: dict) -> dict:
                return {"result": "x"}

    def test_supports_sync_function(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        def sync_skill(input: dict) -> dict:
            return {"result": input["text"].upper()}

        assert sync_skill.name == "sync_skill"
        assert sync_skill._is_async is False


class TestSkillInvocation:
    """Tests for SkillDefinition.invoke() — schema validation + execution."""

    @pytest.fixture
    def echo_skill(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def echo(input: dict) -> dict:
            return {"result": input["text"]}

        return echo

    async def test_valid_input_output(self, echo_skill):
        result = await echo_skill.invoke({"text": "hello"})
        assert result == {"result": "hello"}

    async def test_invalid_input_raises(self, echo_skill):
        with pytest.raises(SchemaValidationError, match="input validation failed"):
            await echo_skill.invoke({"wrong_key": 123})

    async def test_invalid_output_raises(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def bad_return(input: dict) -> dict:
            return {"wrong": "field"}  # Missing "result"

        with pytest.raises(SchemaValidationError, match="output validation failed"):
            await bad_return.invoke({"text": "hello"})

    async def test_sync_function_invocation(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        def sync_echo(input: dict) -> dict:
            return {"result": input["text"]}

        result = await sync_echo.invoke({"text": "world"})
        assert result == {"result": "world"}
