package shipit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ExecuteOptions struct {
	MaxTasks int
}

type StepResult struct {
	TaskID string
	Status string
	Done   bool
}

type GateResult struct {
	RunID       string    `json:"run_id"`
	TaskID      string    `json:"task_id"`
	Gate        string    `json:"gate"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Blocking    bool      `json:"blocking"`
	Severity    string    `json:"severity,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`
	Summary     string    `json:"summary"`
	RequiredFix string    `json:"required_fix,omitempty"`
	LogPath     string    `json:"log_path,omitempty"`
	ExitCode    int       `json:"exit_code,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func ExecuteNextTask(store *Store, run *Run) (StepResult, error) {
	if run.State != StateExecuting {
		return StepResult{}, fmt.Errorf("run %s is in state %q, not %q", run.RunID, run.State, StateExecuting)
	}
	graph, err := store.LoadTaskGraph(run)
	if err != nil {
		return StepResult{}, err
	}
	task, ok := FindNextReadyTask(*graph)
	if !ok {
		if AllTasksAccepted(*graph) {
			if err := FinalizeRun(store, run, StateCompleted); err != nil {
				return StepResult{}, err
			}
			return StepResult{Done: true, Status: StateCompleted}, nil
		}
		return StepResult{}, errors.New("no ready task found")
	}

	taskDir := filepath.Join(run.RunDir, "artifacts", "tasks", task.ID)
	if err := os.MkdirAll(filepath.Join(taskDir, "logs"), 0755); err != nil {
		return StepResult{}, err
	}
	if err := store.SetTaskStatus(run, task.ID, TaskReady, "Task is ready for execution."); err != nil {
		return StepResult{}, err
	}
	if err := store.SetTaskStatus(run, task.ID, TaskRunning, "Task execution started."); err != nil {
		return StepResult{}, err
	}
	if err := store.AppendEvent(run.RunID, Event{Type: "worker_started", TaskID: task.ID, Summary: "Local worker started."}); err != nil {
		return StepResult{}, err
	}

	contextPath, err := writeContextPackage(run, *task, taskDir)
	if err != nil {
		return StepResult{}, err
	}
	workerInputPath := filepath.Join(taskDir, "worker-input.yaml")
	if err := os.WriteFile(workerInputPath, []byte(renderWorkerInput(run, *task, contextPath)), 0644); err != nil {
		return StepResult{}, err
	}

	beforeDiff, err := fullWorkspaceDiff(run.WorkspaceDir)
	if err != nil {
		return StepResult{}, err
	}
	workerSummary, workerErr := runLocalWorker(run, *task, taskDir)
	modifiedPaths, diffPath, diffErr := captureTaskDiff(run, taskDir, beforeDiff)
	if diffErr != nil {
		return StepResult{}, diffErr
	}
	status := "completed"
	if workerErr != nil {
		status = "failed"
	}
	workerResultPath := filepath.Join(taskDir, "worker-result.yaml")
	if err := os.WriteFile(workerResultPath, []byte(renderWorkerResult(run, *task, status, workerSummary, workerErr, modifiedPaths, diffPath)), 0644); err != nil {
		return StepResult{}, err
	}
	if workerErr != nil {
		_ = store.SetTaskStatus(run, task.ID, TaskFailed, "Task worker failed.")
		_ = store.AppendEvent(run.RunID, Event{Type: "worker_failed", TaskID: task.ID, Summary: workerErr.Error()})
		return StepResult{TaskID: task.ID, Status: TaskFailed}, workerErr
	}
	if err := store.AppendEvent(run.RunID, Event{
		Type:    "worker_completed",
		TaskID:  task.ID,
		Summary: "Local worker completed.",
		Data: map[string]any{
			"worker_input":  workerInputPath,
			"worker_result": workerResultPath,
			"context":       contextPath,
			"patch":         diffPath,
		},
	}); err != nil {
		return StepResult{}, err
	}

	if err := store.SetTaskStatus(run, task.ID, TaskChecking, "Task deterministic checks started."); err != nil {
		return StepResult{}, err
	}
	gates, err := runDeterministicGates(run, *task, taskDir)
	if err != nil {
		return StepResult{}, err
	}
	if hasFailedBlockingGate(gates) {
		if err := persistGateResults(store, run, *task, taskDir, gates); err != nil {
			return StepResult{}, err
		}
		return rejectAndScheduleRepair(store, run, *task, gates, "Task rejected by deterministic gates.")
	}
	if err := store.SetTaskStatus(run, task.ID, TaskReviewing, "Task agentic review started."); err != nil {
		return StepResult{}, err
	}
	agenticGates, err := runAgenticGates(run, *task, taskDir)
	if err != nil {
		return StepResult{}, err
	}
	gates = append(gates, agenticGates...)
	if err := persistGateResults(store, run, *task, taskDir, gates); err != nil {
		return StepResult{}, err
	}
	if hasFailedBlockingGate(gates) {
		return rejectAndScheduleRepair(store, run, *task, gates, "Task rejected by review gates.")
	}
	checkpoint, err := checkpointAcceptedTask(run, *task)
	if err != nil {
		return StepResult{}, err
	}
	if err := store.SetTaskStatus(run, task.ID, TaskAccepted, "Task accepted by serial executor."); err != nil {
		return StepResult{}, err
	}
	if checkpoint != "" {
		if err := store.AppendEvent(run.RunID, Event{
			Type:    "artifact_created",
			TaskID:  task.ID,
			Summary: "Checkpoint commit created.",
			Data: map[string]any{
				"checkpoint_commit": checkpoint,
			},
		}); err != nil {
			return StepResult{}, err
		}
	}
	return StepResult{TaskID: task.ID, Status: TaskAccepted}, nil
}

func ExecuteRun(store *Store, run *Run, opts ExecuteOptions) ([]StepResult, error) {
	limit := opts.MaxTasks
	if limit <= 0 {
		limit = 1
	}
	var results []StepResult
	for i := 0; i < limit; i++ {
		result, err := ExecuteNextTask(store, run)
		if err != nil {
			return results, err
		}
		results = append(results, result)
		if result.Done || result.Status == TaskRejected || result.Status == TaskFailed || run.State != StateExecuting {
			break
		}
	}
	return results, nil
}

func writeContextPackage(run *Run, task Task, taskDir string) (string, error) {
	path := filepath.Join(taskDir, "context-package.yaml")
	var b strings.Builder
	fmt.Fprintf(&b, "context_package:\n")
	fmt.Fprintf(&b, "  id: %s\n", yamlQuote("ctx_"+task.ID+"_v1"))
	fmt.Fprintf(&b, "  task_id: %s\n", yamlQuote(task.ID))
	fmt.Fprintf(&b, "  created_at: %s\n", yamlQuote(time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("  sources:\n")
	for _, artifact := range task.Inputs.Artifacts {
		fmt.Fprintf(&b, "    - type: %s\n", yamlQuote("artifact"))
		fmt.Fprintf(&b, "      path: %s\n", yamlQuote(artifact))
	}
	for _, repoPath := range task.Inputs.RepoPaths {
		fmt.Fprintf(&b, "    - type: %s\n", yamlQuote("repo_path"))
		fmt.Fprintf(&b, "      path: %s\n", yamlQuote(repoPath))
	}
	fmt.Fprintf(&b, "  workspace: %s\n", yamlQuote(run.WorkspaceDir))
	return path, os.WriteFile(path, []byte(b.String()), 0644)
}

func runLocalWorker(run *Run, task Task, taskDir string) (string, error) {
	switch task.Type {
	case "research":
		out := filepath.Join(run.RunDir, "artifacts", "context", "repository-inspection.md")
		files, err := listRepoFiles(run.WorkspaceDir, 48)
		if err != nil {
			return "", err
		}
		return "Repository inspection artifact created.", os.WriteFile(out, []byte(renderRepositoryInspection(task, files)), 0644)
	case "implementation":
		out := filepath.Join(run.WorkspaceDir, "SHIPIT_STUB_IMPLEMENTATION.md")
		return "Stub implementation marker written to workspace.", os.WriteFile(out, []byte(renderStubImplementation(run, task)), 0644)
	case "repair":
		return runLocalRepairWorker(run, task)
	case "deterministic_check":
		return "Deterministic check task prepared; gate runner owns command execution.", nil
	case "review":
		out := filepath.Join(run.RunDir, "artifacts", "reviews", "independent-review.md")
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return "", err
		}
		return "Stub independent review artifact created.", os.WriteFile(out, []byte(renderStubReview(run, task)), 0644)
	case "documentation":
		out := filepath.Join(run.RunDir, "artifacts", "reports", "final-delivery-report.md")
		return "Stub final delivery report created.", os.WriteFile(out, []byte(renderStubFinalReport(run)), 0644)
	default:
		return "", fmt.Errorf("no local worker implemented for task type %q", task.Type)
	}
}

func runLocalRepairWorker(run *Run, task Task) (string, error) {
	target := filepath.Join(run.WorkspaceDir, "SHIPIT_STUB_IMPLEMENTATION.md")
	data, err := os.ReadFile(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil {
		repaired := strings.ReplaceAll(string(data), "SHIPIT_FORCE_REPAIR\n", "")
		repaired = strings.ReplaceAll(repaired, "SHIPIT_FORCE_REPAIR", "")
		if err := os.WriteFile(target, []byte(repaired), 0644); err != nil {
			return "", err
		}
	} else {
		if err := os.WriteFile(target, []byte(renderStubRepairImplementation(run, task)), 0644); err != nil {
			return "", err
		}
	}
	note := filepath.Join(run.RunDir, "artifacts", "tasks", task.ID, "repair-note.md")
	return "Repair worker applied fix-in-place strategy.", os.WriteFile(note, []byte("# Repair Note\n\nApplied fix-in-place repair strategy.\n"), 0644)
}

func captureTaskDiff(run *Run, taskDir, beforeDiff string) ([]string, string, error) {
	diff, err := fullWorkspaceDiff(run.WorkspaceDir)
	if err != nil {
		return nil, "", err
	}
	diffPath := filepath.Join(taskDir, "changes.patch")
	if diff == beforeDiff {
		if err := os.WriteFile(diffPath, []byte("# No workspace diff produced by this task.\n"), 0644); err != nil {
			return nil, "", err
		}
		return nil, diffPath, nil
	}
	if diff == "" {
		diff = "# Workspace diff cleared by this task.\n"
	}
	if err := os.WriteFile(diffPath, []byte(diff), 0644); err != nil {
		return nil, "", err
	}
	paths, err := GitChangedPaths(run.WorkspaceDir)
	if err != nil {
		return nil, "", err
	}
	return paths, diffPath, nil
}

func fullWorkspaceDiff(workspace string) (string, error) {
	diff, err := GitDiff(workspace)
	if err != nil {
		return "", err
	}
	untracked, err := GitUntrackedPaths(workspace)
	if err != nil {
		return "", err
	}
	if len(untracked) > 0 {
		diff += renderUntrackedDiff(workspace, untracked)
	}
	return diff, nil
}

func renderUntrackedDiff(workspace string, paths []string) string {
	var b strings.Builder
	for _, rel := range paths {
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", rel, rel)
		fmt.Fprintf(&b, "new file mode 100644\n")
		fmt.Fprintf(&b, "--- /dev/null\n")
		fmt.Fprintf(&b, "+++ b/%s\n", rel)
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				b.WriteString("+\n")
				continue
			}
			fmt.Fprintf(&b, "+%s\n", line)
		}
	}
	return b.String()
}

func runDeterministicGates(run *Run, task Task, taskDir string) ([]GateResult, error) {
	now := time.Now().UTC()
	if len(task.Review.Deterministic) == 0 {
		return []GateResult{{
			RunID:     run.RunID,
			TaskID:    task.ID,
			Gate:      "none",
			Type:      "deterministic",
			Status:    "skipped",
			Blocking:  false,
			Summary:   "Task has no deterministic gates.",
			CreatedAt: now,
		}}, nil
	}
	var results []GateResult
	for _, gate := range task.Review.Deterministic {
		result := GateResult{RunID: run.RunID, TaskID: task.ID, Gate: gate, Type: "deterministic", Blocking: true, CreatedAt: now}
		switch gate {
		case "build", "unit_tests":
			if fileExists(filepath.Join(run.WorkspaceDir, "go.mod")) {
				logPath := filepath.Join(taskDir, "logs", gate+".log")
				exitCode, err := runCommandToLog(run.WorkspaceDir, logPath, "go", "test", "./...")
				result.LogPath = logPath
				result.ExitCode = exitCode
				if err != nil {
					result.Status = "failed"
					result.Summary = err.Error()
				} else {
					result.Status = "passed"
					result.Summary = "go test ./... passed."
				}
			} else {
				result.Status = "skipped"
				result.Blocking = false
				result.Summary = "No go.mod found; no default local command for this gate yet."
			}
		case "integration_tests":
			result = runIntegrationGate(run, taskDir, result)
		case "formatting":
			result.Status = "skipped"
			result.Blocking = false
			result.Summary = "No formatter discovery implemented in serial executor slice."
		default:
			result.Status = "skipped"
			result.Blocking = false
			result.Summary = "Gate runner does not know how to execute this gate yet."
		}
		results = append(results, result)
	}
	return results, nil
}

func runAgenticGates(run *Run, task Task, taskDir string) ([]GateResult, error) {
	if len(task.Review.Agentic) == 0 {
		return nil, nil
	}
	patch, _ := os.ReadFile(filepath.Join(taskDir, "changes.patch"))
	var results []GateResult
	for _, gate := range task.Review.Agentic {
		result := GateResult{
			RunID:      run.RunID,
			TaskID:     task.ID,
			Gate:       gate,
			Type:       "agentic",
			Status:     "passed",
			Blocking:   isBlockingAgenticGate(gate),
			Severity:   "none",
			Confidence: 0.82,
			Summary:    "Local review stub found no blocking issues.",
			CreatedAt:  time.Now().UTC(),
		}
		if strings.Contains(string(patch), "SHIPIT_FORCE_REPAIR") {
			result.Status = "failed"
			result.Blocking = true
			result.Severity = "high"
			result.Confidence = 0.91
			result.Summary = "Patch contains forced repair marker."
			result.RequiredFix = "Remove SHIPIT_FORCE_REPAIR marker and preserve useful implementation work."
		}
		results = append(results, result)
	}
	return results, nil
}

func isBlockingAgenticGate(gate string) bool {
	switch gate {
	case "independent_review", "security_review", "final_report_review":
		return true
	default:
		return false
	}
}

func persistGateResults(store *Store, run *Run, task Task, taskDir string, gates []GateResult) error {
	gatePath := filepath.Join(taskDir, "gate-results.json")
	if err := writeJSON(gatePath, gates); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(taskDir, "gate-results.yaml"), []byte(renderGateResultsYAML(gates)), 0644); err != nil {
		return err
	}
	return store.AppendEvent(run.RunID, Event{
		Type:    "gate_completed",
		TaskID:  task.ID,
		Summary: "Task gates completed.",
		Data: map[string]any{
			"gate_results": gatePath,
			"count":        len(gates),
		},
	})
}

func runCommandToLog(workdir, logPath, name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if writeErr := os.WriteFile(logPath, out.Bytes(), 0644); writeErr != nil {
		return -1, writeErr
	}
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), fmt.Errorf("%s %s exited %d", name, strings.Join(args, " "), exitErr.ExitCode())
	}
	return -1, err
}

func hasFailedBlockingGate(gates []GateResult) bool {
	for _, gate := range gates {
		if gate.Blocking && gate.Status == "failed" {
			return true
		}
	}
	return false
}

func rejectAndScheduleRepair(store *Store, run *Run, task Task, gates []GateResult, summary string) (StepResult, error) {
	if err := store.SetTaskStatus(run, task.ID, TaskRejected, summary); err != nil {
		return StepResult{}, err
	}
	if task.Type == "implementation" || task.Type == "repair" {
		if shouldBlockForCaptainReview(run, task) {
			if err := store.AppendEvent(run.RunID, Event{
				Type:    "human_approval_requested",
				TaskID:  task.ID,
				Summary: "Failure loop threshold reached; Captain review required before more repair attempts.",
				Data: map[string]any{
					"threshold": run.FailureLoopCaptainReviewThreshold,
				},
			}); err != nil {
				return StepResult{}, err
			}
			if err := TransitionRun(store, run, StateBlocked, "Failure loop threshold reached; awaiting Captain review."); err != nil {
				return StepResult{}, err
			}
			return StepResult{TaskID: task.ID, Status: TaskBlocked}, nil
		}
		if err := TransitionRun(store, run, StateRepairing, "Repair task generation started."); err != nil {
			return StepResult{}, err
		}
		strategy := run.DirtyStateStrategy
		if strategy == "" {
			strategy = "fix_in_place"
		}
		if strategy == "orchestrator_rollback" {
			if err := GitResetHard(run.WorkspaceDir, "HEAD"); err != nil {
				return StepResult{}, err
			}
			if err := store.AppendEvent(run.RunID, Event{
				Type:    "decision_recorded",
				TaskID:  task.ID,
				Summary: "Orchestrator rolled workspace back before repair.",
				Data: map[string]any{
					"dirty_state_strategy": strategy,
					"rollback_ref":         "HEAD",
				},
			}); err != nil {
				return StepResult{}, err
			}
		}
		repair, err := store.AppendRepairTask(run, task, gates)
		if err != nil {
			return StepResult{}, err
		}
		if err := store.AppendEvent(run.RunID, Event{
			Type:    "retry_scheduled",
			TaskID:  task.ID,
			Summary: "Repair task scheduled.",
			Data: map[string]any{
				"repair_task_id":       repair.ID,
				"dirty_state_strategy": strategy,
			},
		}); err != nil {
			return StepResult{}, err
		}
		if err := TransitionRun(store, run, StateExecuting, "Repair task generated; run returned to execution."); err != nil {
			return StepResult{}, err
		}
	}
	return StepResult{TaskID: task.ID, Status: TaskRejected}, nil
}

func runIntegrationGate(run *Run, taskDir string, result GateResult) GateResult {
	policy := run.IntegrationTestsBlocking
	if policy == "" {
		policy = "when_runnable"
	}
	runnable := hasRunnableIntegrationTests(run.WorkspaceDir)
	if policy == "never" {
		result.Status = "skipped"
		result.Blocking = false
		result.Summary = "Integration tests skipped by policy."
		return result
	}
	if !runnable {
		result.Status = "skipped"
		result.Blocking = policy == "always"
		if result.Blocking {
			result.Status = "failed"
			result.Summary = "Integration tests are required by policy, but no runnable integration test path was discovered."
		} else {
			result.Summary = "No runnable integration test path discovered."
		}
		return result
	}
	if fileExists(filepath.Join(run.WorkspaceDir, "go.mod")) {
		logPath := filepath.Join(taskDir, "logs", "integration_tests.log")
		exitCode, err := runCommandToLog(run.WorkspaceDir, logPath, "go", "test", "./...")
		result.LogPath = logPath
		result.ExitCode = exitCode
		if err != nil {
			result.Status = "failed"
			result.Summary = err.Error()
		} else {
			result.Status = "passed"
			result.Summary = "Runnable integration test path discovered; go test ./... passed."
		}
		return result
	}
	result.Status = "skipped"
	result.Blocking = policy == "always"
	result.Summary = "Runnable integration markers exist, but no executor command is available for this repository type."
	if result.Blocking {
		result.Status = "failed"
	}
	return result
}

func hasRunnableIntegrationTests(workspace string) bool {
	candidates := []string{
		filepath.Join(workspace, "tests", "integration"),
		filepath.Join(workspace, "test", "integration"),
		filepath.Join(workspace, "integration"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return true
		}
	}
	found := false
	_ = filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".shipit", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.Contains(name, "integration") && strings.HasSuffix(name, "_test.go") {
			found = true
		}
		return nil
	})
	return found
}

func shouldBlockForCaptainReview(run *Run, task Task) bool {
	threshold := run.FailureLoopCaptainReviewThreshold
	if threshold <= 0 {
		threshold = 2
	}
	if task.Type != "repair" {
		return false
	}
	return repairDepth(task.ID) >= threshold
}

func repairDepth(taskID string) int {
	return strings.Count(taskID, "repair-")
}

func checkpointAcceptedTask(run *Run, task Task) (string, error) {
	if task.Type != "implementation" && task.Type != "repair" {
		return "", nil
	}
	return GitCommitAll(run.WorkspaceDir, fmt.Sprintf("shipit: %s", task.ID))
}

func renderWorkerInput(run *Run, task Task, contextPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "worker_input:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(run.RunID))
	fmt.Fprintf(&b, "  task_id: %s\n", yamlQuote(task.ID))
	fmt.Fprintf(&b, "  role: %s\n", yamlQuote(task.Type))
	fmt.Fprintf(&b, "  objective: %s\n", yamlQuote(task.Objective))
	fmt.Fprintf(&b, "  context_package: %s\n", yamlQuote(relToRun(run, contextPath)))
	fmt.Fprintf(&b, "  workspace: %s\n", yamlQuote(run.WorkspaceDir))
	return b.String()
}

func renderWorkerResult(run *Run, task Task, status, summary string, workerErr error, modifiedPaths []string, diffPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "worker_result:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(run.RunID))
	fmt.Fprintf(&b, "  task_id: %s\n", yamlQuote(task.ID))
	fmt.Fprintf(&b, "  role: %s\n", yamlQuote(task.Type))
	fmt.Fprintf(&b, "  status: %s\n", yamlQuote(status))
	fmt.Fprintf(&b, "  summary: %s\n", yamlQuote(summary))
	if workerErr != nil {
		fmt.Fprintf(&b, "  failure: %s\n", yamlQuote(workerErr.Error()))
	}
	b.WriteString("  artifacts:\n")
	writeStringList(&b, "    modified_paths", modifiedPaths)
	fmt.Fprintf(&b, "    patch_file: %s\n", yamlQuote(relToRun(run, diffPath)))
	return b.String()
}

func renderGateResultsYAML(gates []GateResult) string {
	var b strings.Builder
	b.WriteString("gate_results:\n")
	for _, gate := range gates {
		fmt.Fprintf(&b, "  - run_id: %s\n", yamlQuote(gate.RunID))
		fmt.Fprintf(&b, "    task_id: %s\n", yamlQuote(gate.TaskID))
		fmt.Fprintf(&b, "    gate: %s\n", yamlQuote(gate.Gate))
		fmt.Fprintf(&b, "    type: %s\n", yamlQuote(gate.Type))
		fmt.Fprintf(&b, "    status: %s\n", yamlQuote(gate.Status))
		fmt.Fprintf(&b, "    blocking: %t\n", gate.Blocking)
		if gate.Severity != "" {
			fmt.Fprintf(&b, "    severity: %s\n", yamlQuote(gate.Severity))
		}
		if gate.Confidence != 0 {
			fmt.Fprintf(&b, "    confidence: %.2f\n", gate.Confidence)
		}
		fmt.Fprintf(&b, "    summary: %s\n", yamlQuote(gate.Summary))
		if gate.RequiredFix != "" {
			fmt.Fprintf(&b, "    required_fix: %s\n", yamlQuote(gate.RequiredFix))
		}
		if gate.LogPath != "" {
			fmt.Fprintf(&b, "    log_path: %s\n", yamlQuote(gate.LogPath))
		}
		fmt.Fprintf(&b, "    exit_code: %d\n", gate.ExitCode)
		fmt.Fprintf(&b, "    created_at: %s\n", yamlQuote(gate.CreatedAt.Format(time.RFC3339)))
	}
	return b.String()
}

func renderRepositoryInspection(task Task, files []string) string {
	var b strings.Builder
	b.WriteString("# Repository Inspection\n\n")
	fmt.Fprintf(&b, "**Task:** `%s`\n\n", task.ID)
	b.WriteString("## Files Sample\n\n")
	for _, file := range files {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	if len(files) == 0 {
		b.WriteString("- No files discovered.\n")
	}
	b.WriteString("\n## Notes\n\n")
	b.WriteString("This is a local executor stub artifact. Model-backed repository analysis will replace it in a later slice.\n")
	return b.String()
}

func renderStubImplementation(run *Run, task Task) string {
	marker := ""
	if strings.Contains(strings.ToLower(run.Goal), "force repair") {
		marker = "\nSHIPIT_FORCE_REPAIR\n"
	}
	return fmt.Sprintf("# Ship(it) Stub Implementation\n\nRun: `%s`\n\nTask: `%s`\n\nObjective: %s\n%s\nThis file is written by the local serial executor stub so diff capture, worker result recording, and deterministic gate plumbing can be exercised before model-backed implementation is wired.\n", run.RunID, task.ID, task.Objective, marker)
}

func renderStubRepairImplementation(run *Run, task Task) string {
	return fmt.Sprintf("# Ship(it) Stub Repair Implementation\n\nRun: `%s`\n\nTask: `%s`\n\nThis file was recreated by a repair task after the orchestrator rolled back rejected dirty state.\n", run.RunID, task.ID)
}

func renderStubReview(run *Run, task Task) string {
	return fmt.Sprintf("# Stub Independent Review\n\nRun: `%s`\n\nTask: `%s`\n\nNo model-backed review has been wired yet. This artifact proves the serial executor can run a review task and preserve its output.\n", run.RunID, task.ID)
}

func renderStubFinalReport(run *Run) string {
	return fmt.Sprintf("# Final Delivery Report\n\nRun: `%s`\n\nOrder: %s\n\nThis is a local executor stub final report. Review, repair, checkpointing, and real final evidence synthesis come in later slices.\n", run.RunID, run.Goal)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
