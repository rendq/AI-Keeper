"""Red Team report generation with metrics aggregation and multi-format output."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional

from dataplane.redteam.bot import AttackResult, AttackStrategy
from dataplane.redteam.dataset import TestCase

# Severity scoring weights
SEVERITY_SCORES: dict[str, int] = {
    "critical": 10,
    "high": 7,
    "medium": 4,
    "low": 1,
}


@dataclass
class ReportMetrics:
    """Aggregated metrics from red team attack results."""

    total_attacks: int = 0
    successful_attacks: int = 0
    success_rate: float = 0.0
    by_strategy: dict[str, int] = field(default_factory=dict)
    by_severity: dict[str, int] = field(default_factory=dict)
    by_category: dict[str, int] = field(default_factory=dict)
    risk_score: float = 0.0


def aggregate_results(
    results: list[AttackResult], test_cases: list[TestCase]
) -> ReportMetrics:
    """Aggregate attack results into report metrics.

    Computes success rate, breakdowns by strategy/severity/category,
    and an overall risk score based on severity-weighted successful attacks.
    """
    if not results:
        return ReportMetrics()

    # Build lookup from test_case_id -> TestCase
    tc_map: dict[str, TestCase] = {tc.id: tc for tc in test_cases}

    total = len(results)
    successful = sum(1 for r in results if r.success)
    success_rate = successful / total if total > 0 else 0.0

    by_strategy: dict[str, int] = {}
    by_severity: dict[str, int] = {}
    by_category: dict[str, int] = {}
    risk_score = 0.0

    for result in results:
        if not result.success:
            continue

        # By strategy
        strategy_name = result.strategy.value if isinstance(result.strategy, AttackStrategy) else str(result.strategy)
        by_strategy[strategy_name] = by_strategy.get(strategy_name, 0) + 1

        # By severity and category (from test case metadata)
        tc = tc_map.get(result.test_case_id)
        if tc:
            severity = tc.severity.lower()
            by_severity[severity] = by_severity.get(severity, 0) + 1
            by_category[tc.category] = by_category.get(tc.category, 0) + 1
            risk_score += SEVERITY_SCORES.get(severity, 0)

    return ReportMetrics(
        total_attacks=total,
        successful_attacks=successful,
        success_rate=success_rate,
        by_strategy=by_strategy,
        by_severity=by_severity,
        by_category=by_category,
        risk_score=risk_score,
    )


class RedTeamReport:
    """Red team report with multi-format rendering."""

    def __init__(
        self,
        metrics: ReportMetrics,
        dataset_name: str,
        target_agent: str,
        run_timestamp: Optional[datetime] = None,
    ) -> None:
        self.metrics = metrics
        self.dataset_name = dataset_name
        self.target_agent = target_agent
        self.run_timestamp = run_timestamp or datetime.now(tz=timezone.utc)

    def to_markdown(self) -> str:
        """Render report as markdown."""
        m = self.metrics
        lines = [
            f"# Red Team Report",
            "",
            f"**Target Agent:** {self.target_agent}",
            f"**Dataset:** {self.dataset_name}",
            f"**Run Timestamp:** {self.run_timestamp.isoformat()}",
            "",
            "## Summary",
            "",
            f"| Metric | Value |",
            f"|--------|-------|",
            f"| Total Attacks | {m.total_attacks} |",
            f"| Successful Attacks | {m.successful_attacks} |",
            f"| Success Rate | {m.success_rate:.1%} |",
            f"| Risk Score | {m.risk_score:.1f} |",
            "",
        ]

        if m.by_strategy:
            lines.append("## Breakdown by Strategy")
            lines.append("")
            lines.append("| Strategy | Successful Attacks |")
            lines.append("|----------|-------------------|")
            for strategy, count in sorted(m.by_strategy.items()):
                lines.append(f"| {strategy} | {count} |")
            lines.append("")

        if m.by_severity:
            lines.append("## Breakdown by Severity")
            lines.append("")
            lines.append("| Severity | Successful Attacks |")
            lines.append("|----------|-------------------|")
            for severity, count in sorted(m.by_severity.items()):
                lines.append(f"| {severity} | {count} |")
            lines.append("")

        if m.by_category:
            lines.append("## Breakdown by Category")
            lines.append("")
            lines.append("| Category | Successful Attacks |")
            lines.append("|----------|-------------------|")
            for category, count in sorted(m.by_category.items()):
                lines.append(f"| {category} | {count} |")
            lines.append("")

        return "\n".join(lines)

    def to_html(self) -> str:
        """Wrap markdown content in basic HTML structure."""
        md_content = self.to_markdown()
        # Convert markdown to simple HTML
        html_body = _markdown_to_html(md_content)
        return (
            "<!DOCTYPE html>\n"
            "<html lang=\"en\">\n"
            "<head>\n"
            "  <meta charset=\"UTF-8\">\n"
            f"  <title>Red Team Report - {self.target_agent}</title>\n"
            "  <style>\n"
            "    body { font-family: sans-serif; margin: 2em; }\n"
            "    table { border-collapse: collapse; margin: 1em 0; }\n"
            "    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }\n"
            "    th { background-color: #f4f4f4; }\n"
            "  </style>\n"
            "</head>\n"
            "<body>\n"
            f"{html_body}\n"
            "</body>\n"
            "</html>"
        )


def _markdown_to_html(md: str) -> str:
    """Minimal markdown-to-HTML converter for report rendering."""
    lines = md.split("\n")
    html_lines: list[str] = []
    in_table = False

    for line in lines:
        # Skip separator rows in tables
        if line.startswith("|--") or line.startswith("| -") or ("|" in line and set(line.replace("|", "").strip()) <= {"-"}):
            continue

        if line.startswith("# "):
            if in_table:
                html_lines.append("</table>")
                in_table = False
            html_lines.append(f"<h1>{line[2:]}</h1>")
        elif line.startswith("## "):
            if in_table:
                html_lines.append("</table>")
                in_table = False
            html_lines.append(f"<h2>{line[3:]}</h2>")
        elif line.startswith("|"):
            cells = [c.strip() for c in line.split("|")[1:-1]]
            if not in_table:
                html_lines.append("<table>")
                in_table = True
                html_lines.append("<tr>" + "".join(f"<th>{c}</th>" for c in cells) + "</tr>")
            else:
                html_lines.append("<tr>" + "".join(f"<td>{c}</td>" for c in cells) + "</tr>")
        elif line.startswith("**"):
            if in_table:
                html_lines.append("</table>")
                in_table = False
            # Bold text as paragraph
            content = line.replace("**", "<strong>", 1).replace("**", "</strong>", 1)
            html_lines.append(f"<p>{content}</p>")
        elif line.strip() == "":
            if in_table:
                html_lines.append("</table>")
                in_table = False
        else:
            if in_table:
                html_lines.append("</table>")
                in_table = False
            if line.strip():
                html_lines.append(f"<p>{line}</p>")

    if in_table:
        html_lines.append("</table>")

    return "\n".join(html_lines)


def generate_report(
    results: list[AttackResult],
    test_cases: list[TestCase],
    dataset_name: str,
    target_agent: str,
) -> RedTeamReport:
    """Generate a complete red team report from attack results."""
    metrics = aggregate_results(results, test_cases)
    return RedTeamReport(
        metrics=metrics,
        dataset_name=dataset_name,
        target_agent=target_agent,
    )
