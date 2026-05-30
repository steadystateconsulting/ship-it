package shipit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	RepoPath string
	RootDir  string
}

func NewStore(repoPath string) (*Store, error) {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	return &Store{
		RepoPath: absRepo,
		RootDir:  filepath.Join(absRepo, ".shipit", "runs"),
	}, nil
}

func (s *Store) RunDir(runID string) string {
	return filepath.Join(s.RootDir, runID)
}

func (s *Store) InitRunDirs(runID string) error {
	runDir := s.RunDir(runID)
	dirs := []string{
		runDir,
		filepath.Join(runDir, "artifacts"),
		filepath.Join(runDir, "artifacts", "architecture"),
		filepath.Join(runDir, "artifacts", "context"),
		filepath.Join(runDir, "artifacts", "plans"),
		filepath.Join(runDir, "artifacts", "reviews"),
		filepath.Join(runDir, "artifacts", "specs"),
		filepath.Join(runDir, "artifacts", "tasks"),
		filepath.Join(runDir, "artifacts", "reports"),
		filepath.Join(runDir, "evals"),
		filepath.Join(runDir, "logbook"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveRun(run *Run) error {
	run.UpdatedAt = time.Now().UTC()
	if err := writeJSON(filepath.Join(run.RunDir, "run.json"), run); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(run.RunDir, "run.yaml"), []byte(renderRunYAML(run)), 0644); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadRun(runID string) (*Run, error) {
	data, err := os.ReadFile(filepath.Join(s.RunDir(runID), "run.json"))
	if err != nil {
		return nil, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Store) AppendEvent(runID string, event Event) error {
	if event.EventID == "" {
		event.EventID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.RunID == "" {
		event.RunID = runID
	}
	if event.Actor == "" {
		event.Actor = "orchestrator"
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(s.RunDir(runID), "logbook", "run.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) ReadEvents(runID string) ([]Event, error) {
	path := filepath.Join(s.RunDir(runID), "logbook", "run.jsonl")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("parse logbook event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) WriteOrder(run *Run) error {
	return os.WriteFile(filepath.Join(run.RunDir, "order.md"), []byte(run.Goal+"\n"), 0644)
}

func (s *Store) WriteProfile(run *Run) error {
	out := filepath.Join(run.RunDir, "profile.yaml")
	if run.ProfilePath == "" {
		return os.WriteFile(out, []byte("# No profile supplied.\n"), 0644)
	}
	data, err := os.ReadFile(run.ProfilePath)
	if err != nil {
		return err
	}
	return os.WriteFile(out, data, 0644)
}

func TransitionRun(s *Store, run *Run, nextState, summary string) error {
	from := run.State
	run.PreviousState = from
	run.State = nextState
	if err := s.SaveRun(run); err != nil {
		return err
	}
	return s.AppendEvent(run.RunID, Event{
		Type:    "run_state_changed",
		Summary: summary,
		Data: map[string]any{
			"from": from,
			"to":   nextState,
		},
	})
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func renderRunYAML(run *Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "run_id: %s\n", yamlQuote(run.RunID))
	fmt.Fprintf(&b, "goal: %s\n", yamlQuote(run.Goal))
	fmt.Fprintf(&b, "repo_path: %s\n", yamlQuote(run.RepoPath))
	if run.ProfilePath != "" {
		fmt.Fprintf(&b, "profile_path: %s\n", yamlQuote(run.ProfilePath))
	}
	fmt.Fprintf(&b, "mode: %s\n", yamlQuote(run.Mode))
	fmt.Fprintf(&b, "state: %s\n", yamlQuote(run.State))
	if run.PreviousState != "" {
		fmt.Fprintf(&b, "previous_state: %s\n", yamlQuote(run.PreviousState))
	}
	fmt.Fprintf(&b, "created_at: %s\n", yamlQuote(run.CreatedAt.Format(time.RFC3339)))
	fmt.Fprintf(&b, "updated_at: %s\n", yamlQuote(run.UpdatedAt.Format(time.RFC3339)))
	fmt.Fprintf(&b, "run_dir: %s\n", yamlQuote(run.RunDir))
	fmt.Fprintf(&b, "workspace_dir: %s\n", yamlQuote(run.WorkspaceDir))
	fmt.Fprintf(&b, "branch: %s\n", yamlQuote(run.Branch))
	fmt.Fprintf(&b, "plan_approval_required: %t\n", run.PlanApprovalRequired)
	fmt.Fprintf(&b, "plan_approved: %t\n", run.PlanApproved)
	if run.DirtyStateStrategy != "" {
		fmt.Fprintf(&b, "dirty_state_strategy: %s\n", yamlQuote(run.DirtyStateStrategy))
	}
	fmt.Fprintf(&b, "max_parallel_tasks: %d\n", run.MaxParallelTasks)
	if run.IntegrationTestsBlocking != "" {
		fmt.Fprintf(&b, "integration_tests_blocking: %s\n", yamlQuote(run.IntegrationTestsBlocking))
	}
	fmt.Fprintf(&b, "failure_loop_captain_review_threshold: %d\n", run.FailureLoopCaptainReviewThreshold)
	if run.TaskGraphVersion != "" {
		fmt.Fprintf(&b, "task_graph_version: %s\n", yamlQuote(run.TaskGraphVersion))
	}
	if run.TaskGraphPath != "" {
		fmt.Fprintf(&b, "task_graph_path: %s\n", yamlQuote(run.TaskGraphPath))
	}
	b.WriteString("original_git_status:\n")
	for _, line := range run.OriginalGitStatus {
		fmt.Fprintf(&b, "  - %s\n", yamlQuote(line))
	}
	return b.String()
}

func yamlQuote(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return `"` + escaped + `"`
}

func newEventID() string {
	return "evt_" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}
