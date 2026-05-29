"""Data models for Agent Runtime steps and audit recording."""

from __future__ import annotations

import enum
import time
from dataclasses import dataclass, field
from typing import Any


class StepKind(str, enum.Enum):
    """Kind of execution step within the agent runtime."""

    THOUGHT = "thought"
    TOOL_CALL = "tool_call"
    MODEL_CALL = "model_call"
    OBSERVATION = "observation"
    FINAL = "final"


@dataclass
class StepRecord:
    """A single recorded step in AuditEvent.spec.steps[].

    Captures tokens_in, tokens_out, latency_ms, and optional error
    for each runtime step (thought / tool_call / model_call / observation / final).
    """

    step_type: StepKind
    tokens_in: int = 0
    tokens_out: int = 0
    latency_ms: float = 0.0
    error: str | None = None
    step_id: str = ""
    payload: dict[str, Any] = field(default_factory=dict)

    def to_audit_dict(self) -> dict[str, Any]:
        """Serialize to dict compatible with AuditEvent.spec.steps[] schema."""
        result: dict[str, Any] = {
            "step_type": self.step_type.value,
            "tokens_in": self.tokens_in,
            "tokens_out": self.tokens_out,
            "latency_ms": round(self.latency_ms, 2),
        }
        if self.error:
            result["error"] = self.error
        if self.step_id:
            result["step_id"] = self.step_id
        if self.payload:
            result["payload"] = self.payload
        return result


class RuntimeState(str, enum.Enum):
    """FSM states for agent runtime execution."""

    IDLE = "idle"
    THINKING = "thinking"
    CALLING_MODEL = "calling_model"
    CALLING_TOOL = "calling_tool"
    OBSERVING = "observing"
    COMPLETED = "completed"
    FAILED = "failed"


@dataclass
class ModelResponse:
    """Response from a model call."""

    content: str = ""
    tool_calls: list[ToolCallRequest] = field(default_factory=list)
    tokens_in: int = 0
    tokens_out: int = 0
    finish_reason: str = "stop"


@dataclass
class ToolCallRequest:
    """A tool call request from the model."""

    tool_name: str = ""
    tool_id: str = ""
    arguments: dict[str, Any] = field(default_factory=dict)


@dataclass
class ToolCallResult:
    """Result from executing a tool call."""

    tool_name: str = ""
    tool_id: str = ""
    output: str = ""
    error: str | None = None


class Timer:
    """Simple timer for measuring latency in milliseconds."""

    def __init__(self) -> None:
        self._start: float = 0.0

    def start(self) -> None:
        self._start = time.perf_counter()

    def elapsed_ms(self) -> float:
        return (time.perf_counter() - self._start) * 1000.0
