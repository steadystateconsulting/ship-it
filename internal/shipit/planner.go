package shipit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PlanOptions struct {
	AutoApprove  bool
	Planner      string
	PlannerModel string
}

func PlanRun(store *Store, run *Run, opts PlanOptions) error {
	if run.TaskGraphVersion != "" {
		return nil
	}
	if err := TransitionRun(store, run, StatePlanning, "Planner started."); err != nil {
		return err
	}

	repoFiles, err := listRepoFiles(run.WorkspaceDir, 24)
	if err != nil {
		return err
	}
	taskDir := filepath.Join(run.RunDir, "artifacts", "tasks", "plan-run")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return err
	}

	specPath := filepath.Join(run.RunDir, "artifacts", "specs", "mvp-plan-spec.md")
	architecturePath := filepath.Join(run.RunDir, "artifacts", "architecture", "mvp-plan-architecture.md")
	workerInputPath := filepath.Join(taskDir, "worker-input.yaml")
	workerResultPath := filepath.Join(taskDir, "worker-result.yaml")

	if opts.Planner == "" {
		opts.Planner = run.Planner
	}
	if opts.Planner == "" {
		opts.Planner = "local"
	}
	if opts.PlannerModel == "" {
		opts.PlannerModel = run.PlannerModel
	}

	if err := os.WriteFile(workerInputPath, []byte(renderPlannerInput(run, repoFiles)), 0644); err != nil {
		return err
	}
	specText, archText, modelResponseID, err := generatePlanningDocuments(run, repoFiles, opts)
	if err != nil {
		return err
	}
	if err := os.WriteFile(specPath, []byte(specText), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(architecturePath, []byte(archText), 0644); err != nil {
		return err
	}

	graph := buildDefaultTaskGraph(run, specPath, architecturePath, repoFiles)
	if err := store.SaveTaskGraph(run, graph); err != nil {
		return err
	}
	if err := os.WriteFile(workerResultPath, []byte(renderPlannerResult(run, specPath, architecturePath, graph, opts, modelResponseID)), 0644); err != nil {
		return err
	}

	if err := store.AppendEvent(run.RunID, Event{
		Type:    "worker_completed",
		TaskID:  "plan-run",
		Summary: "Local planner produced spec, architecture note, and task graph.",
		Data: map[string]any{
			"worker_input":       workerInputPath,
			"worker_result":      workerResultPath,
			"spec":               specPath,
			"architecture":       architecturePath,
			"task_graph_version": graph.Version,
			"task_count":         len(graph.Tasks),
			"planner":            opts.Planner,
			"planner_model":      opts.PlannerModel,
			"model_response_id":  modelResponseID,
		},
	}); err != nil {
		return err
	}
	if err := store.AppendEvent(run.RunID, Event{
		Type:    "artifact_created",
		Summary: "Task graph v1 created.",
		Data: map[string]any{
			"task_graph": run.TaskGraphPath,
			"current":    filepath.Join(run.RunDir, "task-graph.current.yaml"),
		},
	}); err != nil {
		return err
	}

	if opts.AutoApprove || !run.PlanApprovalRequired {
		run.PlanApproved = true
		if err := TransitionRun(store, run, StateExecuting, "Plan generated and approved; run is ready for execution."); err != nil {
			return err
		}
		return store.AppendEvent(run.RunID, Event{
			Type:    "human_approval_resolved",
			Summary: "Plan approval skipped by policy.",
			Data: map[string]any{
				"approved": true,
				"policy":   "auto_approve_plan",
			},
		})
	}

	return TransitionRun(store, run, StateAwaitingPlan, "Plan generated; awaiting Captain approval before execution.")
}

func ApprovePlan(store *Store, run *Run) error {
	if run.TaskGraphVersion == "" {
		return fmt.Errorf("run %s does not have a task graph to approve", run.RunID)
	}
	if run.State != StateAwaitingPlan {
		return fmt.Errorf("run %s is in state %q, not %q", run.RunID, run.State, StateAwaitingPlan)
	}
	run.PlanApproved = true
	if err := TransitionRun(store, run, StateExecuting, "Plan approved; run is ready for execution."); err != nil {
		return err
	}
	return store.AppendEvent(run.RunID, Event{
		Type:    "human_approval_resolved",
		Summary: "Plan approved by Captain.",
		Data: map[string]any{
			"approved": true,
		},
	})
}

func buildDefaultTaskGraph(run *Run, specPath, architecturePath string, repoFiles []string) TaskGraph {
	specRel := relToRun(run, specPath)
	archRel := relToRun(run, architecturePath)
	commonInputs := TaskInputs{
		Artifacts: []string{specRel, archRel},
		RepoPaths: repoFiles,
	}
	return TaskGraph{
		Version:     "v1",
		RunID:       run.RunID,
		GeneratedAt: time.Now().UTC(),
		Tasks: []Task{
			{
				ID:        "inspect-repository",
				RunID:     run.RunID,
				Type:      "research",
				Title:     "Inspect repository shape",
				Objective: "Inspect the repository structure and identify files relevant to the order.",
				Inputs:    TaskInputs{Artifacts: []string{specRel}, RepoPaths: repoFiles},
				Outputs:   TaskOutputs{ExpectedArtifacts: []string{"artifacts/context/repository-inspection.md"}, ExpectedEvidence: []string{"repository_summary"}},
				Acceptance: TaskAcceptance{Required: []string{
					"repository structure summarized",
					"likely implementation areas identified",
					"unknowns and risks listed",
				}},
				ModelPolicy: ModelPolicy{Primary: "gpt_frontier", Fallback: []string{"claude_code"}},
				ToolGrants:  readOnlyToolGrants(),
				Review:      TaskReview{Deterministic: []string{}, Agentic: []string{"self_review"}},
				RetryPolicy: defaultRetryPolicy(),
				Risk:        TaskRisk{Level: "low", Reasons: []string{"read-only repository inspection"}},
				Status:      "pending",
			},
			{
				ID:           "implement-goal",
				RunID:        run.RunID,
				Type:         "implementation",
				Title:        "Implement requested change",
				Objective:    "Modify the repository to satisfy the approved spec for the order.",
				Dependencies: []string{"inspect-repository"},
				Inputs:       commonInputs,
				Outputs: TaskOutputs{
					ExpectedArtifacts: []string{"source changes", "tests"},
					ExpectedEvidence:  []string{"changes.patch", "test_log"},
				},
				Acceptance: TaskAcceptance{Required: []string{
					"implementation satisfies spec",
					"changes are scoped to the task",
					"tests are added or an explicit test gap is recorded",
					"no production credentials required",
				}},
				ModelPolicy: ModelPolicy{Primary: "claude_code", Fallback: []string{"gpt_frontier"}},
				ToolGrants:  implementationToolGrants(),
				Review: TaskReview{
					Deterministic: []string{"build", "unit_tests", "integration_tests", "formatting"},
					Agentic:       []string{"independent_review"},
				},
				RetryPolicy: defaultRetryPolicy(),
				Risk:        TaskRisk{Level: "medium", Reasons: []string{"modifies repository source files"}},
				Status:      "pending",
			},
			{
				ID:           "run-deterministic-checks",
				RunID:        run.RunID,
				Type:         "deterministic_check",
				Title:        "Run deterministic checks",
				Objective:    "Run the local build and test commands that can verify the implementation.",
				Dependencies: []string{"implement-goal"},
				Inputs:       commonInputs,
				Outputs:      TaskOutputs{ExpectedEvidence: []string{"build_log", "test_log", "format_log"}},
				Acceptance: TaskAcceptance{Required: []string{
					"build result recorded",
					"unit test result recorded",
					"formatting result recorded",
				}, Optional: []string{"integration test result recorded when runnable"}},
				ModelPolicy: ModelPolicy{Primary: "local_shell", Fallback: []string{}},
				ToolGrants:  checkToolGrants(),
				Review:      TaskReview{Deterministic: []string{"build", "unit_tests", "formatting"}, Agentic: []string{}},
				RetryPolicy: defaultRetryPolicy(),
				Risk:        TaskRisk{Level: "low", Reasons: []string{"runs local deterministic commands"}},
				Status:      "pending",
			},
			{
				ID:           "independent-review",
				RunID:        run.RunID,
				Type:         "review",
				Title:        "Review implementation",
				Objective:    "Review the implementation diff, test evidence, and residual risks before acceptance.",
				Dependencies: []string{"run-deterministic-checks"},
				Inputs:       commonInputs,
				Outputs:      TaskOutputs{ExpectedArtifacts: []string{"artifacts/reviews/independent-review.md"}, ExpectedEvidence: []string{"review_findings"}},
				Acceptance: TaskAcceptance{Required: []string{
					"acceptance criteria evaluated",
					"blocking findings identified",
					"residual risks listed",
				}},
				ModelPolicy: ModelPolicy{Primary: "gpt_frontier", Fallback: []string{"opus"}},
				ToolGrants:  readOnlyToolGrants(),
				Review:      TaskReview{Agentic: []string{"self_review"}},
				RetryPolicy: defaultRetryPolicy(),
				Risk:        TaskRisk{Level: "medium", Reasons: []string{"determines whether implementation can be accepted"}},
				Status:      "pending",
			},
			{
				ID:           "final-delivery-report",
				RunID:        run.RunID,
				Type:         "documentation",
				Title:        "Write final delivery report",
				Objective:    "Summarize what changed, what passed, what failed, repairs, decisions, and residual risks.",
				Dependencies: []string{"independent-review"},
				Inputs:       commonInputs,
				Outputs:      TaskOutputs{ExpectedArtifacts: []string{"artifacts/reports/final-delivery-report.md"}},
				Acceptance: TaskAcceptance{Required: []string{
					"original order summarized",
					"changes and evidence listed",
					"residual risks and follow-ups listed",
				}},
				ModelPolicy: ModelPolicy{Primary: "gpt_frontier", Fallback: []string{"haiku"}},
				ToolGrants:  readOnlyToolGrants(),
				Review:      TaskReview{Agentic: []string{"final_report_review"}},
				RetryPolicy: defaultRetryPolicy(),
				Risk:        TaskRisk{Level: "low", Reasons: []string{"documentation-only task"}},
				Status:      "pending",
			},
		},
	}
}

func renderPlannerInput(run *Run, repoFiles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "worker_input:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(run.RunID))
	fmt.Fprintf(&b, "  task_id: %s\n", yamlQuote("plan-run"))
	fmt.Fprintf(&b, "  role: %s\n", yamlQuote("planner"))
	fmt.Fprintf(&b, "  objective: %s\n", yamlQuote("Generate the MVP spec, architecture note, and task graph for the order."))
	fmt.Fprintf(&b, "  order: %s\n", yamlQuote(run.Goal))
	fmt.Fprintf(&b, "  workspace: %s\n", yamlQuote(run.WorkspaceDir))
	writeStringList(&b, "  repo_files", repoFiles)
	return b.String()
}

func generatePlanningDocuments(run *Run, repoFiles []string, opts PlanOptions) (string, string, string, error) {
	if opts.Planner != "openai" {
		return renderSpec(run, repoFiles), renderArchitecture(run), "", nil
	}
	client, err := NewOpenAIClient(opts.PlannerModel)
	if err != nil {
		return "", "", "", err
	}
	input := renderOpenAIPlannerPrompt(run, repoFiles)
	output, responseID, err := client.CreateText(openAIPlannerInstructions(), input)
	if err != nil {
		return "", "", responseID, err
	}
	spec, arch := splitOpenAIPlan(output)
	return spec, arch, responseID, nil
}

func renderPlannerResult(run *Run, specPath, architecturePath string, graph TaskGraph, opts PlanOptions, modelResponseID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "worker_result:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(run.RunID))
	fmt.Fprintf(&b, "  task_id: %s\n", yamlQuote("plan-run"))
	fmt.Fprintf(&b, "  role: %s\n", yamlQuote("planner"))
	fmt.Fprintf(&b, "  status: %s\n", yamlQuote("completed"))
	fmt.Fprintf(&b, "  summary: %s\n", yamlQuote("Generated MVP planning artifacts."))
	fmt.Fprintf(&b, "  planner: %s\n", yamlQuote(opts.Planner))
	if opts.PlannerModel != "" {
		fmt.Fprintf(&b, "  planner_model: %s\n", yamlQuote(opts.PlannerModel))
	}
	if modelResponseID != "" {
		fmt.Fprintf(&b, "  model_response_id: %s\n", yamlQuote(modelResponseID))
	}
	b.WriteString("  artifacts:\n")
	fmt.Fprintf(&b, "    spec: %s\n", yamlQuote(relToRun(run, specPath)))
	fmt.Fprintf(&b, "    architecture: %s\n", yamlQuote(relToRun(run, architecturePath)))
	fmt.Fprintf(&b, "    task_graph: %s\n", yamlQuote("task-graph.v1.yaml"))
	fmt.Fprintf(&b, "  task_count: %d\n", len(graph.Tasks))
	return b.String()
}

func openAIPlannerInstructions() string {
	return strings.Join([]string{
		"You are the Ship(it) planner.",
		"Produce practical planning artifacts for a local software-delivery control loop.",
		"Do not claim implementation has happened.",
		"Return Markdown with exactly two top-level sections:",
		"# MVP Run Spec",
		"# MVP Run Architecture Note",
		"Keep the output concise, specific to the repository files and user order, and implementation-ready.",
	}, "\n")
}

func renderOpenAIPlannerPrompt(run *Run, repoFiles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run ID: %s\n", run.RunID)
	fmt.Fprintf(&b, "Order: %s\n", run.Goal)
	fmt.Fprintf(&b, "Mode: %s\n", run.Mode)
	fmt.Fprintf(&b, "Dirty state strategy: %s\n", run.DirtyStateStrategy)
	fmt.Fprintf(&b, "Integration test policy: %s\n\n", run.IntegrationTestsBlocking)
	b.WriteString("Repository file sample:\n")
	for _, file := range repoFiles {
		fmt.Fprintf(&b, "- %s\n", file)
	}
	b.WriteString("\nTask graph shape will be generated by Ship(it); focus on the run spec and architecture note.\n")
	return b.String()
}

func splitOpenAIPlan(output string) (string, string) {
	output = strings.TrimSpace(output)
	const archHeader = "# MVP Run Architecture Note"
	idx := strings.Index(output, archHeader)
	if idx < 0 {
		return ensureHeading(output, "# MVP Run Spec"), "# MVP Run Architecture Note\n\nOpenAI planner did not provide a separate architecture section.\n"
	}
	spec := strings.TrimSpace(output[:idx])
	arch := strings.TrimSpace(output[idx:])
	return ensureHeading(spec, "# MVP Run Spec"), ensureHeading(arch, archHeader)
}

func ensureHeading(text, heading string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, heading) {
		return text + "\n"
	}
	return heading + "\n\n" + text + "\n"
}

func renderSpec(run *Run, repoFiles []string) string {
	var b strings.Builder
	b.WriteString("# MVP Run Spec\n\n")
	fmt.Fprintf(&b, "**Run:** `%s`\n\n", run.RunID)
	fmt.Fprintf(&b, "**Order:** %s\n\n", run.Goal)
	b.WriteString("## Objective\n\n")
	b.WriteString("Implement the requested engineering change in an isolated workspace, preserve evidence, run deterministic checks, and stop for review before shipping.\n\n")
	b.WriteString("## Initial Repository Context\n\n")
	if len(repoFiles) == 0 {
		b.WriteString("No tracked repository files were found in the initial context sample.\n\n")
	} else {
		for _, file := range repoFiles {
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Acceptance\n\n")
	b.WriteString("- The implementation satisfies the order and approved task graph.\n")
	b.WriteString("- Local deterministic checks are run and recorded.\n")
	b.WriteString("- Independent review evaluates the diff and evidence.\n")
	b.WriteString("- Residual risks and follow-up work are documented.\n")
	return b.String()
}

func renderArchitecture(run *Run) string {
	var b strings.Builder
	b.WriteString("# MVP Run Architecture Note\n\n")
	fmt.Fprintf(&b, "**Run:** `%s`\n\n", run.RunID)
	b.WriteString("The first implementation loop uses the filesystem-backed run directory as the durable control plane state. ")
	b.WriteString("Workers modify only the isolated Git worktree. The orchestrator owns task state, patch capture, checkpoint commits, and final promotion.\n\n")
	b.WriteString("## Execution Shape\n\n")
	b.WriteString("1. Inspect repository context.\n")
	b.WriteString("2. Implement the requested change.\n")
	b.WriteString("3. Run deterministic checks.\n")
	b.WriteString("4. Review the diff and evidence.\n")
	b.WriteString("5. Produce the final delivery report.\n")
	return b.String()
}

func listRepoFiles(root string, limit int) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".shipit", "node_modules", "vendor", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

func relToRun(run *Run, path string) string {
	rel, err := filepath.Rel(run.RunDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func readOnlyToolGrants() ToolGrants {
	return ToolGrants{
		Git:        GitGrant{WorkerPermissions: []string{"read", "diff"}},
		Filesystem: PermissionList{Permissions: []string{"read"}},
		Shell:      PermissionList{Permissions: []string{"inspect"}},
	}
}

func implementationToolGrants() ToolGrants {
	return ToolGrants{
		Git: GitGrant{
			WorkerPermissions:       []string{"read", "diff"},
			OrchestratorPermissions: []string{"branch", "checkpoint_commit", "rollback_branch"},
		},
		Filesystem: PermissionList{Permissions: []string{"read", "write_workspace"}},
		Shell:      PermissionList{Permissions: []string{"build", "test", "format"}},
	}
}

func checkToolGrants() ToolGrants {
	return ToolGrants{
		Git:        GitGrant{WorkerPermissions: []string{"read", "diff"}},
		Filesystem: PermissionList{Permissions: []string{"read"}},
		Shell:      PermissionList{Permissions: []string{"build", "test", "format"}},
	}
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Strategies: []string{
			"revise_prompt_with_feedback",
			"reduce_scope",
			"escalate_model",
		},
	}
}
