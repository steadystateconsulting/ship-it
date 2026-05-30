package shipit

import "time"

const (
	StateCreated      = "created"
	StateInitializing = "initializing"
	StatePlanning     = "planning"
	StateAwaitingPlan = "awaiting_plan_approval"
	StateExecuting    = "executing"
	StateReviewing    = "reviewing"
	StateRepairing    = "repairing"
	StateBlocked      = "blocked"
	StateCompleted    = "completed"
	StateFailed       = "failed"
	StateCancelled    = "cancelled"
)

const (
	TaskPending   = "pending"
	TaskReady     = "ready"
	TaskRunning   = "running"
	TaskChecking  = "checking"
	TaskReviewing = "reviewing"
	TaskAccepted  = "accepted"
	TaskRejected  = "rejected"
	TaskBlocked   = "blocked"
	TaskFailed    = "failed"
	TaskSkipped   = "skipped"
	TaskCancelled = "cancelled"
)

var terminalStates = map[string]bool{
	StateCompleted: true,
	StateFailed:    true,
	StateCancelled: true,
}

type Run struct {
	RunID                             string    `json:"run_id"`
	Goal                              string    `json:"goal"`
	RepoPath                          string    `json:"repo_path"`
	ProfilePath                       string    `json:"profile_path,omitempty"`
	Mode                              string    `json:"mode"`
	State                             string    `json:"state"`
	PreviousState                     string    `json:"previous_state,omitempty"`
	CreatedAt                         time.Time `json:"created_at"`
	UpdatedAt                         time.Time `json:"updated_at"`
	RunDir                            string    `json:"run_dir"`
	WorkspaceDir                      string    `json:"workspace_dir"`
	Branch                            string    `json:"branch"`
	OriginalGitStatus                 []string  `json:"original_git_status"`
	PlanApprovalRequired              bool      `json:"plan_approval_required"`
	PlanApproved                      bool      `json:"plan_approved"`
	Planner                           string    `json:"planner"`
	PlannerModel                      string    `json:"planner_model,omitempty"`
	TaskGraphVersion                  string    `json:"task_graph_version,omitempty"`
	TaskGraphPath                     string    `json:"task_graph_path,omitempty"`
	DirtyStateStrategy                string    `json:"dirty_state_strategy"`
	MaxParallelTasks                  int       `json:"max_parallel_tasks"`
	IntegrationTestsBlocking          string    `json:"integration_tests_blocking"`
	FailureLoopCaptainReviewThreshold int       `json:"failure_loop_captain_review_threshold"`
}

type Event struct {
	EventID   string         `json:"event_id"`
	Timestamp time.Time      `json:"timestamp"`
	RunID     string         `json:"run_id"`
	TaskID    string         `json:"task_id,omitempty"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor"`
	Summary   string         `json:"summary"`
	Data      map[string]any `json:"data,omitempty"`
}
