package service

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
)

func TestBuildCRArchiveDocumentIncludesRecoveryPayload(t *testing.T) {
	t.Parallel()

	cr := seedCR(7, "Archive recovery", seedCROptions{
		UID:         "cr_archive_recovery_007",
		Description: "round trip merged metadata",
		Status:      model.StatusMerged,
		BaseBranch:  "main",
		BaseRef:     "release/archive",
		BaseCommit:  "deadbeef",
		Branch:      "cr-7-archive-recovery",
		ParentCRID:  3,
	})
	cr.Evidence = []model.EvidenceEntry{{Summary: "evidence"}}
	cr.DelegationRuns = []model.DelegationRun{{ID: "run-1", Status: model.DelegationRunStatusCompleted}}
	cr.Contract = model.Contract{Why: "Preserve recovery payload."}
	cr.ContractBaseline = model.CRContractBaseline{CapturedAt: "2026-02-20T00:00:00Z", Scope: []string{"internal/service"}}
	cr.ContractDrifts = []model.CRContractDrift{{ID: 1, Reason: "scope drift"}}
	cr.Subtasks = []model.Subtask{{
		ID:          1,
		Title:       "Delegated slice",
		Status:      model.TaskStatusDone,
		CreatedAt:   harnessTimestamp,
		UpdatedAt:   harnessTimestamp,
		CreatedBy:   "Test User <test@example.com>",
		CompletedAt: harnessTimestamp,
		CompletedBy: "Test User <test@example.com>",
		Delegations: []model.TaskDelegation{{ChildCRID: 8, ChildTaskID: 1}},
		Contract:    model.TaskContract{Intent: "Keep task metadata."},
	}}
	cr.HQ = model.CRHQState{RepoID: "repo-1"}
	cr.PR = model.CRPRLink{Number: 42, URL: "https://example.com/pr/42"}
	cr.MergedAt = "2026-02-20T01:00:00Z"
	cr.MergedBy = "Test User <test@example.com>"
	cr.MergedCommit = "cafebabe"

	archive := buildCRArchiveDocument(cr, 1, "", "2026-02-20T01:00:00Z", model.PolicyArchive{}, model.CRArchiveGitSummary{
		FilesChanged: []string{"internal/service/service_archive.go"},
		DiffStat:     model.CRArchiveDiffStat{Summary: "1 file changed"},
	}, nil)
	if archive.Recovery == nil {
		t.Fatalf("expected recovery payload in archive")
	}
	recovered, err := decodeCRArchiveRecovery(&archive)
	if err != nil {
		t.Fatalf("decodeCRArchiveRecovery() error = %v", err)
	}
	if recovered == nil {
		t.Fatalf("expected decoded recovery CR")
	}
	if recovered.UID != cr.UID || recovered.ParentCRID != cr.ParentCRID {
		t.Fatalf("expected identity fields to round-trip, got %#v", recovered)
	}
	if len(recovered.DelegationRuns) != 1 || recovered.DelegationRuns[0].ID != "run-1" {
		t.Fatalf("expected delegation runs to round-trip, got %#v", recovered.DelegationRuns)
	}
	if recovered.PR.Number != 42 || recovered.HQ.RepoID != "repo-1" {
		t.Fatalf("expected PR/HQ metadata to round-trip, got pr=%#v hq=%#v", recovered.PR, recovered.HQ)
	}
	if len(recovered.Subtasks) != 1 || len(recovered.Subtasks[0].Delegations) != 1 {
		t.Fatalf("expected delegated task metadata to round-trip, got %#v", recovered.Subtasks)
	}
}

func TestRepairFromGitUsesArchiveRecoveryForMergedCR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Archive recovered",
		"-m", "Intent:\nThin merge intent\n\nSubtasks:\n- [x] #1 Thin task\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-20T02:00:00Z\n\nSophia-CR: 2\nSophia-Intent: Archive recovered\nSophia-Tasks: 1 completed",
	)

	rich := seedCR(2, "Archive recovered", seedCROptions{
		UID:         "cr_archive_recovered_002",
		Description: "recover from tracked archive",
		Status:      model.StatusMerged,
		BaseBranch:  "main",
		BaseRef:     "release/archive-recovery",
		BaseCommit:  "deadbeefcafebabe",
		Branch:      "cr-2-archive-recovered",
		ParentCRID:  1,
	})
	rich.Contract = model.Contract{Why: "Recover merged CRs from archive snapshots."}
	rich.ContractDrifts = []model.CRContractDrift{{ID: 1, Reason: "scope expanded"}}
	rich.Evidence = []model.EvidenceEntry{{Summary: "repair evidence"}}
	rich.DelegationRuns = []model.DelegationRun{{ID: "run-archive", Status: model.DelegationRunStatusCompleted}}
	rich.HQ = model.CRHQState{RepoID: "repo-archive"}
	rich.PR = model.CRPRLink{Number: 99, URL: "https://example.com/pr/99"}
	rich.Subtasks = []model.Subtask{{
		ID:               1,
		Title:            "Thin task",
		Status:           model.TaskStatusDone,
		CreatedAt:        harnessTimestamp,
		UpdatedAt:        harnessTimestamp,
		CreatedBy:        "Test User <test@example.com>",
		CompletedAt:      harnessTimestamp,
		CompletedBy:      "Test User <test@example.com>",
		CheckpointCommit: "archivechild",
		Delegations:      []model.TaskDelegation{{ChildCRID: 3, ChildTaskID: 1}},
		Contract:         model.TaskContract{Intent: "Preserve task contract."},
		ContractDrifts:   []model.TaskContractDrift{{ID: 1, Reason: "Added repair selector"}},
	}}
	rich.MergedAt = "2026-02-20T02:00:00Z"
	rich.MergedBy = "Test User <test@example.com>"

	archive := buildCRArchiveDocument(rich, 1, "", "2026-02-20T02:01:00Z", model.PolicyArchive{}, model.CRArchiveGitSummary{
		FilesChanged: []string{"internal/service/service_maintenance.go"},
		DiffStat:     model.CRArchiveDiffStat{Summary: "1 file changed"},
	}, nil)
	payload, err := marshalCRArchiveYAML(archive)
	if err != nil {
		t.Fatalf("marshalCRArchiveYAML() error = %v", err)
	}
	archivePath := filepath.Join(dir, defaultArchivePath, "cr-2.v1.yaml")
	if err := writeArchivePayload(archivePath, payload); err != nil {
		t.Fatalf("writeArchivePayload() error = %v", err)
	}

	if err := os.RemoveAll(filepath.Join(svc.store.SophiaDir(), "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.store.SophiaDir(), "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	if _, err := svc.RepairFromGit("main", true); err != nil {
		t.Fatalf("RepairFromGit(refresh) error = %v", err)
	}

	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.UID != rich.UID || repaired.BaseRef != rich.BaseRef || repaired.ParentCRID != rich.ParentCRID {
		t.Fatalf("expected archive recovery identity/base fields, got %#v", repaired)
	}
	if repaired.Contract.Why != rich.Contract.Why || len(repaired.ContractDrifts) != 1 {
		t.Fatalf("expected contract recovery from archive, got %#v %#v", repaired.Contract, repaired.ContractDrifts)
	}
	if len(repaired.Evidence) != 1 || repaired.Evidence[0].Summary != "repair evidence" {
		t.Fatalf("expected evidence recovery from archive, got %#v", repaired.Evidence)
	}
	if len(repaired.DelegationRuns) != 1 || repaired.DelegationRuns[0].ID != "run-archive" {
		t.Fatalf("expected delegation runs recovery from archive, got %#v", repaired.DelegationRuns)
	}
	if repaired.PR.Number != 99 || repaired.HQ.RepoID != "repo-archive" {
		t.Fatalf("expected PR/HQ recovery from archive, got pr=%#v hq=%#v", repaired.PR, repaired.HQ)
	}
	if len(repaired.Subtasks) != 1 || repaired.Subtasks[0].CheckpointCommit != "archivechild" || len(repaired.Subtasks[0].Delegations) != 1 || len(repaired.Subtasks[0].ContractDrifts) != 1 {
		t.Fatalf("expected rich task recovery from archive, got %#v", repaired.Subtasks)
	}
}
