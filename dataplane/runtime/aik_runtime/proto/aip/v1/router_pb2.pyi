from aip.v1 import pdp_pb2 as _pdp_pb2
from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ModelCapability(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    MODEL_CAPABILITY_UNSPECIFIED: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_REASONER: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_EMBEDDER: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_VISION: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_SUMMARIZER: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_TRANSLATOR: _ClassVar[ModelCapability]
    MODEL_CAPABILITY_GUARDRAIL: _ClassVar[ModelCapability]
MODEL_CAPABILITY_UNSPECIFIED: ModelCapability
MODEL_CAPABILITY_REASONER: ModelCapability
MODEL_CAPABILITY_EMBEDDER: ModelCapability
MODEL_CAPABILITY_VISION: ModelCapability
MODEL_CAPABILITY_SUMMARIZER: ModelCapability
MODEL_CAPABILITY_TRANSLATOR: ModelCapability
MODEL_CAPABILITY_GUARDRAIL: ModelCapability

class ResolvedEndpoint(_message.Message):
    __slots__ = ("endpoint_ref", "provider", "region", "compliance_tags", "input_usd_per_mtok", "output_usd_per_mtok", "cached_input_usd_per_mtok")
    ENDPOINT_REF_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FIELD_NUMBER: _ClassVar[int]
    REGION_FIELD_NUMBER: _ClassVar[int]
    COMPLIANCE_TAGS_FIELD_NUMBER: _ClassVar[int]
    INPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    CACHED_INPUT_USD_PER_MTOK_FIELD_NUMBER: _ClassVar[int]
    endpoint_ref: str
    provider: str
    region: str
    compliance_tags: _containers.RepeatedScalarFieldContainer[str]
    input_usd_per_mtok: str
    output_usd_per_mtok: str
    cached_input_usd_per_mtok: str
    def __init__(self, endpoint_ref: _Optional[str] = ..., provider: _Optional[str] = ..., region: _Optional[str] = ..., compliance_tags: _Optional[_Iterable[str]] = ..., input_usd_per_mtok: _Optional[str] = ..., output_usd_per_mtok: _Optional[str] = ..., cached_input_usd_per_mtok: _Optional[str] = ...) -> None: ...

class RoutingContext(_message.Message):
    __slots__ = ("tenant_id", "classification", "attributes")
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    CLASSIFICATION_FIELD_NUMBER: _ClassVar[int]
    ATTRIBUTES_FIELD_NUMBER: _ClassVar[int]
    tenant_id: str
    classification: _pdp_pb2.Classification
    attributes: _struct_pb2.Struct
    def __init__(self, tenant_id: _Optional[str] = ..., classification: _Optional[_Union[_pdp_pb2.Classification, str]] = ..., attributes: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class RouteRequest(_message.Message):
    __slots__ = ("capability", "alias", "router_ref", "context", "cache_key", "trace_id", "invocation_id")
    CAPABILITY_FIELD_NUMBER: _ClassVar[int]
    ALIAS_FIELD_NUMBER: _ClassVar[int]
    ROUTER_REF_FIELD_NUMBER: _ClassVar[int]
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    CACHE_KEY_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    capability: ModelCapability
    alias: str
    router_ref: str
    context: RoutingContext
    cache_key: str
    trace_id: str
    invocation_id: str
    def __init__(self, capability: _Optional[_Union[ModelCapability, str]] = ..., alias: _Optional[str] = ..., router_ref: _Optional[str] = ..., context: _Optional[_Union[RoutingContext, _Mapping]] = ..., cache_key: _Optional[str] = ..., trace_id: _Optional[str] = ..., invocation_id: _Optional[str] = ...) -> None: ...

class RouteResponse(_message.Message):
    __slots__ = ("endpoint", "cache_hit", "cache_similarity", "fallback")
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    CACHE_HIT_FIELD_NUMBER: _ClassVar[int]
    CACHE_SIMILARITY_FIELD_NUMBER: _ClassVar[int]
    FALLBACK_FIELD_NUMBER: _ClassVar[int]
    endpoint: ResolvedEndpoint
    cache_hit: bool
    cache_similarity: float
    fallback: _containers.RepeatedCompositeFieldContainer[ResolvedEndpoint]
    def __init__(self, endpoint: _Optional[_Union[ResolvedEndpoint, _Mapping]] = ..., cache_hit: bool = ..., cache_similarity: _Optional[float] = ..., fallback: _Optional[_Iterable[_Union[ResolvedEndpoint, _Mapping]]] = ...) -> None: ...

class ChatRequest(_message.Message):
    __slots__ = ("route", "payload", "stream", "timeout")
    ROUTE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    STREAM_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_FIELD_NUMBER: _ClassVar[int]
    route: RouteRequest
    payload: _struct_pb2.Struct
    stream: bool
    timeout: _duration_pb2.Duration
    def __init__(self, route: _Optional[_Union[RouteRequest, _Mapping]] = ..., payload: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., stream: bool = ..., timeout: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...

class ChatChunk(_message.Message):
    __slots__ = ("delta", "usage", "done")
    DELTA_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    DONE_FIELD_NUMBER: _ClassVar[int]
    delta: _struct_pb2.Struct
    usage: ChatUsage
    done: bool
    def __init__(self, delta: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., usage: _Optional[_Union[ChatUsage, _Mapping]] = ..., done: bool = ...) -> None: ...

class ChatUsage(_message.Message):
    __slots__ = ("tokens_in", "tokens_out", "tokens_cached", "usd")
    TOKENS_IN_FIELD_NUMBER: _ClassVar[int]
    TOKENS_OUT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_CACHED_FIELD_NUMBER: _ClassVar[int]
    USD_FIELD_NUMBER: _ClassVar[int]
    tokens_in: int
    tokens_out: int
    tokens_cached: int
    usd: str
    def __init__(self, tokens_in: _Optional[int] = ..., tokens_out: _Optional[int] = ..., tokens_cached: _Optional[int] = ..., usd: _Optional[str] = ...) -> None: ...
