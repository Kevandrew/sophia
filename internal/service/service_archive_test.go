package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

func TestMergeWritesArchiveFileIntoMergeCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeArchivePolicyEnabledForTest(t, dir, true)

	cr, err := svc.AddCR("Archive merge", "archive merge integration")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "archive.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write archive.txt: %v", err)
	}
	runGit(t, dir, "add", "archive.txt")
	runGit(t, dir, "commit", "-m", "feat: archive merge fixture")

	sha, warnings, err := svc.MergeCRWithWarnings(cr.ID, true, "")
	if err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected merge sha")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected merge warnings: %#v", warnings)
	}
	changed := runGit(t, dir, "show", "--name-only", "--pretty=format:", sha)
	if !strings.Contains(changed, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected archive file in merge commit, changed files:\n%s", changed)
	}
	archiveBody := runGit(t, dir, "show", sha+":.sophia-tracked/cr/cr-1.v1.yaml")
	if !strings.Contains(archiveBody, "schema_version: sophia.cr_archive.v1") {
		t.Fatalf("expected archive schema version in archive yaml:\n%s", archiveBody)
	}
	if !strings.Contains(archiveBody, "archive.txt") {
		t.Fatalf("expected changed file in archive git summary:\n%s", archiveBody)
	}
	if strings.Contains(archiveBody, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected archive git summary to exclude archive paths:\n%s", archiveBody)
	}
}

func TestMergeWritesArchiveWhenBaseBranchOwnedByOtherWorktree(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeArchivePolicyEnabledForTest(t, dir, true)

	cr, err := svc.AddCR("Archive merge with worktree", "archive merge integration")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "archive-worktree.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write archive-worktree.txt: %v", err)
	}
	runGit(t, dir, "add", "archive-worktree.txt")
	runGit(t, dir, "commit", "-m", "feat: archive merge worktree fixture")

	mainWorktreeDir := filepath.Join(t.TempDir(), "main-worktree")
	runGit(t, dir, "worktree", "add", mainWorktreeDir, "main")

	sha, warnings, err := svc.MergeCRWithWarnings(cr.ID, true, "")
	if err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected merge sha")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected merge warnings: %#v", warnings)
	}
	changed := runGit(t, dir, "show", "--name-only", "--pretty=format:", sha)
	if !strings.Contains(changed, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected archive file in merge commit, changed files:\n%s", changed)
	}
}

func TestResumeWritesArchiveOnlyAfterConflictsResolved(t *testing.T) {
	svc, cr, dir := setupMergeConflictScenario(t)
	writeArchivePolicyEnabledForTest(t, dir, true)

	_, _, err := svc.MergeCRWithWarnings(cr.ID, false, "")
	if err == nil {
		t.Fatalf("expected merge conflict")
	}
	if !errors.Is(err, ErrMergeConflict) {
		t.Fatalf("expected ErrMergeConflict, got %v", err)
	}
	archivePath := filepath.Join(dir, ".sophia-tracked", "cr", "cr-1.v1.yaml")
	if _, statErr := os.Stat(archivePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected archive file to be absent before resume, stat err=%v", statErr)
	}

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	sha, warnings, err := svc.ResumeMergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("ResumeMergeCR() error = %v", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected resumed merge sha")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected resume warnings: %#v", warnings)
	}
	changed := runGit(t, dir, "show", "--name-only", "--pretty=format:", sha)
	if !strings.Contains(changed, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected archive file in resumed merge commit, changed files:\n%s", changed)
	}
}

func TestResumeReusesExistingArchiveRevisionAfterCommitFailure(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeArchivePolicyEnabledForTest(t, dir, true)

	cr, err := svc.AddCR("Archive resume idempotency", "archive merge integration")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "archive-retry.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write archive-retry.txt: %v", err)
	}
	runGit(t, dir, "add", "archive-retry.txt")
	runGit(t, dir, "commit", "-m", "feat: archive retry fixture")

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho 'reject commit' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing pre-commit hook: %v", err)
	}
	_, _, mergeErr := svc.MergeCRWithWarnings(cr.ID, false, "")
	if mergeErr == nil {
		t.Fatalf("expected merge to fail due to pre-commit hook")
	}
	if errors.Is(mergeErr, ErrMergeConflict) {
		t.Fatalf("unexpected merge conflict error: %v", mergeErr)
	}
	status, statusErr := svc.MergeStatusCR(cr.ID)
	if statusErr != nil {
		t.Fatalf("MergeStatusCR() error = %v", statusErr)
	}
	if !status.InProgress {
		t.Fatalf("expected merge to remain in progress after commit failure")
	}
	if len(status.ConflictFiles) != 0 {
		t.Fatalf("expected no conflicts after commit failure, got %#v", status.ConflictFiles)
	}

	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write passing pre-commit hook: %v", err)
	}
	sha, warnings, resumeErr := svc.ResumeMergeCR(cr.ID, false, "")
	if resumeErr != nil {
		t.Fatalf("ResumeMergeCR() error = %v", resumeErr)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected resumed merge sha")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected resume warnings: %#v", warnings)
	}
	changed := runGit(t, dir, "show", "--name-only", "--pretty=format:", sha)
	if !strings.Contains(changed, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected v1 archive in resumed merge commit, changed files:\n%s", changed)
	}
	if strings.Contains(changed, ".sophia-tracked/cr/cr-1.v2.yaml") {
		t.Fatalf("did not expect v2 archive in resumed merge commit, changed files:\n%s", changed)
	}
}

func TestBackfillCreatesMissingV1ArchivesInOneCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeArchivePolicyEnabledForTest(t, dir, false)

	for i := 1; i <= 2; i++ {
		cr, err := svc.AddCR("Backfill fixture", "archive backfill fixture")
		if err != nil {
			t.Fatalf("AddCR(%d) error = %v", i, err)
		}
		setValidContract(t, svc, cr.ID)
		file := filepath.Join(dir, "backfill-"+string(rune('a'+i-1))+".txt")
		if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
		runGit(t, dir, "add", filepath.Base(file))
		runGit(t, dir, "commit", "-m", "feat: backfill fixture")
		if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err != nil {
			t.Fatalf("MergeCRWithWarnings(%d) error = %v", i, err)
		}
	}

	beforeCount := runGit(t, dir, "rev-list", "--count", "main")
	dryRun, err := svc.BackfillCRArchives(CRArchiveBackfillOptions{Commit: false})
	if err != nil {
		t.Fatalf("BackfillCRArchives(dry-run) error = %v", err)
	}
	if !dryRun.DryRun {
		t.Fatalf("expected dry_run=true")
	}
	if len(dryRun.MissingCRIDs) != 2 {
		t.Fatalf("expected 2 missing archives, got %#v", dryRun.MissingCRIDs)
	}

	view, err := svc.BackfillCRArchives(CRArchiveBackfillOptions{Commit: true})
	if err != nil {
		t.Fatalf("BackfillCRArchives(commit) error = %v", err)
	}
	if !view.Committed || strings.TrimSpace(view.CommitSHA) == "" {
		t.Fatalf("expected commit info, got %#v", view)
	}
	if len(view.WrittenPaths) != 2 {
		t.Fatalf("expected 2 written archive paths, got %#v", view.WrittenPaths)
	}
	afterCount := runGit(t, dir, "rev-list", "--count", "main")
	beforeN := strings.TrimSpace(beforeCount)
	afterN := strings.TrimSpace(afterCount)
	if beforeN == afterN {
		t.Fatalf("expected one additional commit from backfill, before=%s after=%s", beforeN, afterN)
	}
	for _, path := range view.WrittenPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected written archive at %s: %v", path, err)
		}
	}
}

func TestBackfillCommitRequiresBaseBranch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeArchivePolicyEnabledForTest(t, dir, false)

	cr, err := svc.AddCR("Backfill branch guard", "archive backfill fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "backfill-guard.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	runGit(t, dir, "add", "backfill-guard.txt")
	runGit(t, dir, "commit", "-m", "feat: backfill guard fixture")
	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}

	runGit(t, dir, "checkout", "-b", "not-main")
	headBefore := runGit(t, dir, "rev-parse", "HEAD")
	_, backfillErr := svc.BackfillCRArchives(CRArchiveBackfillOptions{Commit: true})
	if backfillErr == nil {
		t.Fatalf("expected backfill commit guard error on non-base branch")
	}
	if !strings.Contains(backfillErr.Error(), "must run on base branch") {
		t.Fatalf("expected base branch guard error, got %v", backfillErr)
	}
	headAfter := runGit(t, dir, "rev-parse", "HEAD")
	if strings.TrimSpace(headBefore) != strings.TrimSpace(headAfter) {
		t.Fatalf("expected no commit on non-base branch, before=%s after=%s", headBefore, headAfter)
	}
	archivePath := filepath.Join(dir, ".sophia-tracked", "cr", "cr-1.v1.yaml")
	if _, statErr := os.Stat(archivePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive file written on guard failure, stat err=%v", statErr)
	}
}

func TestArchiveDocumentEncodingIsDeterministic(t *testing.T) {
	cr := &model.CR{
		ID:          7,
		UID:         "cr_test",
		Title:       "Deterministic archive",
		Description: "Fixture",
		Status:      model.StatusMerged,
		BaseBranch:  "main",
		BaseRef:     "main",
		BaseCommit:  "abc123",
		Branch:      "cr-7-deterministic",
		MergedAt:    "2026-02-24T00:00:00Z",
		MergedBy:    "Test User <test@example.com>",
		Contract: model.Contract{
			Scope:      []string{"internal/service", "cmd", "cmd"},
			NonGoals:   []string{"z", "a"},
			Invariants: []string{"b", "a"},
		},
		Subtasks: []model.Subtask{
			{
				ID:     2,
				Title:  "B",
				Status: model.TaskStatusOpen,
				Contract: model.TaskContract{
					Scope:            []string{"b", "a"},
					AcceptanceChecks: []string{"z", "a"},
				},
			},
			{
				ID:     1,
				Title:  "A",
				Status: model.TaskStatusDone,
				Contract: model.TaskContract{
					Scope:            []string{"d", "c"},
					AcceptanceChecks: []string{"x"},
				},
			},
		},
	}
	summary := buildArchiveGitSummary(
		[]gitx.FileChange{
			{Path: "b.txt"},
			{Path: ".sophia-tracked/cr/cr-7.v1.yaml"},
			{Path: "a.txt"},
		},
		[]gitx.DiffNumStat{
			{Path: "b.txt", Insertions: intPtr(1), Deletions: intPtr(2)},
			{Path: "a.txt", Insertions: intPtr(3), Deletions: intPtr(4)},
		},
		"base",
		"head",
	)
	archiveA := buildCRArchiveDocument(cr, 1, "", "2026-02-24T00:00:00Z", summary)
	archiveB := buildCRArchiveDocument(cr, 1, "", "2026-02-24T00:00:00Z", summary)
	yamlA, err := marshalCRArchiveYAML(archiveA)
	if err != nil {
		t.Fatalf("marshal archive A: %v", err)
	}
	yamlB, err := marshalCRArchiveYAML(archiveB)
	if err != nil {
		t.Fatalf("marshal archive B: %v", err)
	}
	if string(yamlA) != string(yamlB) {
		t.Fatalf("expected deterministic YAML encoding")
	}
	if !strings.Contains(string(yamlA), "files_changed:") {
		t.Fatalf("expected files_changed in yaml:\n%s", string(yamlA))
	}
	idxA := strings.Index(string(yamlA), "- a.txt")
	idxB := strings.Index(string(yamlA), "- b.txt")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("expected sorted files_changed list:\n%s", string(yamlA))
	}
}

func writeArchivePolicyEnabledForTest(t *testing.T, dir string, enabled bool) {
	t.Helper()
	content := "version: v1\narchive:\n  enabled: false\n"
	if enabled {
		content = "version: v1\narchive:\n  enabled: true\n"
	}
	if err := os.WriteFile(filepath.Join(dir, repoPolicyFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write archive policy fixture: %v", err)
	}
	runGit(t, dir, "add", repoPolicyFileName)
	runGit(t, dir, "commit", "-m", "chore: configure archive policy for test")
}
