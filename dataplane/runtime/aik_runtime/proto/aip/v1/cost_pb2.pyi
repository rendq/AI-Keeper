from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class BudgetScopeKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    BUDGET_SCOPE_KIND_UNSPECIFIED: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_TENANT: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_TEAM: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_USER: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_AGENT: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_SKILL: _ClassVar[BudgetScopeKind]
    BUDGET_SCOPE_KIND_PROJECT: _ClassVar[BudgetScopeKind]
BUDGET_SCOPE_KIND_UNSPECIFIED: BudgetScopeKind
BUDGET_SCOPE_KIND_TENANT: BudgetScopeKind
BUDGET_SCOPE_KIND_TEAM: BudgetScopeKind
BUDGET_SCOPE_KIND_USER: BudgetScopeKind
BUDGET_SCOPE_KIND_AGENT: BudgetScopeKind
BUDGET_SCOPE_KIND_SKILL: BudgetScopeKind
BUDGET_SCOPE_KIND_PROJECT: BudgetScopeKind

class TokenUsage(_message.Message):
    __slots__ = ("input_tokens", "output_tokens", "cached_tokens")
    INPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    CACHED_TOKENS_FIELD_NUMBER: _ClassVar[int]
    input_tokens: int
    output_tokens: int
    cached_tokens: int
    def __init__(self, input_tokens: _Optional[int] = ..., output_tokens: _Optional[int] = ..., cached_tokens: _Optional[int] = ...) -> None: ...

class Pricing(_message.Message):
    __slots__ = ("input_usd_per_mtok", "output_usd_per_mtok", "cached_input_usd_per_mtok")
    INPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    CACHED_INPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    input_usd_per_mtok: str
    output_usd_per_mtok: str
    cached_input_usd_per_mtok: str
    def __init__(self, input_usd_per_mtok: _Optional[str] = ..., output_usd_per_mtok: _Optional[str] = ..., cached_input_usd_per_mtok: _Optional[str] = ...) -> None: ...

class BudgetScope(_message.Message):
    __slots__ = ("kind", "id", "period_id")
    KIND_FIELD_NUMBER: _ClassVar[int]
    ID_FIELD_NUMBER: _ClassVar[int]
    PERIOD_ID_FIELD_NUMBER: _ClassVar[int]
    kind: BudgetScopeKind
    id: str
    period_id: str
    def __init__(self, kind: _Optional[_Union[BudgetScopeKind, str]] = ..., id: _Optional[str] = ..., period_id: _Optional[str] = ...) -> None: ...

class ComputeRequest(_message.Message):
    __slots__ = ("usage", "pricing", "duration")
    USAGE_FIELD_NUMBER: _ClassVar[int]
    PRICING_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    usage: TokenUsage
    pricing: Pricing
    duration: _duration_pb2.Duration
    def __init__(self, usage: _Optional[_Union[TokenUsage, _Mapping]] = ..., pricing: _Optional[_Union[Pricing, _Mapping]] = ..., duration: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...

class ComputeResponse(_message.Message):
    __slots__ = ("usd", "usage", "duration")
    USD_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    usd: str
    usage: TokenUsage
    duration: _duration_pb2.Duration
    def __init__(self, usd: _Optional[str] = ..., usage: _Optional[_Union[TokenUsage, _Mapping]] = ..., duration: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...

class RecordRequest(_message.Message):
    __slots__ = ("invocation_id", "tenant_id", "endpoint_ref", "usage", "pricing", "duration", "scopes", "at")
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_REF_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    PRICING_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    SCOPES_FIELD_NUMBER: _ClassVar[int]
    AT_FIELD_NUMBER: _ClassVar[int]
    invocation_id: str
    tenant_id: str
    endpoint_ref: str
    usage: TokenUsage
    pricing: Pricing
    duration: _duration_pb2.Duration
    scopes: _containers.RepeatedCompositeFieldContainer[BudgetScope]
    at: _timestamp_pb2.Timestamp
    def __init__(self, invocation_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., endpoint_ref: _Optional[str] = ..., usage: _Optional[_Union[TokenUsage, _Mapping]] = ..., pricing: _Optional[_Union[Pricing, _Mapping]] = ..., duration: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., scopes: _Optional[_Iterable[_Union[BudgetScope, _Mapping]]] = ..., at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class RecordResponse(_message.Message):
    __slots__ = ("usd", "balances")
    USD_FIELD_NUMBER: _ClassVar[int]
    BALANCES_FIELD_NUMBER: _ClassVar[int]
    usd: str
    balances: _containers.RepeatedCompositeFieldContainer[ScopeBalance]
    def __init__(self, usd: _Optional[str] = ..., balances: _Optional[_Iterable[_Union[ScopeBalance, _Mapping]]] = ...) -> None: ...

class ScopeBalance(_message.Message):
    __slots__ = ("scope", "current_usd", "limit_usd", "burn_rate")
    SCOPE_FIELD_NUMBER: _ClassVar[int]
    CURRENT_USD_FIELD_NUMBER: _ClassVar[int]
    LIMIT_USD_FIELD_NUMBER: _ClassVar[int]
    BURN_RATE_FIELD_NUMBER: _ClassVar[int]
    scope: BudgetScope
    current_usd: str
    limit_usd: str
    burn_rate: float
    def __init__(self, scope: _Optional[_Union[BudgetScope, _Mapping]] = ..., current_usd: _Optional[str] = ..., limit_usd: _Optional[str] = ..., burn_rate: _Optional[float] = ...) -> None: ...

class BalanceRequest(_message.Message):
    __slots__ = ("scopes",)
    SCOPES_FIELD_NUMBER: _ClassVar[int]
    scopes: _containers.RepeatedCompositeFieldContainer[BudgetScope]
    def __init__(self, scopes: _Optional[_Iterable[_Union[BudgetScope, _Mapping]]] = ...) -> None: ...

class BalanceResponse(_message.Message):
    __slots__ = ("balances",)
    BALANCES_FIELD_NUMBER: _ClassVar[int]
    balances: _containers.RepeatedCompositeFieldContainer[ScopeBalance]
    def __init__(self, balances: _Optional[_Iterable[_Union[ScopeBalance, _Mapping]]] = ...) -> None: ...
