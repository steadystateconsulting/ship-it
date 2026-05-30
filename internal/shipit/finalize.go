package shipit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TaskEvaluation struct {
	RunID                string   `json:"run_id"`
	TaskID               string   `json:"task_id"`
	TaskType             string   `json:"task_type"`
	Outcome              string   `json:"outcome"`
	DeterministicPassed  int      `json:"deterministic_passed"`
	DeterministicFailed  int      `json:"deterministic_failed"`
	DeterministicSkipped int      `json:"deterministic_skipped"`
	AgenticPassed        int      `json:"agentic_passed"`
	AgenticFailed        int      `json:"agentic_failed"`
	AgenticSkipped       int      `json:"agentic_skipped"`
	CheckpointCommits    []string `json:"checkpoint_commits,omitempty"`
	PatchArtifact        string   `json:"patch_artifact,omitempty"`
	ResidualRisks        []string `json:"residual_risks,omitempty"`
}

type RunEvaluation struct {
	RunID                  string    `json:"run_id"`
	Outcome                string    `json:"outcome"`
	TotalTasks             int       `json:"total_tasks"`
	AcceptedTasks          int       `json:"accepted_tasks"`
	RejectedTasks          int       `json:"rejected_tasks"`
	FailedTasks            int       `json:"failed_tasks"`
	RepairTasks            int       `json:"repair_tasks"`
	CheckpointCommits      []string  `json:"checkpoint_commits,omitempty"`
	RejectedPatchArtifacts []string  `json:"rejected_patch_artifacts,omitempty"`
	DeterministicPassed    int       `json:"deterministic_passed"`
	DeterministicFailed    int       `json:"deterministic_failed"`
	DeterministicSkipped   int       `json:"deterministic_skipped"`
	AgenticPassed          int       `json:"agentic_passed"`
	AgenticFailed          int       `json:"agentic_failed"`
	AgenticSkipped         int       `json:"agentic_skipped"`
	ResidualRisks          []string  `json:"residual_risks,omitempty"`
	GeneratedAt            time.Time `json:"generated_at"`
}

func FinalizeRun(store *Store, run *Run, outcome string) error {
	if outcome == "" {
		outcome = StateCompleted
	}
	switch outcome {
	case StateCompleted, StateFailed, StateBlocked:
	default:
		return fmt.Errorf("unsupported final outcome %q", outcome)
	}
	graph, err := store.LoadTaskGraph(run)
	if err != nil {
		return err
	}
	taskEvals, runEval, err := BuildEvaluations(store, run, *graph, outcome)
	if err != nil {
		return err
	}
	if err := store.SaveEvaluations(run, taskEvals, runEval); err != nil {
		return err
	}
	report, err := RenderFinalReport(store, run, *graph, taskEvals, runEval)
	if err != nil {
		return err
	}
	reportPath := filepath.Join(run.RunDir, "artifacts", "reports", "final-delivery-report.md")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		return err
	}
	if _, err := GenerateDocumentIndex(store, run); err != nil {
		return err
	}
	if run.State != outcome {
		if err := TransitionRun(store, run, outcome, "Run finalized as "+outcome+"."); err != nil {
			return err
		}
	} else if err := store.SaveRun(run); err != nil {
		return err
	}
	eventType := "run_" + outcome
	if outcome == StateCompleted {
		eventType = "run_completed"
	}
	return store.AppendEvent(run.RunID, Event{
		Type:    eventType,
		Summary: "Final report and evaluations generated.",
		Data: map[string]any{
			"final_report":   reportPath,
			"run_evaluation": filepath.Join(run.RunDir, "evals", "run-evaluation.yaml"),
			"document_index": filepath.Join(run.RunDir, "artifacts", "document-index.yaml"),
		},
	})
}

func BuildEvaluations(store *Store, run *Run, graph TaskGraph, outcome string) ([]TaskEvaluation, RunEvaluation, error) {
	events, err := store.ReadEvents(run.RunID)
	if err != nil {
		return nil, RunEvaluation{}, err
	}
	checkpointsByTask := checkpointCommitsByTask(events)
	var taskEvals []TaskEvaluation
	runEval := RunEvaluation{RunID: run.RunID, Outcome: outcome, TotalTasks: len(graph.Tasks), GeneratedAt: time.Now().UTC()}
	for _, task := range graph.Tasks {
		eval := TaskEvaluation{
			RunID:             run.RunID,
			TaskID:            task.ID,
			TaskType:          task.Type,
			Outcome:           task.Status,
			CheckpointCommits: checkpointsByTask[task.ID],
			PatchArtifact:     filepath.ToSlash(filepath.Join("artifacts", "tasks", task.ID, "changes.patch")),
		}
		gates, err := loadTaskGateResults(run, task.ID)
		if err != nil {
			return nil, RunEvaluation{}, err
		}
		for _, gate := range gates {
			addGateCounts(&eval, &runEval, gate)
			if gate.Status == "failed" && gate.RequiredFix != "" {
				eval.ResidualRisks = append(eval.ResidualRisks, gate.RequiredFix)
			}
		}
		switch task.Status {
		case TaskAccepted:
			runEval.AcceptedTasks++
		case TaskRejected:
			runEval.RejectedTasks++
			runEval.RejectedPatchArtifacts = append(runEval.RejectedPatchArtifacts, eval.PatchArtifact)
		case TaskFailed:
			runEval.FailedTasks++
		}
		if task.Type == "repair" {
			runEval.RepairTasks++
		}
		taskEvals = append(taskEvals, eval)
		runEval.CheckpointCommits = append(runEval.CheckpointCommits, eval.CheckpointCommits...)
		runEval.ResidualRisks = append(runEval.ResidualRisks, eval.ResidualRisks...)
	}
	sort.Strings(runEval.CheckpointCommits)
	sort.Strings(runEval.RejectedPatchArtifacts)
	sort.Strings(runEval.ResidualRisks)
	return taskEvals, runEval, nil
}

func (s *Store) SaveEvaluations(run *Run, tasks []TaskEvaluation, runEval RunEvaluation) error {
	if err := writeJSON(filepath.Join(run.RunDir, "evals", "task-evaluations.json"), tasks); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(run.RunDir, "evals", "task-evaluations.yaml"), []byte(renderTaskEvaluationsYAML(tasks)), 0644); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(run.RunDir, "evals", "run-evaluation.json"), runEval); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(run.RunDir, "evals", "run-evaluation.yaml"), []byte(renderRunEvaluationYAML(runEval)), 0644)
}

func RenderFinalReport(store *Store, run *Run, graph TaskGraph, taskEvals []TaskEvaluation, runEval RunEvaluation) (string, error) {
	events, err := store.ReadEvents(run.RunID)
	if err != nil {
		return "", err
	}
	gitLog, _ := GitLogOneline(run.WorkspaceDir, 20)
	var b strings.Builder
	b.WriteString("# Final Delivery Report\n\n")
	fmt.Fprintf(&b, "**Run:** `%s`\n\n", run.RunID)
	fmt.Fprintf(&b, "**Outcome:** `%s`\n\n", runEval.Outcome)
	fmt.Fprintf(&b, "**Order:** %s\n\n", run.Goal)
	b.WriteString("## What Changed\n\n")
	if len(runEval.CheckpointCommits) == 0 {
		b.WriteString("- No checkpoint commits were created.\n")
	} else {
		for _, commit := range runEval.CheckpointCommits {
			fmt.Fprintf(&b, "- Checkpoint `%s`\n", commit)
		}
	}
	b.WriteString("\n## Task Summary\n\n")
	for _, task := range graph.Tasks {
		fmt.Fprintf(&b, "- `%s` (%s): %s\n", task.ID, task.Type, task.Status)
	}
	b.WriteString("\n## Gate Summary\n\n")
	fmt.Fprintf(&b, "- Deterministic: %d passed, %d failed, %d skipped\n", runEval.DeterministicPassed, runEval.DeterministicFailed, runEval.DeterministicSkipped)
	fmt.Fprintf(&b, "- Agentic: %d passed, %d failed, %d skipped\n", runEval.AgenticPassed, runEval.AgenticFailed, runEval.AgenticSkipped)
	b.WriteString("\n## Repairs\n\n")
	if runEval.RepairTasks == 0 {
		b.WriteString("- No repair tasks were generated.\n")
	} else {
		fmt.Fprintf(&b, "- Repair tasks generated: %d\n", runEval.RepairTasks)
	}
	b.WriteString("\n## Rejected Work\n\n")
	if len(runEval.RejectedPatchArtifacts) == 0 {
		b.WriteString("- No rejected patch artifacts.\n")
	} else {
		for _, patch := range runEval.RejectedPatchArtifacts {
			fmt.Fprintf(&b, "- `%s`\n", patch)
		}
	}
	b.WriteString("\n## Residual Risks\n\n")
	if len(runEval.ResidualRisks) == 0 {
		b.WriteString("- No residual risks recorded by gates.\n")
	} else {
		for _, risk := range runEval.ResidualRisks {
			fmt.Fprintf(&b, "- %s\n", risk)
		}
	}
	b.WriteString("\n## Git Evidence\n\n")
	if len(gitLog) == 0 {
		b.WriteString("- No Git log entries available.\n")
	} else {
		for _, line := range gitLog {
			fmt.Fprintf(&b, "- `%s`\n", line)
		}
	}
	b.WriteString("\n## Artifact Index\n\n")
	fmt.Fprintf(&b, "- Run state: `%s`\n", filepath.ToSlash("run.yaml"))
	fmt.Fprintf(&b, "- Task graph: `%s`\n", filepath.ToSlash(filepath.Base(run.TaskGraphPath)))
	fmt.Fprintf(&b, "- Run evaluation: `%s`\n", filepath.ToSlash("evals/run-evaluation.yaml"))
	fmt.Fprintf(&b, "- Task evaluations: `%s`\n", filepath.ToSlash("evals/task-evaluations.yaml"))
	fmt.Fprintf(&b, "- Logbook: `%s`\n", filepath.ToSlash("logbook/run.jsonl"))
	b.WriteString("\n## Recent Decisions\n\n")
	decisionCount := 0
	for _, event := range events {
		if event.Type != "decision_recorded" && event.Type != "retry_scheduled" && event.Type != "artifact_created" {
			continue
		}
		fmt.Fprintf(&b, "- %s `%s`: %s\n", event.Timestamp.Format(time.RFC3339), event.Type, event.Summary)
		decisionCount++
	}
	if decisionCount == 0 {
		b.WriteString("- No decision events recorded.\n")
	}
	_ = taskEvals
	return b.String(), nil
}

func loadTaskGateResults(run *Run, taskID string) ([]GateResult, error) {
	path := filepath.Join(run.RunDir, "artifacts", "tasks", taskID, "gate-results.json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var gates []GateResult
	if err := readJSON(path, &gates); err != nil {
		return nil, err
	}
	return gates, nil
}

func checkpointCommitsByTask(events []Event) map[string][]string {
	result := map[string][]string{}
	for _, event := range events {
		if event.Type != "artifact_created" || event.TaskID == "" || event.Data == nil {
			continue
		}
		value, ok := event.Data["checkpoint_commit"]
		if !ok {
			continue
		}
		commit, ok := value.(string)
		if !ok || commit == "" {
			continue
		}
		result[event.TaskID] = append(result[event.TaskID], commit)
	}
	return result
}

func addGateCounts(taskEval *TaskEvaluation, runEval *RunEvaluation, gate GateResult) {
	switch gate.Type {
	case "agentic":
		switch gate.Status {
		case "passed":
			taskEval.AgenticPassed++
			runEval.AgenticPassed++
		case "failed":
			taskEval.AgenticFailed++
			runEval.AgenticFailed++
		default:
			taskEval.AgenticSkipped++
			runEval.AgenticSkipped++
		}
	default:
		switch gate.Status {
		case "passed":
			taskEval.DeterministicPassed++
			runEval.DeterministicPassed++
		case "failed":
			taskEval.DeterministicFailed++
			runEval.DeterministicFailed++
		default:
			taskEval.DeterministicSkipped++
			runEval.DeterministicSkipped++
		}
	}
}

func renderTaskEvaluationsYAML(evals []TaskEvaluation) string {
	var b strings.Builder
	b.WriteString("task_evaluations:\n")
	for _, eval := range evals {
		fmt.Fprintf(&b, "  - run_id: %s\n", yamlQuote(eval.RunID))
		fmt.Fprintf(&b, "    task_id: %s\n", yamlQuote(eval.TaskID))
		fmt.Fprintf(&b, "    task_type: %s\n", yamlQuote(eval.TaskType))
		fmt.Fprintf(&b, "    outcome: %s\n", yamlQuote(eval.Outcome))
		fmt.Fprintf(&b, "    deterministic_passed: %d\n", eval.DeterministicPassed)
		fmt.Fprintf(&b, "    deterministic_failed: %d\n", eval.DeterministicFailed)
		fmt.Fprintf(&b, "    deterministic_skipped: %d\n", eval.DeterministicSkipped)
		fmt.Fprintf(&b, "    agentic_passed: %d\n", eval.AgenticPassed)
		fmt.Fprintf(&b, "    agentic_failed: %d\n", eval.AgenticFailed)
		fmt.Fprintf(&b, "    agentic_skipped: %d\n", eval.AgenticSkipped)
		writeStringList(&b, "    checkpoint_commits", eval.CheckpointCommits)
		if eval.PatchArtifact != "" {
			fmt.Fprintf(&b, "    patch_artifact: %s\n", yamlQuote(eval.PatchArtifact))
		}
		writeStringList(&b, "    residual_risks", eval.ResidualRisks)
	}
	return b.String()
}

func renderRunEvaluationYAML(eval RunEvaluation) string {
	var b strings.Builder
	b.WriteString("run_evaluation:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(eval.RunID))
	fmt.Fprintf(&b, "  outcome: %s\n", yamlQuote(eval.Outcome))
	fmt.Fprintf(&b, "  total_tasks: %d\n", eval.TotalTasks)
	fmt.Fprintf(&b, "  accepted_tasks: %d\n", eval.AcceptedTasks)
	fmt.Fprintf(&b, "  rejected_tasks: %d\n", eval.RejectedTasks)
	fmt.Fprintf(&b, "  failed_tasks: %d\n", eval.FailedTasks)
	fmt.Fprintf(&b, "  repair_tasks: %d\n", eval.RepairTasks)
	fmt.Fprintf(&b, "  deterministic_passed: %d\n", eval.DeterministicPassed)
	fmt.Fprintf(&b, "  deterministic_failed: %d\n", eval.DeterministicFailed)
	fmt.Fprintf(&b, "  deterministic_skipped: %d\n", eval.DeterministicSkipped)
	fmt.Fprintf(&b, "  agentic_passed: %d\n", eval.AgenticPassed)
	fmt.Fprintf(&b, "  agentic_failed: %d\n", eval.AgenticFailed)
	fmt.Fprintf(&b, "  agentic_skipped: %d\n", eval.AgenticSkipped)
	writeStringList(&b, "  checkpoint_commits", eval.CheckpointCommits)
	writeStringList(&b, "  rejected_patch_artifacts", eval.RejectedPatchArtifacts)
	writeStringList(&b, "  residual_risks", eval.ResidualRisks)
	fmt.Fprintf(&b, "  generated_at: %s\n", yamlQuote(eval.GeneratedAt.Format(time.RFC3339)))
	return b.String()
}
