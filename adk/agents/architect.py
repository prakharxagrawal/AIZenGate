"""
ZenGate ADK — Architect Agent

Reads a high-level task and produces:
- Architecture document (markdown)
- Interface contracts (Go interfaces)
- Data flow description
- Design decisions & trade-offs
"""

from dataclasses import dataclass
from typing import Optional


@dataclass
class ArchitectureDoc:
    """Output of the Architect agent."""
    overview: str
    interfaces: str  # Go interface definitions
    data_flow: str
    design_decisions: str
    dependencies: list[str]


async def run(task: str, context: Optional[dict] = None) -> ArchitectureDoc:
    """
    Generate an architecture document for the given task.

    Phase 1: Returns a template. Phase 2+: Uses ADK LLM calls.
    """
    # TODO: Integrate with Google ADK Agent class
    return ArchitectureDoc(
        overview=f"Architecture for: {task}",
        interfaces="// TODO: Generate Go interfaces",
        data_flow="// TODO: Generate data flow diagram",
        design_decisions="// TODO: Generate design decisions",
        dependencies=[],
    )
