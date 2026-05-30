# Ship(it) MVP Execution Loop Contract

**Status:** Captain's edits 
**Purpose:** Define the concrete execution contract needed to build the MVP implementation loop.  
**Scope:** Local-first CLI execution against one repository/workspace with durable artifacts, scoped tool authority, deterministic gates, agentic review, repair loops, and final reporting.

---

# 1. Contract Goals

This document turns the control-plane specification into implementation-facing rules.

The MVP loop must be able to:

1. Accept a high-level engineering order.
2. Create a durable run record.
3. Generate or load a spec.
4. Generate an executable task graph.
5. Delegate implementation/review tasks to workers.
6. Run deterministic checks.
7. Decide whether to accept, repair, retry, block, or fail work.
8. Preserve enough evidence to explain every meaningful decision.
9. Produce a final delivery report.

The MVP should optimize for inspectability and repeatability over maximal autonomy.

---

# 2. Core Terms

| Term | Meaning |
|---|---|
| Order | User-provided high-level goal |
| Run | One execution of Ship(it) against an order |
| Task | A unit of planned work in the task graph |
| Worker | A delegated model/tool executor, such as planner, implementer, reviewer, or summarizer |
| Artifact | Durable output file, patch, commit, report, log, evaluation, or decision record |
| Gate | Deterministic or agentic acceptance check |
| Repair | Follow-up task intended to fix rejected work |
| Escalation | Transfer to a stronger model or human decision |
| Policy | Rules controlling permissions, cost, model selection, approvals, and allowed side effects |

---

# 3. MVP Run State Machine

## 3.1 Run States

```text
created
  -> initializing
  -> planning
  -> awaiting_plan_approval
  -> executing
  -> reviewing
  -> repairing
  -> finalizing
  -> completed

terminal alternatives:
  blocked
  failed
  cancelled
```

## 3.2 State Definitions

| State | Meaning |
|---|---|
| created | Run row and run directory exist, but no execution has started |
| initializing | Inputs, repo state, profile, model config, and policy are being resolved |
| planning | Planner is producing spec, architecture notes, and task graph |
| awaiting_plan_approval | Optional human approval point before implementation begins |
| executing | One or more implementation/documentation/test tasks are running |
| reviewing | Completed task outputs are being checked by deterministic and agentic gates |
| repairing | Repair tasks are being generated and executed after rejection |
| finalizing | Final report, evaluation records, and summary artifacts are being produced |
| completed | Run met required acceptance criteria |
| blocked | Run requires human input or permission before meaningful progress can continue |
| failed | Run exhausted retry policy or hit an unrecoverable error |
| cancelled | User explicitly stopped the run |

## 3.3 Run Transition Rules

- Every transition must append a `run_state_changed` event to the logbook.
- A run may enter `repairing` multiple times.
- A run may move from `blocked` back to the previous active state after human input.
- A run may not move from any terminal state to an active state. Resuming terminal work creates a new run linked to the previous run.
- `awaiting_plan_approval` is optional and controlled by policy.

---

# 4. MVP Task State Machine

## 4.1 Task States

```text
pending
  -> ready
  -> running
  -> checking
  -> reviewing
  -> accepted

terminal alternatives:
  rejected
  blocked
  failed
  skipped
  cancelled
```

## 4.2 Task State Definitions

| State | Meaning |
|---|---|
| pending | Task exists but dependencies are incomplete |
| ready | Dependencies are accepted and required inputs are available |
| running | Worker is executing the task |
| checking | Deterministic checks are running |
| reviewing | Agentic review is running |
| accepted | Task passed required gates |
| rejected | Output was completed but did not satisfy required gates |
| blocked | Task needs human input, permission, credential, or external state |
| failed | Task failed due to unrecoverable worker/tool/runtime error or exhausted retries |
| skipped | Task was intentionally not run due to plan revision or policy |
| cancelled | User stopped the task or parent run |

## 4.3 Task Transition Rules

- `pending -> ready` requires all dependencies to be `accepted` or explicitly waived.
- `running -> checking` requires a worker result record.
- `checking -> reviewing` requires deterministic gate records.
- `reviewing -> accepted` requires all mandatory gates to pass.
- `reviewing -> rejected` requires at least one blocking gate failure.
- `rejected -> pending` is not allowed. Repair work is represented as a new task linked to the rejected task.
- `blocked` tasks preserve the worker context and may resume after a human decision.

---

# 5. Task Graph Contract

## 5.1 Required Task Fields

Each task must be serialized as YAML or JSON using this logical shape:

```yaml
id: implement-rule-upload-api
run_id: run_20260529_001
type: implementation
title: Implement rule upload API
objective: Build API endpoint for uploading and validating YAML rule files.
dependencies:
  - write-rule-console-spec
inputs:
  artifacts:
    - specs/rule-console.md
    - architecture/rule-console-architecture.md
  repo_paths:
    - src/
    - tests/
outputs:
  expected_artifacts:
    - src/
    - tests/unit/
    - tests/integration/
  expected_evidence:
    - build_log
    - test_log
acceptance:
  required:
    - compiles_locally
    - unit_tests_pass
    - invalid_yaml_returns_structured_validation_errors
    - no_production_credentials_required
  optional:
    - integration_tests_pass
model_policy:
  primary: claude_code
  fallback:
    - gpt_frontier
tool_grants:
  git:
    worker_permissions: [read, diff]
    orchestrator_permissions: [branch, checkpoint_commit, rollback_branch]
  filesystem:
    permissions: [read, write_workspace]
  shell:
    permissions: [build, test]
review:
  deterministic:
    - build
    - unit_tests
    - formatting
  agentic:
    - independent_review
    - security_review
retry_policy:
  max_attempts: 3
  strategies:
    - revise_prompt_with_feedback
    - reduce_scope
    - escalate_model
risk:
  level: medium
  reasons:
    - touches request validation path
status: pending
```

## 5.2 Task Types

MVP task types:

| Type | Purpose |
|---|---|
| planning | Produce spec, architecture, or task graph |
| research | Inspect repository or external documents |
| implementation | Modify source code or configuration |
| test_authoring | Add or improve tests |
| deterministic_check | Run build, test, lint, format, or other local checks |
| review | Review artifacts from another task |
| repair | Fix rejected output |
| documentation | Produce README, runbook, architecture note, or final report |
| summarization | Condense run evidence |

## 5.3 Task Graph Invariants

- Task IDs are unique within a run.
- The graph must be acyclic for MVP.
- Every task must have at least one acceptance criterion.
- Every implementation task must have at least one deterministic gate unless explicitly waived.
- Every implementation task must be reviewed by a worker that did not produce the implementation.
- A task may read outputs from dependencies only after they are accepted or waived.
- A plan revision must create a new task graph version rather than mutating history silently.

---

# 6. Worker Delegation Contract

## 6.1 Worker Input Envelope

Every worker receives a structured input envelope.

```yaml
worker_input:
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  role: implementer
  objective: Build API endpoint for uploading and validating YAML rule files.
  instructions:
    - Follow the project preference profile.
    - Modify only files required for this task.
    - Produce runnable tests or explain why not possible.
  context_package:
    profile: profiles/ted-defaults.yaml
    order: orders/order.md
    spec: specs/rule-console.md
    architecture: architecture/rule-console-architecture.md
    relevant_files:
      - src/rules/
      - tests/rules/
    prior_findings: []
  workspace:
    repo_path: worktrees/run_20260529_001
    branch: shipit/run_20260529_001
  tool_grants:
    filesystem:
      permissions: [read, write_workspace]
    shell:
      permissions: [build, test]
    git:
      permissions: [read, diff]
  expected_outputs:
    - source changes
    - unit tests
    - task_result.yaml
  constraints:
    cost_limit_usd: 8.00
    timeout_minutes: 45
    no_network_without_approval: true
```

## 6.2 Worker Output Envelope

Every worker must return a structured result.

```yaml
worker_result:
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  role: implementer
  status: completed
  summary: Added upload endpoint, YAML parser validation, and unit tests.
  artifacts:
    modified_paths:
      - src/api/rules.go
      - tests/rules_upload_test.go
    created_paths:
      - docs/rule-upload-validation.md
    checkpoint_commit: null
    patch_file: artifacts/tasks/implement-rule-upload-api/changes.patch
  evidence:
    commands_run:
      - command: go test ./...
        exit_code: 0
        log: artifacts/tasks/implement-rule-upload-api/go-test.log
    checks:
      - name: unit_tests
        status: passed
  open_questions: []
  risks:
    - Integration behavior with production rule store still needs a mock-backed test.
  policy_events: []
  token_usage:
    input_tokens: 42000
    output_tokens: 9000
  cost_usd: 2.31
```

## 6.3 Worker Failure Result

Worker failures must also be structured.

```yaml
worker_result:
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  role: implementer
  status: failed
  failure:
    class: deterministic_test_failure
    message: Integration tests require a missing local database service.
    retryable: true
    suggested_next_action: add_compose_service_or_mock_database
  artifacts:
    modified_paths: []
  evidence:
    commands_run:
      - command: go test ./...
        exit_code: 1
        log: artifacts/tasks/implement-rule-upload-api/go-test.log
```

---

# 7. Context Package Contract

## 7.1 Context Package Contents

A context package is the bounded information bundle passed to a worker.

MVP context packages may include:

- Original order.
- Preference profile.
- Current task.
- Accepted dependency outputs.
- Relevant source files.
- Relevant tests.
- Existing project docs.
- Prior review findings.
- Recent deterministic check logs.
- Policy constraints.

## 7.2 Context Selection Rules

- Context must be task-specific.
- The planner may propose relevant files, but the orchestrator owns final packaging.
- Large files should be summarized unless exact content is required.
- Test logs should be truncated to failing sections plus enough surrounding context.
- Context packages must be recorded by reference so a run can explain what each worker saw.
- Secret material must never be placed in context unless policy explicitly allows it.

## 7.3 Context Package Record

```yaml
context_package:
  id: ctx_implement_rule_upload_api_v1
  task_id: implement-rule-upload-api
  created_at: "2026-05-29T14:12:03Z"
  sources:
    - type: artifact
      path: specs/rule-console.md
    - type: repo_path
      path: src/rules/
    - type: log_excerpt
      path: artifacts/tasks/prior-task/go-test-failures.log
  summaries:
    - source: src/legacy_rules/
      summary_path: artifacts/context/legacy-rules-summary.md
  excluded:
    - path: .env
      reason: secret_or_sensitive
```

---

# 8. Gate Contract

## 8.1 Gate Types

MVP gates:

| Gate | Type | Blocking by default |
|---|---|---|
| build | deterministic | yes |
| unit_tests | deterministic | yes |
| integration_tests | deterministic | policy-dependent |
| e2e_tests | deterministic | policy-dependent |
| formatting | deterministic | yes |
| lint_static_analysis | deterministic | policy-dependent |
| independent_review | agentic | yes |
| security_review | agentic | risk-dependent |
| documentation_review | agentic | policy-dependent |
| final_report_review | agentic | yes |

## 8.2 Gate Result Schema

```yaml
gate_result:
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  gate: independent_review
  type: agentic
  status: failed
  blocking: true
  severity: high
  confidence: 0.86
  summary: Upload endpoint accepts YAML aliases without depth or expansion limits.
  evidence:
    - file: src/api/rules.go
      line: 88
      note: Parser invoked without resource limits.
  required_fix: Add parser limits and tests for alias expansion.
  reviewer:
    model: opus
    prompt_variant: adversarial_review_v1
```

## 8.3 Gate Decision Rules

- All blocking gates must pass before a task can be accepted.
- A failed non-blocking gate must be recorded as residual risk.
- A reviewer finding is blocking when it identifies a correctness, security, data-loss, credential, production-safety, or acceptance-criteria failure with confidence at or above policy threshold.
- Default blocking confidence threshold is `0.70`.
- Human approval may waive a blocking gate, but the waiver must be recorded with rationale.
- The orchestrator may request a second reviewer when severity is high and confidence is below threshold.

---

# 9. Policy and Approval Contract

## 9.1 Permission Scopes

MVP permission scopes:

| Tool Class | Permissions |
|---|---|
| filesystem | read, write_workspace, delete_workspace_scoped |
| git_worker | read, diff |
| git_orchestrator | read, diff, branch, checkpoint_commit, rollback_branch |
| shell | inspect, build, test, format |
| network | disabled, approved_domains, unrestricted |
| browser | local_only, approved_domains |
| secrets | none, named_secret_read |

## 9.2 Actions Requiring Human Approval

Human approval is required for:

- Destructive filesystem operations outside the run workspace.
- Writing outside the configured repository/workspace.
- Accessing secrets.
- Network access outside approved domains.
- Production API access.
- Database writes outside local/dev containers.
- Pushing branches or opening pull requests.
- Cost estimate above run policy.
- Waiving blocking gates.

Destructive filesystem operations inside the run workspace do not require human approval when explicitly granted, but they do require a task-scoped `delete_workspace_scoped` grant and a logbook event. The default worker filesystem grant should allow writes, not cleanup or deletion.

## 9.3 Denied Action Behavior

When a worker attempts an ungranted action:

1. Block the action.
2. Record a `policy_denied` event.
3. Ask the orchestrator whether the action is necessary.
4. If necessary, create a human approval request.
5. If not necessary, continue with a safer alternative or reject the task output.

## 9.4 Approval Record

```yaml
human_approval:
  id: approval_001
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  requested_action: network.approved_domains
  requested_scope:
    domains:
      - docs.example.com
  reason: Worker needs current API docs for the local dependency.
  status: approved
  approved_by: captain
  approved_at: "2026-05-29T15:31:00Z"
  expires_at: "2026-05-29T17:31:00Z"
```

---

# 10. Retry and Repair Contract

## 10.1 Retry Categories

| Category | Meaning |
|---|---|
| same_task_retry | Re-run same task after transient failure or prompt revision |
| repair_task | New task that fixes accepted modifications from a rejected task |
| scope_split | Replace one broad task with smaller tasks |
| model_escalation | Retry with stronger or better-suited model |
| human_escalation | Ask user for decision, permission, or missing information |

## 10.2 Default Retry Policy

```yaml
retry_policy:
  max_attempts_per_task: 3
  max_repair_cycles_per_run: 3
  dirty_state_strategy: fix_in_place
  strategies:
    - revise_prompt_with_reviewer_feedback
    - reduce_scope
    - escalate_model
    - escalate_to_human
```

## 10.3 Dirty State Repair Strategies

Rejected implementation work leaves the run workspace in a dirty state unless the orchestrator explicitly rolls it back.

The MVP supports three dirty-state strategies:

| Strategy | Meaning | Default |
|---|---|---|
| fix_in_place | Repair task starts from the rejected workspace state and fixes forward | yes |
| orchestrator_rollback | Orchestrator rolls the branch back to the last accepted checkpoint before retrying | no |
| executor_discretion | Repair worker first scopes the failed change, then may request rollback-and-restart if fixing in place is not worth it | no |

`fix_in_place` is the normal mode of operation. It preserves context and avoids wasting partial work.

`orchestrator_rollback` is for changes the orchestrator judges too broad, incoherent, risky, or policy-violating to salvage.

`executor_discretion` is a permissive mode. The worker may recommend rollback after inspecting the failed state, but the orchestrator still owns the actual rollback decision and Git operation.

## 10.4 Retry Decision Rules

- Do not retry blindly with the same prompt and context.
- Test failures should feed exact failing output into the next context package.
- Agentic review failures should become explicit repair acceptance criteria.
- Repair normally happens in place on top of the rejected state.
- Rollback is allowed only to the most recent accepted checkpoint unless a human approves a different target.
- If two repair attempts fail for the same root cause, escalate to a stronger model or human.
- If the run hits `max_repair_cycles_per_run`, mark the run `failed` or `blocked` depending on whether human input could resolve it.
- Every retry must create a decision record.

---

# 11. Git and Workspace Contract

## 11.1 Workspace Setup

For MVP, each run gets an isolated working directory.

```text
.shipit/
  runs/
    run_20260529_001/
      workspace/
      artifacts/
      logbook/
      evals/
```

The workspace may be implemented as:

- A Git worktree from the target repository.
- A copied workspace for repositories where worktrees are not practical.

Git worktree is preferred.

## 11.2 Branch Naming

Default branch pattern:

```text
shipit/<date>-<slug>-<run_id>
```

Example:

```text
shipit/20260529-rule-console-run-001
```

## 11.3 Dirty Worktree Handling

Before creating a run workspace:

- If the target repo has uncommitted user changes, do not modify that worktree directly.
- Prefer creating a separate worktree from the current HEAD.
- Record the original repo status.
- If required files exist only as uncommitted changes, ask for human approval before copying them into the run workspace.

## 11.4 Commit Policy

MVP default:

- Workers may edit files and inspect diffs, but must not create commits, reset branches, merge branches, push, or rewrite history.
- The orchestrator owns branch creation, patch capture, checkpoint commits, rollback, and final promotion.
- The orchestrator captures a patch artifact for every implementation or repair attempt before deciding whether it is accepted.
- Each accepted implementation or repair task should create one checkpoint commit.
- Commit messages should include the task ID.
- Rejected work remains dirty for `fix_in_place` repair unless policy or orchestrator judgment selects rollback.
- Final report must list checkpoint commits, rejected patch artifacts, and any remaining uncommitted changes.

The commit is the durable checkpoint. The patch file is the reviewable evidence artifact. They serve different purposes and should both exist for accepted implementation work.

## 11.5 Conflict Handling

- If two tasks modify overlapping files, the orchestrator serializes them unless an explicit merge strategy exists.
- Merge conflicts create a repair or integration task.
- Conflict resolution must be reviewed like any other implementation task.

---

# 12. Artifact Layout

MVP run directory:

```text
.shipit/runs/<run_id>/
  order.md
  profile.yaml
  run.yaml
  task-graph.v1.yaml
  task-graph.current.yaml
  workspace/
  artifacts/
    specs/
    architecture/
    tasks/
      <task_id>/
        worker-input.yaml
        worker-result.yaml
        changes.patch
        logs/
        gate-results.yaml
    reviews/
    reports/
      final-delivery-report.md
  logbook/
    run.jsonl
    decisions.jsonl
    model-invocations.jsonl
    tool-invocations.jsonl
    policy-events.jsonl
  evals/
    run-evaluation.yaml
    task-evaluations.yaml
```

## 12.1 Artifact Rules

- Artifacts are append-only unless explicitly marked as current pointers.
- Mutating files such as `task-graph.current.yaml` must point to immutable versions.
- Logs must be preserved even for failed tasks.
- Large logs may be summarized, but original log paths should be retained when practical.
- Final reports must reference evidence paths.

---

# 13. Logbook Events

## 13.1 Required Event Types

MVP logbook must support:

- `run_created`
- `run_state_changed`
- `task_created`
- `task_state_changed`
- `worker_started`
- `worker_completed`
- `worker_failed`
- `model_invoked`
- `tool_invoked`
- `gate_started`
- `gate_completed`
- `decision_recorded`
- `policy_denied`
- `human_approval_requested`
- `human_approval_resolved`
- `artifact_created`
- `retry_scheduled`
- `run_completed`
- `run_failed`

## 13.2 Event Schema

```json
{
  "event_id": "evt_001",
  "timestamp": "2026-05-29T15:10:11Z",
  "run_id": "run_20260529_001",
  "task_id": "implement-rule-upload-api",
  "type": "task_state_changed",
  "actor": "orchestrator",
  "summary": "Task moved from checking to reviewing.",
  "data": {
    "from": "checking",
    "to": "reviewing"
  }
}
```

---

# 14. Decision Records

## 14.1 Required Decision Points

The orchestrator must record decisions for:

- Model selection.
- Task graph generation.
- Task splitting.
- Gate pass/fail interpretation.
- Retry strategy.
- Repair task generation.
- Human approval request.
- Blocking gate waiver.
- Run completion with residual risk.

## 14.2 Decision Schema

```yaml
decision:
  id: decision_001
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  type: retry_strategy
  made_by: orchestrator
  timestamp: "2026-05-29T15:42:19Z"
  decision: create_repair_task
  rationale: Independent review found a high-confidence YAML parser resource exhaustion risk.
  alternatives_considered:
    - waive_finding
    - retry_original_task
    - escalate_to_human
  evidence:
    - artifacts/tasks/implement-rule-upload-api/gate-results.yaml
  policy_basis:
    - security_review_high_confidence_findings_are_blocking
```

---

# 15. Model Routing Contract

## 15.1 Routing Input

Routing decisions should consider:

- Task type.
- Risk level.
- Context size.
- Required tool use.
- Cost policy.
- Latency policy.
- Historical model performance, when available.
- User preference profile.

## 15.2 Routing Record

```yaml
routing_decision:
  id: route_001
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  task_type: implementation
  selected_model: claude_code
  fallback_models:
    - gpt_frontier
  rationale:
    - implementation task against local repo
    - tool use required
    - profile prefers strong code worker for implementation
  estimated_cost_usd: 4.00
  policy_result: allowed
```

---

# 16. Evaluation Records

## 16.1 Task Evaluation

Each meaningful task execution should produce a task evaluation.

```yaml
task_evaluation:
  run_id: run_20260529_001
  task_id: implement-rule-upload-api
  task_type: implementation
  model: claude_code
  prompt_variant: implement_go_service_v1
  context_package: ctx_implement_rule_upload_api_v1
  outcome: accepted
  attempts: 2
  repair_tasks:
    - repair-yaml-parser-limits
  scores:
    deterministic_pass_rate: 1.0
    reviewer_score: 0.88
    preference_compliance: 0.92
  cost:
    input_tokens: 42000
    output_tokens: 9000
    usd: 2.31
  latency_seconds: 480
  notes:
    - Initial review caught missing parser depth limit.
```

## 16.2 Run Evaluation

```yaml
run_evaluation:
  run_id: run_20260529_001
  outcome: completed
  total_tasks: 12
  accepted_tasks: 12
  failed_tasks: 0
  repair_cycles: 1
  total_cost_usd: 14.72
  total_latency_seconds: 3120
  deterministic_gates:
    passed: 18
    failed: 2
    waived: 0
  agentic_gates:
    passed: 7
    failed: 1
    waived: 0
  residual_risks:
    - Production rule-store integration was mocked, not tested against live service.
```

---

# 17. MVP CLI Workflow

## 17.1 Create and Run

```bash
shipit run \
  --goal "Build an ISO 8583 rule control console" \
  --repo ./control-console \
  --profile ./profiles/ted-defaults.yaml \
  --mode local
```

Default behavior:

1. Create run directory.
2. Snapshot repo status.
3. Create isolated workspace.
4. Load profile and policy.
5. Generate spec and task graph.
6. Ask for plan approval if policy requires it.
7. Execute ready tasks.
8. Run gates.
9. Generate repair tasks as needed.
10. Finalize report.

## 17.2 Inspect

```bash
shipit status --run run_20260529_001
shipit log --run run_20260529_001
shipit tasks --run run_20260529_001
shipit report --run run_20260529_001
```

## 17.3 Resume

```bash
shipit resume --run run_20260529_001
```

Resume behavior:

- Load `run.yaml`.
- Validate workspace still exists.
- Reconcile task states with artifact evidence.
- Continue from the latest non-terminal state.
- If reconciliation fails, mark run `blocked` and explain why.

---

# 18. Final Report Contract

The final report must include:

- Original order.
- What was built.
- What changed.
- Task summary.
- Deterministic checks run and results.
- Agentic reviews run and results.
- Repairs performed.
- Human approvals or waivers.
- Model and tool choices.
- Cost and latency summary.
- Residual risks.
- Follow-up recommendations.
- Artifact index.

The final report should be concise enough for a human to review quickly, but detailed enough to support auditability through links to evidence artifacts.

---

# 19. Resolved MVP Defaults

For the first implementation loop:

- Use filesystem-backed working documents and JSONL/YAML run state before PostgreSQL.
- Keep structured manifests and indexes from day one so agents do not need to discover state by grepping arbitrary files.
- Run tasks serially until task and artifact contracts are stable.
- Include a `max_parallel_tasks` policy knob now, defaulted to `1`, so the scheduler can grow into parallel execution without changing the contract.
- Make plan approval policy-controlled. Default to requiring approval before implementation.
- Use Git worktree branches by default.
- Workers produce filesystem changes and task result records, not commits.
- Orchestrator captures patch artifacts for every implementation and repair attempt.
- Orchestrator creates checkpoint commits after accepted implementation and repair tasks.
- Treat build, unit tests, formatting, independent review, and final report review as blocking.
- Treat existing runnable integration tests as blocking.
- Treat new integration-test creation as planner-recommended work, not a mandatory MVP gate.
- Use both severity labels and numeric confidence for review findings.
- Make blocked runs resumable indefinitely for MVP.
- Require Captain review after repeated failure loops before continuing autonomous retries.
- Require human approval for network, secrets, push, production access, destructive actions outside the workspace, and gate waivers.
- Require explicit task grants for destructive cleanup inside the run workspace.
- Preserve every worker input, worker result, gate result, decision record, patch artifact, and checkpoint reference.

---

# 20. Remaining Design Questions

The core loop is ready to implement. Remaining questions can be deferred behind configuration or narrow interfaces:

1. What exact threshold should trigger Captain review after repeated failure loops?
2. Which structured records should be promoted from filesystem JSONL/YAML into a database first?
3. What is the first useful document index format for agent-facing retrieval?
4. How much autonomy should be allowed when plan approval is disabled?

---

# 21. Implementation Loop Build Plan

## 21.1 Phase 1: Local Run Skeleton

Build the CLI and filesystem-backed run model:

1. `shipit run --goal ... --repo ... --profile ...`
2. Create `.shipit/runs/<run_id>/`.
3. Write `order.md`, `profile.yaml`, `run.yaml`, and initial logbook records.
4. Snapshot target repo status.
5. Create isolated Git worktree and run branch.
6. Support `shipit status`, `shipit log`, and `shipit resume` against filesystem state.

Exit criteria:

- A run can be created, inspected, resumed, cancelled, and finalized without model calls.
- State transitions and logbook events are durable.

## 21.2 Phase 2: Planner and Task Graph

Implement the planning loop:

1. Build planner worker envelope.
2. Generate spec and task graph from the order/profile/repo summary.
3. Validate task graph schema and invariants.
4. Write immutable `task-graph.v1.yaml` and current pointer.
5. Enforce plan approval according to policy.

Exit criteria:

- A high-level order becomes a valid task graph.
- Invalid plans fail before implementation begins.

## 21.3 Phase 3: Serial Task Executor

Implement the first serial scheduler:

1. Select the next `ready` task.
2. Build context package.
3. Invoke the configured worker.
4. Record worker input, worker result, model invocation, and tool invocation records.
5. Capture workspace diff as `changes.patch`.
6. Run deterministic gates.

Exit criteria:

- One implementation task can edit a worktree and produce durable evidence.
- The orchestrator can accept, reject, block, or fail the task.

## 21.4 Phase 4: Review, Repair, and Checkpointing

Implement gate handling and repair behavior:

1. Run independent review against the task diff and context package.
2. Convert blocking findings into repair-task acceptance criteria.
3. Default repair strategy to `fix_in_place`.
4. Support `orchestrator_rollback` to last accepted checkpoint.
5. Support `executor_discretion` as a worker recommendation, with orchestrator-owned rollback.
6. Create checkpoint commits only after all blocking gates pass.

Exit criteria:

- Failed work can be repaired.
- Accepted work creates a clean checkpoint commit.
- Rejected patches remain available as evidence.

## 21.5 Phase 5: Final Report and Evaluation

Close the loop:

1. Generate task evaluations and run evaluation.
2. Produce final delivery report.
3. Include checkpoint commits, rejected patches, gate results, costs, approvals, residual risks, and follow-ups.
4. Mark run `completed`, `failed`, or `blocked`.

Exit criteria:

- A completed run has enough evidence for the Captain to understand what happened without reading raw logs first.

## 21.6 Phase 6: Hardening Knobs

Add the configuration hooks that are already in the contract:

1. `max_parallel_tasks`, default `1`.
2. `plan_approval_required`, default `true`.
3. `dirty_state_strategy`, default `fix_in_place`.
4. `integration_tests_blocking`, default `when_runnable`.
5. `failure_loop_captain_review_threshold`.
6. Structured document index for human-readable artifacts.
