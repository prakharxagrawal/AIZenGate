"""
ZenGate ADK — Reviewer Agent

Reviews generated code for correctness, performance, and security.
Approves or rejects with detailed feedback.
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class ReviewDecision(Enum):
    APPROVE = "approve"
    REJECT = "reject"


@dataclass
class ReviewIssue:
    """A single issue found during review."""
    file: str
    line: Optional[int]
    severity: str  # "critical", "warning", "info"
    description: str
    fix_suggestion: str


@dataclass
class ReviewResult:
    """Output of the Reviewer agent."""
    decision: ReviewDecision
    issues: list[ReviewIssue] = field(default_factory=list)
    summary: str = ""


async def run(
    task: str,
    architecture: str,
    code: str,
    context: Optional[dict] = None,
) -> ReviewResult:
    """
    Review generated code and approve or reject.

    Phase 1: Always approves. Phase 2+: Uses ADK LLM calls.
    """
    # TODO: Integrate with Google ADK Agent class
    return ReviewResult(
        decision=ReviewDecision.APPROVE,
        issues=[],
        summary=f"[Stub] Review approved for: {task[:50]}...",
    )
