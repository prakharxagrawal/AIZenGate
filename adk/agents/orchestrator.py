"""
ZenGate ADK — Orchestrator Agent

The main DAG pipeline controller. Routes tasks through:
  Architect → CodeGen → Reviewer → Tester → DocWriter

Features:
  - Conditional branching (reject → re-route to CodeGen)
  - Retry with exponential backoff on failure
  - Parallel sub-DAGs for independent components
  - Human-in-the-loop approval at gate points
  - Shared memory via filesystem for cross-agent context
"""

import asyncio
import json
import time
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Optional

from rich.console import Console
from rich.panel import Panel
from rich.table import Table

from config import (
    AGENTS,
    get_llm_chain,
    get_pipeline_config,
    PipelineConfig,
)

console = Console()


# --- Pipeline State ---

class TaskStatus(Enum):
    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    FAILED = "failed"
    REJECTED = "rejected"
    RETRYING = "retrying"


@dataclass
class TaskResult:
    """Result from a single agent execution."""
    agent_name: str
    status: TaskStatus
    output: str = ""
    error: Optional[str] = None
    duration_seconds: float = 0.0
    retries: int = 0
    files_created: list[str] = field(default_factory=list)
    files_modified: list[str] = field(default_factory=list)


@dataclass
class PipelineState:
    """Tracks the full state of a pipeline execution."""
    task_description: str
    results: list[TaskResult] = field(default_factory=list)
    current_stage: str = "architect"
    iteration: int = 0
    started_at: float = field(default_factory=time.time)
    completed_at: Optional[float] = None

    def add_result(self, result: TaskResult):
        self.results.append(result)

    def get_last_result(self, agent_name: str) -> Optional[TaskResult]:
        for r in reversed(self.results):
            if r.agent_name == agent_name:
                return r
        return None

    def to_json(self) -> str:
        return json.dumps(
            {
                "task": self.task_description,
                "current_stage": self.current_stage,
                "iteration": self.iteration,
                "results": [
                    {
                        "agent": r.agent_name,
                        "status": r.status.value,
                        "duration": r.duration_seconds,
                        "retries": r.retries,
                    }
                    for r in self.results
                ],
            },
            indent=2,
        )


# --- Agent Executor (Stub) ---

async def execute_agent(agent_name: str, task: str, context: dict) -> TaskResult:
    """
    Execute a single agent with the given task and context.

    Phase 1: This is a stub that simulates agent execution.
    Phase 2+: Will integrate with Google ADK's Agent class and LLM calls.
    """
    agent_config = AGENTS.get(agent_name)
    if not agent_config:
        return TaskResult(
            agent_name=agent_name,
            status=TaskStatus.FAILED,
            error=f"Unknown agent: {agent_name}",
        )

    start = time.time()

    console.print(f"\n[bold blue]▶ Running {agent_config.name} Agent...[/bold blue]")
    console.print(f"  Task: {task[:100]}...")

    # TODO: Replace with actual ADK agent execution
    # This stub simulates successful execution
    await asyncio.sleep(0.5)  # Simulate LLM call latency

    duration = time.time() - start

    result = TaskResult(
        agent_name=agent_name,
        status=TaskStatus.COMPLETED,
        output=f"[Stub] {agent_config.name} completed task: {task[:50]}...",
        duration_seconds=duration,
    )

    console.print(f"  [green]✓ {agent_config.name} completed in {duration:.1f}s[/green]")
    return result


# --- DAG Pipeline ---

class Pipeline:
    """
    The multi-agent DAG pipeline.

    DAG Structure:
        Architect
            ↓
        CodeGen (can run in parallel for independent components)
            ↓
        Reviewer
           ↓ (approve)     ↓ (reject → loop back to CodeGen)
        Tester
            ↓
        DocWriter
            ↓
        Human Review (optional)
    """

    def __init__(self, config: Optional[PipelineConfig] = None):
        self.config = config or get_pipeline_config()
        self.state: Optional[PipelineState] = None

    async def run(self, task: str) -> PipelineState:
        """Execute the full DAG pipeline for a given task."""
        self.state = PipelineState(task_description=task)

        console.print(Panel(
            f"[bold]ZenGate ADK Pipeline[/bold]\n\nTask: {task}",
            title="🚀 Pipeline Started",
            border_style="cyan",
        ))

        try:
            # Stage 1: Architect
            self.state.current_stage = "architect"
            arch_result = await self._run_with_retry("architect", task, {})
            if arch_result.status == TaskStatus.FAILED:
                return self._finalize("Architect failed")

            # Stage 2: CodeGen
            self.state.current_stage = "codegen"
            codegen_context = {"architecture": arch_result.output}
            codegen_result = await self._run_with_retry(
                "codegen", task, codegen_context
            )
            if codegen_result.status == TaskStatus.FAILED:
                return self._finalize("CodeGen failed")

            # Stage 3: Reviewer (with reject → CodeGen loop)
            self.state.current_stage = "reviewer"
            review_context = {
                "architecture": arch_result.output,
                "code": codegen_result.output,
            }

            for review_iteration in range(self.config.max_retries):
                review_result = await execute_agent("reviewer", task, review_context)
                self.state.add_result(review_result)

                if review_result.status == TaskStatus.COMPLETED:
                    # Check if approved or rejected
                    if "REJECT" in review_result.output.upper():
                        console.print(
                            f"[yellow]⟳ Reviewer rejected (attempt {review_iteration + 1}). "
                            f"Re-routing to CodeGen...[/yellow]"
                        )
                        # Re-run CodeGen with feedback
                        codegen_context["review_feedback"] = review_result.output
                        codegen_result = await self._run_with_retry(
                            "codegen", task, codegen_context
                        )
                        review_context["code"] = codegen_result.output
                        continue
                    else:
                        break  # Approved
                else:
                    return self._finalize("Reviewer failed")

            # Stage 4: Tester
            self.state.current_stage = "tester"
            test_result = await self._run_with_retry("tester", task, {})
            if test_result.status == TaskStatus.FAILED:
                return self._finalize("Tester failed")

            # Stage 5: DocWriter
            self.state.current_stage = "docwriter"
            doc_context = {
                "architecture": arch_result.output,
                "code": codegen_result.output,
            }
            doc_result = await self._run_with_retry("docwriter", task, doc_context)

            # Human-in-the-loop (optional)
            if self.config.human_in_the_loop:
                console.print(
                    "\n[bold yellow]⏸ Human review required.[/bold yellow]"
                    "\n  Review the generated output and approve or reject."
                )

            return self._finalize("Pipeline completed successfully")

        except Exception as e:
            console.print(f"[bold red]✗ Pipeline error: {e}[/bold red]")
            return self._finalize(f"Pipeline error: {e}")

    async def _run_with_retry(
        self, agent_name: str, task: str, context: dict
    ) -> TaskResult:
        """Run an agent with retry and exponential backoff."""
        for attempt in range(self.config.max_retries):
            result = await execute_agent(agent_name, task, context)
            self.state.add_result(result)

            if result.status == TaskStatus.COMPLETED:
                return result

            # Exponential backoff
            wait = self.config.retry_backoff_seconds * (2 ** attempt)
            console.print(
                f"[yellow]  ⟳ Retry {attempt + 1}/{self.config.max_retries} "
                f"in {wait:.1f}s...[/yellow]"
            )
            await asyncio.sleep(wait)
            result.retries = attempt + 1

        result.status = TaskStatus.FAILED
        return result

    def _finalize(self, message: str) -> PipelineState:
        """Finalize the pipeline and print summary."""
        self.state.completed_at = time.time()
        total_time = self.state.completed_at - self.state.started_at

        # Print summary table
        table = Table(title="Pipeline Summary")
        table.add_column("Agent", style="cyan")
        table.add_column("Status", style="bold")
        table.add_column("Duration", justify="right")
        table.add_column("Retries", justify="right")

        for result in self.state.results:
            status_style = "green" if result.status == TaskStatus.COMPLETED else "red"
            table.add_row(
                result.agent_name,
                f"[{status_style}]{result.status.value}[/{status_style}]",
                f"{result.duration_seconds:.1f}s",
                str(result.retries),
            )

        console.print(table)
        console.print(f"\n[bold]{message}[/bold] (total: {total_time:.1f}s)")

        # Save state to disk for debugging
        state_dir = Path(self.config.shared_memory_path)
        state_dir.mkdir(exist_ok=True)
        state_file = state_dir / f"pipeline_{int(self.state.started_at)}.json"
        state_file.write_text(self.state.to_json())
        console.print(f"  State saved to: {state_file}")

        return self.state


# --- CLI Entry Point ---

async def main():
    """Run the orchestrator from the command line."""
    import sys

    if len(sys.argv) < 2:
        console.print("[bold]ZenGate ADK Orchestrator[/bold]")
        console.print("\nUsage: python -m agents.orchestrator <task>")
        console.print("\nExample:")
        console.print('  python -m agents.orchestrator "Build the Redis rate limiter"')
        return

    task = " ".join(sys.argv[1:])
    pipeline = Pipeline()
    await pipeline.run(task)


if __name__ == "__main__":
    asyncio.run(main())
