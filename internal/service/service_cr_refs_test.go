package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRRefLifecycleAddMergeReopen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Ref lifecycle", "track CR ref transitions")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if target := symbolicRefTarget(t, dir, crRefName(cr.ID)); target != "refs/heads/"+cr.Branch {
		t.Fatalf("expected symbolic ref target %q, got %q", "refs/heads/"+cr.Branch, target)
	}
	if target := symbolicRefTarget(t, dir, crUIDRefName(cr.UID)); target != "refs/heads/"+cr.Branch {
		t.Fatalf("expected uid symbolic ref target %q, got %q", "refs/heads/"+cr.Branch, target)
	}

	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "checkpoint for merge")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"ref.txt"})
	if err := os.WriteFile(filepath.Join(dir, "ref.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write ref.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	mergedSHA, err := svc.MergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}
	if _, err := symbolicRefTargetE(dir, crRefName(cr.ID)); err == nil {
		t.Fatalf("expected merged CR ref to be direct, not symbolic")
	}
	if _, err := symbolicRefTargetE(dir, crUIDRefName(cr.UID)); err == nil {
		t.Fatalf("expected merged UID CR ref to be direct, not symbolic")
	}
	refSHA := runGit(t, dir, "rev-parse", crRefName(cr.ID))
	if !strings.HasPrefix(refSHA, mergedSHA) && !strings.HasPrefix(mergedSHA, refSHA) {
		t.Fatalf("expected CR ref %s to point at merged commit %s", refSHA, mergedSHA)
	}
	uidRefSHA := runGit(t, dir, "rev-parse", crUIDRefName(cr.UID))
	if !strings.HasPrefix(uidRefSHA, mergedSHA) && !strings.HasPrefix(mergedSHA, uidRefSHA) {
		t.Fatalf("expected UID CR ref %s to point at merged commit %s", uidRefSHA, mergedSHA)
	}

	if _, err := svc.ReopenCR(cr.ID); err != nil {
		t.Fatalf("ReopenCR() error = %v", err)
	}
	if target := symbolicRefTarget(t, dir, crRefName(cr.ID)); target != "refs/heads/"+cr.Branch {
		t.Fatalf("expected symbolic ref target %q after reopen, got %q", "refs/heads/"+cr.Branch, target)
	}
	if target := symbolicRefTarget(t, dir, crUIDRefName(cr.UID)); target != "refs/heads/"+cr.Branch {
		t.Fatalf("expected uid symbolic ref target %q after reopen, got %q", "refs/heads/"+cr.Branch, target)
	}
}

func TestRepairFromGitSynchronizesAndCleansCRRefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	mergedCR, err := svc.AddCR("Merged ref", "repair sync merged ref")
	if err != nil {
		t.Fatalf("AddCR(merged) error = %v", err)
	}
	setValidContract(t, svc, mergedCR.ID)
	task, err := svc.AddTask(mergedCR.ID, "merged checkpoint")
	if err != nil {
		t.Fatalf("AddTask(merged) error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, mergedCR.ID, task.ID, []string{"merged-ref.txt"})
	if err := os.WriteFile(filepath.Join(dir, "merged-ref.txt"), []byte("merged\n"), 0o644); err != nil {
		t.Fatalf("write merged-ref.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(mergedCR.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(merged) error = %v", err)
	}
	mergedSHA, err := svc.MergeCR(mergedCR.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR(merged) error = %v", err)
	}

	openCR, err := svc.AddCR("Open ref", "repair sync open ref")
	if err != nil {
		t.Fatalf("AddCR(open) error = %v", err)
	}

	if err := runGitErr(dir, "update-ref", crRefName(openCR.ID), mergedSHA); err != nil {
		t.Fatalf("seed direct open ref: %v", err)
	}
	if err := runGitErr(dir, "update-ref", crRefName(999), "HEAD"); err != nil {
		t.Fatalf("seed stale ref: %v", err)
	}
	if err := runGitErr(dir, "update-ref", crUIDRefName("cr_stale-uid"), "HEAD"); err != nil {
		t.Fatalf("seed stale uid ref: %v", err)
	}
	if !gitRefExists(t, dir, crRefName(999)) {
		t.Fatalf("expected stale ref to exist before repair")
	}
	if !gitRefExists(t, dir, crUIDRefName("cr_stale-uid")) {
		t.Fatalf("expected stale uid ref to exist before repair")
	}

	if _, err := svc.RepairFromGit("main", true); err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}

	if !gitRefExists(t, dir, crRefName(mergedCR.ID)) {
		t.Fatalf("expected merged CR ref to exist after repair")
	}
	if !gitRefExists(t, dir, crUIDRefName(mergedCR.UID)) {
		t.Fatalf("expected merged UID CR ref to exist after repair")
	}
	refSHA := runGit(t, dir, "rev-parse", crRefName(mergedCR.ID))
	if !strings.HasPrefix(refSHA, mergedSHA) && !strings.HasPrefix(mergedSHA, refSHA) {
		t.Fatalf("expected merged CR ref %s to point at merged commit %s", refSHA, mergedSHA)
	}

	if target := symbolicRefTarget(t, dir, crRefName(openCR.ID)); target != "refs/heads/"+openCR.Branch {
		t.Fatalf("expected open CR symbolic target %q after repair, got %q", "refs/heads/"+openCR.Branch, target)
	}
	if target := symbolicRefTarget(t, dir, crUIDRefName(openCR.UID)); target != "refs/heads/"+openCR.Branch {
		t.Fatalf("expected open UID CR symbolic target %q after repair, got %q", "refs/heads/"+openCR.Branch, target)
	}
	if gitRefExists(t, dir, crRefName(999)) {
		t.Fatalf("expected stale CR ref to be removed by repair")
	}
	if gitRefExists(t, dir, crUIDRefName("cr_stale-uid")) {
		t.Fatalf("expected stale UID CR ref to be removed by repair")
	}
}

func TestRangeAndRevParseCR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range anchors", "range/rev-parse fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "range-ref.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write range-ref.txt: %v", err)
	}
	runGit(t, dir, "add", "range-ref.txt")
	runGit(t, dir, "commit", "-m", "feat: range anchor commit")

	rangeView, err := svc.RangeCR(cr.ID)
	if err != nil {
		t.Fatalf("RangeCR() error = %v", err)
	}
	if strings.TrimSpace(rangeView.Base) == "" || strings.TrimSpace(rangeView.Head) == "" || strings.TrimSpace(rangeView.MergeBase) == "" {
		t.Fatalf("expected non-empty anchors, got %#v", rangeView)
	}

	headView, err := svc.RevParseCR(cr.ID, "head")
	if err != nil {
		t.Fatalf("RevParseCR(head) error = %v", err)
	}
	if strings.TrimSpace(headView.Commit) != strings.TrimSpace(rangeView.Head) {
		t.Fatalf("expected rev-parse head %q to match range head %q", headView.Commit, rangeView.Head)
	}
	if _, err := svc.RevParseCR(cr.ID, "invalid-kind"); err == nil {
		t.Fatalf("expected invalid kind error")
	}
}

func TestRangeCRFailsButTolerantAnchorsSucceedWhenInProgressBranchMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	result, err := svc.AddCRWithOptions("Missing branch fallback", "tolerant anchors for metadata-only preview", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	cr := result.CR
	if cr == nil {
		t.Fatalf("expected CR payload")
	}

	stored, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	stored.BaseRef = "refs/heads/missing-parent-ref"
	stored.BaseCommit = ""
	stored.UpdatedAt = svc.timestamp()
	if err := svc.store.SaveCR(stored); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	runGit(t, dir, "branch", "-D", cr.Branch)

	if _, err := svc.RangeCR(cr.ID); err == nil {
		t.Fatalf("expected strict RangeCR() to fail when head anchor is missing")
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	resolved, err := svc.resolveCRAnchorsWithOptions(reloaded, CRAnchorResolveOptions{AllowMetadataOnlyHeadFallback: true})
	if err != nil {
		t.Fatalf("resolveCRAnchorsWithOptions() error = %v", err)
	}
	if strings.TrimSpace(resolved.baseCommit) == "" {
		t.Fatalf("expected non-empty base commit in tolerant mode")
	}
	if strings.TrimSpace(resolved.headCommit) != strings.TrimSpace(resolved.baseCommit) {
		t.Fatalf("expected metadata-only head to match base commit, got base=%q head=%q", resolved.baseCommit, resolved.headCommit)
	}
	foundWarning := false
	for _, warning := range resolved.warnings {
		if strings.Contains(strings.ToLower(warning), "metadata-only") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected metadata-only warning, got %#v", resolved.warnings)
	}
}

func symbolicRefTarget(t *testing.T, dir, ref string) string {
	t.Helper()
	target, err := symbolicRefTargetE(dir, ref)
	if err != nil {
		t.Fatalf("symbolicRefTarget(%s): %v", ref, err)
	}
	return target
}

func symbolicRefTargetE(dir, ref string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "--quiet", ref)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func runGitErr(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(raw)))
	}
	return nil
}

func gitRefExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil
}
