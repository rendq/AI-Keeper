"""Individual enrichment step functions.

Each function takes a Chunk (and optional context) and returns a dict
of metadata to attach to the chunk.
"""

from __future__ import annotations

import re
import unicodedata

from dataplane.kb.chunking import Chunk

# PII patterns for Chinese context
# Note: use (?<!\d) and (?!\d) instead of \b for digit boundaries in CJK text
_PII_PATTERNS: dict[str, re.Pattern[str]] = {
    "身份证": re.compile(r"(?<!\d)\d{17}[\dXx](?!\d)"),
    "手机号": re.compile(r"(?<!\d)1[3-9]\d{9}(?!\d)"),
    "银行卡号": re.compile(r"(?<!\d)\d{16,19}(?!\d)"),
    "邮箱": re.compile(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"),
}

# Entity extraction patterns — dates checked before numbers to avoid partial matches
_ENTITY_PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("dates", re.compile(
        r"\d{4}[-/年]\d{1,2}[-/月]\d{1,2}[日]?"
    )),
    ("emails", re.compile(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}")),
    ("organizations", re.compile(
        r"[\u4e00-\u9fff]{2,}(?:公司|集团|银行|大学|学院|研究院|委员会|局|部)"
    )),
    ("numbers", re.compile(r"(?<!\d)(?<!\d[-/])\d+(?:\.\d+)?(?![-/]\d)")),
]


def pii_tagging(chunk: Chunk) -> dict[str, str]:
    """Detect PII in chunk text.

    Scans for: 身份证, 手机号, 银行卡号, 邮箱.

    Returns:
        Dict with "pii_types" key containing comma-separated PII types found,
        or "none" if no PII detected.
    """
    found: list[str] = []
    for pii_type, pattern in _PII_PATTERNS.items():
        if pattern.search(chunk.text):
            found.append(pii_type)

    return {"pii_types": ",".join(found) if found else "none"}


def classification_inheritance(chunk: Chunk, source_classification: str = "") -> dict[str, str]:
    """Inherit classification level from source document.

    Args:
        chunk: The chunk to classify.
        source_classification: Classification level of the source document.

    Returns:
        Dict with "classification" key set to the inherited level.
    """
    classification = source_classification if source_classification else "unclassified"
    return {"classification": classification}


def language_detection(chunk: Chunk) -> dict[str, str]:
    """Detect language using CJK character ratio heuristic.

    If CJK characters make up more than 30% of non-whitespace characters,
    the chunk is considered Chinese; otherwise English.

    Returns:
        Dict with "language" key set to "zh" or "en".
    """
    text = chunk.text.strip()
    if not text:
        return {"language": "en"}

    non_whitespace = [ch for ch in text if not ch.isspace()]
    if not non_whitespace:
        return {"language": "en"}

    cjk_count = sum(
        1
        for ch in non_whitespace
        if unicodedata.category(ch).startswith("Lo")
        and ("\u4e00" <= ch <= "\u9fff" or "\u3400" <= ch <= "\u4dbf")
    )

    ratio = cjk_count / len(non_whitespace)
    return {"language": "zh" if ratio > 0.3 else "en"}


def entity_extraction(chunk: Chunk) -> dict[str, str]:
    """Extract named entities using regex patterns.

    Extracts: dates, emails, numbers, organizations.

    Returns:
        Dict with "entities" key containing JSON-like semicolon-separated
        representation of found entities by type.
    """
    entities: dict[str, list[str]] = {}

    for entity_type, pattern in _ENTITY_PATTERNS:
        matches = pattern.findall(chunk.text)
        if matches:
            # Deduplicate and limit to first 10
            unique_matches = list(dict.fromkeys(matches))[:10]
            entities[entity_type] = unique_matches

    if not entities:
        return {"entities": "none"}

    # Format: "type1:val1,val2;type2:val3,val4"
    parts = []
    for etype, vals in entities.items():
        parts.append(f"{etype}:{','.join(vals)}")
    return {"entities": ";".join(parts)}
