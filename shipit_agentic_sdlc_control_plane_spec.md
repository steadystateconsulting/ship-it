# Ship(it)

## Agentic SDLC Control Plane Specification

**Working name:** Ship(it)  
**Tagline:** Set the course. Command the crew. Ship the work.  
**Status:** Draft specification  
**Primary user:** Technical operator / engineer / architect who wants repeatable, autonomous, reviewable software delivery workflows.

---

# 1. Overview

Ship(it) is an opinionated agentic software engineering control plane.

It coordinates frontier models, smaller models, local models, MCP tools, code agents, test runners, and review loops to transform high-level engineering goals into reviewed, tested, documented, and reproducible artifacts.

Ship(it) is not a generic chatbot and not merely a multi-agent swarm. It is a workflow harness for shipping software with durable state, explicit authority boundaries, model routing, artifact review, and feedback-driven improvement.

The human remains accountable. Ship(it) supplies the crew, instruments, logbook, and execution machinery.

---

# 2. Naming and Metaphor

## Product Name

**Ship(it)**

The name intentionally carries multiple meanings:

- Nautical metaphor: a ship, captain, crew, orders, watch, logbook, chartroom.
- DevOps metaphor: ship the software.
- IT nod: Ship(IT).
- Cultural callback: "just ship it," but with guardrails, reviews, testing, and traceability.

## Internal Vocabulary

| Concept | Ship(it) Term | Meaning |
|---|---|---|
| User | Captain | Sets direction, grants authority, owns risk |
| Agents | Crew | Specialized workers executing tasks |
| Goals | Orders | High-level objectives |
| Task graph | Chart | Planned route through work |
| Observability | Watch | Monitoring, metrics, evals, quality signals |
| Traces/artifacts | Logbook | Durable record of decisions and outputs |
| Model/tool registry | Quartermaster | Available resources and capabilities |
| Workspace | Dockyard | Where artifacts are built and tested |
| Review gates | Inspection | Quality control before promotion |

---

# 3. Core Thesis

The valuable unit is not the agent. The valuable unit is the repeatable workflow.

Most strong engineering workflows follow a recognizable pattern:

1. Clarify goal.
2. Produce spec.
3. Break work into tasks.
4. Implement in isolated workspaces.
5. Test locally and deterministically.
6. Review adversarially.
7. Repair failures.
8. Document aggressively.
9. Summarize evidence and remaining risk.
10. Ship.

Ship(it) codifies this loop and gives it memory, instrumentation, and model-aware routing.

---

# 4. Design Principles

## Simple Is Not Unsophisticated

Ship(it) should prefer simple, inspectable mechanisms over clever hidden complexity.

A boring task graph with strong state, artifact checks, and review gates is preferable to an impressive swarm that cannot explain itself.

## Artifacts Over Chat

The primary outputs are not conversations.

The primary outputs are:

- Specifications
- Architecture notes
- Task plans
- Source code
- Tests
- Pull requests
- Runbooks
- Review reports
- Decision records

## Explicit Authority

Agents do not receive broad standing permission.

Every task receives scoped tool grants.

Write actions, destructive actions, credential access, production changes, and external communications require explicit policy handling and may require human approval.

## Review Before Trust

No generated artifact is trusted simply because it was produced by a powerful model.

Artifacts should be inspected by an independent reviewer agent and, where possible, by deterministic tests.

## Learning Is a Product Feature

Ship(it) should not merely retry failures. It should learn which prompts, models, task shapes, context packages, and review strategies work best.

Learning should be explicit, measurable, and inspectable.

---

# 5. Target Workflows

## Software Feature Delivery

Input:

> Build an internal control console for ISO 8583 rule changes.

Outputs:

- Product spec
- Architecture document
- Task breakdown
- Implementation branch
- Unit tests
- Integration tests
- End-to-end tests
- Adversarial review
- Final delivery report

## Repository Modernization

Input:

> Migrate this service from ad-hoc scripts to Docker Compose with deterministic local testing.

Outputs:

- Dockerfile
- compose.yaml
- test harness
- README
- migration notes
- review report

## Compliance / Audit Research

Input:

> Read Jira, GitLab, Confluence, and Vanta evidence and produce a readiness report.

Outputs:

- Source summary
- Evidence map
- Gaps
- Suggested remediations
- Risk register

## Incident / Reliability Improvement

Input:

> Analyze recurring alerts and propose better paging thresholds and runbooks.

Outputs:

- Alert inventory
- Noise analysis
- Proposed changes
- Runbooks
- Test plan
- Rollout plan

---

# 6. Preference Profiles

Ship(it) should treat user preferences as first-class, versioned configuration.

Example:

```yaml
profile: ted-defaults

architecture:
  backend:
    prefer:
      - Go
    avoid:
      - heavyweight frameworks unless justified

  frontend:
    prefer:
      - static hosting
      - React when complexity warrants
      - Go templates with htmx for simple internal tools

  deployment:
    prefer:
      - container-first applications
      - Docker Compose for local integration
      - twelve-factor configuration

coding:
  require:
    - clear boundaries
    - boring interfaces
    - readable implementation

  avoid:
    - clever abstractions without payoff
    - framework sprawl
    - vibe-coded architecture

testing:
  require:
    - deterministic local seeds
    - unit tests for core logic
    - integration tests for service boundaries
    - end-to-end tests for critical workflows

  prefer:
    ui: Playwright
    go: table-driven tests

  reject_if:
    - no runnable local test path
    - tests require live vendor services without mocks
    - no evidence of test execution

documentation:
  require:
    - README
    - architecture notes
    - local development instructions
    - operational runbook
    - known limitations

review:
  require:
    - adversarial review
    - self-critique
    - test evidence
    - rollback notes
```

Preference profiles should influence planning, routing, reviews, and final acceptance checks.

---

# 7. System Architecture

```text
CLI / Web UI
  ↓
Ship(it) API
  ↓
Orchestration Engine
  ↓
Task Graph / Chart
  ↓
Crew Runtime
  ↓
Model Router + MCP Gateway
  ↓
Tools, Repositories, Documents, APIs

Parallel systems:

State Store
Artifact Store
Logbook
Watch
Learning Engine
Policy Engine
```

## Recommended Technology Choices

| Component | Recommendation |
|---|---|
| Control plane service | Go or Python |
| Workflow engine | LangGraph |
| Model abstraction | LiteLLM or custom provider adapter |
| Tool boundary | MCP gateway |
| State store | PostgreSQL |
| Artifact store | Git + filesystem, optionally S3/blob storage |
| Observability | Langfuse and/or OpenTelemetry |
| Local runtime | Docker Compose |
| Code implementation worker | Claude Code SDK/CLI, Codex/OpenAI agents, shell-isolated workers |
| Review workers | GPT frontier, Opus, Gemini Pro |
| Low-cost workers | Haiku, GPT mini, Gemini Flash, Ollama/local models |

---

# 8. Core Components

## 8.1 Ship(it) API

Responsibilities:

- Accept Orders
- Load preference profiles
- Create task graph
- Track state
- Enforce policy
- Coordinate agents
- Persist artifacts
- Report outcomes

## 8.2 Task Graph / Chart

The task graph represents the route from goal to shipped artifact.

Each task includes:

- Objective
- Inputs
- Outputs
- Acceptance criteria
- Model policy
- Tool grants
- Dependencies
- Retry policy
- Review requirements

Example:

```yaml
task:
  id: implement-rule-upload-api
  type: implementation
  objective: Build API endpoint for uploading and validating YAML rule files.
  inputs:
    - spec.md
    - architecture.md
  outputs:
    - source code
    - unit tests
    - integration tests
  acceptance:
    - compiles locally
    - tests pass
    - invalid YAML returns structured validation errors
    - no production credentials required
  model_policy:
    preferred: claude_code
    fallback: gpt_frontier
  tools:
    git:
      permissions: [read, branch, commit]
    shell:
      permissions: [test, build]
    filesystem:
      permissions: [read, write_workspace]
```

## 8.3 Crew Runtime

Crew members are specialized agents.

Initial Crew types:

- Planner
- Researcher
- Architect
- Implementer
- Test Writer
- Reviewer
- Security Reviewer
- Documentation Writer
- Summarizer
- Critic
- Repair Agent

Crew members should be composable but bounded.

The system should prefer supervisor-controlled delegation over free-form recursive swarming.

## 8.4 Model Router / Quartermaster

The router chooses models based on task type, risk, cost, context size, and historical performance.

Example routing policy:

```yaml
routing:
  planning:
    primary: gpt_frontier
    fallback: opus

  implementation:
    primary: claude_code
    fallback: gpt_frontier

  adversarial_review:
    primary: opus
    fallback: gpt_frontier

  summarization:
    primary: gemini_flash
    fallback: haiku

  extraction:
    primary: haiku
    fallback: gpt_mini

  private_low_risk_transform:
    primary: local_ollama
    fallback: gpt_mini
```

Routing decisions should be recorded in the Logbook.

## 8.5 MCP Gateway

The MCP gateway provides tool access with scoped authorization.

Supported tool classes:

- Git providers
- Jira
- Confluence
- Vanta
- Filesystem
- Shell
- Browser
- Databases
- Observability systems
- Cloud APIs

Tool access should be task-scoped.

Example:

```yaml
tool_grant:
  task_id: summarize-vanta-evidence
  agent: compliance_researcher
  tools:
    vanta:
      permissions: [read]
    confluence:
      permissions: [read]
    gitlab:
      permissions: []
    shell:
      permissions: []
```

---

# 9. Review and Error Checking

Ship(it) must support layered quality gates.

## Deterministic Gates

- Build succeeds
- Unit tests pass
- Integration tests pass
- End-to-end tests pass
- Lint/static analysis passes
- Generated files are formatted

## Agentic Gates

- Self-review
- Independent reviewer
- Adversarial reviewer
- Security reviewer
- Documentation reviewer

## Orchestrator Gates

The orchestrator may reject delegated work when:

- Acceptance criteria are unmet
- Tests are missing
- Test evidence is missing
- Implementation contradicts preferences
- Artifacts are malformed
- Review confidence is low
- Cost exceeds policy
- Tool permissions were exceeded

---

# 10. Learning Engine

The Learning Engine is a first-class subsystem.

Its purpose is to improve future behavior by measuring outcomes across models, prompts, task shapes, context packages, retry strategies, and review strategies.

This is not generic LLM fine-tuning. The initial target is operational learning through evaluation, routing, prompt evolution, and policy refinement.

## 10.1 What the System Learns

Ship(it) should learn:

- Which models perform best for specific task types
- Which prompt variants improve outputs
- Which context packages are necessary or wasteful
- Which reviewers catch which classes of defects
- Which low-cost models are good enough for specific tasks
- Which tasks require frontier models
- Which tasks should be split differently
- Which acceptance criteria predict successful outcomes
- Which retry strategies work
- Which preferences are commonly violated

## 10.2 Feedback Types

### Positive Feedback

Examples:

- Output accepted by reviewer
- Tests passed
- Human accepted artifact
- Subsequent implementation succeeded
- Low-cost model matched frontier-quality output
- Prompt variant improved score

### Negative Feedback

Examples:

- Reviewer rejected output
- Tests failed
- Human rejected artifact
- Model hallucinated unavailable APIs
- Context was ignored
- Output violated user preferences
- Repair loop failed
- Cost was excessive for output quality

## 10.3 Evaluation Records

Each meaningful task execution should produce an evaluation record.

```yaml
evaluation:
  run_id: run_123
  task_id: task_456
  task_type: implementation
  model: claude_code
  prompt_template: implement_go_service_v3
  context_package: repo_spec_tests_v2
  output_artifact: commit_sha
  scores:
    reviewer_score: 0.82
    test_pass_rate: 1.0
    preference_compliance: 0.91
    human_acceptance: true
  cost:
    tokens_in: 42000
    tokens_out: 9000
    dollars: 2.31
  latency_seconds: 480
  outcome: accepted
  notes:
    - Needed one repair loop for missing integration seed.
```

## 10.4 Prompt Variant Testing

Ship(it) should support A/B/C/D prompt testing.

Example:

```yaml
experiment:
  name: implementation_prompt_go_service
  task_type: implementation
  variants:
    - implement_go_service_v1
    - implement_go_service_v2
    - implement_go_service_v3
    - implement_go_service_with_test_contract_first
  sample_policy:
    strategy: epsilon_greedy
    exploration_rate: 0.15
  success_metrics:
    - reviewer_score
    - test_pass_rate
    - preference_compliance
    - repair_loops_required
    - cost_per_accepted_artifact
```

## 10.5 Model Benchmarking by Task Type

The system should compare models on actual user workflows.

Example:

```yaml
benchmark:
  task_type: confluence_summary
  candidates:
    - gemini_flash
    - haiku
    - gpt_mini
    - local_ollama
  judge:
    model: gpt_frontier
  metrics:
    - factuality
    - completeness
    - cost
    - latency
    - citation_quality
```

## 10.6 Retry Strategies

Retries should not be blind repetition.

Retry types:

- Same model, revised prompt
- Same model, reduced scope
- Different model, same context
- Different model, enriched context
- Split task into smaller tasks
- Escalate to frontier model
- Escalate to human

Example retry policy:

```yaml
retry_policy:
  max_attempts: 4
  strategy:
    - revise_prompt_with_reviewer_feedback
    - reduce_scope
    - escalate_model
    - escalate_to_human
```

## 10.7 Learning Outputs

The Learning Engine should periodically produce:

- Model capability reports
- Prompt performance reports
- Cost/quality tradeoff reports
- Recommended routing changes
- Common failure modes
- Preference violation summaries

Example:

```text
For Jira ticket summarization, Gemini Flash achieved 88% of GPT frontier reviewer score at 14% of the cost.
Recommendation: route Jira summarization to Gemini Flash by default, with GPT frontier review for high-risk epics.
```

## 10.8 Memory and Policy Updates

Ship(it) should distinguish between:

- Run memory: temporary execution context
- Project memory: repo/project-specific lessons
- User memory: durable preferences
- Global learning: anonymized/system-wide heuristics, if ever applicable

Policy updates should be explicit and reviewable.

Example proposed policy update:

```yaml
proposal:
  type: routing_policy_update
  reason: haiku_underperformed_on_schema_design
  change:
    schema_design:
      primary: gpt_frontier
      low_cost_allowed: false
  evidence:
    runs: [run_123, run_145, run_188]
```

No durable preference should be silently changed without visibility.

---

# 11. Data Model

Minimum tables:

```sql
runs
orders
tasks
task_dependencies
agents
models
model_invocations
tool_invocations
artifacts
reviews
evaluations
experiments
prompt_variants
routing_decisions
policy_decisions
checkpoints
human_approvals
```

## Key Record: Decision

Every important decision should be recorded.

Examples:

- Why was this model selected?
- Why was this task split?
- Why was the output rejected?
- Why was a retry attempted?
- Why was a human approval required?

---

# 12. MVP Scope

## MVP Goal

Build a local-first CLI that can take a high-level engineering order, generate a spec, plan implementation tasks, delegate implementation, review outputs, run tests, and produce a final report.

## MVP Features

- CLI entrypoint
- Preference profile loading
- Task graph generation
- Model routing
- Claude Code delegation
- OpenAI frontier planning/review
- Opus adversarial review option
- Local filesystem workspace
- Git branch/worktree isolation
- Shell-based build/test execution
- Artifact generation
- Logbook records
- Basic evaluation records
- Simple prompt variant testing

## MVP Non-Goals

- Fully autonomous production deployment
- Broad enterprise RBAC
- Multi-tenant SaaS
- Fine-tuning
- Unbounded recursive agents
- Silent destructive actions

---

# 13. Example CLI

```bash
shipit run \
  --goal "Build an ISO 8583 rule control console" \
  --repo ./control-console \
  --profile ./profiles/ted-defaults.yaml \
  --mode local
```

```bash
shipit eval report \
  --project control-console \
  --group-by model,task_type
```

```bash
shipit prompts compare \
  --task-type implementation \
  --metric cost_per_accepted_artifact
```

```bash
shipit policy propose \
  --from-learning \
  --project control-console
```

---

# 14. Example Run

Input:

```text
Build a web console for authenticated users to upload YAML authorization rules, validate them, compare them against current production rules, and stage approved changes.
```

Outputs:

```text
/specs/rule-console.md
/architecture/rule-console-architecture.md
/plans/task-graph.yaml
/src/...
/tests/unit/...
/tests/integration/...
/tests/e2e/...
/reviews/adversarial-review.md
/reports/final-delivery-report.md
/logbook/run.jsonl
/evals/run-evaluation.yaml
```

Final report includes:

- What was built
- What passed
- What failed
- What was repaired
- What remains risky
- What model/tool choices were made
- What learning records were produced

---

# 15. Acceptance Criteria

Ship(it) is successful when it can repeatedly:

- Turn high-level goals into usable specs
- Decompose work into sensible task graphs
- Delegate tasks to appropriate models
- Produce runnable local artifacts
- Run deterministic tests
- Reject poor outputs
- Repair failed work
- Explain decisions
- Improve routing and prompting over time
- Reduce required human babysitting

---

# 16. Strategic Differentiator

Most agent frameworks optimize for agent composition.

Ship(it) optimizes for shipping reviewed software.

The differentiator is the combination of:

- Durable workflow state
- Personal engineering taste profiles
- Scoped tool authority
- Multi-model routing
- Artifact-first output
- Adversarial review
- Deterministic testing
- Explicit feedback loops
- Prompt/model experimentation
- Cost-quality learning

The product is not "AI agents talking to each other."

The product is a repeatable, inspectable harness for turning intent into shipped engineering work.

