package shipit

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func Main(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}
	switch args[0] {
	case "run":
		return runCommand(args[1:], stdout)
	case "status":
		return statusCommand(args[1:], stdout)
	case "log":
		return logCommand(args[1:], stdout)
	case "report":
		return reportCommand(args[1:], stdout)
	case "index":
		return indexCommand(args[1:], stdout)
	case "tasks":
		return tasksCommand(args[1:], stdout)
	case "approve-plan":
		return approvePlanCommand(args[1:], stdout)
	case "step":
		return stepCommand(args[1:], stdout)
	case "execute":
		return executeCommand(args[1:], stdout)
	case "resume":
		return resumeCommand(args[1:], stdout)
	case "cancel":
		return terminalCommand(args[1:], stdout, StateCancelled, "Run cancelled by user.")
	case "finalize":
		return finalizeCommand(args[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	goal := fs.String("goal", "", "engineering goal")
	repo := fs.String("repo", ".", "target repository")
	profile := fs.String("profile", "", "preference profile")
	mode := fs.String("mode", "local", "run mode")
	runIDOverride := fs.String("run-id", "", "explicit run ID")
	autoApprovePlan := fs.Bool("auto-approve-plan", false, "skip plan approval and move to execution")
	dirtyStateStrategy := fs.String("dirty-state-strategy", "fix_in_place", "fix_in_place, orchestrator_rollback, or executor_discretion")
	maxParallelTasks := fs.Int("max-parallel-tasks", 1, "scheduler parallelism policy knob; execution remains serial in MVP")
	integrationTestsBlocking := fs.String("integration-tests-blocking", "when_runnable", "when_runnable, always, or never")
	failureThreshold := fs.Int("failure-loop-captain-review-threshold", 2, "repair failures before blocking for Captain review")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*goal) == "" {
		return errors.New("run requires --goal")
	}
	if *mode != "local" {
		return fmt.Errorf("unsupported mode %q; only local is implemented", *mode)
	}
	if err := validateDirtyStateStrategy(*dirtyStateStrategy); err != nil {
		return err
	}
	if err := validateRunPolicy(*maxParallelTasks, *integrationTestsBlocking, *failureThreshold); err != nil {
		return err
	}

	store, err := NewStore(*repo)
	if err != nil {
		return err
	}
	if err := IsGitRepo(store.RepoPath); err != nil {
		return fmt.Errorf("--repo must point to a git work tree: %w", err)
	}
	if *profile != "" {
		absProfile, err := filepath.Abs(*profile)
		if err != nil {
			return err
		}
		*profile = absProfile
	}

	runID := *runIDOverride
	if runID == "" {
		runID = newRunID(*goal)
	}
	if err := validateRunID(runID); err != nil {
		return err
	}

	if err := store.InitRunDirs(runID); err != nil {
		return err
	}
	now := time.Now().UTC()
	runDir := store.RunDir(runID)
	run := &Run{
		RunID:                             runID,
		Goal:                              strings.TrimSpace(*goal),
		RepoPath:                          store.RepoPath,
		ProfilePath:                       *profile,
		Mode:                              *mode,
		State:                             StateCreated,
		CreatedAt:                         now,
		UpdatedAt:                         now,
		RunDir:                            runDir,
		WorkspaceDir:                      filepath.Join(runDir, "workspace"),
		Branch:                            "shipit/" + time.Now().Format("20060102") + "-" + slugify(*goal) + "-" + shortRunID(runID),
		PlanApprovalRequired:              !*autoApprovePlan,
		DirtyStateStrategy:                *dirtyStateStrategy,
		MaxParallelTasks:                  *maxParallelTasks,
		IntegrationTestsBlocking:          *integrationTestsBlocking,
		FailureLoopCaptainReviewThreshold: *failureThreshold,
	}
	status, err := GitStatus(store.RepoPath)
	if err != nil {
		return err
	}
	run.OriginalGitStatus = status

	if err := store.SaveRun(run); err != nil {
		return err
	}
	if err := store.WriteOrder(run); err != nil {
		return err
	}
	if err := store.WriteProfile(run); err != nil {
		return err
	}
	if err := store.AppendEvent(run.RunID, Event{
		Type:    "run_created",
		Summary: "Run created.",
		Data: map[string]any{
			"repo_path": run.RepoPath,
			"run_dir":   run.RunDir,
		},
	}); err != nil {
		return err
	}

	if err := TransitionRun(store, run, StateInitializing, "Run initialization started."); err != nil {
		return err
	}
	if err := EnsureShipitIgnored(store.RepoPath); err != nil {
		return err
	}
	if err := CreateWorktree(store.RepoPath, run.WorkspaceDir, run.Branch); err != nil {
		_ = TransitionRun(store, run, StateBlocked, "Run blocked while creating isolated workspace.")
		return err
	}
	if err := store.AppendEvent(run.RunID, Event{
		Type:    "artifact_created",
		Summary: "Isolated Git worktree created.",
		Data: map[string]any{
			"workspace_dir": run.WorkspaceDir,
			"branch":        run.Branch,
		},
	}); err != nil {
		return err
	}
	if err := TransitionRun(store, run, StatePlanning, "Run initialized and ready for planning."); err != nil {
		return err
	}
	if err := PlanRun(store, run, PlanOptions{AutoApprove: *autoApprovePlan}); err != nil {
		_ = TransitionRun(store, run, StateBlocked, "Run blocked while generating plan.")
		return err
	}

	fmt.Fprintf(stdout, "created run %s\nstate: %s\nrun_dir: %s\nworkspace: %s\nbranch: %s\n", run.RunID, run.State, run.RunDir, run.WorkspaceDir, run.Branch)
	return nil
}

func statusCommand(args []string, stdout io.Writer) error {
	run, _, err := loadRunFromFlags("status", args)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "run: %s\nstate: %s\nrepo: %s\nworkspace: %s\nbranch: %s\nupdated: %s\n", run.RunID, run.State, run.RepoPath, run.WorkspaceDir, run.Branch, run.UpdatedAt.Format(time.RFC3339))
	return nil
}

func logCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("log", args)
	if err != nil {
		return err
	}
	events, err := store.ReadEvents(run.RunID)
	if err != nil {
		return err
	}
	for _, event := range events {
		fmt.Fprintf(stdout, "%s %-24s %s\n", event.Timestamp.Format(time.RFC3339), event.Type, event.Summary)
	}
	return nil
}

func reportCommand(args []string, stdout io.Writer) error {
	run, _, err := loadRunFromFlags("report", args)
	if err != nil {
		return err
	}
	path := filepath.Join(run.RunDir, "artifacts", "reports", "final-delivery-report.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
}

func indexCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("index", args)
	if err != nil {
		return err
	}
	index, err := GenerateDocumentIndex(store, run)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "indexed %d documents\nindex: %s\n", len(index.Documents), filepath.Join(run.RunDir, "artifacts", "document-index.yaml"))
	return nil
}

func tasksCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("tasks", args)
	if err != nil {
		return err
	}
	graph, err := store.LoadTaskGraph(run)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "task_graph: %s\n", graph.Version)
	for _, task := range graph.Tasks {
		deps := "-"
		if len(task.Dependencies) > 0 {
			deps = strings.Join(task.Dependencies, ",")
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\tdeps:%s\n", task.ID, task.Type, task.Status, deps)
	}
	return nil
}

func approvePlanCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("approve-plan", args)
	if err != nil {
		return err
	}
	if err := ApprovePlan(store, run); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "run %s plan approved\nstate: %s\n", run.RunID, run.State)
	return nil
}

func stepCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("step", args)
	if err != nil {
		return err
	}
	result, err := ExecuteNextTask(store, run)
	if err != nil {
		return err
	}
	if result.Done {
		fmt.Fprintf(stdout, "run %s -> %s\n", run.RunID, result.Status)
		return nil
	}
	fmt.Fprintf(stdout, "task %s -> %s\n", result.TaskID, result.Status)
	return nil
}

func executeCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.String("run", "", "run ID")
	repo := fs.String("repo", ".", "target repository")
	maxTasks := fs.Int("max-tasks", 1, "maximum tasks to execute serially")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return errors.New("command requires --run")
	}
	store, err := NewStore(*repo)
	if err != nil {
		return err
	}
	run, err := store.LoadRun(*runID)
	if err != nil {
		return err
	}
	limit := *maxTasks
	if limit <= 0 {
		limit = run.MaxParallelTasks
	}
	results, err := ExecuteRun(store, run, ExecuteOptions{MaxTasks: limit})
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.Done {
			fmt.Fprintf(stdout, "run %s -> %s\n", run.RunID, result.Status)
			continue
		}
		fmt.Fprintf(stdout, "task %s -> %s\n", result.TaskID, result.Status)
	}
	return nil
}

func resumeCommand(args []string, stdout io.Writer) error {
	run, store, err := loadRunFromFlags("resume", args)
	if err != nil {
		return err
	}
	if terminalStates[run.State] {
		return fmt.Errorf("run %s is terminal with state %q; create a new linked run to continue", run.RunID, run.State)
	}
	if _, err := os.Stat(run.WorkspaceDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if txErr := TransitionRun(store, run, StateBlocked, "Run blocked because workspace is missing."); txErr != nil {
				return txErr
			}
			return fmt.Errorf("workspace is missing: %s", run.WorkspaceDir)
		}
		return err
	}
	if run.State == StateBlocked {
		nextState := run.PreviousState
		if nextState == "" || nextState == StateBlocked || terminalStates[nextState] {
			nextState = StatePlanning
		}
		if err := TransitionRun(store, run, nextState, "Blocked run resumed after reconciliation."); err != nil {
			return err
		}
	}
	if run.State == StatePlanning {
		if err := PlanRun(store, run, PlanOptions{AutoApprove: !run.PlanApprovalRequired}); err != nil {
			if txErr := TransitionRun(store, run, StateBlocked, "Run blocked while generating plan."); txErr != nil {
				return txErr
			}
			return err
		}
	}
	if err := store.AppendEvent(run.RunID, Event{
		Type:    "run_resumed",
		Summary: "Run resume requested.",
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "resumed run %s\nstate: %s\n", run.RunID, run.State)
	return nil
}

func terminalCommand(args []string, stdout io.Writer, state, summary string) error {
	run, store, err := loadRunFromFlags(state, args)
	if err != nil {
		return err
	}
	if terminalStates[run.State] {
		return fmt.Errorf("run %s is already terminal with state %q", run.RunID, run.State)
	}
	if err := TransitionRun(store, run, state, summary); err != nil {
		return err
	}
	eventType := "run_" + state
	if state == StateCancelled {
		eventType = "run_cancelled"
	}
	if err := store.AppendEvent(run.RunID, Event{Type: eventType, Summary: summary}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "run %s -> %s\n", run.RunID, state)
	return nil
}

func finalizeCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outcome := fs.String("outcome", StateCompleted, "completed, failed, or blocked")
	runID := fs.String("run", "", "run ID")
	repo := fs.String("repo", ".", "target repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *outcome {
	case StateCompleted, StateFailed, StateBlocked:
	default:
		return fmt.Errorf("unsupported outcome %q", *outcome)
	}
	if *runID == "" {
		return errors.New("command requires --run")
	}
	store, err := NewStore(*repo)
	if err != nil {
		return err
	}
	run, err := store.LoadRun(*runID)
	if err != nil {
		return err
	}
	if err := FinalizeRun(store, run, *outcome); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "run %s -> %s\nreport: %s\n", run.RunID, *outcome, filepath.Join(run.RunDir, "artifacts", "reports", "final-delivery-report.md"))
	return nil
}

func loadRunFromFlags(name string, args []string) (*Run, *Store, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.String("run", "", "run ID")
	repo := fs.String("repo", ".", "target repository")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	if *runID == "" {
		return nil, nil, errors.New("command requires --run")
	}
	store, err := NewStore(*repo)
	if err != nil {
		return nil, nil, err
	}
	run, err := store.LoadRun(*runID)
	if err != nil {
		return nil, nil, err
	}
	return run, store, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  shipit run --goal <goal> --repo <path> [--profile <path>] [--mode local] [--auto-approve-plan]")
	fmt.Fprintln(w, "             [--max-parallel-tasks 1] [--integration-tests-blocking when_runnable|always|never]")
	fmt.Fprintln(w, "             [--dirty-state-strategy fix_in_place|orchestrator_rollback|executor_discretion]")
	fmt.Fprintln(w, "  shipit status --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit log --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit report --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit index --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit tasks --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit approve-plan --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit step --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit execute --run <run_id> [--repo <path>] [--max-tasks <n>]")
	fmt.Fprintln(w, "  shipit resume --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit cancel --run <run_id> [--repo <path>]")
	fmt.Fprintln(w, "  shipit finalize --run <run_id> [--repo <path>] [--outcome completed|failed|blocked]")
}

func newRunID(goal string) string {
	return "run_" + time.Now().UTC().Format("20060102_150405") + "_" + slugify(goal)
}

func shortRunID(runID string) string {
	if len(runID) <= 8 {
		return runID
	}
	return runID[len(runID)-8:]
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "run"
	}
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if s == "" {
		return "run"
	}
	return s
}

func validateRunID(runID string) error {
	if runID == "" {
		return errors.New("run ID cannot be empty")
	}
	ok, err := regexp.MatchString(`^[A-Za-z0-9_.-]+$`, runID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid run ID %q", runID)
	}
	return nil
}

func validateDirtyStateStrategy(strategy string) error {
	switch strategy {
	case "", "fix_in_place", "orchestrator_rollback", "executor_discretion":
		return nil
	default:
		return fmt.Errorf("unsupported dirty-state strategy %q", strategy)
	}
}

func validateRunPolicy(maxParallelTasks int, integrationTestsBlocking string, failureThreshold int) error {
	if maxParallelTasks < 1 {
		return errors.New("max parallel tasks must be at least 1")
	}
	switch integrationTestsBlocking {
	case "when_runnable", "always", "never":
	default:
		return fmt.Errorf("unsupported integration-tests-blocking value %q", integrationTestsBlocking)
	}
	if failureThreshold < 1 {
		return errors.New("failure-loop-captain-review-threshold must be at least 1")
	}
	return nil
}
