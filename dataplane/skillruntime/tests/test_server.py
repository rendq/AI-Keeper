"""Tests for the gRPC SkillServer."""

from __future__ import annotations

import json

import pytest

from dataplane.skillruntime.decorator import clear_registry, skill
from dataplane.skillruntime.server import SkillServer

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


@pytest.fixture(autouse=True)
def _clean():
    clear_registry()
    yield
    clear_registry()


class TestSkillServer:
    """Tests for SkillServer lifecycle."""

    async def test_server_starts_and_stops(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def echo(input: dict) -> dict:
            return {"result": input["text"]}

        server = SkillServer(port=0)  # port 0 picks random available port
        await server.start()
        assert server._server is not None
        await server.stop(grace=0.5)

    async def test_server_registers_all_skills(self):
        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def skill_a(input: dict) -> dict:
            return {"result": "a"}

        @skill(input_schema=INPUT_SCHEMA, output_schema=OUTPUT_SCHEMA)
        async def skill_b(input: dict) -> dict:
            return {"result": "b"}

        from dataplane.skillruntime.decorator import get_registry

        assert len(get_registry()) == 2

        server = SkillServer(port=0)
        await server.start()
        await server.stop(grace=0.5)
