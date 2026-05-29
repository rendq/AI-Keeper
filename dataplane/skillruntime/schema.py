"""JSON Schema validation utilities for the Skill SDK."""

from __future__ import annotations

import json
from typing import Any

import jsonschema
import jsonschema.validators


class SchemaValidationError(Exception):
    """Raised when input or output fails JSON Schema validation."""

    def __init__(self, message: str, errors: list[str] | None = None) -> None:
        super().__init__(message)
        self.errors = errors or []


def validate(instance: Any, schema: dict[str, Any], *, label: str = "value") -> None:
    """Validate *instance* against a JSON Schema dict.

    Raises SchemaValidationError with details on failure.
    """
    validator_cls = jsonschema.validators.validator_for(schema)
    validator = validator_cls(schema)
    errors = sorted(validator.iter_errors(instance), key=lambda e: list(e.path))
    if errors:
        messages = [
            f"{'.'.join(str(p) for p in e.absolute_path) or '(root)'}: {e.message}"
            for e in errors
        ]
        raise SchemaValidationError(
            f"{label} validation failed: {messages[0]}",
            errors=messages,
        )


def validate_schema_itself(schema: dict[str, Any]) -> None:
    """Check that a schema dict is itself a valid JSON Schema (Draft 2020-12 / Draft 7)."""
    try:
        validator_cls = jsonschema.validators.validator_for(schema)
        validator_cls.check_schema(schema)
    except jsonschema.exceptions.SchemaError as exc:
        raise SchemaValidationError(f"Invalid JSON Schema: {exc.message}") from exc
