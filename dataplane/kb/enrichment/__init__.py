"""Enrichment pipeline for the KB data plane.

Each chunk goes through up to 4 enrichment steps:
- pii_tagging: detect PII patterns in chunk text
- classification_inheritance: inherit classification level from source document
- language_detection: detect language (Chinese vs English heuristic)
- entity_extraction: extract named entities via regex
"""

from __future__ import annotations

from dataclasses import dataclass, field

from dataplane.kb.chunking import Chunk
from dataplane.kb.enrichment.steps import (
    classification_inheritance,
    entity_extraction,
    language_detection,
    pii_tagging,
)


@dataclass
class EnrichmentConfig:
    """Configuration flags for each enrichment step."""

    pii_tagging: bool = True
    classification_inheritance: bool = True
    language_detection: bool = True
    entity_extraction: bool = True
    source_classification: str = ""


def enrich(chunks: list[Chunk], config: EnrichmentConfig | None = None) -> list[Chunk]:
    """Run all enabled enrichment steps, attaching metadata to each chunk.

    Args:
        chunks: List of Chunk objects to enrich.
        config: Enrichment configuration. Defaults to all steps enabled.

    Returns:
        The same list of chunks with metadata attached.
    """
    if config is None:
        config = EnrichmentConfig()

    for chunk in chunks:
        if config.pii_tagging:
            result = pii_tagging(chunk)
            chunk.metadata.update(result)

        if config.classification_inheritance:
            result = classification_inheritance(chunk, config.source_classification)
            chunk.metadata.update(result)

        if config.language_detection:
            result = language_detection(chunk)
            chunk.metadata.update(result)

        if config.entity_extraction:
            result = entity_extraction(chunk)
            chunk.metadata.update(result)

    return chunks
