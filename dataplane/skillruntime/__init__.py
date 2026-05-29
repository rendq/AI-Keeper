"""AIP Skill Runtime SDK — Python Skill SDK for building gRPC-served skills.

Usage:
    from dataplane.skillruntime import skill

    @skill(
        input_schema={"type": "object", "properties": {"contract": {"type": "string"}}},
        output_schema={"type": "object", "properties": {"summary": {"type": "string"}}},
    )
    async def contract_review(input: dict) -> dict:
        return {"summary": "..."}
"""

from dataplane.skillruntime.decorator import skill
from dataplane.skillruntime.server import SkillServer

__all__ = ["skill", "SkillServer"]
