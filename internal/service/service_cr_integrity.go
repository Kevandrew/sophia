package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

const reconcileCommitScanLimit = 5000

func (s *Service) DoctorCR(id int) (*CRDoctorReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return s.buildCRDoctorReport(cr)
}

func (s *Service) ReconcileCR(id int, opts ReconcileCROptions) (*ReconcileCRReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	now := s.timestamp()
	actor := s.git.Actor()

	branchExists := s.git.BranchExists(cr.Branch)
	scanRef := reconcileScanRef(cr, branchExists)
	taskCommits, scanned, err := s.scanTaskFooterCommits(cr, scanRef, reconcileCommitScanLimit)
	if err != nil {
		return nil, err
	}

	report := &ReconcileCRReport{
		CRID:             cr.ID,
		CRUID:            strings.TrimSpace(cr.UID),
		Branch:           cr.Branch,
		BranchExists:     branchExists,
		PreviousParentID: cr.ParentCRID,
		CurrentParentID:  cr.ParentCRID,
		ScanRef:          scanRef,
		ScannedCommits:   scanned,
		Warnings:         []string{},
		TaskResults:      make([]ReconcileTaskResult, 0, len(cr.Subtasks)),
	}

	changed := false
	expectedParentID := expectedParentCRIDFromBaseRef(strings.TrimSpace(cr.BaseRef), cr.ID)
	if expectedParentID > 0 && expectedParentID != cr.ParentCRID {
		if _, parentErr := s.store.LoadCR(expectedParentID); parentErr == nil {
			cr.ParentCRID = expectedParentID
			report.CurrentParentID = expectedParentID
			report.ParentRelinked = true
			changed = true
		} else {
			report.Warnings = append(report.Warnings, fmt.Sprintf("unable to relink parent_cr_id from base_ref %q: parent CR %d is unavailable", cr.BaseRef, expectedParentID))
		}
	}

	for i := range cr.Subtasks {
		task := &cr.Subtasks[i]
		prev := checkpointSnapshot(task)
		result := ReconcileTaskResult{
			TaskID:           task.ID,
			Title:            task.Title,
			Status:           task.Status,
			PreviousCommit:   prev.Commit,
			CurrentCommit:    prev.Commit,
			Source:           prev.Source,
			CheckpointAt:     prev.At,
			CheckpointOrphan: prev.Orphan,
			Action:           "unchanged",
		}

		needsOrphan, orphanReason := s.taskNeedsCheckpointOrphan(cr, task, branchExists)
		if entry, ok := taskCommits[task.ID]; ok {
			candidateHash := strings.TrimSpace(entry.Hash)
			if candidateHash != "" && (prev.Commit == "" || needsOrphan) {
				task.CheckpointCommit = candidateHash
				task.CheckpointAt = nonEmptyTrimmed(entry.When, task.CheckpointAt)
				task.CheckpointMessage = nonEmptyTrimmed(entry.Subject, task.CheckpointMessage)
				task.CheckpointSource = "footer_scan"
				task.CheckpointSyncAt = now
				task.CheckpointOrphan = false
				task.CheckpointReason = ""

				result.Action = "relinked"
				result.CurrentCommit = strings.TrimSpace(task.CheckpointCommit)
				result.Source = task.CheckpointSource
				result.CheckpointAt = task.CheckpointAt
				result.CheckpointOrphan = false
				report.Relinked++
				if prev.Orphan || strings.TrimSpace(prev.Reason) != "" {
					report.ClearedOrphans++
				}
			}
		}

		if result.Action != "relinked" {
			if needsOrphan {
				task.CheckpointOrphan = true
				task.CheckpointReason = orphanReason
				task.CheckpointSyncAt = now
				if strings.TrimSpace(task.CheckpointSource) == "" {
					task.CheckpointSource = "reconcile"
				}
				result.Action = "orphaned"
				result.Reason = orphanReason
				result.Source = task.CheckpointSource
				result.CurrentCommit = strings.TrimSpace(task.CheckpointCommit)
				result.CheckpointOrphan = true
				report.Orphaned++
			} else if prev.Orphan || strings.TrimSpace(prev.Reason) != "" {
				task.CheckpointOrphan = false
				task.CheckpointReason = ""
				task.CheckpointSyncAt = now
				if strings.TrimSpace(task.CheckpointSource) == "" {
					task.CheckpointSource = "reconcile"
				}
				result.Action = "cleared_orphan"
				result.Source = task.CheckpointSource
				result.CurrentCommit = strings.TrimSpace(task.CheckpointCommit)
				result.CheckpointOrphan = false
				report.ClearedOrphans++
			}
		}

		if checkpointChanged(prev, task) {
			changed = true
		}
		report.TaskResults = append(report.TaskResults, result)
	}

	if opts.Regenerate {
		diff, diffErr := s.summarizeCRDiff(cr)
		if diffErr != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("unable to regenerate derived diff metadata: %v", diffErr))
		} else {
			report.Regenerated = true
			report.FilesChanged = len(diff.Files)
			report.DiffStat = diff.ShortStat
			if cr.FilesTouchedCount != len(diff.Files) {
				cr.FilesTouchedCount = len(diff.Files)
				changed = true
			}
		}
	}

	postReport, doctorErr := s.buildCRDoctorReport(cr)
	if doctorErr != nil {
		return nil, doctorErr
	}
	report.Findings = append(report.Findings, postReport.Findings...)

	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_reconciled",
		Summary: fmt.Sprintf("Reconciled CR %d checkpoint integrity (relinked=%d orphaned=%d cleared=%d parent_relinked=%t)", cr.ID, report.Relinked, report.Orphaned, report.ClearedOrphans, report.ParentRelinked),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"scan_ref":           report.ScanRef,
			"scanned_commits":    strconv.Itoa(report.ScannedCommits),
			"relinked":           strconv.Itoa(report.Relinked),
			"orphaned":           strconv.Itoa(report.Orphaned),
			"cleared_orphans":    strconv.Itoa(report.ClearedOrphans),
			"regenerated":        strconv.FormatBool(report.Regenerated),
			"changed":            strconv.FormatBool(changed),
			"findings":           strconv.Itoa(len(report.Findings)),
			"parent_relinked":    strconv.FormatBool(report.ParentRelinked),
			"previous_parent_id": strconv.Itoa(report.PreviousParentID),
			"current_parent_id":  strconv.Itoa(report.CurrentParentID),
		},
	})
	cr.UpdatedAt = now
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) buildCRDoctorReport(cr *model.CR) (*CRDoctorReport, error) {
	if cr == nil {
		return nil, fmt.Errorf("cr cannot be nil")
	}
	report := &CRDoctorReport{
		CRID:         cr.ID,
		CRUID:        strings.TrimSpace(cr.UID),
		Branch:       cr.Branch,
		BaseRef:      strings.TrimSpace(cr.BaseRef),
		BaseCommit:   strings.TrimSpace(cr.BaseCommit),
		ParentCRID:   cr.ParentCRID,
		Findings:     []CRDoctorFinding{},
		BranchExists: s.git.BranchExists(cr.Branch),
	}
	if report.BranchExists {
		head, err := s.git.ResolveRef(cr.Branch)
		if err != nil {
			return nil, err
		}
		report.BranchHead = strings.TrimSpace(head)
	}
	if report.BaseRef != "" {
		if resolved, err := s.git.ResolveRef(report.BaseRef); err == nil {
			report.ResolvedBaseRef = strings.TrimSpace(resolved)
		} else {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "base_ref_unresolved",
				Message: fmt.Sprintf("base ref %q cannot be resolved: %v", report.BaseRef, err),
			})
		}
	}

	report.ExpectedParentID = expectedParentCRIDFromBaseRef(report.BaseRef, cr.ID)
	if report.ExpectedParentID > 0 && cr.ParentCRID != report.ExpectedParentID {
		report.Findings = append(report.Findings, CRDoctorFinding{
			Code:    "parent_base_ref_mismatch",
			Message: fmt.Sprintf("base_ref %q implies parent CR %d but parent_cr_id is %d", report.BaseRef, report.ExpectedParentID, cr.ParentCRID),
		})
	}
	if cr.ParentCRID > 0 {
		parent, err := s.store.LoadCR(cr.ParentCRID)
		if err != nil {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "parent_cr_missing",
				Message: fmt.Sprintf("parent CR %d metadata is missing", cr.ParentCRID),
			})
		} else if parent.Status == model.StatusInProgress && s.git.BranchExists(parent.Branch) && strings.TrimSpace(cr.BaseRef) != strings.TrimSpace(parent.Branch) {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "parent_base_ref_mismatch",
				Message: fmt.Sprintf("parent CR %d is in progress on %q but base_ref is %q", parent.ID, parent.Branch, cr.BaseRef),
			})
		}
	}

	if report.ResolvedBaseRef != "" && report.BaseCommit != "" && report.ResolvedBaseRef != report.BaseCommit {
		report.Findings = append(report.Findings, CRDoctorFinding{
			Code:    "base_commit_drift",
			Commit:  report.BaseCommit,
			Message: fmt.Sprintf("base_ref %q resolves to %s but recorded base_commit is %s", report.BaseRef, shortHash(report.ResolvedBaseRef), shortHash(report.BaseCommit)),
		})
	}
	if report.BranchExists && report.BaseCommit != "" {
		reachable, err := s.git.IsAncestor(report.BaseCommit, cr.Branch)
		if err != nil {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "base_commit_unreachable",
				Commit:  report.BaseCommit,
				Message: fmt.Sprintf("unable to verify base commit reachability: %v", err),
			})
		} else if !reachable {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "base_commit_unreachable",
				Commit:  report.BaseCommit,
				Message: fmt.Sprintf("base_commit %s is not reachable from branch %q", shortHash(report.BaseCommit), cr.Branch),
			})
		}
	}

	commitOwners := map[string][]int{}
	commitCache := map[string]*gitx.Commit{}
	for _, task := range cr.Subtasks {
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit != "" {
			commitOwners[commit] = append(commitOwners[commit], task.ID)
		}

		if task.Status == model.TaskStatusDone && commit == "" {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "done_task_missing_checkpoint",
				TaskID:  task.ID,
				Message: fmt.Sprintf("task #%d is done without checkpoint_commit", task.ID),
			})
			continue
		}
		if commit == "" {
			continue
		}

		commitObj, ok := commitCache[commit]
		if !ok {
			resolved, commitErr := s.git.CommitByHash(commit)
			if commitErr != nil {
				report.Findings = append(report.Findings, CRDoctorFinding{
					Code:    "checkpoint_commit_missing",
					TaskID:  task.ID,
					Commit:  commit,
					Message: fmt.Sprintf("task #%d checkpoint_commit %s cannot be resolved", task.ID, shortHash(commit)),
				})
				continue
			}
			commitObj = resolved
			commitCache[commit] = resolved
		}

		if report.BranchExists {
			reachable, reachErr := s.git.IsAncestor(commit, cr.Branch)
			if reachErr != nil {
				report.Findings = append(report.Findings, CRDoctorFinding{
					Code:    "checkpoint_unreachable",
					TaskID:  task.ID,
					Commit:  commit,
					Message: fmt.Sprintf("unable to verify checkpoint reachability for task #%d: %v", task.ID, reachErr),
				})
			} else if !reachable {
				report.Findings = append(report.Findings, CRDoctorFinding{
					Code:    "checkpoint_unreachable",
					TaskID:  task.ID,
					Commit:  commit,
					Message: fmt.Sprintf("task #%d checkpoint_commit %s is not reachable from %q", task.ID, shortHash(commit), cr.Branch),
				})
			}
		}

		footerTaskID := taskIDFromBody(commitObj.Body)
		if footerTaskID > 0 && footerTaskID != task.ID {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "checkpoint_task_mismatch",
				TaskID:  task.ID,
				Commit:  commit,
				Message: fmt.Sprintf("task #%d references checkpoint_commit %s but commit footer says task #%d", task.ID, shortHash(commit), footerTaskID),
			})
		}
		commitUID := strings.TrimSpace(crUIDFromBody(commitObj.Body))
		if commitUID != "" && report.CRUID != "" && commitUID != report.CRUID {
			report.Findings = append(report.Findings, CRDoctorFinding{
				Code:    "checkpoint_uid_mismatch",
				TaskID:  task.ID,
				Commit:  commit,
				Message: fmt.Sprintf("task #%d checkpoint_commit %s has Sophia-CR-UID %q but CR UID is %q", task.ID, shortHash(commit), commitUID, report.CRUID),
			})
		}
	}

	for commit, owners := range commitOwners {
		if len(owners) <= 1 {
			continue
		}
		sort.Ints(owners)
		taskRefs := make([]string, 0, len(owners))
		for _, taskID := range owners {
			taskRefs = append(taskRefs, fmt.Sprintf("#%d", taskID))
		}
		report.Findings = append(report.Findings, CRDoctorFinding{
			Code:    "duplicate_checkpoint_commit",
			Commit:  commit,
			Message: fmt.Sprintf("checkpoint_commit %s is associated with multiple tasks: %s", shortHash(commit), strings.Join(taskRefs, ", ")),
		})
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Code != report.Findings[j].Code {
			return report.Findings[i].Code < report.Findings[j].Code
		}
		if report.Findings[i].TaskID != report.Findings[j].TaskID {
			return report.Findings[i].TaskID < report.Findings[j].TaskID
		}
		if report.Findings[i].Commit != report.Findings[j].Commit {
			return report.Findings[i].Commit < report.Findings[j].Commit
		}
		return report.Findings[i].Message < report.Findings[j].Message
	})

	return report, nil
}

func (s *Service) taskNeedsCheckpointOrphan(cr *model.CR, task *model.Subtask, branchExists bool) (bool, string) {
	if task == nil {
		return false, ""
	}
	commit := strings.TrimSpace(task.CheckpointCommit)
	if task.Status == model.TaskStatusDone && commit == "" {
		return true, "done task missing checkpoint commit"
	}
	if commit == "" {
		return false, ""
	}
	commitObj, err := s.git.CommitByHash(commit)
	if err != nil {
		return true, "checkpoint commit cannot be resolved"
	}
	if branchExists {
		reachable, reachErr := s.git.IsAncestor(commit, cr.Branch)
		if reachErr != nil {
			return true, fmt.Sprintf("unable to verify checkpoint reachability: %v", reachErr)
		}
		if !reachable {
			return true, "checkpoint commit is not reachable from CR branch head"
		}
	}
	footerTaskID := taskIDFromBody(commitObj.Body)
	if footerTaskID > 0 && footerTaskID != task.ID {
		return true, fmt.Sprintf("checkpoint commit footer task #%d does not match task #%d", footerTaskID, task.ID)
	}
	footerCRUID := strings.TrimSpace(crUIDFromBody(commitObj.Body))
	if footerCRUID != "" && strings.TrimSpace(cr.UID) != "" && footerCRUID != strings.TrimSpace(cr.UID) {
		return true, fmt.Sprintf("checkpoint commit footer uid %q does not match CR uid %q", footerCRUID, strings.TrimSpace(cr.UID))
	}
	return false, ""
}

func (s *Service) scanTaskFooterCommits(cr *model.CR, ref string, limit int) (map[int]gitx.Commit, int, error) {
	commits, err := s.git.RecentCommits(ref, limit)
	if err != nil {
		return nil, 0, err
	}
	byTask := map[int]gitx.Commit{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok || id != cr.ID {
			continue
		}
		commitUID := strings.TrimSpace(crUIDFromBody(commit.Body))
		if commitUID != "" && strings.TrimSpace(cr.UID) != "" && commitUID != strings.TrimSpace(cr.UID) {
			continue
		}
		taskID := taskIDFromBody(commit.Body)
		if taskID <= 0 {
			continue
		}
		if _, exists := byTask[taskID]; exists {
			continue
		}
		byTask[taskID] = commit
	}
	return byTask, len(commits), nil
}

func reconcileScanRef(cr *model.CR, branchExists bool) string {
	if branchExists {
		return cr.Branch
	}
	if cr.Status == model.StatusMerged && strings.TrimSpace(cr.MergedCommit) != "" {
		return strings.TrimSpace(cr.MergedCommit)
	}
	return nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
}

func expectedParentCRIDFromBaseRef(baseRef string, currentCRID int) int {
	id, ok := parseCRBranchID(strings.TrimSpace(baseRef))
	if !ok || id <= 0 || id == currentCRID {
		return 0
	}
	return id
}

type checkpointState struct {
	Commit string
	At     string
	Source string
	Orphan bool
	Reason string
	SyncAt string
}

func checkpointSnapshot(task *model.Subtask) checkpointState {
	if task == nil {
		return checkpointState{}
	}
	return checkpointState{
		Commit: strings.TrimSpace(task.CheckpointCommit),
		At:     strings.TrimSpace(task.CheckpointAt),
		Source: strings.TrimSpace(task.CheckpointSource),
		Orphan: task.CheckpointOrphan,
		Reason: strings.TrimSpace(task.CheckpointReason),
		SyncAt: strings.TrimSpace(task.CheckpointSyncAt),
	}
}

func checkpointChanged(before checkpointState, task *model.Subtask) bool {
	after := checkpointSnapshot(task)
	return before.Commit != after.Commit ||
		before.At != after.At ||
		before.Source != after.Source ||
		before.Orphan != after.Orphan ||
		before.Reason != after.Reason ||
		before.SyncAt != after.SyncAt
}
