package shipit

import (
	"strings"
	"testing"
	"time"
)

func TestValidateTaskGraphRejectsCycles(t *testing.T) {
	graph := TaskGraph{
		Version:     "v1",
		RunID:       "run_test",
		GeneratedAt: time.Now().UTC(),
		Tasks: []Task{
			minimalTask("a", "implementation", []string{"b"}),
			minimalTask("b", "review", []string{"a"}),
		},
	}
	if err := ValidateTaskGraph(graph); err == nil || !strings.Contains(err.Error(), "acyclic") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestValidateTaskGraphRejectsImplementationWithoutGates(t *testing.T) {
	graph := TaskGraph{
		Version:     "v1",
		RunID:       "run_test",
		GeneratedAt: time.Now().UTC(),
		Tasks: []Task{
			minimalTask("implement", "implementation", nil),
		},
	}
	graph.Tasks[0].Review = TaskReview{}
	if err := ValidateTaskGraph(graph); err == nil || !strings.Contains(err.Error(), "deterministic gate") {
		t.Fatalf("expected deterministic gate error, got %v", err)
	}
}

func minimalTask(id, taskType string, deps []string) Task {
	task := Task{
		ID:           id,
		RunID:        "run_test",
		Type:         taskType,
		Title:        id,
		Objective:    "Do " + id,
		Dependencies: deps,
		Acceptance:   TaskAcceptance{Required: []string{"done"}},
		ModelPolicy:  ModelPolicy{Primary: "test"},
		ToolGrants:   readOnlyToolGrants(),
		Review:       TaskReview{Deterministic: []string{"unit_tests"}, Agentic: []string{"independent_review"}},
		RetryPolicy:  defaultRetryPolicy(),
		Risk:         TaskRisk{Level: "low", Reasons: []string{"test"}},
		Status:       "pending",
	}
	if taskType != "implementation" {
		task.Review = TaskReview{}
	}
	return task
}
