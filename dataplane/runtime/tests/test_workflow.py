"""Tests for the Workflow pattern."""

from __future__ import annotations

from aik_runtime.patterns.workflow import (
    PatternResult,
    WorkflowDefinition,
    WorkflowPattern,
    WorkflowStep,
)


class TestLinearWorkflow:
    """Test linear workflow (A → B → C)."""

    def test_linear_three_steps(self):
        steps = {
            "A": WorkflowStep(
                id="A",
                handler=lambda ctx: "result_a",
                next_steps=["B"],
            ),
            "B": WorkflowStep(
                id="B",
                handler=lambda ctx: "result_b",
                next_steps=["C"],
            ),
            "C": WorkflowStep(
                id="C",
                handler=lambda ctx: "result_c",
                next_steps=[],
            ),
        }
        definition = WorkflowDefinition(steps=steps, entry_point="A")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {})

        assert result.steps_executed == 3
        assert result.trace == ["A", "B", "C"]
        assert "result_a" in result.output
        assert "result_b" in result.output
        assert "result_c" in result.output


class TestConditionalBranching:
    """Test conditional branching (A → B or C based on condition)."""

    def test_branch_to_b_when_condition_true(self):
        steps = {
            "A": WorkflowStep(
                id="A",
                handler=lambda ctx: "start",
                next_steps=["B", "C"],
            ),
            "B": WorkflowStep(
                id="B",
                handler=lambda ctx: "took_b",
                next_steps=[],
                condition=lambda ctx: ctx.get("go_b", False),
            ),
            "C": WorkflowStep(
                id="C",
                handler=lambda ctx: "took_c",
                next_steps=[],
                condition=lambda ctx: ctx.get("go_c", False),
            ),
        }
        definition = WorkflowDefinition(steps=steps, entry_point="A")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {"go_b": True, "go_c": False})

        assert result.trace == ["A", "B"]
        assert "took_b" in result.output
        assert "took_c" not in result.output

    def test_branch_to_c_when_b_condition_false(self):
        steps = {
            "A": WorkflowStep(
                id="A",
                handler=lambda ctx: "start",
                next_steps=["B", "C"],
            ),
            "B": WorkflowStep(
                id="B",
                handler=lambda ctx: "took_b",
                next_steps=[],
                condition=lambda ctx: ctx.get("go_b", False),
            ),
            "C": WorkflowStep(
                id="C",
                handler=lambda ctx: "took_c",
                next_steps=[],
                condition=lambda ctx: ctx.get("go_c", False),
            ),
        }
        definition = WorkflowDefinition(steps=steps, entry_point="A")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {"go_b": False, "go_c": True})

        assert result.trace == ["A", "C"]
        assert "took_c" in result.output
        assert "took_b" not in result.output


class TestNoMatchingCondition:
    """Test workflow with no matching condition."""

    def test_stops_when_no_condition_matches(self):
        steps = {
            "A": WorkflowStep(
                id="A",
                handler=lambda ctx: "start",
                next_steps=["B", "C"],
            ),
            "B": WorkflowStep(
                id="B",
                handler=lambda ctx: "took_b",
                next_steps=[],
                condition=lambda ctx: False,
            ),
            "C": WorkflowStep(
                id="C",
                handler=lambda ctx: "took_c",
                next_steps=[],
                condition=lambda ctx: False,
            ),
        }
        definition = WorkflowDefinition(steps=steps, entry_point="A")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {})

        assert result.trace == ["A"]
        assert result.steps_executed == 1
        assert "start" in result.output


class TestTerminatesAtEnd:
    """Test workflow terminates at step with no next_steps."""

    def test_single_step_no_next(self):
        steps = {
            "only": WorkflowStep(
                id="only",
                handler=lambda ctx: "done",
                next_steps=[],
            ),
        }
        definition = WorkflowDefinition(steps=steps, entry_point="only")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {})

        assert result.trace == ["only"]
        assert result.steps_executed == 1
        assert result.output == "done"

    def test_empty_entry_point(self):
        definition = WorkflowDefinition(steps={}, entry_point="")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {})

        assert result.steps_executed == 0
        assert result.trace == []

    def test_invalid_entry_point(self):
        definition = WorkflowDefinition(steps={}, entry_point="nonexistent")
        pattern = WorkflowPattern()

        result = pattern.run(definition, {})

        assert result.steps_executed == 0
        assert result.trace == []
