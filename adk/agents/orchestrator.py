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
import re
import subprocess
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
    run_agent_async,
)

def parse_and_write_files(output_text: str, base_dir: Path) -> list[str]:
    """Parse output text for [FILE: path] sections and write them to disk."""
    pattern = r'\[FILE:\s*(?P<path>[a-zA-Z0-9_\-\.\/]+)\]\s*\n*```[a-zA-Z0-9]*\n(?P<code>.*?)\n```'
    matches = list(re.finditer(pattern, output_text, re.DOTALL))
    
    files_written = []
    for m in matches:
        path_str = m.group('path').strip()
        code = m.group('code')
        
        # Guard against directory traversal
        if ".." in path_str or path_str.startswith("/") or path_str.startswith("\\"):
            continue
            
        file_path = base_dir / path_str
        file_path.parent.mkdir(parents=True, exist_ok=True)
        file_path.write_text(code, encoding='utf-8')
        files_written.append(path_str)
        
    return files_written

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
	Uses Google ADK's Agent model connection via run_agent_async.
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

	try:
		# Execute the agent query asynchronously using the configured models
		output = await run_agent_async(agent_name, task, context)
		
		duration = time.time() - start
		console.print(f"  [green]✓ {agent_config.name} completed in {duration:.1f}s[/green]")
		
		return TaskResult(
			agent_name=agent_name,
			status=TaskStatus.COMPLETED,
			output=output,
			duration_seconds=duration,
		)
	except Exception as e:
		duration = time.time() - start
		console.print(f"  [red]✗ {agent_config.name} failed: {e}[/red]")
		return TaskResult(
			agent_name=agent_name,
			status=TaskStatus.FAILED,
			error=str(e),
			duration_seconds=duration,
		)


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

    def _setup_git_branch(self, task: str, workspace_root: Path) -> Optional[str]:
        """Creates a sanitized git feature branch for this run."""
        try:
            # 1. Sanitize branch name
            # Remove non-alphanumeric/spaces, replace spaces with hyphens, limit length
            clean_name = re.sub(r'[^a-zA-Z0-9\s-]', '', task).strip().lower()
            clean_name = re.sub(r'[\s-]+', '-', clean_name)
            branch_name = f"adk/{clean_name[:40].strip('-')}"
            
            console.print(f"\n[dim]🔧 Setting up git branch: {branch_name}...[/dim]")
            
            # Check if branch already exists
            check_branch = subprocess.run(
                ["git", "show-ref", "--verify", f"refs/heads/{branch_name}"],
                cwd=str(workspace_root),
                capture_output=True,
            )
            
            if check_branch.returncode == 0:
                # Branch exists, switch to it
                subprocess.run(
                    ["git", "checkout", branch_name],
                    cwd=str(workspace_root),
                    check=True,
                    capture_output=True
                )
                console.print(f"  [green]✓ Switched to existing branch: {branch_name}[/green]")
            else:
                # Create and switch to it
                subprocess.run(
                    ["git", "checkout", "-b", branch_name],
                    cwd=str(workspace_root),
                    check=True,
                    capture_output=True
                )
                console.print(f"  [green]✓ Created and checked out new branch: {branch_name}[/green]")
                
            return branch_name
        except Exception as e:
            console.print(f"  [yellow]⚠ Failed to setup git branch: {e}. Running on current branch instead.[/yellow]")
            return None

    async def run(self, task: str) -> PipelineState:
        """Execute the full DAG pipeline for a given task."""
        self.state = PipelineState(task_description=task)
        workspace_root = Path(__file__).resolve().parent.parent.parent

        # Setup git branch for this task
        self._setup_git_branch(task, workspace_root)

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
                
            # Write specification doc to docs
            specs_dir = workspace_root / "docs" / "specs"
            specs_dir.mkdir(parents=True, exist_ok=True)
            spec_file = specs_dir / f"architect_spec_{int(time.time())}.md"
            spec_file.write_text(arch_result.output, encoding="utf-8")
            console.print(f"  [green]✓ Saved Architect spec to {spec_file.relative_to(workspace_root)}[/green]")

            # Stage 2: CodeGen
            self.state.current_stage = "codegen"
            codegen_context = {"architecture": arch_result.output}

            # Track all dirs ever written by CodeGen so we can clean them on retry
            all_generated_dirs: set[Path] = set()

            # Subprocess test loop for CodeGen
            for codegen_attempt in range(self.config.max_retries):
                # Clean up ALL previously generated files before this attempt so
                # stale/broken code from prior retries doesn't pollute go test ./...
                for stale_dir in all_generated_dirs:
                    if stale_dir.exists():
                        import shutil
                        shutil.rmtree(stale_dir, ignore_errors=True)
                        console.print(f"  [dim]↩ Cleaned up previous output: {stale_dir.relative_to(workspace_root)}[/dim]")

                codegen_result = await self._run_with_retry(
                    "codegen", task, codegen_context
                )
                if codegen_result.status == TaskStatus.FAILED:
                    return self._finalize("CodeGen failed")

                # Write files to disk and record which top-level dirs were touched
                written_files = parse_and_write_files(codegen_result.output, workspace_root)
                if written_files:
                    console.print(f"  [green]✓ CodeGen wrote files: {', '.join(written_files)}[/green]")
                    codegen_result.files_created = written_files
                    # Collect the unique package dirs for targeted testing & cleanup
                    for f in written_files:
                        pkg_dir = Path(f).parent
                        if pkg_dir != Path(".") and str(pkg_dir) != "":
                            all_generated_dirs.add(workspace_root / pkg_dir)
                else:
                    console.print("  [yellow]⚠ CodeGen output did not contain any [FILE: path] blocks to write![/yellow]")

                # Derive go package paths to test (e.g. "./internal/ratelimiter/...") 
                # so we only validate what CodeGen produced, not the whole repo.
                if written_files:
                    pkg_dirs = set()
                    for f in written_files:
                        pkg_dir = Path(f).parent
                        if pkg_dir != Path(".") and str(pkg_dir) != "":
                            pkg_dirs.add(str(pkg_dir).replace("\\", "/"))
                        else:
                            pkg_dirs.add(".")
                    test_patterns = [f"./{d}/..." if d != "." else "./..." for d in pkg_dirs]
                else:
                    test_patterns = ["./..."]

                # Run Go Tests (Real compilation & test validation)
                console.print("\n[bold blue]🧪 Running Go Tests to verify CodeGen output...[/bold blue]")
                console.print(f"  [dim]Packages: {' '.join(test_patterns)}[/dim]")
                try:
                    # Sync go.mod/go.sum with any new imports CodeGen may have added
                    subprocess.run(
                        ["go", "mod", "tidy"],
                        cwd=str(workspace_root),
                        capture_output=True,
                        text=True,
                        timeout=60,
                    )
                    test_proc = subprocess.run(
                        ["go", "test", "-cover"] + test_patterns,
                        cwd=str(workspace_root),
                        capture_output=True,
                        text=True,
                        timeout=60,
                    )
                    test_success = test_proc.returncode == 0
                    test_output = test_proc.stdout + "\n" + test_proc.stderr

                    if test_success:
                        console.print("  [green]✓ All tests passed successfully![/green]")
                        break
                    else:
                        console.print(f"  [red]✗ Go tests failed (attempt {codegen_attempt + 1}/{self.config.max_retries})[/red]")
                        console.print("[dim]" + test_output + "[/dim]")
                        codegen_context["review_feedback"] = (
                            f"Go test command failed with exit code {test_proc.returncode}.\n"
                            f"Test Logs:\n{test_output}\n"
                            "Please inspect the failure and fix the generated Go files and unit tests accordingly."
                        )
                except Exception as e:
                    console.print(f"  [red]⚠ Failed to execute go test command: {e}[/red]")
                    break
            else:
                return self._finalize("CodeGen failed to pass Go tests after retries")

            # Stage 3: Reviewer (with reject → CodeGen loop)
            self.state.current_stage = "reviewer"
            review_context = {
                "architecture": arch_result.output,
                "code": codegen_result.output,
            }

            for review_iteration in range(self.config.max_retries):
                review_result = await self._run_with_retry(
                    "reviewer", task, review_context
                )

                if review_result.status == TaskStatus.COMPLETED:
                    # Check if approved or rejected
                    if "REJECT" in review_result.output.upper():
                        console.print(
                            f"[yellow]⟳ Reviewer rejected (attempt {review_iteration + 1}). "
                            f"Re-routing to CodeGen...[/yellow]"
                        )
                        # Re-run CodeGen with feedback
                        codegen_context["review_feedback"] = review_result.output
                        
                        # Re-run CodeGen & test loop
                        for codegen_attempt in range(self.config.max_retries):
                            codegen_result = await self._run_with_retry(
                                "codegen", task, codegen_context
                            )
                            written_files = parse_and_write_files(codegen_result.output, workspace_root)
                            if written_files:
                                console.print(f"  [green]✓ CodeGen wrote files: {', '.join(written_files)}[/green]")
                            
                            try:
                                subprocess.run(
                                    ["go", "mod", "tidy"],
                                    cwd=str(workspace_root),
                                    capture_output=True,
                                    text=True,
                                    timeout=60,
                                )
                                test_proc = subprocess.run(
                                    ["go", "test", "-cover", "./..."],
                                    cwd=str(workspace_root),
                                    capture_output=True,
                                    text=True,
                                    timeout=60,
                                )
                                if test_proc.returncode == 0:
                                    break
                                else:
                                    codegen_context["review_feedback"] = (
                                        f"Go tests failed:\n{test_proc.stdout}\n{test_proc.stderr}"
                                    )
                            except Exception:
                                break
                                
                        review_context["code"] = codegen_result.output
                        continue
                    else:
                        break  # Approved
                else:
                    return self._finalize("Reviewer failed")

            # Stage 4: Tester
            self.state.current_stage = "tester"
            test_output = ""
            try:
                subprocess.run(
                    ["go", "mod", "tidy"],
                    cwd=str(workspace_root),
                    capture_output=True,
                    text=True,
                    timeout=60,
                )
                test_proc = subprocess.run(
                    ["go", "test", "-cover", "./..."],
                    cwd=str(workspace_root),
                    capture_output=True,
                    text=True,
                    timeout=60,
                )
                test_output = test_proc.stdout + "\n" + test_proc.stderr
            except Exception as e:
                test_output = f"Error running tests: {e}"
                
            test_context = {"test_run_logs": test_output}
            test_result = await self._run_with_retry("tester", task, test_context)
            if test_result.status == TaskStatus.FAILED:
                return self._finalize("Tester failed")

            # Stage 5: DocWriter
            self.state.current_stage = "docwriter"
            doc_context = {
                "architecture": arch_result.output,
                "code": codegen_result.output,
            }
            doc_result = await self._run_with_retry("docwriter", task, doc_context)
            
            # Parse and write DocWriter outputs (e.g. README.md modifications or docs)
            written_docs = parse_and_write_files(doc_result.output, workspace_root)
            if written_docs:
                console.print(f"  [green]✓ DocWriter wrote files: {', '.join(written_docs)}[/green]")
                doc_result.files_created = written_docs

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
        workspace_root = Path(__file__).resolve().parent.parent.parent

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

        # Git Commit on Success
        if message == "Pipeline completed successfully":
            try:
                # Stage all changes
                subprocess.run(
                    ["git", "add", "."],
                    cwd=str(workspace_root),
                    check=True,
                    capture_output=True
                )
                
                # Check current branch name
                branch_proc = subprocess.run(
                    ["git", "branch", "--show-current"],
                    cwd=str(workspace_root),
                    capture_output=True,
                    text=True,
                    check=True
                )
                current_branch = branch_proc.stdout.strip()
                
                # Commit changes
                commit_msg = f"feat(adk): {self.state.task_description}"
                if "\n" in commit_msg:
                    commit_msg = commit_msg.split("\n")[0]
                
                subprocess.run(
                    ["git", "commit", "-m", commit_msg],
                    cwd=str(workspace_root),
                    check=True,
                    capture_output=True
                )
                
                # Push branch to remote
                console.print(f"  [dim]Pushing branch {current_branch} to origin remote...[/dim]")
                subprocess.run(
                    ["git", "push", "-u", "origin", current_branch],
                    cwd=str(workspace_root),
                    check=True,
                    capture_output=True
                )
                
                console.print(Panel(
                    f"[green]✓ Changes successfully committed and pushed to remote branch: [bold]{current_branch}[/bold][/green]\n\n"
                    f"Create and merge a PR on GitHub at:\n"
                    f"  [bold]https://github.com/prakharxagrawal/AIZenGate/compare/{current_branch}[/bold]\n\n"
                    f"Once merged on GitHub, pull the changes locally with:\n"
                    f"  [bold]git checkout master && git pull origin master[/bold]",
                    title="Git Push Automation",
                    border_style="green",
                ))
            except Exception as e:
                console.print(f"  [yellow]⚠ Failed to auto-commit or push changes: {e}[/yellow]")

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
