"""Runtime exceptions."""


class UnsupportedPatternError(Exception):
    """Raised when an unsupported execution pattern is requested.

    P0 supports only 'react' and 'tool_calling'. Other patterns
    (plan_execute, reflection, workflow, multi_agent) raise this error.
    """

    def __init__(self, pattern: str) -> None:
        self.pattern = pattern
        super().__init__(
            f"Unsupported pattern '{pattern}'. "
            f"P0 supports: react, tool_calling."
        )


class MaxStepsExceededError(Exception):
    """Raised when runtime.maxSteps limit is reached."""

    def __init__(self, max_steps: int) -> None:
        self.max_steps = max_steps
        super().__init__(f"Max steps exceeded: {max_steps}")


class MaxToolCallsExceededError(Exception):
    """Raised when runtime.maxToolCalls limit is reached."""

    def __init__(self, max_tool_calls: int) -> None:
        self.max_tool_calls = max_tool_calls
        super().__init__(f"Max tool calls exceeded: {max_tool_calls}")


class TimeoutExceededError(Exception):
    """Raised when runtime.timeout is exceeded."""

    def __init__(self, timeout: float) -> None:
        self.timeout = timeout
        super().__init__(f"Execution timed out after {timeout}s")
