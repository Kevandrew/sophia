package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestRepairFromGitRebuildsCRsAndRealignsIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Existing intent",
		"-m", "Intent:\nRecovered why\n\nSubtasks:\n- [x] #1 Do thing\n\nNotes:\n- recovered note\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 2\nSophia-CR-UID: cr_fixture-uid-2\nSophia-Base-Ref: release/2026-q1\nSophia-Base-Commit: deadbeefcafebabe\nSophia-Branch: kevandrew/cr-2-existing-intent\nSophia-Branch-Scheme: human_alias_v1\nSophia-Parent-CR: 1\nSophia-Intent: Existing intent\nSophia-Tasks: 1 completed",
	)

	if err := svc.store.SaveIndex(model.Index{NextID: 1}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(svc.store.SophiaDir(), "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.store.SophiaDir(), "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	report, err := svc.RepairFromGit("main", false)
	if err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}
	if report.Imported < 1 || report.HighestCRID < 2 || report.NextID < 3 {
		t.Fatalf("unexpected repair report: %#v", report)
	}

	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.Status != "merged" || repaired.Title != "Existing intent" {
		t.Fatalf("unexpected repaired CR: %#v", repaired)
	}
	if repaired.UID != "cr_fixture-uid-2" {
		t.Fatalf("expected repaired UID from footer, got %#v", repaired.UID)
	}
	if repaired.BaseRef != "release/2026-q1" || repaired.BaseCommit != "deadbeefcafebabe" || repaired.ParentCRID != 1 {
		t.Fatalf("expected repaired base/parent metadata from footers, got %#v", repaired)
	}
	if repaired.Branch != "kevandrew/cr-2-existing-intent" {
		t.Fatalf("expected repaired branch from footer, got %#v", repaired.Branch)
	}
	if len(repaired.Events) == 0 {
		t.Fatalf("expected repaired events to include repair marker, got %#v", repaired.Events)
	}
	lastEvent := repaired.Events[len(repaired.Events)-1]
	if lastEvent.Type != model.EventTypeCRRepaired {
		t.Fatalf("expected last event type %q, got %#v", model.EventTypeCRRepaired, lastEvent)
	}
	if got := strings.TrimSpace(lastEvent.Meta["repair_timestamp_source"]); got != "commit_author_time" {
		t.Fatalf("expected repair_timestamp_source=commit_author_time, got %q event=%#v", got, lastEvent)
	}
	if len(repaired.Notes) != 1 || repaired.Notes[0] != "recovered note" {
		t.Fatalf("unexpected repaired notes: %#v", repaired.Notes)
	}
	if len(repaired.Subtasks) != 1 || repaired.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("unexpected repaired subtasks: %#v", repaired.Subtasks)
	}

	nextCR, err := svc.AddCR("Next intent", "after repair")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if nextCR.ID != 3 {
		t.Fatalf("expected next CR id 3, got %d", nextCR.ID)
	}
}

func TestRepairBackfillsMissingUIDOnExistingCRMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("UID backfill", "repair should set uid")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.UID = ""
	loaded.BaseRef = ""
	loaded.BaseCommit = ""
	loaded.CreatedAt = ""
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(clear uid/base) error = %v", err)
	}

	report, err := svc.RepairFromGit("main", false)
	if err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}

	repaired, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(repaired) error = %v", err)
	}
	if strings.TrimSpace(repaired.UID) == "" {
		t.Fatalf("expected repair to backfill uid, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.BaseRef) == "" || strings.TrimSpace(repaired.BaseCommit) == "" {
		t.Fatalf("expected repair to backfill base metadata, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.CreatedAt) != "" {
		t.Fatalf("expected repair to preserve unknown created_at instead of fabricating timestamp, got %#v", repaired)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected repair warning for preserved unknown chronology, got %#v", report)
	}
	foundChronologyWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning, "empty created_at") {
			foundChronologyWarning = true
			break
		}
	}
	if !foundChronologyWarning {
		t.Fatalf("expected chronology warning in repair report, got %#v", report.Warnings)
	}
}

func TestRepairFromGitRefreshPreservesRichMergedMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-5] Aggregate parent",
		"-m", "Intent:\nRecovered why\n\nSubtasks:\n- [x] #1 Delegated child slice\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 5\nSophia-Intent: Aggregate parent\nSophia-Tasks: 1 completed",
	)

	existing := seedCR(5, "Aggregate parent", seedCROptions{
		UID:         "cr_fixture_rich_005",
		Description: "retain rich merged metadata",
		Status:      model.StatusMerged,
		BaseBranch:  "main",
		BaseRef:     "release/aggregate-parent",
		BaseCommit:  "deadbeefcafebabe",
		Branch:      "kevandrew/cr-5-aggregate-parent",
		ParentCRID:  4,
	})
	existing.MergedAt = "2026-02-17T00:00:00Z"
	existing.MergedBy = "Test User <test@example.com>"
	existing.MergedCommit = "feedface"
	existing.FilesTouchedCount = 7
	existing.Notes = []string{"rich note"}
	exitCode := 0
	existing.Evidence = []model.EvidenceEntry{{
		TS:          "2026-02-16T23:59:00Z",
		Actor:       "Test User <test@example.com>",
		Type:        "command_run",
		Summary:     "targeted tests",
		Command:     "npm test -- repair",
		ExitCode:    &exitCode,
		Attachments: []string{"tmp/repair.log"},
	}}
	existing.Contract = model.Contract{
		Why:                "Preserve aggregate parent metadata during repair refresh.",
		Scope:              []string{"internal/service"},
		NonGoals:           []string{"Rebuild child CRs"},
		Invariants:         []string{"Repair must not discard richer local state."},
		BlastRadius:        "Repair refresh for merged CRs",
		RiskCriticalScopes: []string{"internal/service/service_maintenance.go"},
		RiskTierHint:       "high",
		RiskRationale:      "Losing delegation or PR metadata breaks merged stack forensics.",
		TestPlan:           "go test ./internal/service -run Repair",
		RollbackPlan:       "revert merge",
		UpdatedAt:          "2026-02-16T22:00:00Z",
		UpdatedBy:          "Test User <test@example.com>",
	}
	existing.ContractBaseline = model.CRContractBaseline{
		CapturedAt: "2026-02-16T22:00:00Z",
		CapturedBy: "Test User <test@example.com>",
		Scope:      []string{"internal/service"},
	}
	existing.ContractDrifts = []model.CRContractDrift{{
		ID:          1,
		TS:          "2026-02-16T22:30:00Z",
		Actor:       "Test User <test@example.com>",
		Fields:      []string{"scope"},
		BeforeScope: []string{"internal/service"},
		AfterScope:  []string{"internal/service", "internal/model"},
		Reason:      "Expanded to preserve model metadata too.",
	}}
	existing.DelegationRuns = []model.DelegationRun{{
		ID:         "run-1",
		Status:     model.DelegationRunStatusCompleted,
		CreatedAt:  "2026-02-16T21:00:00Z",
		CreatedBy:  "Test User <test@example.com>",
		UpdatedAt:  "2026-02-16T21:10:00Z",
		FinishedAt: "2026-02-16T21:10:00Z",
		Result: &model.DelegationResult{
			Status:       model.DelegationRunStatusCompleted,
			Summary:      "Child slices completed",
			FilesChanged: []string{"internal/service/service_maintenance.go"},
		},
	}}
	existing.HQ = model.CRHQState{
		RemoteAlias:         "origin",
		RepoID:              "repo-1",
		UpstreamFingerprint: "fingerprint-1",
		LastPullAt:          "2026-02-16T20:00:00Z",
		LastPushAt:          "2026-02-16T20:30:00Z",
	}
	existing.PR = model.CRPRLink{
		Provider:                 "github",
		Repo:                     "acme/sophia",
		Number:                   67,
		URL:                      "https://example.com/pr/67",
		State:                    "merged",
		Draft:                    true,
		LastHeadSHA:              "abc123",
		LastBaseRef:              "main",
		LastBodyHash:             "bodyhash",
		LastSyncedAt:             "2026-02-16T20:45:00Z",
		LastStatusCheckedAt:      "2026-02-16T20:50:00Z",
		LastMergedAt:             "2026-02-17T00:00:00Z",
		LastMergedCommit:         "feedface",
		CheckpointCommentKeys:    []string{"task:1"},
		CheckpointSyncKeys:       []string{"sync:1"},
		AwaitingOpenApproval:     true,
		AwaitingOpenApprovalNote: "approval note",
	}
	task := seedTask(1, "Delegated child slice", model.TaskStatusDone, "Test User <test@example.com>")
	task.CheckpointCommit = "childcommit"
	task.CheckpointAt = "2026-02-16T23:00:00Z"
	task.CheckpointMessage = "feat: delegated child slice"
	task.CheckpointScope = []string{"internal/service/service_maintenance.go"}
	task.CheckpointChunks = []model.CheckpointChunk{{
		ID:       "chk-1",
		Path:     "internal/service/service_maintenance.go",
		OldStart: 10,
		OldLines: 1,
		NewStart: 10,
		NewLines: 3,
	}}
	task.CheckpointReason = "delegated proof"
	task.CheckpointSource = "task_done"
	task.CheckpointSyncAt = "2026-02-16T23:05:00Z"
	task.Delegations = []model.TaskDelegation{{
		ChildCRID:   6,
		ChildCRUID:  "cr_fixture_child_006",
		ChildTaskID: 1,
		LinkedAt:    "2026-02-16T20:15:00Z",
		LinkedBy:    "Test User <test@example.com>",
	}}
	task.Contract = model.TaskContract{
		Intent:             "Preserve delegated task provenance.",
		AcceptanceCriteria: []string{"Delegations remain visible after repair."},
		Scope:              []string{"internal/service/service_maintenance.go"},
		AcceptanceChecks:   []string{"go test ./internal/service -run Repair"},
		UpdatedAt:          "2026-02-16T20:05:00Z",
		UpdatedBy:          "Test User <test@example.com>",
	}
	task.ContractBaseline = model.TaskContractBaseline{
		CapturedAt:         "2026-02-16T20:05:00Z",
		CapturedBy:         "Test User <test@example.com>",
		Intent:             "Preserve delegated task provenance.",
		AcceptanceCriteria: []string{"Delegations remain visible after repair."},
		Scope:              []string{"internal/service/service_maintenance.go"},
		AcceptanceChecks:   []string{"go test ./internal/service -run Repair"},
	}
	task.ContractDrifts = []model.TaskContractDrift{{
		ID:               1,
		TS:               "2026-02-16T20:20:00Z",
		Actor:            "Test User <test@example.com>",
		Fields:           []string{"acceptance_checks"},
		CheckpointCommit: "childcommit",
		Reason:           "Added repair-focused selector.",
	}}
	existing.Subtasks = []model.Subtask{task}
	existing.Events = []model.Event{{
		TS:      "2026-02-16T21:30:00Z",
		Actor:   "Test User <test@example.com>",
		Type:    model.EventTypeTaskDelegated,
		Summary: "Delegated parent task to child CR 6",
		Ref:     "task:1",
	}}
	if err := svc.store.SaveCR(existing); err != nil {
		t.Fatalf("SaveCR(existing) error = %v", err)
	}
	if err := svc.store.SaveIndex(model.Index{NextID: 6}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	if _, err := svc.RepairFromGit("main", true); err != nil {
		t.Fatalf("RepairFromGit(refresh) error = %v", err)
	}

	repaired, err := svc.store.LoadCR(5)
	if err != nil {
		t.Fatalf("LoadCR(5) error = %v", err)
	}
	if repaired.UID != existing.UID || repaired.BaseRef != existing.BaseRef || repaired.BaseCommit != existing.BaseCommit || repaired.ParentCRID != existing.ParentCRID {
		t.Fatalf("expected identity/base metadata preserved, got %#v", repaired)
	}
	if len(repaired.Evidence) != 1 || repaired.Evidence[0].Summary != "targeted tests" {
		t.Fatalf("expected evidence preserved, got %#v", repaired.Evidence)
	}
	if repaired.Contract.Why != existing.Contract.Why || len(repaired.Contract.Scope) != 1 {
		t.Fatalf("expected CR contract preserved, got %#v", repaired.Contract)
	}
	if len(repaired.ContractDrifts) != 1 || repaired.ContractDrifts[0].Reason != "Expanded to preserve model metadata too." {
		t.Fatalf("expected CR contract drifts preserved, got %#v", repaired.ContractDrifts)
	}
	if len(repaired.DelegationRuns) != 1 || repaired.DelegationRuns[0].Result == nil || repaired.DelegationRuns[0].Result.Summary != "Child slices completed" {
		t.Fatalf("expected delegation runs preserved, got %#v", repaired.DelegationRuns)
	}
	if repaired.PR.Number != 67 || repaired.PR.URL != "https://example.com/pr/67" || !repaired.PR.Draft || !repaired.PR.AwaitingOpenApproval {
		t.Fatalf("expected PR linkage preserved, got %#v", repaired.PR)
	}
	if repaired.HQ.RepoID != "repo-1" || repaired.HQ.UpstreamFingerprint != "fingerprint-1" {
		t.Fatalf("expected HQ state preserved, got %#v", repaired.HQ)
	}
	if len(repaired.Subtasks) != 1 {
		t.Fatalf("expected repaired task preserved, got %#v", repaired.Subtasks)
	}
	repairedTask := repaired.Subtasks[0]
	if repairedTask.CheckpointCommit != "childcommit" || len(repairedTask.Delegations) != 1 {
		t.Fatalf("expected delegated checkpoint metadata preserved, got %#v", repairedTask)
	}
	if repairedTask.Contract.Intent != "Preserve delegated task provenance." || len(repairedTask.ContractDrifts) != 1 {
		t.Fatalf("expected task contract metadata preserved, got %#v", repairedTask)
	}
	if len(repaired.Events) < 2 {
		t.Fatalf("expected prior events plus repair marker, got %#v", repaired.Events)
	}
	if repaired.Events[0].Type != model.EventTypeTaskDelegated {
		t.Fatalf("expected prior event preserved, got %#v", repaired.Events)
	}
	if repaired.Events[len(repaired.Events)-1].Type != model.EventTypeCRRepaired {
		t.Fatalf("expected repair event appended, got %#v", repaired.Events)
	}
}

func TestLegacyAndChunkCheckpointMetadataCoexistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Metadata coexistence", "legacy and chunk checkpoint data")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	legacyTask, err := svc.AddTask(cr.ID, "feat: legacy scope task")
	if err != nil {
		t.Fatalf("AddTask(legacy) error = %v", err)
	}
	chunkTask, err := svc.AddTask(cr.ID, "feat: chunk scope task")
	if err != nil {
		t.Fatalf("AddTask(chunk) error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, legacyTask.ID)
	setValidTaskContract(t, svc, cr.ID, chunkTask.ID)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[legacyTask.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[legacyTask.ID-1].CheckpointScope = []string{"internal/service/legacy.go"}

	loaded.Subtasks[chunkTask.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[chunkTask.ID-1].CheckpointScope = nil
	loaded.Subtasks[chunkTask.ID-1].CheckpointChunks = []model.CheckpointChunk{
		{
			ID:       "chk_mixed",
			Path:     "internal/service/chunk.go",
			OldStart: 10,
			OldLines: 1,
			NewStart: 10,
			NewLines: 1,
		},
	}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if len(reloaded.Subtasks[legacyTask.ID-1].CheckpointScope) != 1 {
		t.Fatalf("expected legacy checkpoint_scope preserved, got %#v", reloaded.Subtasks[legacyTask.ID-1])
	}
	if len(reloaded.Subtasks[chunkTask.ID-1].CheckpointChunks) != 1 {
		t.Fatalf("expected checkpoint_chunks preserved, got %#v", reloaded.Subtasks[chunkTask.ID-1])
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !report.Valid {
		t.Fatalf("expected valid report for coexistence fixture, got errors=%#v warnings=%#v", report.Errors, report.Warnings)
	}
}

func TestRepairFromGitLegacyCommitWithoutBaseOrParentFootersStillReconstructs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Legacy intent",
		"-m", "Intent:\nLegacy why\n\nSubtasks:\n- [x] #1 Legacy task\n\nNotes:\n- legacy note\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 2\nSophia-Intent: Legacy intent\nSophia-Tasks: 1 completed",
	)
	if err := os.RemoveAll(filepath.Join(svc.store.SophiaDir(), "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.store.SophiaDir(), "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	if _, err := svc.RepairFromGit("main", false); err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}
	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.ParentCRID != 0 {
		t.Fatalf("expected missing parent footer to default to 0, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.BaseRef) == "" || strings.TrimSpace(repaired.BaseCommit) == "" {
		t.Fatalf("expected base metadata backfilled for legacy repair, got %#v", repaired)
	}
}

func TestNormalizeRepairCommitTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		raw        string
		wantValue  string
		wantSource string
	}{
		{
			name:       "valid timestamp",
			raw:        "2026-03-05T10:15:42+08:00",
			wantValue:  "2026-03-05T10:15:42+08:00",
			wantSource: "commit_author_time",
		},
		{
			name:       "missing timestamp",
			raw:        "",
			wantValue:  "",
			wantSource: "missing",
		},
		{
			name:       "invalid timestamp",
			raw:        "not-a-time",
			wantValue:  "",
			wantSource: "invalid",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gotValue, gotSource := normalizeRepairCommitTimestamp(test.raw)
			if gotValue != test.wantValue || gotSource != test.wantSource {
				t.Fatalf("normalizeRepairCommitTimestamp(%q) = (%q, %q), want (%q, %q)", test.raw, gotValue, gotSource, test.wantValue, test.wantSource)
			}
		})
	}
}
