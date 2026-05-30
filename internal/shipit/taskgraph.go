package shipit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TaskGraph struct {
	Version     string    `json:"version"`
	RunID       string    `json:"run_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Tasks       []Task    `json:"tasks"`
}

type Task struct {
	ID           string         `json:"id"`
	RunID        string         `json:"run_id"`
	Type         string         `json:"type"`
	Title        string         `json:"title"`
	Objective    string         `json:"objective"`
	Dependencies []string       `json:"dependencies"`
	Inputs       TaskInputs     `json:"inputs"`
	Outputs      TaskOutputs    `json:"outputs"`
	Acceptance   TaskAcceptance `json:"acceptance"`
	ModelPolicy  ModelPolicy    `json:"model_policy"`
	ToolGrants   ToolGrants     `json:"tool_grants"`
	Review       TaskReview     `json:"review"`
	RetryPolicy  RetryPolicy    `json:"retry_policy"`
	Risk         TaskRisk       `json:"risk"`
	Status       string         `json:"status"`
}

type TaskInputs struct {
	Artifacts []string `json:"artifacts,omitempty"`
	RepoPaths []string `json:"repo_paths,omitempty"`
}

type TaskOutputs struct {
	ExpectedArtifacts []string `json:"expected_artifacts,omitempty"`
	ExpectedEvidence  []string `json:"expected_evidence,omitempty"`
}

type TaskAcceptance struct {
	Required []string `json:"required"`
	Optional []string `json:"optional,omitempty"`
}

type ModelPolicy struct {
	Primary  string   `json:"primary"`
	Fallback []string `json:"fallback,omitempty"`
}

type ToolGrants struct {
	Git        GitGrant       `json:"git,omitempty"`
	Filesystem PermissionList `json:"filesystem,omitempty"`
	Shell      PermissionList `json:"shell,omitempty"`
}

type GitGrant struct {
	WorkerPermissions       []string `json:"worker_permissions,omitempty"`
	OrchestratorPermissions []string `json:"orchestrator_permissions,omitempty"`
}

type PermissionList struct {
	Permissions []string `json:"permissions"`
}

type TaskReview struct {
	Deterministic []string `json:"deterministic,omitempty"`
	Agentic       []string `json:"agentic,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts int      `json:"max_attempts"`
	Strategies  []string `json:"strategies"`
}

type TaskRisk struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

func ValidateTaskGraph(graph TaskGraph) error {
	if graph.RunID == "" {
		return errors.New("task graph run_id is required")
	}
	if len(graph.Tasks) == 0 {
		return errors.New("task graph must include at least one task")
	}
	ids := make(map[string]Task, len(graph.Tasks))
	for _, task := range graph.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return errors.New("task id is required")
		}
		if _, exists := ids[task.ID]; exists {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		if strings.TrimSpace(task.Type) == "" {
			return fmt.Errorf("task %q type is required", task.ID)
		}
		if strings.TrimSpace(task.Objective) == "" {
			return fmt.Errorf("task %q objective is required", task.ID)
		}
		if len(task.Acceptance.Required) == 0 {
			return fmt.Errorf("task %q must have at least one required acceptance criterion", task.ID)
		}
		if task.Type == "implementation" && len(task.Review.Deterministic) == 0 {
			return fmt.Errorf("implementation task %q must have at least one deterministic gate", task.ID)
		}
		if task.Type == "implementation" && len(task.Review.Agentic) == 0 {
			return fmt.Errorf("implementation task %q must have independent review", task.ID)
		}
		ids[task.ID] = task
	}
	for _, task := range graph.Tasks {
		for _, dep := range task.Dependencies {
			if _, exists := ids[dep]; !exists {
				return fmt.Errorf("task %q depends on unknown task %q", task.ID, dep)
			}
		}
	}
	if hasCycle(graph.Tasks) {
		return errors.New("task graph must be acyclic")
	}
	return nil
}

func (s *Store) SaveTaskGraph(run *Run, graph TaskGraph) error {
	if err := ValidateTaskGraph(graph); err != nil {
		return err
	}
	if graph.Version == "" {
		graph.Version = "v1"
	}
	graphPathJSON := filepath.Join(run.RunDir, "task-graph."+graph.Version+".json")
	graphPathYAML := filepath.Join(run.RunDir, "task-graph."+graph.Version+".yaml")
	if err := writeJSON(graphPathJSON, graph); err != nil {
		return err
	}
	if err := os.WriteFile(graphPathYAML, []byte(renderTaskGraphYAML(graph)), 0644); err != nil {
		return err
	}
	currentYAML := "current: " + yamlQuote(filepath.Base(graphPathYAML)) + "\n"
	if err := os.WriteFile(filepath.Join(run.RunDir, "task-graph.current.yaml"), []byte(currentYAML), 0644); err != nil {
		return err
	}
	currentJSON := map[string]string{
		"current": filepath.Base(graphPathJSON),
	}
	if err := writeJSON(filepath.Join(run.RunDir, "task-graph.current.json"), currentJSON); err != nil {
		return err
	}
	run.TaskGraphVersion = graph.Version
	run.TaskGraphPath = graphPathYAML
	if err := s.SaveRun(run); err != nil {
		return err
	}
	statuses := make(map[string]string, len(graph.Tasks))
	existingStatuses, err := s.LoadTaskStatuses(run)
	if err != nil {
		return err
	}
	for _, task := range graph.Tasks {
		status := task.Status
		if existingStatus := existingStatuses[task.ID]; existingStatus != "" {
			status = existingStatus
		}
		if status == "" {
			status = TaskPending
		}
		statuses[task.ID] = status
	}
	return s.SaveTaskStatuses(run, statuses)
}

func (s *Store) LoadTaskGraph(run *Run) (*TaskGraph, error) {
	current := struct {
		Current string `json:"current"`
	}{}
	currentPath := filepath.Join(run.RunDir, "task-graph.current.json")
	if err := readJSON(currentPath, &current); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			current.Current = "task-graph.v1.json"
		} else {
			return nil, err
		}
	}
	path := filepath.Join(run.RunDir, current.Current)
	var graph TaskGraph
	if err := readJSON(path, &graph); err != nil {
		return nil, err
	}
	statuses, err := s.LoadTaskStatuses(run)
	if err != nil {
		return nil, err
	}
	for i := range graph.Tasks {
		if status := statuses[graph.Tasks[i].ID]; status != "" {
			graph.Tasks[i].Status = status
		}
	}
	return &graph, nil
}

func (s *Store) LoadTaskStatuses(run *Run) (map[string]string, error) {
	path := filepath.Join(run.RunDir, "task-status.json")
	statuses := map[string]string{}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return statuses, nil
		}
		return nil, err
	}
	if err := readJSON(path, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

func (s *Store) SaveTaskStatuses(run *Run, statuses map[string]string) error {
	return writeJSON(filepath.Join(run.RunDir, "task-status.json"), statuses)
}

func (s *Store) SetTaskStatus(run *Run, taskID, nextStatus, summary string) error {
	statuses, err := s.LoadTaskStatuses(run)
	if err != nil {
		return err
	}
	from := statuses[taskID]
	if from == "" {
		from = TaskPending
	}
	statuses[taskID] = nextStatus
	if err := s.SaveTaskStatuses(run, statuses); err != nil {
		return err
	}
	return s.AppendEvent(run.RunID, Event{
		Type:    "task_state_changed",
		TaskID:  taskID,
		Summary: summary,
		Data: map[string]any{
			"from": from,
			"to":   nextStatus,
		},
	})
}

func FindNextReadyTask(graph TaskGraph) (*Task, bool) {
	statusByID := make(map[string]string, len(graph.Tasks))
	for _, task := range graph.Tasks {
		status := task.Status
		if status == "" {
			status = TaskPending
		}
		statusByID[task.ID] = status
	}
	for i := range graph.Tasks {
		task := &graph.Tasks[i]
		status := task.Status
		if status == "" {
			status = TaskPending
		}
		if status != TaskPending && status != TaskReady {
			continue
		}
		ready := true
		for _, dep := range task.Dependencies {
			if task.Type == "repair" && statusByID[dep] == TaskRejected {
				continue
			}
			if statusByID[dep] != TaskAccepted {
				ready = false
				break
			}
		}
		if ready {
			copy := *task
			copy.Status = TaskReady
			return &copy, true
		}
	}
	return nil, false
}

func AllTasksAccepted(graph TaskGraph) bool {
	if len(graph.Tasks) == 0 {
		return false
	}
	for _, task := range graph.Tasks {
		if task.Status == TaskAccepted {
			continue
		}
		if task.Status == TaskRejected && hasAcceptedRepairFor(graph, task.ID) {
			continue
		}
		if task.Status != TaskAccepted {
			return false
		}
	}
	return true
}

func hasAcceptedRepairFor(graph TaskGraph, taskID string) bool {
	for _, task := range graph.Tasks {
		if task.Type != "repair" || task.Status != TaskAccepted {
			continue
		}
		for _, dep := range task.Dependencies {
			if dep == taskID {
				return true
			}
		}
	}
	return false
}

func (s *Store) AppendRepairTask(run *Run, rejectedTask Task, gates []GateResult) (*Task, error) {
	graph, err := s.LoadTaskGraph(run)
	if err != nil {
		return nil, err
	}
	statuses, err := s.LoadTaskStatuses(run)
	if err != nil {
		return nil, err
	}
	attempt := 1
	for {
		candidate := fmt.Sprintf("repair-%s-%d", rejectedTask.ID, attempt)
		exists := false
		for _, task := range graph.Tasks {
			if task.ID == candidate {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		attempt++
	}
	repairID := fmt.Sprintf("repair-%s-%d", rejectedTask.ID, attempt)
	repair := buildRepairTask(run, rejectedTask, repairID, gates)
	for i := range graph.Tasks {
		if graph.Tasks[i].ID == rejectedTask.ID || graph.Tasks[i].ID == repair.ID {
			continue
		}
		for j, dep := range graph.Tasks[i].Dependencies {
			if dep == rejectedTask.ID {
				graph.Tasks[i].Dependencies[j] = repair.ID
			}
		}
	}
	graph.Tasks = append(graph.Tasks, repair)
	graph.Version = nextGraphVersion(graph.Version)
	statuses[repair.ID] = TaskPending
	if err := s.SaveTaskGraph(run, *graph); err != nil {
		return nil, err
	}
	if err := s.SaveTaskStatuses(run, statuses); err != nil {
		return nil, err
	}
	if err := s.AppendEvent(run.RunID, Event{
		Type:    "decision_recorded",
		TaskID:  rejectedTask.ID,
		Summary: "Repair task generated for rejected work.",
		Data: map[string]any{
			"decision":       "create_repair_task",
			"repair_task_id": repair.ID,
			"new_graph":      graph.Version,
		},
	}); err != nil {
		return nil, err
	}
	return &repair, nil
}

func buildRepairTask(run *Run, rejectedTask Task, repairID string, gates []GateResult) Task {
	required := []string{"repair rejected task output", "preserve useful work when practical", "record residual risks"}
	for _, gate := range gates {
		if gate.Blocking && gate.Status == "failed" && gate.RequiredFix != "" {
			required = append(required, gate.RequiredFix)
		}
	}
	return Task{
		ID:           repairID,
		RunID:        run.RunID,
		Type:         "repair",
		Title:        "Repair " + rejectedTask.Title,
		Objective:    "Repair rejected output from task " + rejectedTask.ID + " using fix-in-place dirty-state strategy.",
		Dependencies: []string{rejectedTask.ID},
		Inputs: TaskInputs{
			Artifacts: append([]string{}, rejectedTask.Inputs.Artifacts...),
			RepoPaths: append([]string{}, rejectedTask.Inputs.RepoPaths...),
		},
		Outputs: TaskOutputs{ExpectedArtifacts: []string{"source changes", "tests"}, ExpectedEvidence: []string{"changes.patch", "gate-results"}},
		Acceptance: TaskAcceptance{
			Required: required,
		},
		ModelPolicy: ModelPolicy{Primary: "claude_code", Fallback: []string{"gpt_frontier"}},
		ToolGrants:  implementationToolGrants(),
		Review: TaskReview{
			Deterministic: rejectedTask.Review.Deterministic,
			Agentic:       rejectedTask.Review.Agentic,
		},
		RetryPolicy: defaultRetryPolicy(),
		Risk:        TaskRisk{Level: "medium", Reasons: []string{"repairs rejected output from " + rejectedTask.ID}},
		Status:      TaskPending,
	}
}

func nextGraphVersion(version string) string {
	var n int
	if _, err := fmt.Sscanf(version, "v%d", &n); err != nil || n < 1 {
		return "v2"
	}
	return fmt.Sprintf("v%d", n+1)
}

func hasCycle(tasks []Task) bool {
	deps := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		deps[task.ID] = task.Dependencies
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dep := range deps[id] {
			if visit(dep) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for _, task := range tasks {
		if visit(task.ID) {
			return true
		}
	}
	return false
}

func renderTaskGraphYAML(graph TaskGraph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: %s\n", yamlQuote(graph.Version))
	fmt.Fprintf(&b, "run_id: %s\n", yamlQuote(graph.RunID))
	fmt.Fprintf(&b, "generated_at: %s\n", yamlQuote(graph.GeneratedAt.Format(time.RFC3339)))
	b.WriteString("tasks:\n")
	for _, task := range graph.Tasks {
		fmt.Fprintf(&b, "  - id: %s\n", yamlQuote(task.ID))
		fmt.Fprintf(&b, "    run_id: %s\n", yamlQuote(task.RunID))
		fmt.Fprintf(&b, "    type: %s\n", yamlQuote(task.Type))
		fmt.Fprintf(&b, "    title: %s\n", yamlQuote(task.Title))
		fmt.Fprintf(&b, "    objective: %s\n", yamlQuote(task.Objective))
		writeStringList(&b, "    dependencies", task.Dependencies)
		b.WriteString("    inputs:\n")
		writeStringList(&b, "      artifacts", task.Inputs.Artifacts)
		writeStringList(&b, "      repo_paths", task.Inputs.RepoPaths)
		b.WriteString("    outputs:\n")
		writeStringList(&b, "      expected_artifacts", task.Outputs.ExpectedArtifacts)
		writeStringList(&b, "      expected_evidence", task.Outputs.ExpectedEvidence)
		b.WriteString("    acceptance:\n")
		writeStringList(&b, "      required", task.Acceptance.Required)
		writeStringList(&b, "      optional", task.Acceptance.Optional)
		b.WriteString("    model_policy:\n")
		fmt.Fprintf(&b, "      primary: %s\n", yamlQuote(task.ModelPolicy.Primary))
		writeStringList(&b, "      fallback", task.ModelPolicy.Fallback)
		b.WriteString("    tool_grants:\n")
		b.WriteString("      git:\n")
		writeStringList(&b, "        worker_permissions", task.ToolGrants.Git.WorkerPermissions)
		writeStringList(&b, "        orchestrator_permissions", task.ToolGrants.Git.OrchestratorPermissions)
		b.WriteString("      filesystem:\n")
		writeStringList(&b, "        permissions", task.ToolGrants.Filesystem.Permissions)
		b.WriteString("      shell:\n")
		writeStringList(&b, "        permissions", task.ToolGrants.Shell.Permissions)
		b.WriteString("    review:\n")
		writeStringList(&b, "      deterministic", task.Review.Deterministic)
		writeStringList(&b, "      agentic", task.Review.Agentic)
		b.WriteString("    retry_policy:\n")
		fmt.Fprintf(&b, "      max_attempts: %d\n", task.RetryPolicy.MaxAttempts)
		writeStringList(&b, "      strategies", task.RetryPolicy.Strategies)
		b.WriteString("    risk:\n")
		fmt.Fprintf(&b, "      level: %s\n", yamlQuote(task.Risk.Level))
		writeStringList(&b, "      reasons", task.Risk.Reasons)
		fmt.Fprintf(&b, "    status: %s\n", yamlQuote(task.Status))
	}
	return b.String()
}

func writeStringList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	itemIndent := leadingWhitespace(key) + "  "
	for _, value := range values {
		fmt.Fprintf(b, "%s- %s\n", itemIndent, yamlQuote(value))
	}
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}
