"""Tests for the enrichment pipeline."""

from __future__ import annotations

from dataplane.kb.chunking import Chunk
from dataplane.kb.enrichment import EnrichmentConfig, enrich
from dataplane.kb.enrichment.steps import (
    classification_inheritance,
    entity_extraction,
    language_detection,
    pii_tagging,
)


class TestPiiTagging:
    """Tests for PII detection step."""

    def test_detects_id_card(self):
        chunk = Chunk(text="身份证号码是110101199001011234")
        result = pii_tagging(chunk)
        assert "身份证" in result["pii_types"]

    def test_detects_phone_number(self):
        chunk = Chunk(text="联系电话13812345678")
        result = pii_tagging(chunk)
        assert "手机号" in result["pii_types"]

    def test_detects_email(self):
        chunk = Chunk(text="请联系 test@example.com 获取更多信息")
        result = pii_tagging(chunk)
        assert "邮箱" in result["pii_types"]

    def test_no_pii(self):
        chunk = Chunk(text="这是一段普通文本，没有任何敏感信息。")
        result = pii_tagging(chunk)
        assert result["pii_types"] == "none"

    def test_multiple_pii_types(self):
        chunk = Chunk(text="电话13812345678，邮箱user@test.com")
        result = pii_tagging(chunk)
        assert "手机号" in result["pii_types"]
        assert "邮箱" in result["pii_types"]


class TestClassificationInheritance:
    """Tests for classification inheritance step."""

    def test_inherits_source_classification(self):
        chunk = Chunk(text="some text")
        result = classification_inheritance(chunk, "confidential")
        assert result["classification"] == "confidential"

    def test_defaults_to_unclassified(self):
        chunk = Chunk(text="some text")
        result = classification_inheritance(chunk, "")
        assert result["classification"] == "unclassified"


class TestLanguageDetection:
    """Tests for language detection step."""

    def test_detects_chinese(self):
        chunk = Chunk(text="这是一段中文文本，用于测试语言检测功能。")
        result = language_detection(chunk)
        assert result["language"] == "zh"

    def test_detects_english(self):
        chunk = Chunk(text="This is an English text for testing language detection.")
        result = language_detection(chunk)
        assert result["language"] == "en"

    def test_empty_text(self):
        chunk = Chunk(text="")
        result = language_detection(chunk)
        assert result["language"] == "en"


class TestEntityExtraction:
    """Tests for entity extraction step."""

    def test_extracts_dates(self):
        chunk = Chunk(text="会议日期为2024-01-15，截止日期2024/02/28")
        result = entity_extraction(chunk)
        assert "dates" in result["entities"]

    def test_extracts_emails(self):
        chunk = Chunk(text="联系人邮箱admin@company.com")
        result = entity_extraction(chunk)
        assert "emails" in result["entities"]

    def test_extracts_organizations(self):
        chunk = Chunk(text="由中国科学研究院和北京大学联合发布")
        result = entity_extraction(chunk)
        assert "organizations" in result["entities"]

    def test_no_entities(self):
        chunk = Chunk(text="hello world")
        result = entity_extraction(chunk)
        assert result["entities"] == "none"


class TestEnrichPipeline:
    """Tests for the full enrichment pipeline."""

    def test_all_steps_enabled(self):
        chunks = [Chunk(text="联系人邮箱admin@company.com，电话13900001111")]
        config = EnrichmentConfig(source_classification="internal")
        result = enrich(chunks, config)
        assert len(result) == 1
        meta = result[0].metadata
        assert "pii_types" in meta
        assert "classification" in meta
        assert "language" in meta
        assert "entities" in meta

    def test_selective_steps(self):
        chunks = [Chunk(text="Hello world")]
        config = EnrichmentConfig(
            pii_tagging=False,
            classification_inheritance=False,
            language_detection=True,
            entity_extraction=False,
        )
        result = enrich(chunks, config)
        meta = result[0].metadata
        assert "language" in meta
        assert "pii_types" not in meta
        assert "classification" not in meta
        assert "entities" not in meta

    def test_default_config(self):
        chunks = [Chunk(text="测试文本")]
        result = enrich(chunks)
        meta = result[0].metadata
        assert "pii_types" in meta
        assert "classification" in meta
        assert "language" in meta
        assert "entities" in meta

    def test_empty_chunks_list(self):
        result = enrich([])
        assert result == []
