"""Red Team test dataset management with CRUD, versioning, and tag filtering."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import Optional


# OWASP LLM Top 10 categories
OWASP_LLM_TOP10: list[str] = [
    "prompt_injection",
    "insecure_output",
    "training_data_poisoning",
    "model_dos",
    "supply_chain",
    "sensitive_info_disclosure",
    "insecure_plugin",
    "excessive_agency",
    "overreliance",
    "model_theft",
]


@dataclass
class TestCase:
    """A single red team test case."""

    id: str
    category: str
    payload: str
    expected_behavior: str
    severity: str = "medium"
    tags: list[str] = field(default_factory=list)
    version: str = "1.0.0"

    def __post_init__(self) -> None:
        if not self.id:
            self.id = str(uuid.uuid4())


@dataclass
class TestDataset:
    """A versioned collection of red team test cases."""

    name: str
    version: str
    description: str
    test_cases: list[TestCase] = field(default_factory=list)

    @property
    def categories(self) -> set[str]:
        """Return unique categories across all test cases."""
        return {tc.category for tc in self.test_cases}

    @property
    def all_tags(self) -> set[str]:
        """Return unique tags across all test cases."""
        tags: set[str] = set()
        for tc in self.test_cases:
            tags.update(tc.tags)
        return tags


def _increment_patch(version: str) -> str:
    """Increment the patch component of a semver string."""
    parts = version.split(".")
    if len(parts) != 3:
        raise ValueError(f"Invalid semver: {version}")
    major, minor, patch = int(parts[0]), int(parts[1]), int(parts[2])
    return f"{major}.{minor}.{patch + 1}"


class DatasetManager:
    """Manages red team test datasets with CRUD, versioning, and filtering."""

    def __init__(self) -> None:
        # Storage: {name: [TestDataset, ...]} ordered by version (latest last)
        self._datasets: dict[str, list[TestDataset]] = {}

    def create_dataset(
        self,
        name: str,
        description: str,
        test_cases: Optional[list[TestCase]] = None,
        version: str = "1.0.0",
    ) -> TestDataset:
        """Create a new dataset. Raises ValueError if name already exists."""
        if name in self._datasets:
            raise ValueError(f"Dataset '{name}' already exists")
        dataset = TestDataset(
            name=name,
            version=version,
            description=description,
            test_cases=test_cases or [],
        )
        self._datasets[name] = [dataset]
        return dataset

    def get_dataset(self, name: str, version: Optional[str] = None) -> TestDataset:
        """Get a dataset by name. Returns latest version if version not specified."""
        if name not in self._datasets:
            raise KeyError(f"Dataset '{name}' not found")
        versions = self._datasets[name]
        if version is None:
            return versions[-1]
        for ds in versions:
            if ds.version == version:
                return ds
        raise KeyError(f"Dataset '{name}' version '{version}' not found")

    def list_datasets(
        self,
        category: Optional[str] = None,
        tags: Optional[list[str]] = None,
    ) -> list[TestDataset]:
        """List datasets, optionally filtered by category or tags.

        Returns the latest version of each matching dataset.
        """
        results: list[TestDataset] = []
        for versions in self._datasets.values():
            latest = versions[-1]
            if category and category not in latest.categories:
                continue
            if tags:
                if not set(tags).intersection(latest.all_tags):
                    continue
            results.append(latest)
        return results

    def add_test_case(self, dataset_name: str, test_case: TestCase) -> TestDataset:
        """Add a test case to a dataset, creating a new patch version."""
        if dataset_name not in self._datasets:
            raise KeyError(f"Dataset '{dataset_name}' not found")
        latest = self._datasets[dataset_name][-1]
        new_version = _increment_patch(latest.version)
        new_dataset = TestDataset(
            name=latest.name,
            version=new_version,
            description=latest.description,
            test_cases=[*latest.test_cases, test_case],
        )
        self._datasets[dataset_name].append(new_dataset)
        return new_dataset

    def delete_dataset(self, name: str, version: Optional[str] = None) -> None:
        """Delete a dataset or a specific version.

        If version is None, deletes all versions.
        If version is specified, deletes only that version.
        Raises KeyError if not found.
        """
        if name not in self._datasets:
            raise KeyError(f"Dataset '{name}' not found")
        if version is None:
            del self._datasets[name]
            return
        versions = self._datasets[name]
        original_len = len(versions)
        self._datasets[name] = [ds for ds in versions if ds.version != version]
        if len(self._datasets[name]) == original_len:
            raise KeyError(f"Dataset '{name}' version '{version}' not found")
        if not self._datasets[name]:
            del self._datasets[name]
