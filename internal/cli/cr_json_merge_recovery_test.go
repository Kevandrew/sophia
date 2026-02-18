package cli

import (
	"os"
	"path/filepath"
	"sophia/internal/service"
	"testing"
)

func TestCRMergeJSONConflictStatusAbortFlow(t *testing.T) {
	dir := setupCLIMergeConflictRepo(t)

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected merge conflict error")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "merge_conflict" {
		t.Fatalf("expected merge_conflict code, got %#v", env.Error)
	}
	if env.Error.Details == nil {
		t.Fatalf("expected merge_conflict details")
	}
	if _, ok := env.Error.Details["conflict_files"]; !ok {
		t.Fatalf("expected conflict_files in details, got %#v", env.Error.Details)
	}

	statusOut, _, statusErr := runCLI(t, dir, "cr", "merge", "status", "1", "--json")
	if statusErr != nil {
		t.Fatalf("cr merge status --json error = %v\noutput=%s", statusErr, statusOut)
	}
	statusEnv := decodeEnvelope(t, statusOut)
	if !statusEnv.OK {
		t.Fatalf("expected ok status envelope, got %#v", statusEnv)
	}
	if inProgress, ok := statusEnv.Data["in_progress"].(bool); !ok || !inProgress {
		t.Fatalf("expected in_progress=true, got %#v", statusEnv.Data["in_progress"])
	}

	abortOut, _, abortErr := runCLI(t, dir, "cr", "merge", "abort", "1", "--json")
	if abortErr != nil {
		t.Fatalf("cr merge abort --json error = %v\noutput=%s", abortErr, abortOut)
	}
	abortEnv := decodeEnvelope(t, abortOut)
	if !abortEnv.OK {
		t.Fatalf("expected ok abort envelope, got %#v", abortEnv)
	}

	statusOut, _, statusErr = runCLI(t, dir, "cr", "merge", "status", "1", "--json")
	if statusErr != nil {
		t.Fatalf("cr merge status --json after abort error = %v\noutput=%s", statusErr, statusOut)
	}
	statusEnv = decodeEnvelope(t, statusOut)
	if inProgress, ok := statusEnv.Data["in_progress"].(bool); !ok || inProgress {
		t.Fatalf("expected in_progress=false after abort, got %#v", statusEnv.Data["in_progress"])
	}
}

func TestCRMergeResumeJSONSuccess(t *testing.T) {
	dir := setupCLIMergeConflictRepo(t)

	if _, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--json"); runErr == nil {
		t.Fatalf("expected initial merge conflict")
	}
	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")

	out, _, runErr := runCLI(t, dir, "cr", "merge", "resume", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr merge resume --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if _, ok := env.Data["merged_commit"]; !ok {
		t.Fatalf("expected merged_commit in resume payload, got %#v", env.Data)
	}
}

func TestCRApplyBlockedDuringMergeInProgressReturnsJSONCode(t *testing.T) {
	dir := setupCLIMergeConflictRepo(t)

	if _, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--json"); runErr == nil {
		t.Fatalf("expected initial merge conflict")
	}
	out, _, runErr := runCLI(t, dir, "cr", "apply", "--file", "does-not-matter.yaml", "--json")
	if runErr == nil {
		t.Fatalf("expected cr apply to be blocked during merge in progress")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "merge_in_progress" {
		t.Fatalf("expected merge_in_progress code, got %#v", env.Error)
	}
}

func TestCRMergeAbortJSONReturnsNoMergeInProgressCode(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if _, err := svc.AddCR("No merge in progress", "json error path"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setCLIValidContract(t, svc, 1)

	out, _, runErr := runCLI(t, dir, "cr", "merge", "abort", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected no_merge_in_progress error")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "no_merge_in_progress" {
		t.Fatalf("expected no_merge_in_progress code, got %#v", env.Error)
	}
}

func setupCLIMergeConflictRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "chore: seed conflict")

	cr, err := svc.AddCR("Merge conflict recovery", "exercise conflict flow")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setCLIValidContract(t, svc, 1)

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("from-cr\n"), 0o644); err != nil {
		t.Fatalf("write cr file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "feat: cr side change")

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("from-main\n"), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "feat: main side change")
	runGit(t, dir, "checkout", cr.Branch)

	return dir
}

func setCLIValidContract(t *testing.T, svc *service.Service, crID int) {
	t.Helper()

	scope := []string{"."}
	nonGoals := []string{"No unrelated refactors"}
	invariants := []string{"Existing behavior stays compatible"}
	why := "Deliver scoped intent safely."
	blast := "Limited to this CR branch."
	testPlan := "Run go test ./... and go vet ./..."
	rollback := "Revert merge commit."
	if _, err := svc.SetCRContract(crID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
}
