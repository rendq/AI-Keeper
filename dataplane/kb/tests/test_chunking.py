"""Tests for KB chunking strategies.

Validates: Requirements B9.1
"""

from __future__ import annotations

import pytest

from dataplane.kb.chunking import Chunk, ChunkConfig, ChunkStrategy, chunk


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

SAMPLE_DOC = (
    "The quick brown fox jumps over the lazy dog. "
    "Pack my box with five dozen liquor jugs. "
    "How vexingly quick daft zebras jump. "
    "The five boxing wizards jump quickly. "
    "Sphinx of black quartz, judge my vow."
)

STRUCTURED_DOC = """# Introduction

This is the introduction paragraph with some context about the topic.

# Methods

We used method A and method B to analyze the data. The results were significant.

## Sub-section

Additional details about the methodology go here. This is important context.

# Results

The results show clear improvement across all metrics. Performance doubled."""

LONG_DOC = " ".join(["Sentence number {}.".format(i) for i in range(200)])


# ---------------------------------------------------------------------------
# Fixed chunking tests
# ---------------------------------------------------------------------------


class TestFixedChunk:
    def test_chunks_within_size_limit(self):
        config = ChunkConfig(strategy=ChunkStrategy.FIXED, chunk_size=50, chunk_overlap=10)
        chunks = chunk(SAMPLE_DOC, config)
        assert all(len(c.text) <= 50 for c in chunks)

    def test_overlap_correct(self):
        config = ChunkConfig(strategy=ChunkStrategy.FIXED, chunk_size=50, chunk_overlap=10)
        chunks = chunk(SAMPLE_DOC, config)
        assert len(chunks) > 1
        # Check that consecutive chunks overlap
        for i in range(len(chunks) - 1):
            # The end of chunk[i] should overlap with the start of chunk[i+1]
            overlap_start = chunks[i + 1].start_offset
            overlap_end = chunks[i].end_offset
            if overlap_end > overlap_start:
                assert chunks[i].text[-(overlap_end - overlap_start) :] == chunks[
                    i + 1
                ].text[: overlap_end - overlap_start]

    def test_full_coverage(self):
        config = ChunkConfig(strategy=ChunkStrategy.FIXED, chunk_size=50, chunk_overlap=10)
        chunks = chunk(SAMPLE_DOC, config)
        # Every character should be in at least one chunk
        covered = set()
        for c in chunks:
            covered.update(range(c.start_offset, c.end_offset))
        assert covered == set(range(len(SAMPLE_DOC)))

    def test_chunk_indices_sequential(self):
        config = ChunkConfig(strategy=ChunkStrategy.FIXED, chunk_size=50, chunk_overlap=10)
        chunks = chunk(SAMPLE_DOC, config)
        for i, c in enumerate(chunks):
            assert c.chunk_index == i

    def test_doc_smaller_than_chunk_size(self):
        config = ChunkConfig(strategy=ChunkStrategy.FIXED, chunk_size=10000, chunk_overlap=10)
        chunks = chunk(SAMPLE_DOC, config)
        assert len(chunks) == 1
        assert chunks[0].text == SAMPLE_DOC


# ---------------------------------------------------------------------------
# Semantic chunking tests
# ---------------------------------------------------------------------------


class TestSemanticChunk:
    def test_splits_at_sentence_boundaries(self):
        config = ChunkConfig(strategy=ChunkStrategy.SEMANTIC, chunk_size=100, chunk_overlap=0)
        chunks = chunk(SAMPLE_DOC, config)
        assert len(chunks) >= 1
        # Each chunk should end at or near a sentence boundary (ends with . ! or ?)
        for c in chunks[:-1]:  # Last chunk may not end with punctuation
            assert c.text.rstrip().endswith((".", "!", "?"))

    def test_chunks_within_size_limit(self):
        config = ChunkConfig(strategy=ChunkStrategy.SEMANTIC, chunk_size=100, chunk_overlap=0)
        chunks = chunk(SAMPLE_DOC, config)
        # Each chunk should be at or near the size limit (single sentence may exceed)
        for c in chunks:
            # Allow some tolerance for single long sentences
            assert len(c.text) <= config.chunk_size * 2

    def test_preserves_content(self):
        config = ChunkConfig(strategy=ChunkStrategy.SEMANTIC, chunk_size=100, chunk_overlap=0)
        chunks = chunk(SAMPLE_DOC, config)
        combined = " ".join(c.text for c in chunks)
        # All sentences should be preserved
        assert "quick brown fox" in combined
        assert "boxing wizards" in combined


# ---------------------------------------------------------------------------
# Recursive chunking tests
# ---------------------------------------------------------------------------


class TestRecursiveChunk:
    def test_uses_hierarchy_of_separators(self):
        doc_with_paras = "Para one line one.\nPara one line two.\n\nPara two line one.\nPara two line two."
        config = ChunkConfig(
            strategy=ChunkStrategy.RECURSIVE,
            chunk_size=50,
            chunk_overlap=0,
            separators=["\n\n", "\n", " "],
        )
        chunks = chunk(doc_with_paras, config)
        assert len(chunks) >= 2
        # Should prefer splitting on \n\n first
        for c in chunks:
            assert len(c.text) <= 50

    def test_falls_back_to_hard_split(self):
        # A very long word that can't be split by any separator
        long_word = "a" * 100
        config = ChunkConfig(
            strategy=ChunkStrategy.RECURSIVE, chunk_size=30, chunk_overlap=0
        )
        chunks = chunk(long_word, config)
        assert len(chunks) >= 2
        assert all(len(c.text) <= 30 for c in chunks)

    def test_chunks_within_size_limit(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.RECURSIVE, chunk_size=60, chunk_overlap=0
        )
        chunks = chunk(LONG_DOC, config)
        assert all(len(c.text) <= 60 for c in chunks)


# ---------------------------------------------------------------------------
# Structural chunking tests
# ---------------------------------------------------------------------------


class TestStructuralChunk:
    def test_recognizes_headers(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.STRUCTURAL, chunk_size=500, chunk_overlap=0
        )
        chunks = chunk(STRUCTURED_DOC, config)
        assert len(chunks) >= 3
        # Check that chunks start with headers
        header_chunks = [c for c in chunks if c.text.startswith("#")]
        assert len(header_chunks) >= 3

    def test_recognizes_paragraphs(self):
        doc_no_headers = "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
        config = ChunkConfig(
            strategy=ChunkStrategy.STRUCTURAL, chunk_size=30, chunk_overlap=0
        )
        chunks = chunk(doc_no_headers, config)
        assert len(chunks) >= 2

    def test_chunks_within_size_limit(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.STRUCTURAL, chunk_size=100, chunk_overlap=0
        )
        chunks = chunk(STRUCTURED_DOC, config)
        assert all(len(c.text) <= 100 for c in chunks)

    def test_preserves_structure(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.STRUCTURAL, chunk_size=500, chunk_overlap=0
        )
        chunks = chunk(STRUCTURED_DOC, config)
        combined = " ".join(c.text for c in chunks)
        assert "Introduction" in combined
        assert "Methods" in combined
        assert "Results" in combined


# ---------------------------------------------------------------------------
# Hybrid chunking tests
# ---------------------------------------------------------------------------


class TestHybridChunk:
    def test_combines_structural_and_semantic(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.HYBRID, chunk_size=150, chunk_overlap=0
        )
        chunks = chunk(STRUCTURED_DOC, config)
        assert len(chunks) >= 2
        # Should respect structural boundaries (headers)
        header_chunks = [c for c in chunks if "# " in c.text[:10]]
        assert len(header_chunks) >= 1

    def test_chunks_within_size_limit(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.HYBRID, chunk_size=150, chunk_overlap=0
        )
        chunks = chunk(STRUCTURED_DOC, config)
        # Allow tolerance for headers that start sections
        for c in chunks:
            assert len(c.text) <= config.chunk_size * 2

    def test_falls_back_to_semantic_without_headers(self):
        config = ChunkConfig(
            strategy=ChunkStrategy.HYBRID, chunk_size=80, chunk_overlap=0
        )
        chunks = chunk(SAMPLE_DOC, config)
        # Should still produce valid chunks even without headers
        assert len(chunks) >= 1
        assert all(c.text for c in chunks)


# ---------------------------------------------------------------------------
# Edge cases
# ---------------------------------------------------------------------------


class TestEdgeCases:
    @pytest.mark.parametrize("strategy", list(ChunkStrategy))
    def test_empty_doc(self, strategy: ChunkStrategy):
        config = ChunkConfig(strategy=strategy)
        result = chunk("", config)
        assert result == []

    @pytest.mark.parametrize("strategy", list(ChunkStrategy))
    def test_very_short_doc(self, strategy: ChunkStrategy):
        config = ChunkConfig(strategy=strategy, chunk_size=512)
        result = chunk("Hi.", config)
        assert len(result) == 1
        assert result[0].text == "Hi."

    @pytest.mark.parametrize("strategy", list(ChunkStrategy))
    def test_doc_smaller_than_chunk_size(self, strategy: ChunkStrategy):
        small_doc = "A small document."
        config = ChunkConfig(strategy=strategy, chunk_size=10000)
        result = chunk(small_doc, config)
        assert len(result) == 1
        assert result[0].text == small_doc

    @pytest.mark.parametrize("strategy", list(ChunkStrategy))
    def test_metadata_contains_strategy(self, strategy: ChunkStrategy):
        config = ChunkConfig(strategy=strategy, chunk_size=50)
        result = chunk(SAMPLE_DOC, config)
        for c in result:
            assert c.metadata.get("strategy") == strategy.value

    def test_default_config(self):
        result = chunk(SAMPLE_DOC)
        assert len(result) >= 1
        assert all(isinstance(c, Chunk) for c in result)
