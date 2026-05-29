"""Five chunking strategies for the KB pipeline.

Each function takes (doc: str, config: ChunkConfig) and returns list[Chunk].
"""

from __future__ import annotations

import re

from dataplane.kb.chunking import Chunk, ChunkConfig


def _make_chunks(segments: list[tuple[int, int, str]], config: ChunkConfig) -> list[Chunk]:
    """Helper to wrap raw (start, end, text) tuples into Chunk objects."""
    return [
        Chunk(
            text=text,
            metadata={"strategy": config.strategy.value},
            start_offset=start,
            end_offset=end,
            chunk_index=i,
        )
        for i, (start, end, text) in enumerate(segments)
    ]


# ---------------------------------------------------------------------------
# 1. Fixed chunking — split at fixed character count with overlap
# ---------------------------------------------------------------------------


def fixed_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Split document at fixed character intervals with overlap."""
    size = config.chunk_size
    overlap = config.chunk_overlap
    segments: list[tuple[int, int, str]] = []

    if len(doc) <= size:
        segments.append((0, len(doc), doc))
        return _make_chunks(segments, config)

    step = max(size - overlap, 1)
    pos = 0
    while pos < len(doc):
        end = min(pos + size, len(doc))
        segments.append((pos, end, doc[pos:end]))
        if end >= len(doc):
            break
        pos += step

    return _make_chunks(segments, config)


# ---------------------------------------------------------------------------
# 2. Semantic chunking — split at sentence boundaries, merge until size limit
# ---------------------------------------------------------------------------

_SENTENCE_RE = re.compile(r"(?<=[.!?])\s+")


def _split_sentences(text: str) -> list[tuple[int, str]]:
    """Split text into (offset, sentence) pairs."""
    parts: list[tuple[int, str]] = []
    prev = 0
    for m in _SENTENCE_RE.finditer(text):
        end = m.start()
        parts.append((prev, text[prev : m.end()].rstrip()))
        prev = m.end()
    if prev < len(text):
        parts.append((prev, text[prev:]))
    return parts


def semantic_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Split at sentence boundaries, merging sentences until chunk_size is reached."""
    sentences = _split_sentences(doc)
    if not sentences:
        return []

    segments: list[tuple[int, int, str]] = []
    current_text = ""
    current_start = sentences[0][0]

    for offset, sentence in sentences:
        candidate = (current_text + " " + sentence).strip() if current_text else sentence
        if len(candidate) > config.chunk_size and current_text:
            end_offset = offset
            segments.append((current_start, end_offset, current_text))
            current_text = sentence
            current_start = offset
        else:
            current_text = candidate

    # Flush remaining
    if current_text:
        segments.append((current_start, current_start + len(current_text), current_text))

    return _make_chunks(segments, config)


# ---------------------------------------------------------------------------
# 3. Recursive chunking — recursively split by hierarchy of separators
# ---------------------------------------------------------------------------


def recursive_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Recursively split using a hierarchy of separators."""
    separators = config.separators or ["\n\n", "\n", ". ", " "]

    def _split(text: str, start_offset: int, sep_idx: int) -> list[tuple[int, int, str]]:
        if len(text) <= config.chunk_size:
            return [(start_offset, start_offset + len(text), text)]

        if sep_idx >= len(separators):
            # Fallback: hard split at chunk_size
            return _hard_split(text, start_offset, config.chunk_size)

        sep = separators[sep_idx]
        parts = text.split(sep)

        if len(parts) <= 1:
            # This separator doesn't split — try next
            return _split(text, start_offset, sep_idx + 1)

        results: list[tuple[int, int, str]] = []
        current = ""
        current_offset = start_offset

        for i, part in enumerate(parts):
            piece = part if i == 0 else (sep + part)
            if not current:
                candidate = piece.lstrip(sep) if i == 0 else piece
                current = part
            else:
                candidate = current + sep + part
                if len(candidate) <= config.chunk_size:
                    current = candidate
                else:
                    # Flush current
                    results.extend(_split(current, current_offset, sep_idx + 1))
                    current_offset = current_offset + len(current) + len(sep)
                    current = part

        if current:
            results.extend(_split(current, current_offset, sep_idx + 1))

        return results

    segments = _split(doc, 0, 0)
    return _make_chunks(segments, config)


def _hard_split(
    text: str, start_offset: int, chunk_size: int
) -> list[tuple[int, int, str]]:
    """Hard split when no separator works."""
    results: list[tuple[int, int, str]] = []
    pos = 0
    while pos < len(text):
        end = min(pos + chunk_size, len(text))
        results.append((start_offset + pos, start_offset + end, text[pos:end]))
        pos = end
    return results


# ---------------------------------------------------------------------------
# 4. Structural chunking — split by document structure (headers, paragraphs)
# ---------------------------------------------------------------------------

_HEADER_RE = re.compile(r"^(#{1,6})\s+(.+)$", re.MULTILINE)
_PARAGRAPH_SEP = re.compile(r"\n{2,}")


def structural_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Split by document structure: headers and paragraphs."""
    # Find all header positions
    headers = [(m.start(), m.end(), m.group(0)) for m in _HEADER_RE.finditer(doc)]

    if not headers:
        # No headers — fall back to paragraph splitting
        return _paragraph_chunk(doc, config)

    # Split into sections by header
    sections: list[tuple[int, int, str]] = []
    for i, (start, _end, _title) in enumerate(headers):
        section_end = headers[i + 1][0] if i + 1 < len(headers) else len(doc)
        section_text = doc[start:section_end].rstrip()
        if section_text:
            sections.append((start, start + len(section_text), section_text))

    # Handle text before first header
    if headers[0][0] > 0:
        preamble = doc[: headers[0][0]].rstrip()
        if preamble:
            sections.insert(0, (0, len(preamble), preamble))

    # If sections exceed chunk_size, split paragraphs within
    final_segments: list[tuple[int, int, str]] = []
    for start, end, text in sections:
        if len(text) <= config.chunk_size:
            final_segments.append((start, end, text))
        else:
            # Sub-split by paragraphs
            para_segments = _split_paragraphs(text, start, config.chunk_size)
            final_segments.extend(para_segments)

    return _make_chunks(final_segments, config)


def _paragraph_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Split by paragraphs when no headers are found."""
    segments = _split_paragraphs(doc, 0, config.chunk_size)
    return _make_chunks(segments, config)


def _split_paragraphs(
    text: str, base_offset: int, chunk_size: int
) -> list[tuple[int, int, str]]:
    """Split text by paragraphs, merging until chunk_size."""
    paragraphs = _PARAGRAPH_SEP.split(text)
    segments: list[tuple[int, int, str]] = []
    current = ""
    current_offset = base_offset
    offset = base_offset

    for para in paragraphs:
        if not current:
            current = para
            current_offset = offset
        else:
            candidate = current + "\n\n" + para
            if len(candidate) <= chunk_size:
                current = candidate
            else:
                segments.append((current_offset, current_offset + len(current), current))
                current = para
                current_offset = offset

        # Move offset past paragraph + separator
        offset += len(para)
        # Account for the separator that was split away
        sep_match = _PARAGRAPH_SEP.search(text, offset - base_offset)
        if sep_match and sep_match.start() == offset - base_offset:
            offset = base_offset + sep_match.end()

    if current:
        segments.append((current_offset, current_offset + len(current), current))

    # If any segment still exceeds chunk_size, hard split it
    final: list[tuple[int, int, str]] = []
    for start, end, seg_text in segments:
        if len(seg_text) <= chunk_size:
            final.append((start, end, seg_text))
        else:
            final.extend(_hard_split(seg_text, start, chunk_size))

    return final


# ---------------------------------------------------------------------------
# 5. Hybrid chunking — combine structural boundaries with semantic merging
# ---------------------------------------------------------------------------


def hybrid_chunk(doc: str, config: ChunkConfig) -> list[Chunk]:
    """Combine structural boundaries with semantic sentence-level merging.

    First splits by structure (headers/paragraphs), then within each section
    applies semantic sentence-boundary splitting.
    """
    # Get structural sections first
    headers = [(m.start(), m.end(), m.group(0)) for m in _HEADER_RE.finditer(doc)]

    if not headers:
        # Fall back to pure semantic
        return semantic_chunk(doc, config)

    sections: list[tuple[int, str]] = []

    # Preamble before first header
    if headers[0][0] > 0:
        preamble = doc[: headers[0][0]].rstrip()
        if preamble:
            sections.append((0, preamble))

    for i, (start, _end, _title) in enumerate(headers):
        section_end = headers[i + 1][0] if i + 1 < len(headers) else len(doc)
        section_text = doc[start:section_end].rstrip()
        if section_text:
            sections.append((start, section_text))

    # Apply semantic splitting within each structural section
    all_segments: list[tuple[int, int, str]] = []
    for section_offset, section_text in sections:
        if len(section_text) <= config.chunk_size:
            all_segments.append(
                (section_offset, section_offset + len(section_text), section_text)
            )
        else:
            # Semantic split within this section
            sentences = _split_sentences(section_text)
            if not sentences:
                all_segments.append(
                    (section_offset, section_offset + len(section_text), section_text)
                )
                continue

            current_text = ""
            current_start = section_offset + sentences[0][0]

            for local_offset, sentence in sentences:
                candidate = (
                    (current_text + " " + sentence).strip() if current_text else sentence
                )
                if len(candidate) > config.chunk_size and current_text:
                    end_offset = section_offset + local_offset
                    all_segments.append((current_start, end_offset, current_text))
                    current_text = sentence
                    current_start = section_offset + local_offset
                else:
                    current_text = candidate

            if current_text:
                all_segments.append(
                    (current_start, current_start + len(current_text), current_text)
                )

    return _make_chunks(all_segments, config)
