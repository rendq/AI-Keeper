"""Reciprocal Rank Fusion (RRF) implementation.

RRF formula: for each document, fused_score = sum over all lists of 1/(k + rank_in_list).
Documents not present in a list receive no contribution from that list.
"""

from __future__ import annotations

from dataplane.kb.retriever import RetrievalResult


def rrf_fuse(
    result_lists: list[list[RetrievalResult]],
    k: int = 60,
) -> list[RetrievalResult]:
    """Fuse multiple ranked result lists using Reciprocal Rank Fusion.

    Args:
        result_lists: A list of ranked result lists to fuse.
        k: RRF constant (default 60). Higher values reduce the impact of high rankings.

    Returns:
        A single fused list of RetrievalResult, sorted by descending fused score.
    """
    if not result_lists:
        return []

    # Track scores and best result object per unique document (keyed by source_id + text)
    scores: dict[str, float] = {}
    best_result: dict[str, RetrievalResult] = {}

    for result_list in result_lists:
        for rank, result in enumerate(result_list, start=1):
            doc_key = f"{result.source_id}::{result.text}"
            scores[doc_key] = scores.get(doc_key, 0.0) + 1.0 / (k + rank)
            # Keep the result with the highest original score
            if doc_key not in best_result or result.score > best_result[doc_key].score:
                best_result[doc_key] = result

    # Build fused results with updated scores
    fused: list[RetrievalResult] = []
    for doc_key, fused_score in scores.items():
        original = best_result[doc_key]
        fused.append(
            RetrievalResult(
                text=original.text,
                score=fused_score,
                metadata=original.metadata,
                source_id=original.source_id,
            )
        )

    # Sort by descending fused score
    fused.sort(key=lambda r: r.score, reverse=True)
    return fused
