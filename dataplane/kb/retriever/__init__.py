"""Hybrid retrieval module for the KB pipeline.

Provides vector search (Qdrant) + BM25 (ClickHouse full-text) → RRF fusion → Rerank.
Universal interface: retrieve(query, config) → list[RetrievalResult]
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class RetrievalResult:
    """A single retrieval result."""

    text: str
    score: float
    metadata: dict[str, str] = field(default_factory=dict)
    source_id: str = ""


@dataclass
class RetrieverConfig:
    """Configuration for the hybrid retriever."""

    top_k: int = 10
    vector_weight: float = 0.6
    bm25_weight: float = 0.4
    rerank_enabled: bool = True
    rerank_top_k: int = 5


def retrieve(
    query: str,
    config: RetrieverConfig | None = None,
    *,
    vector_client: object | None = None,
    bm25_client: object | None = None,
    reranker: object | None = None,
) -> list[RetrievalResult]:
    """Main entry point for hybrid retrieval.

    Args:
        query: The search query string.
        config: Retriever configuration. Defaults to standard hybrid settings.
        vector_client: Optional VectorSearchClient instance.
        bm25_client: Optional BM25SearchClient instance.
        reranker: Optional CrossEncoderReranker instance.

    Returns:
        A list of RetrievalResult objects, sorted by relevance.
    """
    if config is None:
        config = RetrieverConfig()

    if not query:
        return []

    from dataplane.kb.retriever.hybrid import hybrid_retrieve

    return hybrid_retrieve(
        query,
        config,
        vector_client=vector_client,
        bm25_client=bm25_client,
        reranker=reranker,
    )
