from aip.v1 import pdp_pb2 as _pdp_pb2
from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class RuntimePattern(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUNTIME_PATTERN_UNSPECIFIED: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_REACT: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_PLAN_EXECUTE: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_REFLECTION: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_WORKFLOW: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_TOOL_CALLING: _ClassVar[RuntimePattern]
    RUNTIME_PATTERN_MULTI_AGENT: _ClassVar[RuntimePattern]

class StepKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    STEP_KIND_UNSPECIFIED: _ClassVar[StepKind]
    STEP_KIND_THOUGHT: _ClassVar[StepKind]
    STEP_KIND_TOOL_CALL: _ClassVar[StepKind]
    STEP_KIND_MODEL_CALL: _ClassVar[StepKind]
    STEP_KIND_OBSERVATION: _ClassVar[StepKind]
    STEP_KIND_GUARDRAIL: _ClassVar[StepKind]
    STEP_KIND_RETRIEVAL: _ClassVar[StepKind]
    STEP_KIND_FINAL: _ClassVar[StepKind]

class InvokeStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    INVOKE_STATUS_UNSPECIFIED: _ClassVar[InvokeStatus]
    INVOKE_STATUS_SUCCESS: _ClassVar[InvokeStatus]
    INVOKE_STATUS_ERROR: _ClassVar[InvokeStatus]
    INVOKE_STATUS_TIMEOUT: _ClassVar[InvokeStatus]
    INVOKE_STATUS_BLOCKED: _ClassVar[InvokeStatus]
    INVOKE_STATUS_PARTIAL: _ClassVar[InvokeStatus]
RUNTIME_PATTERN_UNSPECIFIED: RuntimePattern
RUNTIME_PATTERN_REACT: RuntimePattern
RUNTIME_PATTERN_PLAN_EXECUTE: RuntimePattern
RUNTIME_PATTERN_REFLECTION: RuntimePattern
RUNTIME_PATTERN_WORKFLOW: RuntimePattern
RUNTIME_PATTERN_TOOL_CALLING: RuntimePattern
RUNTIME_PATTERN_MULTI_AGENT: RuntimePattern
STEP_KIND_UNSPECIFIED: StepKind
STEP_KIND_THOUGHT: StepKind
STEP_KIND_TOOL_CALL: StepKind
STEP_KIND_MODEL_CALL: StepKind
STEP_KIND_OBSERVATION: StepKind
STEP_KIND_GUARDRAIL: StepKind
STEP_KIND_RETRIEVAL: StepKind
STEP_KIND_FINAL: StepKind
INVOKE_STATUS_UNSPECIFIED: InvokeStatus
INVOKE_STATUS_SUCCESS: InvokeStatus
INVOKE_STATUS_ERROR: InvokeStatus
INVOKE_STATUS_TIMEOUT: InvokeStatus
INVOKE_STATUS_BLOCKED: InvokeStatus
INVOKE_STATUS_PARTIAL: InvokeStatus

class Determinism(_message.Message):
    __slots__ = ("temperature", "top_p", "seed")
    TEMPERATURE_FIELD_NUMBER: _ClassVar[int]
    TOP_P_FIELD_NUMBER: _ClassVar[int]
    SEED_FIELD_NUMBER: _ClassVar[int]
    temperature: float
    top_p: float
    seed: int
    def __init__(self, temperature: _Optional[float] = ..., top_p: _Optional[float] = ..., seed: _Optional[int] = ...) -> None: ...

class RuntimeLimits(_message.Message):
    __slots__ = ("max_steps", "max_tool_calls", "timeout", "parallelism")
    MAX_STEPS_FIELD_NUMBER: _ClassVar[int]
    MAX_TOOL_CALLS_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_FIELD_NUMBER: _ClassVar[int]
    PARALLELISM_FIELD_NUMBER: _ClassVar[int]
    max_steps: int
    max_tool_calls: int
    timeout: _duration_pb2.Duration
    parallelism: int
    def __init__(self, max_steps: _Optional[int] = ..., max_tool_calls: _Optional[int] = ..., timeout: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., parallelism: _Optional[int] = ...) -> None: ...

class Message(_message.Message):
    __slots__ = ("role", "content", "attachments")
    ROLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENTS_FIELD_NUMBER: _ClassVar[int]
    role: str
    content: str
    attachments: _struct_pb2.Struct
    def __init__(self, role: _Optional[str] = ..., content: _Optional[str] = ..., attachments: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class InvokeRequest(_message.Message):
    __slots__ = ("agent_ref", "session_id", "invocation_id", "trace_id", "principal", "pattern", "limits", "determinism", "messages", "user_context")
    AGENT_REF_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    PRINCIPAL_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    LIMITS_FIELD_NUMBER: _ClassVar[int]
    DETERMINISM_FIELD_NUMBER: _ClassVar[int]
    MESSAGES_FIELD_NUMBER: _ClassVar[int]
    USER_CONTEXT_FIELD_NUMBER: _ClassVar[int]
    agent_ref: str
    session_id: str
    invocation_id: str
    trace_id: str
    principal: _pdp_pb2.Principal
    pattern: RuntimePattern
    limits: RuntimeLimits
    determinism: Determinism
    messages: _containers.RepeatedCompositeFieldContainer[Message]
    user_context: _struct_pb2.Struct
    def __init__(self, agent_ref: _Optional[str] = ..., session_id: _Optional[str] = ..., invocation_id: _Optional[str] = ..., trace_id: _Optional[str] = ..., principal: _Optional[_Union[_pdp_pb2.Principal, _Mapping]] = ..., pattern: _Optional[_Union[RuntimePattern, str]] = ..., limits: _Optional[_Union[RuntimeLimits, _Mapping]] = ..., determinism: _Optional[_Union[Determinism, _Mapping]] = ..., messages: _Optional[_Iterable[_Union[Message, _Mapping]]] = ..., user_context: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class Step(_message.Message):
    __slots__ = ("kind", "id", "started_at", "latency", "tokens_in", "tokens_out", "tokens_cached", "payload", "guardrails_triggered", "guardrail_blocked", "error_code", "error_message")
    KIND_FIELD_NUMBER: _ClassVar[int]
    ID_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    LATENCY_FIELD_NUMBER: _ClassVar[int]
    TOKENS_IN_FIELD_NUMBER: _ClassVar[int]
    TOKENS_OUT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_CACHED_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    GUARDRAILS_TRIGGERED_FIELD_NUMBER: _ClassVar[int]
    GUARDRAIL_BLOCKED_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    kind: StepKind
    id: str
    started_at: _timestamp_pb2.Timestamp
    latency: _duration_pb2.Duration
    tokens_in: int
    tokens_out: int
    tokens_cached: int
    payload: _struct_pb2.Struct
    guardrails_triggered: _containers.RepeatedScalarFieldContainer[str]
    guardrail_blocked: bool
    error_code: str
    error_message: str
    def __init__(self, kind: _Optional[_Union[StepKind, str]] = ..., id: _Optional[str] = ..., started_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., latency: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., tokens_in: _Optional[int] = ..., tokens_out: _Optional[int] = ..., tokens_cached: _Optional[int] = ..., payload: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., guardrails_triggered: _Optional[_Iterable[str]] = ..., guardrail_blocked: bool = ..., error_code: _Optional[str] = ..., error_message: _Optional[str] = ...) -> None: ...

class InvokeEvent(_message.Message):
    __slots__ = ("step", "final", "delta")
    STEP_FIELD_NUMBER: _ClassVar[int]
    FINAL_FIELD_NUMBER: _ClassVar[int]
    DELTA_FIELD_NUMBER: _ClassVar[int]
    step: Step
    final: InvokeFinal
    delta: Message
    def __init__(self, step: _Optional[_Union[Step, _Mapping]] = ..., final: _Optional[_Union[InvokeFinal, _Mapping]] = ..., delta: _Optional[_Union[Message, _Mapping]] = ...) -> None: ...

class InvokeFinal(_message.Message):
    __slots__ = ("status", "response", "error_code", "error_message", "tokens_in", "tokens_out", "tokens_cached", "cost_usd", "duration", "citations")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    TOKENS_IN_FIELD_NUMBER: _ClassVar[int]
    TOKENS_OUT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_CACHED_FIELD_NUMBER: _ClassVar[int]
    COST_USD_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    CITATIONS_FIELD_NUMBER: _ClassVar[int]
    status: InvokeStatus
    response: Message
    error_code: str
    error_message: str
    tokens_in: int
    tokens_out: int
    tokens_cached: int
    cost_usd: float
    duration: _duration_pb2.Duration
    citations: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, status: _Optional[_Union[InvokeStatus, str]] = ..., response: _Optional[_Union[Message, _Mapping]] = ..., error_code: _Optional[str] = ..., error_message: _Optional[str] = ..., tokens_in: _Optional[int] = ..., tokens_out: _Optional[int] = ..., tokens_cached: _Optional[int] = ..., cost_usd: _Optional[float] = ..., duration: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., citations: _Optional[_Iterable[str]] = ...) -> None: ...
