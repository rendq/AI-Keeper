"""ACL pre-filter for knowledge base retrieval.

Implements access control filtering so that chunks without proper permission
are excluded from recall results before they reach the user.

Classification hierarchy: public(0) < internal(1) < confidential(2) < restricted(3) < secret(4).
A user can see chunks at or below their clearance level.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from dataplane.kb.retriever import RetrievalResult

# Classification level ordering (lower number = less restricted)
CLASSIFICATION_LEVELS: dict[str, int] = {
    "public": 0,
    "internal": 1,
    "confidential": 2,
    "restricted": 3,
    "secret": 4,
}


@dataclass
class UserContext:
    """Represents the requesting user's identity and access attributes."""

    user_id: str
    groups: list[str] = field(default_factory=list)
    tenant_id: str = ""
    classification_level: str = "public"


class ACLFilter:
    """Access control list filter for retrieval results.

    Enforcement mode: pre_filter — chunks that don't pass ACL checks
    are removed before being returned to the caller.
    """

    enforcement: str = "pre_filter"

    def apply_filter(
        self, user_context: UserContext, results: list[RetrievalResult]
    ) -> list[RetrievalResult]:
        """Filter retrieval results based on user's ACL permissions.

        For each chunk, access is granted only if ALL of the following hold:
        1. The user or one of their groups is listed in chunk ACL (user/group check).
        2. The chunk's classification level is at or below the user's clearance.

        If a chunk has no ACL metadata (empty acl_users AND empty acl_groups),
        it is denied by default (secure-by-default).

        Args:
            user_context: The requesting user's identity and access attributes.
            results: Raw retrieval results to filter.

        Returns:
            Filtered list containing only permitted results.
        """
        filtered: list[RetrievalResult] = []
        for result in results:
            if self._check_access(user_context, result):
                filtered.append(result)
        return filtered

    def _check_access(self, user_context: UserContext, result: RetrievalResult) -> bool:
        """Check if user has access to a single result."""
        metadata = result.metadata

        # Classification check
        chunk_classification = metadata.get("classification", "public")
        if not self._check_classification(user_context.classification_level, chunk_classification):
            return False

        # ACL identity check (user or group must match)
        acl_users_raw = metadata.get("acl_users", "")
        acl_groups_raw = metadata.get("acl_groups", "")

        # Secure by default: if no ACL is set, deny access
        if not acl_users_raw and not acl_groups_raw:
            return False

        # Check user ID
        if acl_users_raw:
            acl_users = {u.strip() for u in acl_users_raw.split(",") if u.strip()}
            if user_context.user_id in acl_users:
                return True

        # Check group membership
        if acl_groups_raw:
            acl_groups = {g.strip() for g in acl_groups_raw.split(",") if g.strip()}
            if acl_groups.intersection(user_context.groups):
                return True

        return False

    @staticmethod
    def _check_classification(user_level: str, chunk_level: str) -> bool:
        """Return True if user's clearance is sufficient for the chunk's classification."""
        user_rank = CLASSIFICATION_LEVELS.get(user_level, 0)
        chunk_rank = CLASSIFICATION_LEVELS.get(chunk_level, 0)
        return user_rank >= chunk_rank


def build_qdrant_filter(user_context: UserContext) -> dict:
    """Build a Qdrant payload filter dict for ACL pre-filtering at query time.

    The filter ensures only chunks accessible to the user are returned by Qdrant.
    It combines:
    - Classification level filter (chunk classification <= user clearance)
    - Identity filter (user_id in acl_users OR any user group in acl_groups)

    Args:
        user_context: The requesting user's identity and access attributes.

    Returns:
        A Qdrant filter dict suitable for use in search requests.
    """
    user_rank = CLASSIFICATION_LEVELS.get(user_context.classification_level, 0)

    # Allowed classification values: all levels at or below user's clearance
    allowed_classifications = [
        level for level, rank in CLASSIFICATION_LEVELS.items() if rank <= user_rank
    ]

    # Identity conditions: user_id match OR group match
    identity_conditions: list[dict] = []

    # Match user_id in acl_users field
    identity_conditions.append(
        {"key": "acl_users", "match": {"text": user_context.user_id}}
    )

    # Match any of user's groups in acl_groups field
    for group in user_context.groups:
        identity_conditions.append(
            {"key": "acl_groups", "match": {"text": group}}
        )

    return {
        "must": [
            # Classification filter
            {
                "key": "classification",
                "match": {"any": allowed_classifications},
            },
            # Identity filter (at least one must match)
            {"should": identity_conditions},
        ],
    }
