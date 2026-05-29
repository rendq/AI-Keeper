from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Decision(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DECISION_UNSPECIFIED: _ClassVar[Decision]
    DECISION_ALLOW: _ClassVar[Decision]
    DECISION_DENY: _ClassVar[Decision]
    DECISION_REQUIRE_APPROVAL: _ClassVar[Decision]
    DECISION_TIMEOUT: _ClassVar[Decision]

class Classification(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    CLASSIFICATION_UNSPECIFIED: _ClassVar[Classification]
    CLASSIFICATION_PUBLIC: _ClassVar[Classification]
    CLASSIFICATION_INTERNAL: _ClassVar[Classification]
    CLASSIFICATION_CONFIDENTIAL: _ClassVar[Classification]
    CLASSIFICATION_RESTRICTED: _ClassVar[Classification]
    CLASSIFICATION_SECRET: _ClassVar[Classification]
DECISION_UNSPECIFIED: Decision
DECISION_ALLOW: Decision
DECISION_DENY: Decision
DECISION_REQUIRE_APPROVAL: Decision
DECISION_TIMEOUT: Decision
CLASSIFICATION_UNSPECIFIED: Classification
CLASSIFICATION_PUBLIC: Classification
CLASSIFICATION_INTERNAL: Classification
CLASSIFICATION_CONFIDENTIAL: Classification
CLASSIFICATION_RESTRICTED: Classification
CLASSIFICATION_SECRET: Classification

class Principal(_message.Message):
    __slots__ = ("tenant_id", "user_id", "user_groups", "agent_name", "service_account", "on_behalf_of", "source_ip", "channel", "mfa", "device_id")
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    USER_GROUPS_FIELD_NUMBER: _ClassVar[int]
    AGENT_NAME_FIELD_NUMBER: _ClassVar[int]
    SERVICE_ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    ON_BEHALF_OF_FIELD_NUMBER: _ClassVar[int]
    SOURCE_IP_FIELD_NUMBER: _ClassVar[int]
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    MFA_FIELD_NUMBER: _ClassVar[int]
    DEVICE_ID_FIELD_NUMBER: _ClassVar[int]
    tenant_id: str
    user_id: str
    user_groups: _containers.RepeatedScalarFieldContainer[str]
    agent_name: str
    service_account: str
    on_behalf_of: str
    source_ip: str
    channel: str
    mfa: bool
    device_id: str
    def __init__(self, tenant_id: _Optional[str] = ..., user_id: _Optional[str] = ..., user_groups: _Optional[_Iterable[str]] = ..., agent_name: _Optional[str] = ..., service_account: _Optional[str] = ..., on_behalf_of: _Optional[str] = ..., source_ip: _Optional[str] = ..., channel: _Optional[str] = ..., mfa: bool = ..., device_id: _Optional[str] = ...) -> None: ...

class Action(_message.Message):
    __slots__ = ("verb", "resource_kind", "resource_name", "method")
    VERB_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_KIND_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_NAME_FIELD_NUMBER: _ClassVar[int]
    METHOD_FIELD_NUMBER: _ClassVar[int]
    verb: str
    resource_kind: str
    resource_name: str
    method: str
    def __init__(self, verb: _Optional[str] = ..., resource_kind: _Optional[str] = ..., resource_name: _Optional[str] = ..., method: _Optional[str] = ...) -> None: ...

class Resource(_message.Message):
    __slots__ = ("kind", "namespace", "name", "classification", "labels")
    class LabelsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    KIND_FIELD_NUMBER: _ClassVar[int]
    NAMESPACE_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    LABELS_FIELD_NUMBER: _ClassVar[int]
    kind: str
    namespace: str
    name: str
    classification: Classification
    labels: _containers.ScalarMap[str, str]
    def __init__(self, kind: _Optional[str] = ..., namespace: _Optional[str] = ..., name: _Optional[str] = ..., classification: _Optional[_Union[Classification, str]] = ..., labels: _Optional[_Mapping[str, str]] = ...) -> None: ...

class Conditions(_message.Message):
    __slots__ = ("attributes",)
    ATTRIBUTES_FIELD_NUMBER: _ClassVar[int]
    attributes: _struct_pb2.Struct
    def __init__(self, attributes: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class Obligation(_message.Message):
    __slots__ = ("kind", "parameters")
    KIND_FIELD_NUMBER: _ClassVar[int]
    PARAMETERS_FIELD_NUMBER: _ClassVar[int]
    kind: str
    parameters: _struct_pb2.Struct
    def __init__(self, kind: _Optional[str] = ..., parameters: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class DecisionRequest(_message.Message):
    __slots__ = ("principal", "action", "resource", "conditions", "trace_id", "invocation_id")
    PRINCIPAL_FIELD_NUMBER: _ClassVar[int]
    ACTION_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    CONDITIONS_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    principal: Principal
    action: Action
    resource: Resource
    conditions: Conditions
    trace_id: str
    invocation_id: str
    def __init__(self, principal: _Optional[_Union[Principal, _Mapping]] = ..., action: _Optional[_Union[Action, _Mapping]] = ..., resource: _Optional[_Union[Resource, _Mapping]] = ..., conditions: _Optional[_Union[Conditions, _Mapping]] = ..., trace_id: _Optional[str] = ..., invocation_id: _Optional[str] = ...) -> None: ...

class DecisionResponse(_message.Message):
    __slots__ = ("decision", "matched_policies", "obligations", "reason", "bundle_version", "evaluated_at")
    DECISION_FIELD_NUMBER: _ClassVar[int]
    MATCHED_POLICIES_FIELD_NUMBER: _ClassVar[int]
    OBLIGATIONS_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_VERSION_FIELD_NUMBER: _ClassVar[int]
    EVALUATED_AT_FIELD_NUMBER: _ClassVar[int]
    decision: Decision
    matched_policies: _containers.RepeatedScalarFieldContainer[str]
    obligations: _containers.RepeatedCompositeFieldContainer[Obligation]
    reason: str
    bundle_version: int
    evaluated_at: _timestamp_pb2.Timestamp
    def __init__(self, decision: _Optional[_Union[Decision, str]] = ..., matched_policies: _Optional[_Iterable[str]] = ..., obligations: _Optional[_Iterable[_Union[Obligation, _Mapping]]] = ..., reason: _Optional[str] = ..., bundle_version: _Optional[int] = ..., evaluated_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class BundleSubscribeRequest(_message.Message):
    __slots__ = ("pdp_id", "last_bundle_hash")
    PDP_ID_FIELD_NUMBER: _ClassVar[int]
    LAST_BUNDLE_HASH_FIELD_NUMBER: _ClassVar[int]
    pdp_id: str
    last_bundle_hash: str
    def __init__(self, pdp_id: _Optional[str] = ..., last_bundle_hash: _Optional[str] = ...) -> None: ...

class BundleEvent(_message.Message):
    __slots__ = ("kind", "bundle_version", "bundle_hash", "payload", "emitted_at")
    class Kind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        KIND_UNSPECIFIED: _ClassVar[BundleEvent.Kind]
        KIND_PUSH: _ClassVar[BundleEvent.Kind]
        KIND_RELOAD: _ClassVar[BundleEvent.Kind]
        KIND_INVALIDATE: _ClassVar[BundleEvent.Kind]
    KIND_UNSPECIFIED: BundleEvent.Kind
    KIND_PUSH: BundleEvent.Kind
    KIND_RELOAD: BundleEvent.Kind
    KIND_INVALIDATE: BundleEvent.Kind
    KIND_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_VERSION_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_HASH_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    EMITTED_AT_FIELD_NUMBER: _ClassVar[int]
    kind: BundleEvent.Kind
    bundle_version: int
    bundle_hash: str
    payload: bytes
    emitted_at: _timestamp_pb2.Timestamp
    def __init__(self, kind: _Optional[_Union[BundleEvent.Kind, str]] = ..., bundle_version: _Optional[int] = ..., bundle_hash: _Optional[str] = ..., payload: _Optional[bytes] = ..., emitted_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...
