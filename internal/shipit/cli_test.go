package shipit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatusLogAndCancel(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	err := Main([]string{
		"run",
		"--goal", "Build skeleton",
		"--repo", repo,
		"--run-id", "run_test",
	}, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "created run run_test") || !strings.Contains(out.String(), "state: awaiting_plan_approval") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	if !strings.Contains(out.String(), "Plan\n- inspect-repository") {
		t.Fatalf("run output did not include plan: %s", out.String())
	}
	if !strings.Contains(out.String(), "go run ./cmd/shipit execute \\\n  --run run_test \\\n  --repo . \\\n  --max-tasks 10") {
		t.Fatalf("run output did not include concrete next command: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"status", "--repo", repo, "--run", "run_test"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "state: awaiting_plan_approval") {
		t.Fatalf("unexpected status: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".shipit", "runs", "run_test", "task-graph.v1.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".shipit", "runs", "run_test", "artifacts", "specs", "mvp-plan-spec.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"log", "--repo", repo, "--run", "run_test"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run_created") || !strings.Contains(out.String(), "worker_completed") {
		t.Fatalf("unexpected log: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"tasks", "--repo", repo, "--run", "run_test"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "implement-goal") || !strings.Contains(out.String(), "final-delivery-report") {
		t.Fatalf("unexpected tasks: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"approve-plan", "--repo", repo, "--run", "run_test"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "state: executing") {
		t.Fatalf("unexpected approve output: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"cancel", "--repo", repo, "--run", "run_test"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run run_test -> cancelled") {
		t.Fatalf("unexpected cancel output: %s", out.String())
	}
}

func TestBlockedRunCanResume(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "Resume blocked",
		"--repo", repo,
		"--run-id", "run_blocked",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"finalize", "--repo", repo, "--run", "run_blocked", "--outcome", "blocked"}, &out, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"resume", "--repo", repo, "--run", "run_blocked"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "state: awaiting_plan_approval") {
		t.Fatalf("unexpected resume output: %s", out.String())
	}
}

func TestRunCanAutoApprovePlan(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "Build auto approved plan",
		"--repo", repo,
		"--run-id", "run_auto",
		"--auto-approve-plan",
		"--max-parallel-tasks", "3",
		"--integration-tests-blocking", "never",
		"--failure-loop-captain-review-threshold", "4",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "state: executing") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	runYAML, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_auto", "run.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"max_parallel_tasks: 3",
		`integration_tests_blocking: "never"`,
		"failure_loop_captain_review_threshold: 4",
	} {
		if !strings.Contains(string(runYAML), expected) {
			t.Fatalf("run policy missing %q from run.yaml:\n%s", expected, string(runYAML))
		}
	}
}

func TestRunFullExecutesAndPrintsReport(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "Run full loop",
		"--repo", repo,
		"--run-id", "run_full",
		"--full",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	output := out.String()
	if !strings.Contains(output, "Executing serial run...") {
		t.Fatalf("full run did not execute: %s", output)
	}
	if !strings.Contains(output, "# Final Delivery Report") {
		t.Fatalf("full run did not print final report: %s", output)
	}
	if !strings.Contains(output, "run run_full -> completed") {
		t.Fatalf("full run did not complete: %s", output)
	}
}

func TestStepExecutesReadyTaskAndPersistsArtifacts(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "Exercise executor",
		"--repo", repo,
		"--run-id", "run_step",
		"--auto-approve-plan",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"step", "--repo", repo, "--run", "run_step"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "task inspect-repository -> accepted") {
		t.Fatalf("unexpected step output: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".shipit", "runs", "run_step", "artifacts", "tasks", "inspect-repository", "worker-input.yaml")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"tasks", "--repo", repo, "--run", "run_step"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "inspect-repository\tresearch\taccepted") {
		t.Fatalf("unexpected tasks output: %s", out.String())
	}
}

func TestExecuteRunsSerialTasksAndCapturesImplementationDiff(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "Exercise full executor",
		"--repo", repo,
		"--run-id", "run_execute",
		"--auto-approve-plan",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_execute", "--max-tasks", "10"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run run_execute -> completed") {
		t.Fatalf("unexpected execute output: %s", out.String())
	}
	patchPath := filepath.Join(repo, ".shipit", "runs", "run_execute", "artifacts", "tasks", "implement-goal", "changes.patch")
	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "SHIPIT_STUB_IMPLEMENTATION.md") {
		t.Fatalf("patch did not include stub implementation: %s", string(patch))
	}
	if _, err := os.Stat(filepath.Join(repo, ".shipit", "runs", "run_execute", "artifacts", "tasks", "implement-goal", "gate-results.yaml")); err != nil {
		t.Fatal(err)
	}
	log := gitOutput(t, filepath.Join(repo, ".shipit", "runs", "run_execute", "workspace"), "log", "--oneline")
	if !strings.Contains(log, "shipit: implement-goal") {
		t.Fatalf("checkpoint commit missing from log: %s", log)
	}
	runDir := filepath.Join(repo, ".shipit", "runs", "run_execute")
	report, err := os.ReadFile(filepath.Join(runDir, "artifacts", "reports", "final-delivery-report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "## Gate Summary") || !strings.Contains(string(report), "Checkpoint") {
		t.Fatalf("final report missing synthesized sections: %s", string(report))
	}
	if _, err := os.Stat(filepath.Join(runDir, "evals", "run-evaluation.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "artifacts", "document-index.yaml")); err != nil {
		t.Fatal(err)
	}
	taskEvals, err := os.ReadFile(filepath.Join(runDir, "evals", "task-evaluations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(taskEvals), "implement-goal") {
		t.Fatalf("task evaluations missing implementation task: %s", string(taskEvals))
	}

	out.Reset()
	if err := Main([]string{"report", "--repo", repo, "--run", "run_execute"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "# Final Delivery Report") {
		t.Fatalf("report command did not print report: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"index", "--repo", repo, "--run", "run_execute"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "indexed ") {
		t.Fatalf("index command did not index documents: %s", out.String())
	}
	index, err := os.ReadFile(filepath.Join(runDir, "artifacts", "document-index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "final-delivery-report.md") || !strings.Contains(string(index), "run-evaluation.yaml") {
		t.Fatalf("document index missing expected entries: %s", string(index))
	}
}

func TestIntegrationTestsAlwaysBlocksWhenNoRunnablePath(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "require integration tests",
		"--repo", repo,
		"--run-id", "run_integration_required",
		"--auto-approve-plan",
		"--integration-tests-blocking", "always",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_integration_required", "--max-tasks", "2"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "task implement-goal -> rejected") {
		t.Fatalf("expected integration gate rejection, got: %s", out.String())
	}
	gates, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_integration_required", "artifacts", "tasks", "implement-goal", "gate-results.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gates), "Integration tests are required by policy") {
		t.Fatalf("integration policy failure missing from gates: %s", string(gates))
	}
}

func TestFailureLoopThresholdBlocksForCaptainReview(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "force repeated repair",
		"--repo", repo,
		"--run-id", "run_failure_threshold",
		"--auto-approve-plan",
		"--integration-tests-blocking", "always",
		"--failure-loop-captain-review-threshold", "1",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_failure_threshold", "--max-tasks", "2"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_failure_threshold", "--max-tasks", "1"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Main([]string{"status", "--repo", repo, "--run", "run_failure_threshold"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "state: blocked") {
		t.Fatalf("expected run to block for Captain review, got: %s", out.String())
	}
	logbook, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_failure_threshold", "logbook", "run.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logbook), "Failure loop threshold reached") {
		t.Fatalf("Captain review request missing from logbook: %s", string(logbook))
	}
}

func TestReviewFailureSchedulesRepairAndCheckpointsRepair(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "force repair",
		"--repo", repo,
		"--run-id", "run_repair",
		"--auto-approve-plan",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_repair", "--max-tasks", "10"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "task implement-goal -> rejected") {
		t.Fatalf("expected implementation rejection, got: %s", out.String())
	}

	out.Reset()
	if err := Main([]string{"tasks", "--repo", repo, "--run", "run_repair"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "repair-implement-goal-1") {
		t.Fatalf("repair task missing: %s", out.String())
	}
	gates, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_repair", "artifacts", "tasks", "implement-goal", "gate-results.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gates), "Remove SHIPIT_FORCE_REPAIR") {
		t.Fatalf("required fix missing from gates: %s", string(gates))
	}

	out.Reset()
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_repair", "--max-tasks", "10"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run run_repair -> completed") {
		t.Fatalf("expected repaired run completion, got: %s", out.String())
	}
	workspace := filepath.Join(repo, ".shipit", "runs", "run_repair", "workspace")
	stub, err := os.ReadFile(filepath.Join(workspace, "SHIPIT_STUB_IMPLEMENTATION.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stub), "SHIPIT_FORCE_REPAIR") {
		t.Fatalf("repair marker was not removed: %s", string(stub))
	}
	log := gitOutput(t, workspace, "log", "--oneline")
	if strings.Contains(log, "shipit: implement-goal") {
		t.Fatalf("rejected implementation should not be checkpointed: %s", log)
	}
	if !strings.Contains(log, "shipit: repair-implement-goal-1") {
		t.Fatalf("repair checkpoint missing: %s", log)
	}
	runEval, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_repair", "evals", "run-evaluation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runEval), "repair_tasks: 1") || !strings.Contains(string(runEval), "rejected_tasks: 1") {
		t.Fatalf("repair run evaluation missing repair/rejection counts: %s", string(runEval))
	}
}

func TestOrchestratorRollbackRepairStrategyStartsFresh(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	if err := Main([]string{
		"run",
		"--goal", "force repair",
		"--repo", repo,
		"--run-id", "run_rollback",
		"--auto-approve-plan",
		"--dirty-state-strategy", "orchestrator_rollback",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_rollback", "--max-tasks", "10"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"execute", "--repo", repo, "--run", "run_rollback", "--max-tasks", "10"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(repo, ".shipit", "runs", "run_rollback", "workspace")
	stub, err := os.ReadFile(filepath.Join(workspace, "SHIPIT_STUB_IMPLEMENTATION.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stub), "Stub Repair Implementation") {
		t.Fatalf("rollback repair should recreate a fresh implementation: %s", string(stub))
	}
	logbook, err := os.ReadFile(filepath.Join(repo, ".shipit", "runs", "run_rollback", "logbook", "run.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logbook), "orchestrator_rollback") {
		t.Fatalf("rollback decision missing from logbook: %s", string(logbook))
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "shipit@example.test")
	runGit(t, repo, "config", "user.name", "Shipit Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}
