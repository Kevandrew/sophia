//go:build integration
// +build integration

package service

import (
	"errors"
	"os"
	"path/filepath"
	"sophia/internal/gitx"
	"strconv"
	"strings"
	"testing"
)

// Integration coverage: worktree behavior requires real git worktree orchestration.
func TestWorktreeSharedLocalMetadataAndCRIDSequence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svcMain := New(dir)
	if _, err := svcMain.Init("main", ""); err != nil {
		t.Fatalf("Init(main) error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	first, err := svcMain.AddCR("Main CR", "from main worktree")
	if err != nil {
		t.Fatalf("AddCR(main) error = %v", err)
	}
	if first.ID != 1 {
		t.Fatalf("expected first CR id 1, got %d", first.ID)
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt2")
	runGit(t, dir, "worktree", "add", wtDir, "-b", "feature/wt2", "main")

	svcWT := New(wtDir)
	crs, err := svcWT.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs(worktree) error = %v", err)
	}
	if len(crs) != 1 || crs[0].ID != 1 {
		t.Fatalf("expected shared metadata to include CR 1, got %#v", crs)
	}

	second, err := svcWT.AddCR("WT CR", "from secondary worktree")
	if err != nil {
		t.Fatalf("AddCR(worktree) error = %v", err)
	}
	if second.ID != 2 {
		t.Fatalf("expected second CR id 2, got %d", second.ID)
	}
}

func TestInitInSecondaryWorktreeDoesNotRequireBaseCheckout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-m", "seed")

	wtDir := filepath.Join(t.TempDir(), "wt-init")
	runGit(t, dir, "worktree", "add", wtDir, "-b", "feature/wt-init", "main")

	svcWT := New(wtDir)
	if _, err := svcWT.Init("main", ""); err != nil {
		t.Fatalf("Init(worktree, base=main) error = %v", err)
	}
}

func TestSwitchCRFailsWithBranchOwnerPathWhenCheckedOutElsewhere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Switch ownership", "ownership test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	runGit(t, dir, "checkout", "main")

	wtDir := filepath.Join(t.TempDir(), "wt-switch")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	_, err = svc.SwitchCR(cr.ID)
	if err == nil || !errors.Is(err, ErrBranchInOtherWorktree) {
		t.Fatalf("expected ErrBranchInOtherWorktree, got %v", err)
	}
	if !strings.Contains(err.Error(), wtDir) {
		t.Fatalf("expected owner worktree path in error, got %v", err)
	}
	var details *BranchInOtherWorktreeError
	if !errors.As(err, &details) {
		t.Fatalf("expected BranchInOtherWorktreeError details, got %T", err)
	}
	if details.CRID != cr.ID {
		t.Fatalf("expected cr id %d, got %d", cr.ID, details.CRID)
	}
	if details.Operation != "cr_switch" {
		t.Fatalf("expected operation cr_switch, got %q", details.Operation)
	}
	if !samePath(details.OwnerWorktreePath, wtDir) {
		t.Fatalf("expected owner worktree path %q, got %q", wtDir, details.OwnerWorktreePath)
	}
	if !strings.Contains(details.SuggestedCommand, "sophia cr switch 1") {
		t.Fatalf("expected suggested switch command, got %q", details.SuggestedCommand)
	}
}

func TestReopenCRFailsWithBranchOwnerPathWhenCheckedOutElsewhere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Reopen ownership", "ownership test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	stored, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	stored.Status = "merged"
	if err := svc.store.SaveCR(stored); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-reopen")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	_, err = svc.ReopenCR(cr.ID)
	if err == nil || !errors.Is(err, ErrBranchInOtherWorktree) {
		t.Fatalf("expected ErrBranchInOtherWorktree, got %v", err)
	}
	var details *BranchInOtherWorktreeError
	if !errors.As(err, &details) {
		t.Fatalf("expected BranchInOtherWorktreeError details, got %T", err)
	}
	if details.CRID != cr.ID {
		t.Fatalf("expected cr id %d, got %d", cr.ID, details.CRID)
	}
	if details.Operation != "cr_reopen" {
		t.Fatalf("expected operation cr_reopen, got %q", details.Operation)
	}
	if !samePath(details.OwnerWorktreePath, wtDir) {
		t.Fatalf("expected owner worktree path %q, got %q", wtDir, details.OwnerWorktreePath)
	}
	if !strings.Contains(details.SuggestedCommand, "sophia cr reopen 1") {
		t.Fatalf("expected suggested reopen command, got %q", details.SuggestedCommand)
	}
}

func TestWhereCRResolvesOwnerWorktreeAcrossLinkedWorktrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svcMain := New(dir)
	if _, err := svcMain.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svcMain.AddCR("Locate owner worktree", "where command coverage")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-where")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	mainView, err := svcMain.WhereCR(cr.ID)
	if err != nil {
		t.Fatalf("WhereCR(main) error = %v", err)
	}
	if !samePath(mainView.OwnerWorktreePath, wtDir) {
		t.Fatalf("expected owner worktree %q, got %q", wtDir, mainView.OwnerWorktreePath)
	}
	if mainView.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current=false in main worktree view")
	}
	if !mainView.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=true in main worktree view")
	}

	svcWT := New(wtDir)
	wtView, err := svcWT.WhereCR(cr.ID)
	if err != nil {
		t.Fatalf("WhereCR(secondary worktree) error = %v", err)
	}
	if !samePath(wtView.OwnerWorktreePath, wtDir) {
		t.Fatalf("expected owner worktree %q, got %q", wtDir, wtView.OwnerWorktreePath)
	}
	if !wtView.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current=true in owner worktree view")
	}
	if wtView.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=false in owner worktree view")
	}
}

func TestStatusCRIncludesWorktreeOwnershipSignals(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Status ownership signals", "status worktree context")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	statusCurrent, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR(current owner) error = %v", err)
	}
	if !statusCurrent.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current_worktree=true on active CR branch")
	}
	if statusCurrent.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=false on active CR branch")
	}
	if strings.TrimSpace(statusCurrent.OwnerWorktreePath) == "" {
		t.Fatalf("expected non-empty owner worktree path on active CR branch")
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-status")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)
	statusOther, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR(other owner) error = %v", err)
	}
	if statusOther.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current_worktree=false when branch owned by another worktree")
	}
	if !statusOther.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=true when branch owned by another worktree")
	}
	if !samePath(statusOther.OwnerWorktreePath, wtDir) {
		t.Fatalf("expected owner worktree path %q, got %q", wtDir, statusOther.OwnerWorktreePath)
	}

	runGit(t, dir, "worktree", "remove", wtDir, "--force")
	statusMissing, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR(branch not checked out) error = %v", err)
	}
	if strings.TrimSpace(statusMissing.OwnerWorktreePath) != "" {
		t.Fatalf("expected empty owner worktree path when branch is not checked out, got %q", statusMissing.OwnerWorktreePath)
	}
	if statusMissing.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current_worktree=false when no owner worktree exists")
	}
	if statusMissing.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=false when no owner worktree exists")
	}
}

func TestMergeCRUsesBaseOwnerWorktreeAndWarnsWhenCRBranchOwnedElsewhere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merge ownership", "merge ownership test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "merge.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write merge.txt: %v", err)
	}
	runGit(t, dir, "add", "merge.txt")
	runGit(t, dir, "commit", "-m", "feat: merge ownership")
	setValidContract(t, svc, cr.ID)

	baseWT := filepath.Join(t.TempDir(), "wt-main-owner")
	runGit(t, dir, "worktree", "add", baseWT, "main")

	sha, warnings, err := svc.MergeCRWithWarnings(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected non-empty merge sha")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected keep-branch warning when CR branch is checked out elsewhere")
	}

	merged, loadErr := svc.store.LoadCR(cr.ID)
	if loadErr != nil {
		t.Fatalf("LoadCR(merged) error = %v", loadErr)
	}
	if merged.Status != "merged" {
		t.Fatalf("expected merged status, got %q", merged.Status)
	}
}

func TestRebaseBranchOntoConflictIncludesCRIDWhenKnown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	cr, err := svc.AddCR("Rebase conflict", "rebase conflict details")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	branch := cr.Branch
	ownerPath := filepath.Join(t.TempDir(), "wt-owner")

	mergeGit := newFakeMergeGit("Test User <test@example.com>", "main")
	mergeGit.worktrees[branch] = &gitx.Worktree{
		Path:   ownerPath,
		Branch: branch,
	}
	ownerGit := newFakeMergeGit("Test User <test@example.com>", "main")
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, nil, func(root string) mergeRuntimeGit {
		if samePath(root, ownerPath) {
			return ownerGit
		}
		return mergeGit
	})

	err = svc.rebaseBranchOnto(branch, "main")
	if err == nil {
		t.Fatalf("expected branch ownership conflict error")
	}
	if !errors.Is(err, ErrBranchInOtherWorktree) {
		t.Fatalf("expected ErrBranchInOtherWorktree, got %v", err)
	}
	var details *BranchInOtherWorktreeError
	if !errors.As(err, &details) {
		t.Fatalf("expected BranchInOtherWorktreeError details, got %T", err)
	}
	if details.CRID != cr.ID {
		t.Fatalf("expected cr_id=%d in conflict details, got %d", cr.ID, details.CRID)
	}
	if details.Operation != "rebase_branch" {
		t.Fatalf("expected operation rebase_branch, got %q", details.Operation)
	}
	if !strings.Contains(details.SuggestedCommand, "sophia cr switch "+strconv.Itoa(cr.ID)) {
		t.Fatalf("expected suggested command to target CR switch, got %q", details.SuggestedCommand)
	}
}

func TestWithWorktreePathPrefixShellQuotesWorktreePath(t *testing.T) {
	t.Parallel()
	suggested := withWorktreePathPrefix("/tmp/wt-$HOME", "sophia cr switch 1")
	if !strings.HasPrefix(suggested, "cd '/tmp/wt-$HOME' && ") {
		t.Fatalf("expected single-quoted worktree path, got %q", suggested)
	}
	if strings.Contains(suggested, `"/tmp/wt-$HOME"`) {
		t.Fatalf("expected no double-quoted shell path, got %q", suggested)
	}
}

func TestBranchResolveCommandShellQuotesBranchSelector(t *testing.T) {
	t.Parallel()
	command := branchResolveCommand("feature/$HOME")
	if command != "sophia cr branch resolve --branch 'feature/$HOME'" {
		t.Fatalf("expected single-quoted branch selector command, got %q", command)
	}
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	aresolved, aerr := filepath.EvalSymlinks(a)
	bresolved, berr := filepath.EvalSymlinks(b)
	if aerr == nil {
		a = aresolved
	}
	if berr == nil {
		b = bresolved
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
