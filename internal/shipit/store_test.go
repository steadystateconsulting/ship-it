package shipit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSavesRunAndEvents(t *testing.T) {
	repo := t.TempDir()
	store, err := NewStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	runID := "run_test"
	if err := store.InitRunDirs(runID); err != nil {
		t.Fatal(err)
	}
	run := &Run{
		RunID:        runID,
		Goal:         "test goal",
		RepoPath:     repo,
		Mode:         "local",
		State:        StateCreated,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		RunDir:       store.RunDir(runID),
		WorkspaceDir: filepath.Join(store.RunDir(runID), "workspace"),
		Branch:       "shipit/test",
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(runID, Event{Type: "run_created", Summary: "Run created."}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != runID || loaded.State != StateCreated {
		t.Fatalf("unexpected run: %+v", loaded)
	}
	events, err := store.ReadEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "run_created" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if _, err := os.Stat(filepath.Join(store.RunDir(runID), "run.yaml")); err != nil {
		t.Fatal(err)
	}
}
