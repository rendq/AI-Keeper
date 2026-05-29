"""AIP Eval Runner — offline evaluation for Skills using DeepEval metrics.

This package is invoked inside an Argo Workflow container to:
1. Load the evalSet / redTeamSet (from file or configmap)
2. Run DeepEval metrics against Skill outputs
3. Write results back to the Skill CR status.evalResults

Covers requirements: A3.8, C4
"""

from aip_eval.runner import EvalRunner  # noqa: F401
from aip_eval.loader import load_eval_set, load_red_team_set  # noqa: F401
from aip_eval.metrics import AVAILABLE_METRICS  # noqa: F401

__version__ = "0.1.0"
