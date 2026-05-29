from aip.v1 import pdp_pb2 as _pdp_pb2
from aip.v1 import runtime_pb2 as _runtime_pb2
from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class OutcomeStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    OUTCOME_STATUS_UNSPECIFIED: _ClassVar[OutcomeStatus]
    OUTCOME_STATUS_SUCCESS: _ClassVar[OutcomeStatus]
    OUTCOME_STATUS_ERROR: _ClassVar[OutcomeStatus]
    OUTCOME_STATUS_TIMEOUT: _ClassVar[OutcomeStatus]
    OUTCOME_STATUS_BLOCKED: _ClassVar[OutcomeStatus]
    OUTCOME_STATUS_PARTIAL: _ClassVar[OutcomeStatus]
OUTCOME_STATUS_UNSPECIFIED: OutcomeStatus
OUTCOME_STATUS_SUCCESS: OutcomeStatus
OUTCOME_STATUS_ERROR: OutcomeStatus
OUTCOME_STATUS_TIMEOUT: OutcomeStatus
OUTCOME_STATUS_BLOCKED: OutcomeStatus
OUTCOME_STATUS_PARTIAL: OutcomeStatus

class CostBlock(_message.Message):
    __slots__ = ("tokens_input", "tokens_output", "tokens_cached", "usd", "duration")
    TOKENS_INPUT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_OUTPUT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_CACHED_FIELD_NUMBER: _ClassVar[int]
    USD_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    tokens_input: int
    tokens_output: int
    tokens_cached: int
    usd: str
    duration: _duration_pb2.Duration
    def __init__(self, tokens_input: _Optional[int] = ..., tokens_output: _Optional[int] = ..., tokens_cached: _Optional[int] = ..., usd: _Optional[str] = ..., duration: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...

class RequestBlock(_message.Message):
    __slots__ = ("input_hash", "classification", "redactions", "size_bytes")
    INPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    REDACTIONS_FIELD_NUMBER: _ClassVar[int]
    SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    input_hash: str
    classification: _pdp_pb2.Classification
    redactions: _containers.RepeatedScalarFieldContainer[str]
    size_bytes: int
    def __init__(self, input_hash: _Optional[str] = ..., classification: _Optional[_Union[_pdp_pb2.Classification, str]] = ..., redactions: _Optional[_Iterable[str]] = ..., size_bytes: _Optional[int] = ...) -> None: ...

class ResponseBlock(_message.Message):
    __slots__ = ("output_hash", "citations", "classification", "size_bytes")
    OUTPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    CITATIONS_FIELD_NUMBER: _ClassVar[int]
    CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    output_hash: str
    citations: _containers.RepeatedScalarFieldContainer[str]
    classification: _pdp_pb2.Classification
    size_bytes: int
    def __init__(self, output_hash: _Optional[str] = ..., citations: _Optional[_Iterable[str]] = ..., classification: _Optional[_Union[_pdp_pb2.Classification, str]] = ..., size_bytes: _Optional[int] = ...) -> None: ...

class PolicyDecisionBlock(_message.Message):
    __slots__ = ("decision", "matched_policies", "reason", "obligations_applied", "approval_id", "bundle_version")
    DECISION_FIELD_NUMBER: _ClassVar[int]
    MATCHED_POLICIES_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    OBLIGATIONS_APPLIED_FIELD_NUMBER: _ClassVar[int]
    APPROVAL_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_VERSION_FIELD_NUMBER: _ClassVar[int]
    decision: _pdp_pb2.Decision
    matched_policies: _containers.RepeatedScalarFieldContainer[str]
    reason: str
    obligations_applied: _containers.RepeatedScalarFieldContainer[str]
    approval_id: str
    bundle_version: int
    def __init__(self, decision: _Optional[_Union[_pdp_pb2.Decision, str]] = ..., matched_policies: _Optional[_Iterable[str]] = ..., reason: _Optional[str] = ..., obligations_applied: _Optional[_Iterable[str]] = ..., approval_id: _Optional[str] = ..., bundle_version: _Optional[int] = ...) -> None: ...

class GuardrailBlock(_message.Message):
    __slots__ = ("triggered", "blocked")
    TRIGGERED_FIELD_NUMBER: _ClassVar[int]
    BLOCKED_FIELD_NUMBER: _ClassVar[int]
    triggered: _containers.RepeatedScalarFieldContainer[str]
    blocked: bool
    def __init__(self, triggered: _Optional[_Iterable[str]] = ..., blocked: bool = ...) -> None: ...

class ComplianceBlock(_message.Message):
    __slots__ = ("tags", "data_residency", "holds")
    TAGS_FIELD_NUMBER: _ClassVar[int]
    DATA_RESIDENCY_FIELD_NUMBER: _ClassVar[int]
    HOLDS_FIELD_NUMBER: _ClassVar[int]
    tags: _containers.RepeatedScalarFieldContainer[str]
    data_residency: str
    holds: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, tags: _Optional[_Iterable[str]] = ..., data_residency: _Optional[str] = ..., holds: _Optional[_Iterable[str]] = ...) -> None: ...

class OutcomeBlock(_message.Message):
    __slots__ = ("status", "error_code", "error_message")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    status: OutcomeStatus
    error_code: str
    error_message: str
    def __init__(self, status: _Optional[_Union[OutcomeStatus, str]] = ..., error_code: _Optional[str] = ..., error_message: _Optional[str] = ...) -> None: ...

class AuditEvent(_message.Message):
    __slots__ = ("invocation_id", "step_id", "timestamp", "tenant_id", "namespace", "principal", "action", "request", "response", "policy", "guardrails", "cost", "compliance", "outcome", "steps", "trace_id", "span_id", "event_hash", "raw_object_uri")
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    STEP_ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    NAMESPACE_FIELD_NUMBER: _ClassVar[int]
    PRINCIPAL_FIELD_NUMBER: _ClassVar[int]
    ACTION_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FIELD_NUMBER: _ClassVar[int]
    POLICY_FIELD_NUMBER: _ClassVar[int]
    GUARDRAILS_FIELD_NUMBER: _ClassVar[int]
    COST_FIELD_NUMBER: _ClassVar[int]
    COMPLIANCE_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    STEPS_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    SPAN_ID_FIELD_NUMBER: _ClassVar[int]
    EVENT_HASH_FIELD_NUMBER: _ClassVar[int]
    RAW_OBJECT_URI_FIELD_NUMBER: _ClassVar[int]
    invocation_id: str
    step_id: str
    timestamp: _timestamp_pb2.Timestamp
    tenant_id: str
    namespace: str
    principal: _pdp_pb2.Principal
    action: _pdp_pb2.Action
    request: RequestBlock
    response: ResponseBlock
    policy: PolicyDecisionBlock
    guardrails: GuardrailBlock
    cost: CostBlock
    compliance: ComplianceBlock
    outcome: OutcomeBlock
    steps: _containers.RepeatedCompositeFieldContainer[_runtime_pb2.Step]
    trace_id: str
    span_id: str
    event_hash: str
    raw_object_uri: str
    def __init__(self, invocation_id: _Optional[str] = ..., step_id: _Optional[str] = ..., timestamp: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., tenant_id: _Optional[str] = ..., namespace: _Optional[str] = ..., principal: _Optional[_Union[_pdp_pb2.Principal, _Mapping]] = ..., action: _Optional[_Union[_pdp_pb2.Action, _Mapping]] = ..., request: _Optional[_Union[RequestBlock, _Mapping]] = ..., response: _Optional[_Union[ResponseBlock, _Mapping]] = ..., policy: _Optional[_Union[PolicyDecisionBlock, _Mapping]] = ..., guardrails: _Optional[_Union[GuardrailBlock, _Mapping]] = ..., cost: _Optional[_Union[CostBlock, _Mapping]] = ..., compliance: _Optional[_Union[ComplianceBlock, _Mapping]] = ..., outcome: _Optional[_Union[OutcomeBlock, _Mapping]] = ..., steps: _Optional[_Iterable[_Union[_runtime_pb2.Step, _Mapping]]] = ..., trace_id: _Optional[str] = ..., span_id: _Optional[str] = ..., event_hash: _Optional[str] = ..., raw_object_uri: _Optional[str] = ...) -> None: ...

class EmitRequest(_message.Message):
    __slots__ = ("event",)
    EVENT_FIELD_NUMBER: _ClassVar[int]
    event: AuditEvent
    def __init__(self, event: _Optional[_Union[AuditEvent, _Mapping]] = ...) -> None: ...

class EmitResponse(_message.Message):
    __slots__ = ("accepted", "event_hash", "raw_object_uri")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    EVENT_HASH_FIELD_NUMBER: _ClassVar[int]
    RAW_OBJECT_URI_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    event_hash: str
    raw_object_uri: str
    def __init__(self, accepted: bool = ..., event_hash: _Optional[str] = ..., raw_object_uri: _Optional[str] = ...) -> None: ...

class QueryRequest(_message.Message):
    __slots__ = ("sql", "limit", "tenant_id")
    SQL_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    sql: str
    limit: int
    tenant_id: str
    def __init__(self, sql: _Optional[str] = ..., limit: _Optional[int] = ..., tenant_id: _Optional[str] = ...) -> None: ...
