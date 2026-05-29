from aip.v1 import pdp_pb2 as _pdp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class DlpAction(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DLP_ACTION_UNSPECIFIED: _ClassVar[DlpAction]
    DLP_ACTION_PASS: _ClassVar[DlpAction]
    DLP_ACTION_MASK: _ClassVar[DlpAction]
    DLP_ACTION_BLOCK: _ClassVar[DlpAction]
    DLP_ACTION_DETECT: _ClassVar[DlpAction]

class InspectStage(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    INSPECT_STAGE_UNSPECIFIED: _ClassVar[InspectStage]
    INSPECT_STAGE_INPUT: _ClassVar[InspectStage]
    INSPECT_STAGE_OUTPUT: _ClassVar[InspectStage]
DLP_ACTION_UNSPECIFIED: DlpAction
DLP_ACTION_PASS: DlpAction
DLP_ACTION_MASK: DlpAction
DLP_ACTION_BLOCK: DlpAction
DLP_ACTION_DETECT: DlpAction
INSPECT_STAGE_UNSPECIFIED: InspectStage
INSPECT_STAGE_INPUT: InspectStage
INSPECT_STAGE_OUTPUT: InspectStage

class DlpPolicy(_message.Message):
    __slots__ = ("patterns_ref", "categories", "action", "declared_classification")
    PATTERNS_REF_FIELD_NUMBER: _ClassVar[int]
    CATEGORIES_FIELD_NUMBER: _ClassVar[int]
    ACTION_FIELD_NUMBER: _ClassVar[int]
    DECLARED_CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    patterns_ref: str
    categories: _containers.RepeatedScalarFieldContainer[str]
    action: DlpAction
    declared_classification: _pdp_pb2.Classification
    def __init__(self, patterns_ref: _Optional[str] = ..., categories: _Optional[_Iterable[str]] = ..., action: _Optional[_Union[DlpAction, str]] = ..., declared_classification: _Optional[_Union[_pdp_pb2.Classification, str]] = ...) -> None: ...

class DlpFinding(_message.Message):
    __slots__ = ("category", "start", "end", "confidence", "rule")
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    START_FIELD_NUMBER: _ClassVar[int]
    END_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    RULE_FIELD_NUMBER: _ClassVar[int]
    category: str
    start: int
    end: int
    confidence: float
    rule: str
    def __init__(self, category: _Optional[str] = ..., start: _Optional[int] = ..., end: _Optional[int] = ..., confidence: _Optional[float] = ..., rule: _Optional[str] = ...) -> None: ...

class InspectRequest(_message.Message):
    __slots__ = ("text", "policy", "stage", "invocation_id", "tenant_id")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    POLICY_FIELD_NUMBER: _ClassVar[int]
    STAGE_FIELD_NUMBER: _ClassVar[int]
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    text: str
    policy: DlpPolicy
    stage: InspectStage
    invocation_id: str
    tenant_id: str
    def __init__(self, text: _Optional[str] = ..., policy: _Optional[_Union[DlpPolicy, _Mapping]] = ..., stage: _Optional[_Union[InspectStage, str]] = ..., invocation_id: _Optional[str] = ..., tenant_id: _Optional[str] = ...) -> None: ...

class InspectResponse(_message.Message):
    __slots__ = ("action_taken", "masked_text", "redactions", "findings", "classification")
    ACTION_TAKEN_FIELD_NUMBER: _ClassVar[int]
    MASKED_TEXT_FIELD_NUMBER: _ClassVar[int]
    REDACTIONS_FIELD_NUMBER: _ClassVar[int]
    FINDINGS_FIELD_NUMBER: _ClassVar[int]
    CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    action_taken: DlpAction
    masked_text: str
    redactions: _containers.RepeatedScalarFieldContainer[str]
    findings: _containers.RepeatedCompositeFieldContainer[DlpFinding]
    classification: _pdp_pb2.Classification
    def __init__(self, action_taken: _Optional[_Union[DlpAction, str]] = ..., masked_text: _Optional[str] = ..., redactions: _Optional[_Iterable[str]] = ..., findings: _Optional[_Iterable[_Union[DlpFinding, _Mapping]]] = ..., classification: _Optional[_Union[_pdp_pb2.Classification, str]] = ...) -> None: ...
