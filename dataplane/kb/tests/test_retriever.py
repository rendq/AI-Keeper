"""Tests for the hybrid retriever module."""

from __future__ import annotations

from dataplane.kb.retriever import RetrievalResult, RetrieverConfig, retrieve
from dataplane.kb.retriever.hybrid import (
    BM25SearchClient,
    CrossEncoderReranker,
    VectorSearchClient,
)
from dataplane.kb.retriever.rrf import rrf_fuse


# --- Helpers / Fake Clients ---


class FakeVectorClient:
    """Fake vector search client that returns predetermined results."""

    def __init__(self, results: list[RetrievalResult]) -> None:
        self.results = results

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        return self.results[:top_k]


class FakeBM25Client:
    """Fake BM25 search client that returns predetermined results."""

    def __init__(self, results: list[RetrievalResult]) -> None:
        self.results = results

    def search(self, query: str, top_k: int) -> list[RetrievalResult]:
        return self.results[:top_k]


class FakeReranker:
    """Fake reranker that reverses results and assigns new scores."""

    def rerank(self, query: str, results: list[RetrievalResult]) -> list[RetrievalResult]:
        reversed_results = list(reversed(results))
        for i, r in enumerate(reversed_results):
            r.score = 1.0 - (i * 0.1)
        return reversed_results


# --- Protocol compliance ---


def test_fake_clients_satisfy_protocols() -> None:
    """Verify fake clients satisfy the protocol interfaces."""
    vec = FakeVectorClient([])
    bm25 = FakeBM25Client([])
    reranker = FakeReranker()
    assert isinstance(vec, VectorSearchClient)
    assert isinstance(bm25, BM25SearchClient)
    assert isinstance(reranker, CrossEncoderReranker)


# --- RRF Fusion Tests ---


def test_rrf_fuse_with_known_rankings() -> None:
    """Test RRF fusion produces correct scores for known rankings."""
    list_a = [
        RetrievalResult(text="doc1", score=0.9, source_id="1"),
        RetrievalResult(text="doc2", score=0.8, source_id="2"),
        RetrievalResult(text="doc3", score=0.7, source_id="3"),
    ]
    list_b = [
        RetrievalResult(text="doc2", score=0.95, source_id="2"),
        RetrievalResult(text="doc3", score=0.85, source_id="3"),
        RetrievalResult(text="doc4", score=0.75, source_id="4"),
    ]

    fused = rrf_fuse([list_a, list_b], k=60)

    # doc2 appears rank 2 in list_a and rank 1 in list_b:
    # score = 1/(60+2) + 1/(60+1) = 1/62 + 1/61
    expected_doc2_score = 1.0 / 62 + 1.0 / 61

    # doc1 appears rank 1 in list_a only:
    # score = 1/(60+1)
    expected_doc1_score = 1.0 / 61

    # Find results by source_id
    scores_by_id = {r.source_id: r.score for r in fused}

    assert abs(scores_by_id["2"] - expected_doc2_score) < 1e-10
    assert abs(scores_by_id["1"] - expected_doc1_score) < 1e-10

    # doc2 should be ranked highest (appears in both lists)
    assert fused[0].source_id == "2"


def test_rrf_fuse_single_list() -> None:
    """RRF with a single list just assigns 1/(k+rank) scores."""
    results = [
        RetrievalResult(text="a", score=1.0, source_id="a"),
        RetrievalResult(text="b", score=0.5, source_id="b"),
    ]
    fused = rrf_fuse([results], k=60)

    assert len(fused) == 2
    assert abs(fused[0].score - 1.0 / 61) < 1e-10
    assert abs(fused[1].score - 1.0 / 62) < 1e-10


def test_rrf_fuse_empty_lists() -> None:
    """RRF with empty input returns empty results."""
    assert rrf_fuse([]) == []
    assert rrf_fuse([[]]) == []


# --- Hybrid Retrieval Tests ---


def test_hybrid_retrieval_combines_vector_and_bm25() -> None:
    """Hybrid retrieval combines results from both vector and BM25 sources."""
    vector_results = [
        RetrievalResult(text="vec_doc1", score=0.9, source_id="v1"),
        RetrievalResult(text="shared_doc", score=0.8, source_id="s1"),
    ]
    bm25_results = [
        RetrievalResult(text="shared_doc", score=0.95, source_id="s1"),
        RetrievalResult(text="bm25_doc1", score=0.7, source_id="b1"),
    ]

    config = RetrieverConfig(top_k=10, rerank_enabled=False)
    results = retrieve(
        "test query",
        config,
        vector_client=FakeVectorClient(vector_results),
        bm25_client=FakeBM25Client(bm25_results),
    )

    # shared_doc appears in both lists, should rank highest
    assert len(results) > 0
    source_ids = [r.source_id for r in results]
    assert "s1" in source_ids
    assert "v1" in source_ids
    assert "b1" in source_ids
    # shared_doc should be first (highest RRF score)
    assert results[0].source_id == "s1"


def test_reranking_reorders_results() -> None:
    """Reranking changes the order of results."""
    vector_results = [
        RetrievalResult(text="doc1", score=0.9, source_id="1"),
        RetrievalResult(text="doc2", score=0.8, source_id="2"),
        RetrievalResult(text="doc3", score=0.7, source_id="3"),
    ]

    config = RetrieverConfig(top_k=10, rerank_enabled=True, rerank_top_k=3)
    results = retrieve(
        "test query",
        config,
        vector_client=FakeVectorClient(vector_results),
        bm25_client=FakeBM25Client([]),
        reranker=FakeReranker(),
    )

    # FakeReranker reverses the order, so doc3 should now be first
    assert len(results) > 0
    assert results[0].source_id == "3"


def test_top_k_limiting() -> None:
    """Results are limited to config.top_k."""
    vector_results = [
        RetrievalResult(text=f"doc{i}", score=1.0 - i * 0.1, source_id=str(i))
        for i in range(10)
    ]

    config = RetrieverConfig(top_k=3, rerank_enabled=False)
    results = retrieve(
        "test query",
        config,
        vector_client=FakeVectorClient(vector_results),
        bm25_client=FakeBM25Client([]),
    )

    assert len(results) == 3


def test_empty_results_handling() -> None:
    """Empty query and no results are handled gracefully."""
    # Empty query
    assert retrieve("") == []
    assert retrieve("", RetrieverConfig()) == []

    # Both clients return empty
    config = RetrieverConfig(top_k=5, rerank_enabled=False)
    results = retrieve(
        "test query",
        config,
        vector_client=FakeVectorClient([]),
        bm25_client=FakeBM25Client([]),
    )
    assert results == []
