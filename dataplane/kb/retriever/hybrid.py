"""Hybrid retrieval: Vector + BM25 with RRF fusion and optional reranking."""

from __future__ import annotations

from typing import Protocol, runtime_checkable

from dataplane.kb.retriever import RetrievalResult, RetrieverConfig
from dataplane.kb.retriever.rrf import rrf_fuse


@runtime_checkable
class VectorSearchClient(Protocol):
    """Protocol for vector search backends (e.g., Qdrant)."""

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        """Search for similar vectors given a query string."""
        ...


@runtime_checkable
class BM25SearchClient(Protocol):
    """Protocol for BM25/full-text search backends (e.g., ClickHouse)."""

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        """Search using BM25 full-text ranking."""
        ...


@runtime_checkable
class CrossEncoderReranker(Protocol):
    """Protocol for cross-encoder reranking models."""

    def rerank(self, query: str, results: list[RetrievalResult]) -> list[RetrievalResult]:
        """Rerank results using a cross-encoder model.

        Returns results sorted by relevance with updated scores.
        """
        ...


class StubVectorSearchClient:
    """Stub implementation of VectorSearchClient for testing."""

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        """Return empty results (stub)."""
        return []


class StubBM25SearchClient:
    """Stub implementation of BM25SearchClient for testing."""

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        """Return empty results (stub)."""
        return []


class StubCrossEncoderReranker:
    """Stub implementation of CrossEncoderReranker for testing."""

    def rerank(self, query: str, results: list[RetrievalResult]) -> list[RetrievalResult]:
        """Return results unchanged (stub)."""
        return results


def hybrid_retrieve(
    query: str,
    config: RetrieverConfig,
    *,
    vector_client: object | None = None,
    bm25_client: object | None = None,
    reranker: object | None = None,
) -> list[RetrievalResult]:
    """Perform hybrid retrieval with vector + BM25 search, RRF fusion, and optional reranking.

    Args:
        query: The search query.
        config: Retriever configuration.
        vector_client: VectorSearchClient instance. Uses stub if None.
        bm25_client: BM25SearchClient instance. Uses stub if None.
        reranker: CrossEncoderReranker instance. Uses stub if None.

    Returns:
        Fused and optionally reranked results, limited to top_k.
    """
    vec_client: VectorSearchClient = (
        vector_client if isinstance(vector_client, VectorSearchClient) else StubVectorSearchClient()
    )
    bm25: BM25SearchClient = (
        bm25_client if isinstance(bm25_client, BM25SearchClient) else StubBM25SearchClient()
    )
    rerank_client: CrossEncoderReranker = (
        reranker if isinstance(reranker, CrossEncoderReranker) else StubCrossEncoderReranker()
    )

    # Fetch from both sources
    vector_results = vec_client.search(query, config.top_k)
    bm25_results = bm25.search(query, config.top_k)

    # Fuse with RRF
    result_lists: list[list[RetrievalResult]] = []
    if vector_results:
        result_lists.append(vector_results)
    if bm25_results:
        result_lists.append(bm25_results)

    if not result_lists:
        return []

    fused = rrf_fuse(result_lists)

    # Optional reranking
    if config.rerank_enabled and fused:
        # Rerank top candidates, then merge back
        candidates = fused[: config.rerank_top_k] if config.rerank_top_k else fused
        reranked = rerank_client.rerank(query, candidates)
        # Replace the top portion with reranked results
        fused = reranked + fused[len(candidates) :]

    # Limit to top_k
    return fused[: config.top_k]
