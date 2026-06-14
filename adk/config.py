"""
ZenGate ADK Configuration

LLM Fallback Chain:
  1. DeepSeek V4 Flash Free (primary — free, 200K context)
  2. Gemini API Free Tier (fallback — permanent free, 1500 req/day)

All agent configurations and tool definitions are centralized here.
"""

import os
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class LLMConfig:
    """Configuration for an LLM provider."""
    name: str
    model_id: str
    api_key: str
    base_url: Optional[str] = None
    max_tokens: int = 8192
    temperature: float = 0.3
    is_available: bool = True

    def __post_init__(self):
        """Check if the API key is set; mark unavailable if not."""
        if not self.api_key:
            self.is_available = False


@dataclass
class AgentConfig:
    """Configuration for an individual ADK agent."""
    name: str
    role: str
    system_prompt: str
    tools: list[str] = field(default_factory=list)
    llm_preference: str = "primary"  # "primary", "fallback", or "any"


# --- LLM Providers ---

def get_primary_llm() -> LLMConfig:
    """DeepSeek V4 Flash Free — primary LLM for all agents."""
    return LLMConfig(
        name="DeepSeek V4 Flash Free",
        model_id="deepseek-v4-flash-free",
        api_key=os.getenv("DEEPSEEK_API_KEY", ""),
        base_url=os.getenv("DEEPSEEK_BASE_URL", "https://opencode.ai/zen/v1"),
        max_tokens=16384,
        temperature=0.2,
    )


def get_fallback_llm() -> LLMConfig:
    """Gemini API Free Tier — fallback LLM for reliability."""
    return LLMConfig(
        name="Gemini Free Tier",
        model_id="gemini-2.0-flash",
        api_key=os.getenv("GEMINI_API_KEY", ""),
        max_tokens=8192,
        temperature=0.3,
    )


def get_llm_chain() -> list[LLMConfig]:
    """Returns the LLM fallback chain: primary → fallback."""
    chain = []
    primary = get_primary_llm()
    if primary.is_available:
        chain.append(primary)

    fallback = get_fallback_llm()
    if fallback.is_available:
        chain.append(fallback)

    if not chain:
        raise RuntimeError(
            "No LLM available. Set DEEPSEEK_API_KEY or GEMINI_API_KEY environment variable."
        )

    return chain


# --- Agent Definitions ---

AGENTS: dict[str, AgentConfig] = {
    "architect": AgentConfig(
        name="Architect",
        role="system_architect",
        system_prompt="""You are the System Architect agent for ZenGate AI.
Your responsibilities:
1. Break down high-level tasks into detailed technical specifications
2. Define interface contracts between components
3. Create data flow diagrams and architecture decisions
4. Ensure designs follow distributed systems best practices

Output format: Markdown document with sections:
- Overview
- Interface Contracts (Go interfaces)
- Data Flow
- Design Decisions & Trade-offs
- Dependencies
""",
        tools=["read_file", "write_file", "search_codebase"],
    ),
    "codegen": AgentConfig(
        name="CodeGen",
        role="code_generator",
        system_prompt="""You are the Code Generator agent for ZenGate AI.
Your responsibilities:
1. Read architecture documents and generate production-grade Go code
2. Follow Go best practices: effective Go, standard library first
3. Write idiomatic error handling with wrapped errors
4. Generate comprehensive test files alongside implementation
5. Use structured logging (log/slog) throughout

Rules:
- ALWAYS include package documentation
- ALWAYS handle errors explicitly (no _ = err)
- Use interfaces for testability
- Keep functions under 50 lines
- Write table-driven tests
""",
        tools=["read_file", "write_file", "run_command"],
    ),
    "reviewer": AgentConfig(
        name="Reviewer",
        role="code_reviewer",
        system_prompt="""You are the Code Reviewer agent for ZenGate AI.
Your responsibilities:
1. Review generated code for correctness, performance, and security
2. Check for race conditions in concurrent code
3. Verify error handling is comprehensive
4. Ensure tests cover edge cases
5. Approve or reject with detailed feedback

Output format:
- APPROVE: Code is ready to merge
- REJECT: Code needs changes (list specific issues)
- Each issue must include: file, line, severity (critical/warning/info), description, fix suggestion
""",
        tools=["read_file", "search_codebase"],
    ),
    "tester": AgentConfig(
        name="Tester",
        role="test_runner",
        system_prompt="""You are the Tester agent for ZenGate AI.
Your responsibilities:
1. Run go test ./... with -race and -cover flags
2. Run golangci-lint for static analysis
3. Execute integration tests if Redis/etcd are available
4. Report test results with pass/fail counts and coverage percentage
5. If tests fail, provide the exact error output

Output format:
- Test Results: X passed, Y failed, Z skipped
- Coverage: XX%
- Lint Issues: [list]
- Recommendation: PASS / FAIL / RETRY
""",
        tools=["run_command", "read_file"],
    ),
    "docwriter": AgentConfig(
        name="DocWriter",
        role="documentation_writer",
        system_prompt="""You are the Documentation Writer agent for ZenGate AI.
Your responsibilities:
1. Generate and update README.md with project overview, setup, and usage
2. Write API documentation from Go handler code
3. Update architecture diagrams when code changes
4. Generate inline code documentation (godoc format)
5. Create example requests/responses for each API endpoint

Style:
- Professional but approachable tone
- Include runnable code examples
- Add mermaid diagrams for architecture
- Use badges (build status, coverage, license)
""",
        tools=["read_file", "write_file", "search_codebase"],
    ),
}


# --- DAG Pipeline Configuration ---

@dataclass
class PipelineConfig:
    """Configuration for the multi-agent DAG pipeline."""
    max_retries: int = 3
    retry_backoff_seconds: float = 2.0
    human_in_the_loop: bool = True
    parallel_codegen: bool = True
    shared_memory_path: str = ".zengate-adk-memory"


def get_pipeline_config() -> PipelineConfig:
    """Returns the pipeline configuration."""
    return PipelineConfig(
        max_retries=int(os.getenv("ADK_MAX_RETRIES", "3")),
        retry_backoff_seconds=float(os.getenv("ADK_RETRY_BACKOFF", "2.0")),
        human_in_the_loop=os.getenv("ADK_HUMAN_IN_LOOP", "true").lower() == "true",
        parallel_codegen=os.getenv("ADK_PARALLEL_CODEGEN", "true").lower() == "true",
    )
