"""Chunking module for the KB pipeline.

Provides 5 chunking strategies: fixed, semantic, recursive, structural, hybrid.
Universal interface: chunk(doc, config) → list[Chunk]
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


class ChunkStrategy(str, Enum):
    """Available chunking strategies."""

    FIXED = "fixed"
    SEMANTIC = "semantic"
    RECURSIVE = "recursive"
    STRUCTURAL = "structural"
    HYBRID = "hybrid"


@dataclass
class Chunk:
    """A single chunk produced by a chunking strategy."""

    text: str
    metadata: dict[str, str] = field(default_factory=dict)
    start_offset: int = 0
    end_offset: int = 0
    chunk_index: int = 0


@dataclass
class ChunkConfig:
    """Configuration for the chunking dispatcher."""

    strategy: ChunkStrategy = ChunkStrategy.FIXED
    chunk_size: int = 512
    chunk_overlap: int = 64
    separators: list[str] = field(default_factory=lambda: ["\n\n", "\n", ". ", " "])


def chunk(doc: str, config: ChunkConfig | None = None) -> list[Chunk]:
    """Dispatch to the appropriate chunking strategy.

    Args:
        doc: The document text to chunk.
        config: Chunking configuration. Defaults to fixed strategy with 512 char chunks.

    Returns:
        A list of Chunk objects.
    """
    if config is None:
        config = ChunkConfig()

    if not doc:
        return []

    from dataplane.kb.chunking.strategies import (
        fixed_chunk,
        hybrid_chunk,
        recursive_chunk,
        semantic_chunk,
        structural_chunk,
    )

    dispatch = {
        ChunkStrategy.FIXED: fixed_chunk,
        ChunkStrategy.SEMANTIC: semantic_chunk,
        ChunkStrategy.RECURSIVE: recursive_chunk,
        ChunkStrategy.STRUCTURAL: structural_chunk,
        ChunkStrategy.HYBRID: hybrid_chunk,
    }

    strategy_fn = dispatch[config.strategy]
    return strategy_fn(doc, config)
