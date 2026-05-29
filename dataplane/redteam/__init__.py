"""AIP Red Team Platform — adversarial testing for AI/LLM applications.

This package provides:
1. Test dataset management (CRUD) with OWASP LLM Top10 categories
2. Versioned test cases with tag-based filtering
3. Built-in payload categories: jailbreak, prompt injection, info disclosure

Covers requirements: C4.5
"""

from dataplane.redteam.dataset import (  # noqa: F401
    TestCase,
    TestDataset,
    DatasetManager,
    OWASP_LLM_TOP10,
)

__version__ = "0.1.0"
