"""Tests for KB ACL pre-filter implementation."""

from __future__ import annotations

import pytest

from dataplane.kb.retriever import RetrievalResult
from dataplane.kb.retriever.acl import ACLFilter, UserContext, build_qdrant_filter


@pytest.fixture
def acl_filter() -> ACLFilter:
    return ACLFilter()


def _make_result(
    text: str = "chunk",
    acl_users: str = "",
    acl_groups: str = "",
    classification: str = "public",
) -> RetrievalResult:
    metadata: dict[str, str] = {}
    if acl_users:
        metadata["acl_users"] = acl_users
    if acl_groups:
        metadata["acl_groups"] = acl_groups
    metadata["classification"] = classification
    return RetrievalResult(text=text, score=1.0, metadata=metadata)


class TestACLFilterUserAccess:
    """Test user-level ACL checks."""

    def test_user_in_acl_users_allowed(self, acl_filter: ACLFilter) -> None:
        """User listed in acl_users should be allowed access."""
        user = UserContext(user_id="alice", groups=[], classification_level="internal")
        result = _make_result(text="secret doc", acl_users="alice,bob", classification="public")

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 1
        assert filtered[0].text == "secret doc"

    def test_user_not_in_acl_users_filtered(self, acl_filter: ACLFilter) -> None:
        """User NOT in acl_users (and no group match) should be filtered out."""
        user = UserContext(user_id="charlie", groups=[], classification_level="internal")
        result = _make_result(text="private doc", acl_users="alice,bob", classification="public")

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 0


class TestACLFilterGroupAccess:
    """Test group-level ACL checks."""

    def test_group_membership_allowed(self, acl_filter: ACLFilter) -> None:
        """User with matching group membership should be allowed."""
        user = UserContext(
            user_id="charlie", groups=["engineering", "ops"], classification_level="internal"
        )
        result = _make_result(text="eng doc", acl_groups="engineering,hr", classification="public")

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 1
        assert filtered[0].text == "eng doc"

    def test_no_group_match_filtered(self, acl_filter: ACLFilter) -> None:
        """User without matching group should be filtered."""
        user = UserContext(
            user_id="charlie", groups=["marketing"], classification_level="internal"
        )
        result = _make_result(text="eng doc", acl_groups="engineering,hr", classification="public")

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 0


class TestACLFilterClassification:
    """Test classification level checks."""

    def test_user_can_see_public_and_internal(self, acl_filter: ACLFilter) -> None:
        """User with internal clearance can see public and internal chunks."""
        user = UserContext(user_id="alice", groups=[], classification_level="internal")
        results = [
            _make_result(text="public doc", acl_users="alice", classification="public"),
            _make_result(text="internal doc", acl_users="alice", classification="internal"),
        ]

        filtered = acl_filter.apply_filter(user, results)

        assert len(filtered) == 2

    def test_user_cannot_see_confidential(self, acl_filter: ACLFilter) -> None:
        """User with internal clearance cannot see confidential chunks."""
        user = UserContext(user_id="alice", groups=[], classification_level="internal")
        result = _make_result(
            text="confidential doc", acl_users="alice", classification="confidential"
        )

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 0

    def test_high_clearance_sees_lower_levels(self, acl_filter: ACLFilter) -> None:
        """User with secret clearance can see all lower levels."""
        user = UserContext(user_id="admin", groups=[], classification_level="secret")
        results = [
            _make_result(text="public", acl_users="admin", classification="public"),
            _make_result(text="internal", acl_users="admin", classification="internal"),
            _make_result(text="confidential", acl_users="admin", classification="confidential"),
            _make_result(text="restricted", acl_users="admin", classification="restricted"),
            _make_result(text="secret", acl_users="admin", classification="secret"),
        ]

        filtered = acl_filter.apply_filter(user, results)

        assert len(filtered) == 5


class TestACLFilterSecureDefault:
    """Test secure-by-default behavior."""

    def test_empty_acl_deny_by_default(self, acl_filter: ACLFilter) -> None:
        """Chunks with no ACL metadata should be denied (secure by default)."""
        user = UserContext(user_id="alice", groups=["admin"], classification_level="secret")
        # No acl_users, no acl_groups → denied
        result = RetrievalResult(
            text="unprotected",
            score=1.0,
            metadata={"classification": "public"},
        )

        filtered = acl_filter.apply_filter(user, [result])

        assert len(filtered) == 0


class TestBuildQdrantFilter:
    """Test Qdrant filter generation."""

    def test_basic_filter_structure(self) -> None:
        """Filter should have must clause with classification and identity conditions."""
        user = UserContext(
            user_id="alice", groups=["engineering"], classification_level="internal"
        )

        qfilter = build_qdrant_filter(user)

        assert "must" in qfilter
        must_clauses = qfilter["must"]
        assert len(must_clauses) == 2

        # Classification filter
        classification_clause = must_clauses[0]
        assert classification_clause["key"] == "classification"
        allowed = set(classification_clause["match"]["any"])
        assert allowed == {"public", "internal"}

        # Identity filter
        identity_clause = must_clauses[1]
        assert "should" in identity_clause
        conditions = identity_clause["should"]
        # One for user_id + one for each group
        assert len(conditions) == 2
        assert conditions[0] == {"key": "acl_users", "match": {"text": "alice"}}
        assert conditions[1] == {"key": "acl_groups", "match": {"text": "engineering"}}

    def test_secret_clearance_allows_all_levels(self) -> None:
        """Secret clearance should allow all classification levels in filter."""
        user = UserContext(user_id="admin", groups=[], classification_level="secret")

        qfilter = build_qdrant_filter(user)

        classification_clause = qfilter["must"][0]
        allowed = set(classification_clause["match"]["any"])
        assert allowed == {"public", "internal", "confidential", "restricted", "secret"}

    def test_multiple_groups(self) -> None:
        """Multiple groups should each produce a should condition."""
        user = UserContext(
            user_id="bob", groups=["eng", "ops", "sre"], classification_level="public"
        )

        qfilter = build_qdrant_filter(user)

        identity_clause = qfilter["must"][1]
        conditions = identity_clause["should"]
        # 1 user_id condition + 3 group conditions
        assert len(conditions) == 4
