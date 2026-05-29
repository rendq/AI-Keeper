from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class IdentityProvider(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    IDENTITY_PROVIDER_UNSPECIFIED: _ClassVar[IdentityProvider]
    IDENTITY_PROVIDER_OIDC: _ClassVar[IdentityProvider]
    IDENTITY_PROVIDER_SAML: _ClassVar[IdentityProvider]
    IDENTITY_PROVIDER_SPIFFE: _ClassVar[IdentityProvider]
IDENTITY_PROVIDER_UNSPECIFIED: IdentityProvider
IDENTITY_PROVIDER_OIDC: IdentityProvider
IDENTITY_PROVIDER_SAML: IdentityProvider
IDENTITY_PROVIDER_SPIFFE: IdentityProvider

class Subject(_message.Message):
    __slots__ = ("tenant_id", "user_id", "user_groups", "service_account", "spiffe_id", "email", "claims", "expires_at")
    class ClaimsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    USER_GROUPS_FIELD_NUMBER: _ClassVar[int]
    SERVICE_ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    SPIFFE_ID_FIELD_NUMBER: _ClassVar[int]
    EMAIL_FIELD_NUMBER: _ClassVar[int]
    CLAIMS_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    tenant_id: str
    user_id: str
    user_groups: _containers.RepeatedScalarFieldContainer[str]
    service_account: str
    spiffe_id: str
    email: str
    claims: _containers.ScalarMap[str, str]
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, tenant_id: _Optional[str] = ..., user_id: _Optional[str] = ..., user_groups: _Optional[_Iterable[str]] = ..., service_account: _Optional[str] = ..., spiffe_id: _Optional[str] = ..., email: _Optional[str] = ..., claims: _Optional[_Mapping[str, str]] = ..., expires_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class VerifyRequest(_message.Message):
    __slots__ = ("token", "provider", "audience")
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FIELD_NUMBER: _ClassVar[int]
    AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    token: str
    provider: IdentityProvider
    audience: str
    def __init__(self, token: _Optional[str] = ..., provider: _Optional[_Union[IdentityProvider, str]] = ..., audience: _Optional[str] = ...) -> None: ...

class VerifyResponse(_message.Message):
    __slots__ = ("valid", "subject", "failure_reason")
    VALID_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_FIELD_NUMBER: _ClassVar[int]
    FAILURE_REASON_FIELD_NUMBER: _ClassVar[int]
    valid: bool
    subject: Subject
    failure_reason: str
    def __init__(self, valid: bool = ..., subject: _Optional[_Union[Subject, _Mapping]] = ..., failure_reason: _Optional[str] = ...) -> None: ...

class ExchangeRequest(_message.Message):
    __slots__ = ("subject_token", "subject_token_type", "audience", "tenant_id", "agent_name", "scopes")
    SUBJECT_TOKEN_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_TOKEN_TYPE_FIELD_NUMBER: _ClassVar[int]
    AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    AGENT_NAME_FIELD_NUMBER: _ClassVar[int]
    SCOPES_FIELD_NUMBER: _ClassVar[int]
    subject_token: str
    subject_token_type: str
    audience: str
    tenant_id: str
    agent_name: str
    scopes: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, subject_token: _Optional[str] = ..., subject_token_type: _Optional[str] = ..., audience: _Optional[str] = ..., tenant_id: _Optional[str] = ..., agent_name: _Optional[str] = ..., scopes: _Optional[_Iterable[str]] = ...) -> None: ...

class ExchangeResponse(_message.Message):
    __slots__ = ("access_token", "issued_token_type", "expires_at", "scopes")
    ACCESS_TOKEN_FIELD_NUMBER: _ClassVar[int]
    ISSUED_TOKEN_TYPE_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    SCOPES_FIELD_NUMBER: _ClassVar[int]
    access_token: str
    issued_token_type: str
    expires_at: _timestamp_pb2.Timestamp
    scopes: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, access_token: _Optional[str] = ..., issued_token_type: _Optional[str] = ..., expires_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., scopes: _Optional[_Iterable[str]] = ...) -> None: ...

class RevokeRequest(_message.Message):
    __slots__ = ("service_account", "access_token", "reason")
    SERVICE_ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    ACCESS_TOKEN_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    service_account: str
    access_token: str
    reason: str
    def __init__(self, service_account: _Optional[str] = ..., access_token: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class RevokeResponse(_message.Message):
    __slots__ = ("revoked_count", "revoked_at")
    REVOKED_COUNT_FIELD_NUMBER: _ClassVar[int]
    REVOKED_AT_FIELD_NUMBER: _ClassVar[int]
    revoked_count: int
    revoked_at: _timestamp_pb2.Timestamp
    def __init__(self, revoked_count: _Optional[int] = ..., revoked_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...
