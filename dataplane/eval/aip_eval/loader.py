"""EvalSet and RedTeamSet loaders.

Loads evaluation datasets from:
- Local JSON/YAML files (for testing and Argo volume mounts)
- ConfigMap references (when running in-cluster)

Each eval set is a list of test cases with:
- input: the query/prompt
- expected_output: the expected response (optional)
- retrieval_context: list of context chunks (optional)
- metadata: arbitrary metadata dict (optional)
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class EvalTestCase:
    """A single test case in an eval set."""

    input: str
    expected_output: str | None = None
    actual_output: str | None = None
    retrieval_context: list[str] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class EvalSet:
    """A collection of eval test cases."""

    name: str
    test_cases: list[EvalTestCase]
    metrics: list[str] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class RedTeamTestCase:
    """A red team test case designed to probe adversarial behavior."""

    input: str
    attack_type: str  # e.g. "prompt_injection", "jailbreak", "pii_leak"
    expected_blocked: bool = True
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class RedTeamSet:
    """A collection of red team test cases."""

    name: str
    test_cases: list[RedTeamTestCase]
    metadata: dict[str, Any] = field(default_factory=dict)


def load_eval_set(ref: str | None = None) -> EvalSet:
    """Load an eval set from a file path or environment configuration.

    Resolution order:
    1. If `ref` is a file path that exists, load from that file.
    2. If `ref` starts with 'ref://', resolve the eval set name and look for
       a mounted file at /data/eval-sets/<name>.json
    3. Fall back to AIP_EVAL_SET_PATH environment variable.
    4. Return an empty eval set if nothing found.
    """
    path = _resolve_eval_path(ref, kind="eval-sets")
    if path and path.exists():
        return _parse_eval_set(path)
    return EvalSet(name="empty", test_cases=[], metrics=[])


def load_red_team_set(ref: str | None = None) -> RedTeamSet:
    """Load a red team set from a file path or environment configuration.

    Resolution order mirrors load_eval_set but for red team sets.
    """
    path = _resolve_eval_path(ref, kind="red-team-sets")
    if path and path.exists():
        return _parse_red_team_set(path)
    return RedTeamSet(name="empty", test_cases=[])


def _resolve_eval_path(ref: str | None, kind: str) -> Path | None:
    """Resolve a ref string to a filesystem path."""
    if ref is None:
        # Try environment variable.
        env_key = f"AIP_{kind.upper().replace('-', '_')}_PATH"
        env_val = os.environ.get(env_key)
        if env_val:
            return Path(env_val)
        return None

    # Direct file path.
    p = Path(ref)
    if p.exists():
        return p

    # ResourceRef format: ref://eval-sets/my-set
    if ref.startswith("ref://"):
        name = ref.split("/")[-1]
        # Convention: mounted at /data/<kind>/<name>.json
        mounted = Path(f"/data/{kind}/{name}.json")
        if mounted.exists():
            return mounted
        # Also check local ./data/ for dev
        local = Path(f"data/{kind}/{name}.json")
        if local.exists():
            return local

    return None


def _parse_eval_set(path: Path) -> EvalSet:
    """Parse an eval set JSON file."""
    data = json.loads(path.read_text(encoding="utf-8"))

    test_cases = []
    for tc in data.get("test_cases", []):
        test_cases.append(
            EvalTestCase(
                input=tc["input"],
                expected_output=tc.get("expected_output"),
                actual_output=tc.get("actual_output"),
                retrieval_context=tc.get("retrieval_context", []),
                metadata=tc.get("metadata", {}),
            )
        )

    return EvalSet(
        name=data.get("name", path.stem),
        test_cases=test_cases,
        metrics=data.get("metrics", []),
        metadata=data.get("metadata", {}),
    )


def _parse_red_team_set(path: Path) -> RedTeamSet:
    """Parse a red team set JSON file."""
    data = json.loads(path.read_text(encoding="utf-8"))

    test_cases = []
    for tc in data.get("test_cases", []):
        test_cases.append(
            RedTeamTestCase(
                input=tc["input"],
                attack_type=tc.get("attack_type", "unknown"),
                expected_blocked=tc.get("expected_blocked", True),
                metadata=tc.get("metadata", {}),
            )
        )

    return RedTeamSet(
        name=data.get("name", path.stem),
        test_cases=test_cases,
        metadata=data.get("metadata", {}),
    )
