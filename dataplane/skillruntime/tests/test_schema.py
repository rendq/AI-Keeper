"""Tests for JSON Schema validation utilities."""

from __future__ import annotations

import pytest

from dataplane.skillruntime.schema import SchemaValidationError, validate, validate_schema_itself


class TestValidate:
    """Tests for validate() function."""

    def test_valid_object(self):
        schema = {"type": "object", "properties": {"name": {"type": "string"}}}
        validate({"name": "test"}, schema, label="input")

    def test_invalid_type(self):
        schema = {"type": "object", "properties": {"age": {"type": "integer"}}}
        with pytest.raises(SchemaValidationError, match="input validation failed"):
            validate({"age": "not-an-int"}, schema, label="input")

    def test_missing_required_field(self):
        schema = {
            "type": "object",
            "properties": {"name": {"type": "string"}},
            "required": ["name"],
        }
        with pytest.raises(SchemaValidationError):
            validate({}, schema, label="input")

    def test_errors_list_populated(self):
        schema = {
            "type": "object",
            "properties": {"a": {"type": "integer"}, "b": {"type": "string"}},
            "required": ["a", "b"],
        }
        with pytest.raises(SchemaValidationError) as exc_info:
            validate({}, schema, label="output")
        assert len(exc_info.value.errors) >= 1


class TestValidateSchemaItself:
    """Tests for validate_schema_itself()."""

    def test_valid_schema_passes(self):
        validate_schema_itself({"type": "object", "properties": {"x": {"type": "number"}}})

    def test_invalid_type_keyword(self):
        with pytest.raises(SchemaValidationError, match="Invalid JSON Schema"):
            validate_schema_itself({"type": "not-real"})
