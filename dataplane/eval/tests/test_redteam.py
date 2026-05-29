"""Tests for OWASP LLM Top 10 red team evaluation set."""

from __future__ import annotations

import pytest

from aip_eval.loader import RedTeamSet, RedTeamTestCase
from aip_eval.redteam import (
    LLM01_PROMPT_INJECTION,
    LLM02_INSECURE_OUTPUT,
    LLM04_MODEL_DOS,
    LLM06_SENSITIVE_INFO,
    LLM07_INSECURE_PLUGIN,
    LLM08_EXCESSIVE_AGENCY,
    LLM09_OVERRELIANCE,
    OWASP_CATEGORIES,
    get_owasp_redteam_set,
)
from aip_eval.runner import EvalRunner


class TestRedTeamSetLoading:
    """Test that the red team set loads correctly."""

    def test_returns_red_team_set_instance(self) -> None:
        result = get_owasp_redteam_set()
        assert isinstance(result, RedTeamSet)

    def test_set_has_name(self) -> None:
        result = get_owasp_redteam_set()
        assert result.name == "owasp-llm-top10-v1"

    def test_set_has_test_cases(self) -> None:
        result = get_owasp_redteam_set()
        assert len(result.test_cases) >= 20

    def test_all_cases_are_red_team_test_case(self) -> None:
        result = get_owasp_redteam_set()
        for tc in result.test_cases:
            assert isinstance(tc, RedTeamTestCase)

    def test_all_cases_have_input(self) -> None:
        result = get_owasp_redteam_set()
        for tc in result.test_cases:
            assert tc.input, f"Test case with attack_type={tc.attack_type} has empty input"

    def test_all_cases_have_attack_type(self) -> None:
        result = get_owasp_redteam_set()
        for tc in result.test_cases:
            assert tc.attack_type, "Test case has empty attack_type"

    def test_metadata_contains_version(self) -> None:
        result = get_owasp_redteam_set()
        assert result.metadata["version"] == "v1"
        assert result.metadata["source"] == "ref://evals/redteam/v1"
        assert result.metadata["skill"] == "evaluation.redTeamSet"


class TestRedTeamCategories:
    """Test that individual attack categories are represented."""

    @pytest.fixture()
    def red_team_set(self) -> RedTeamSet:
        return get_owasp_redteam_set()

    def test_prompt_injection_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM01_PROMPT_INJECTION in types

    def test_insecure_output_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM02_INSECURE_OUTPUT in types

    def test_model_dos_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM04_MODEL_DOS in types

    def test_sensitive_info_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM06_SENSITIVE_INFO in types

    def test_insecure_plugin_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM07_INSECURE_PLUGIN in types

    def test_excessive_agency_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM08_EXCESSIVE_AGENCY in types

    def test_overreliance_present(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        assert LLM09_OVERRELIANCE in types

    def test_all_owasp_categories_covered(self, red_team_set: RedTeamSet) -> None:
        types = {tc.attack_type for tc in red_team_set.test_cases}
        for category in OWASP_CATEGORIES:
            assert category in types, f"Missing category: {category}"

    def test_each_category_has_multiple_cases(self, red_team_set: RedTeamSet) -> None:
        from collections import Counter

        counts = Counter(tc.attack_type for tc in red_team_set.test_cases)
        for category in OWASP_CATEGORIES:
            assert counts[category] >= 2, (
                f"Category {category} should have at least 2 test cases, got {counts[category]}"
            )


class TestRedTeamEvalRunner:
    """Test that running EvalRunner.run_red_team with the OWASP set produces valid results."""

    @pytest.fixture()
    def runner(self) -> EvalRunner:
        return EvalRunner(
            skill_namespace="test-ns",
            skill_name="test-skill",
            run_id="redteam-run-001",
        )

    def test_run_red_team_returns_result(self, runner: EvalRunner) -> None:
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        assert result.run_id == "redteam-run-001"
        assert result.skill_namespace == "test-ns"
        assert result.skill_name == "test-skill"

    def test_run_red_team_has_metrics(self, runner: EvalRunner) -> None:
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        assert len(result.metrics) > 0
        assert result.metrics[0].name == "red_team_block_rate"

    def test_run_red_team_block_rate_valid(self, runner: EvalRunner) -> None:
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        score = result.metrics[0].score
        assert 0.0 <= score <= 1.0

    def test_run_red_team_passes_with_all_blocked(self, runner: EvalRunner) -> None:
        """All test cases have expected_blocked=True, so the stub should pass."""
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        assert result.passed is True
        assert result.metrics[0].score == 1.0

    def test_run_red_team_has_timestamps(self, runner: EvalRunner) -> None:
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        assert result.started_at > 0
        assert result.completed_at >= result.started_at

    def test_run_red_team_no_error(self, runner: EvalRunner) -> None:
        red_team_set = get_owasp_redteam_set()
        result = runner.run_red_team(red_team_set)
        assert result.error is None
