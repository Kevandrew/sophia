package service

import (
	"fmt"
	"path/filepath"
	"sophia/internal/model"
	servicemerge "sophia/internal/service/merge"
	"strconv"
	"strings"
)

type mergeDomain struct {
	svc *Service
}

func newMergeDomain(svc *Service) *mergeDomain {
	domain := &mergeDomain{}
	domain.bind(svc)
	return domain
}

func (d *mergeDomain) bind(svc *Service) {
	d.svc = svc
}

func (s *Service) mergeDomainService() *mergeDomain {
	if s == nil {
		return newMergeDomain(nil)
	}
	if s.mergeSvc == nil {
		s.mergeSvc = newMergeDomain(s)
	} else {
		s.mergeSvc.bind(s)
	}
	return s.mergeSvc
}

func (d *mergeDomain) mergeCRWithWarningsUnlocked(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	if d == nil || d.svc == nil {
		return "", nil, fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	baseGit := d.svc.activeMergeGitProvider()
	warnings := []string{}
	cr, err := mergeStore.LoadCR(id)
	if err != nil {
		return "", warnings, err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", warnings, err
	}
	if _, err := d.svc.ensureCRBaseFields(cr, false); err != nil {
		return "", warnings, err
	}
	if guardErr := d.svc.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return "", warnings, guardErr
	}
	if cr.Status == model.StatusMerged {
		return "", warnings, ErrCRAlreadyMerged
	}
	preflight, err := d.prepareMergePreflight(id, cr, overrideReason, true)
	if err != nil {
		return "", warnings, err
	}
	targetRef := d.svc.mergeTargetRef(cr)
	if !baseGit.BranchExists(targetRef) {
		return "", warnings, fmt.Errorf("merge target %q does not exist", targetRef)
	}
	if !baseGit.BranchExists(cr.Branch) {
		return "", warnings, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}
	archiveConfig, archiveEnabled, err := d.resolveMergeArchiveConfig()
	if err != nil {
		return "", warnings, err
	}

	mergeGit, worktreePath, err := d.svc.effectiveMergeGitForCR(cr)
	if err != nil {
		return "", warnings, err
	}
	if dirty, summary, err := d.svc.workingTreeDirtySummaryFor(mergeGit); err != nil {
		return "", warnings, err
	} else if dirty {
		return "", warnings, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	currentBranch, err := mergeGit.CurrentBranch()
	if err != nil {
		return "", warnings, err
	}
	if strings.TrimSpace(currentBranch) != targetRef {
		if err := mergeGit.CheckoutBranch(targetRef); err != nil {
			return "", warnings, err
		}
	}

	actor := mergeGit.Actor()
	mergedAt := d.svc.timestamp()
	msg := buildMergeCommitMessage(cr, actor, mergedAt)
	baseParent, err := mergeGit.ResolveRef("HEAD")
	if err != nil {
		return "", warnings, err
	}
	if err := mergeGit.MergeNoFFNoCommitOnCurrentBranch(cr.Branch, msg); err != nil {
		conflictFiles, conflictErr := mergeGit.MergeConflictFiles()
		if conflictErr != nil {
			return "", warnings, err
		}
		inProgress, progressErr := mergeGit.IsMergeInProgress()
		if progressErr != nil {
			return "", warnings, err
		}
		if inProgress {
			if persistErr := d.recordMergeConflictEvent(cr, actor, worktreePath, targetRef, conflictFiles, err); persistErr != nil {
				return "", warnings, persistErr
			}
			return "", warnings, &MergeConflictError{
				CRID:          cr.ID,
				BaseBranch:    targetRef,
				CRBranch:      cr.Branch,
				WorktreePath:  worktreePath,
				ConflictFiles: conflictFiles,
				Cause:         err,
			}
		}
		return "", warnings, err
	}
	inProgress, err := mergeGit.IsMergeInProgress()
	if err != nil {
		return "", warnings, err
	}
	if inProgress {
		if archiveEnabled {
			mergeHead, mergeHeadErr := mergeGit.MergeHeadSHA()
			if mergeHeadErr != nil {
				return "", warnings, mergeHeadErr
			}
			if strings.TrimSpace(mergeHead) == "" {
				return "", warnings, fmt.Errorf("unable to determine merge head for CR %d archive generation", cr.ID)
			}
			if err := d.writeAutomaticCRArchiveForMerge(mergeGit, worktreePath, cr, archiveConfig, actor, mergedAt, baseParent, mergeHead, false); err != nil {
				return "", warnings, err
			}
		}
		if err := mergeGit.Commit(msg); err != nil {
			return "", warnings, err
		}
	} else if archiveEnabled {
		warnings = append(warnings, fmt.Sprintf("Skipped archive write for CR %d because merge produced no new commit", cr.ID))
	}

	branchWarnings, err := d.maybeDeleteMergedCRBranch(cr.Branch, keepBranch)
	if err != nil {
		return "", warnings, err
	}
	warnings = append(warnings, branchWarnings...)
	mergedHead, err := mergeGit.ResolveRef("HEAD")
	if err != nil {
		return "", warnings, err
	}
	mergedHead = strings.TrimSpace(mergedHead)
	if mergedHead == "" {
		return "", warnings, fmt.Errorf("unable to resolve merged HEAD for CR %d", cr.ID)
	}
	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}

	if err := d.finalizeCRMergedState(cr, preflight.validation, preflight.trust, preflight.overrideReason, actor, mergedAt, sha, mergedHead, false); err != nil {
		return "", warnings, err
	}

	return sha, warnings, nil
}

func (d *mergeDomain) mergeStatusCR(id int) (*MergeStatusView, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	cr, err := mergeStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := d.svc.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}
	mergeGit, worktreePath, err := d.svc.effectiveMergeGitForCR(cr)
	if err != nil {
		return nil, err
	}
	inProgress, err := mergeGit.IsMergeInProgress()
	if err != nil {
		return nil, err
	}
	mergeHead, err := mergeGit.MergeHeadSHA()
	if err != nil {
		return nil, err
	}
	conflictFiles, err := mergeGit.MergeConflictFiles()
	if err != nil {
		return nil, err
	}

	targetMatches := false
	if inProgress && strings.TrimSpace(mergeHead) != "" {
		targetHead, resolveErr := mergeGit.ResolveRef(cr.Branch)
		if resolveErr == nil && strings.TrimSpace(targetHead) != "" {
			targetMatches = strings.TrimSpace(targetHead) == strings.TrimSpace(mergeHead)
		}
	}
	advice := servicemerge.BuildStatusAdvice(inProgress, targetMatches, cr.ID)

	return &MergeStatusView{
		CRID:          cr.ID,
		CRUID:         strings.TrimSpace(cr.UID),
		BaseBranch:    cr.BaseBranch,
		CRBranch:      cr.Branch,
		WorktreePath:  worktreePath,
		InProgress:    inProgress,
		ConflictFiles: conflictFiles,
		TargetMatches: targetMatches,
		MergeHead:     strings.TrimSpace(mergeHead),
		Advice:        advice,
	}, nil
}

func (d *mergeDomain) abortMergeCRUnlocked(id int) error {
	if d == nil || d.svc == nil {
		return fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	status, err := d.mergeStatusCR(id)
	if err != nil {
		return err
	}
	if !status.InProgress {
		return &NoMergeInProgressError{
			WorktreePath: status.WorktreePath,
			Summary:      fmt.Sprintf("%s: no merge in progress for CR %d", ErrNoMergeInProgress, id),
		}
	}
	if !status.TargetMatches {
		return &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: in-progress merge in %s does not target CR %d", ErrMergeInProgress, status.WorktreePath, id),
		}
	}
	cr, err := mergeStore.LoadCR(id)
	if err != nil {
		return err
	}
	mergeGit, _, err := d.svc.effectiveMergeGitForCR(cr)
	if err != nil {
		return err
	}
	if err := mergeGit.MergeAbort(); err != nil {
		return err
	}
	now := d.svc.timestamp()
	actor := d.svc.activeMergeGitProvider().Actor()
	return d.svc.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRMergeAborted,
		Summary: fmt.Sprintf("Aborted in-progress merge for CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"worktree_path":  status.WorktreePath,
			"conflict_count": strconv.Itoa(len(status.ConflictFiles)),
		},
	})
}

func (d *mergeDomain) resumeMergeCRUnlocked(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	if d == nil || d.svc == nil {
		return "", nil, fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	status, err := d.mergeStatusCR(id)
	if err != nil {
		return "", nil, err
	}
	if !status.InProgress {
		return "", nil, &NoMergeInProgressError{
			WorktreePath: status.WorktreePath,
			Summary:      fmt.Sprintf("%s: no merge in progress for CR %d", ErrNoMergeInProgress, id),
		}
	}
	if !status.TargetMatches {
		return "", nil, &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: in-progress merge in %s does not target CR %d", ErrMergeInProgress, status.WorktreePath, id),
		}
	}
	if len(status.ConflictFiles) > 0 {
		return "", nil, &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: unresolved conflicts remain for CR %d", ErrMergeInProgress, id),
		}
	}

	cr, err := mergeStore.LoadCR(id)
	if err != nil {
		return "", nil, err
	}
	if cr.Status == model.StatusMerged {
		return "", nil, ErrCRAlreadyMerged
	}
	preflight, err := d.prepareMergePreflight(id, cr, overrideReason, false)
	if err != nil {
		return "", nil, err
	}
	archiveConfig, archiveEnabled, err := d.resolveMergeArchiveConfig()
	if err != nil {
		return "", nil, err
	}

	mergeGit, worktreePath, err := d.svc.effectiveMergeGitForCR(cr)
	if err != nil {
		return "", nil, err
	}
	actor := mergeGit.Actor()
	mergedAt := d.svc.timestamp()
	if archiveEnabled {
		baseParent, baseParentErr := mergeGit.ResolveRef("HEAD")
		if baseParentErr != nil {
			return "", nil, baseParentErr
		}
		mergeHead, mergeHeadErr := mergeGit.MergeHeadSHA()
		if mergeHeadErr != nil {
			return "", nil, mergeHeadErr
		}
		if strings.TrimSpace(mergeHead) == "" {
			return "", nil, fmt.Errorf("unable to determine merge head for CR %d archive generation", cr.ID)
		}
		if err := d.writeAutomaticCRArchiveForMerge(mergeGit, worktreePath, cr, archiveConfig, actor, mergedAt, baseParent, mergeHead, true); err != nil {
			return "", nil, err
		}
	}
	if err := mergeGit.MergeContinue(); err != nil {
		return "", nil, err
	}
	warnings := []string{}
	branchWarnings, err := d.maybeDeleteMergedCRBranch(cr.Branch, keepBranch)
	if err != nil {
		return "", warnings, err
	}
	warnings = append(warnings, branchWarnings...)
	mergedHead, err := mergeGit.ResolveRef("HEAD")
	if err != nil {
		return "", warnings, err
	}
	mergedHead = strings.TrimSpace(mergedHead)
	if mergedHead == "" {
		return "", warnings, fmt.Errorf("unable to resolve merged HEAD for CR %d", cr.ID)
	}
	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}
	if err := d.finalizeCRMergedState(cr, preflight.validation, preflight.trust, preflight.overrideReason, actor, mergedAt, sha, mergedHead, true); err != nil {
		return "", warnings, err
	}
	return sha, warnings, nil
}

func (d *mergeDomain) resolveMergeArchiveConfig() (model.PolicyArchive, bool, error) {
	if d == nil || d.svc == nil {
		return model.PolicyArchive{}, false, fmt.Errorf("merge domain is not initialized")
	}
	archiveConfig, err := d.svc.archivePolicyConfig()
	if err != nil {
		return model.PolicyArchive{}, false, err
	}
	enabled := archivePolicyEnabled(archiveConfig)
	if enabled {
		if err := d.svc.requireArchiveConfigSupported(archiveConfig); err != nil {
			return model.PolicyArchive{}, false, err
		}
	}
	return archiveConfig, enabled, nil
}

func (d *mergeDomain) maybeDeleteMergedCRBranch(branch string, keepBranch bool) ([]string, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("merge domain is not initialized")
	}
	mergeGit := d.svc.activeMergeGitProvider()
	if keepBranch {
		return nil, nil
	}
	crOwner, ownerErr := d.svc.branchOwnerWorktree(branch)
	if ownerErr != nil {
		return nil, ownerErr
	}
	if crOwner != nil {
		return []string{fmt.Sprintf("Kept branch %s because it is checked out in worktree %s", branch, crOwner.Path)}, nil
	}
	if err := mergeGit.DeleteBranch(branch, true); err != nil {
		return nil, err
	}
	return nil, nil
}

func (d *mergeDomain) writeAutomaticCRArchiveForMerge(mergeGit mergeRuntimeGit, worktreePath string, cr *model.CR, archiveConfig model.PolicyArchive, actor, mergedAt, baseParent, crParent string, reuseExisting bool) error {
	if d == nil || d.svc == nil {
		return fmt.Errorf("merge domain is not initialized")
	}
	if mergeGit == nil {
		return fmt.Errorf("merge git client is required for archive generation")
	}
	if cr == nil {
		return fmt.Errorf("cr is required for archive generation")
	}
	workDir := strings.TrimSpace(worktreePath)
	if workDir == "" {
		return fmt.Errorf("merge worktree path is required for archive generation")
	}
	archivePath := archiveRevisionPath(filepath.Join(workDir, archiveConfig.Path), cr.ID, 1)
	if reuseExisting {
		exists, err := archiveFileExists(archivePath)
		if err != nil {
			return err
		}
		if exists {
			relativeArchivePath, relErr := relativeToRootPath(workDir, archivePath)
			if relErr != nil {
				return relErr
			}
			return mergeGit.StagePaths([]string{relativeArchivePath})
		}
	}
	gitSummary, summaryErr := buildArchiveGitSummaryFromCachedDiff(mergeGit, baseParent, crParent)
	if summaryErr != nil {
		return summaryErr
	}
	fullDiff, diffErr := buildArchiveFullDiffFromCachedDiff(mergeGit, gitSummary.FilesChanged, archivePolicyIncludeFullDiffs(archiveConfig))
	if diffErr != nil {
		return diffErr
	}
	archiveCR := *cr
	archiveCR.Status = model.StatusMerged
	archiveCR.MergedAt = mergedAt
	archiveCR.MergedBy = actor
	archive := buildCRArchiveDocument(&archiveCR, 1, "", mergedAt, archiveConfig, gitSummary, fullDiff)
	payload, payloadErr := marshalCRArchiveYAML(archive)
	if payloadErr != nil {
		return payloadErr
	}
	if err := writeArchivePayload(archivePath, payload); err != nil {
		return err
	}
	relativeArchivePath, relErr := relativeToRootPath(workDir, archivePath)
	if relErr != nil {
		return relErr
	}
	return mergeGit.StagePaths([]string{relativeArchivePath})
}

func (d *mergeDomain) prepareMergePreflight(id int, cr *model.CR, overrideReason string, enforceParentState bool) (*mergePreflightView, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	policy, err := d.svc.repoPolicy()
	if err != nil {
		return nil, err
	}
	validation, err := d.svc.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := d.svc.summarizeCRDiffWithPolicy(cr, policy)
	if err != nil {
		return nil, err
	}
	trust := d.svc.trustDomainService().buildReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)
	trimmedOverride := strings.TrimSpace(overrideReason)
	if trimmedOverride != "" && !policyAllowsMergeOverride(policy) {
		return nil, fmt.Errorf("%w: merge override is disabled by repository policy", ErrPolicyViolation)
	}
	if enforceParentState && cr.ParentCRID > 0 {
		parent, parentErr := mergeStore.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return nil, fmt.Errorf("parent cr %d not found: %w", cr.ParentCRID, parentErr)
		}
		if parent.Status != model.StatusMerged && trimmedOverride == "" && !childDelegatedFromParent(parent, cr.ID) {
			return nil, fmt.Errorf("%w: CR %d depends on parent CR %d (%s)", ErrParentCRNotMerged, cr.ID, parent.ID, parent.Status)
		}
	}
	if !validation.Valid && trimmedOverride == "" {
		return nil, fmt.Errorf("%w: %s", ErrCRValidationFailed, strings.Join(validation.Errors, "; "))
	}
	if trimmedOverride == "" && trust.Gate.Blocked {
		return nil, fmt.Errorf("merge blocked: %s", strings.TrimSpace(trust.Gate.Reason))
	}
	if trimmedOverride == "" {
		blockers := d.svc.mergeBlockersForCR(cr, validation)
		if len(blockers) > 0 {
			return nil, fmt.Errorf("merge blocked: %s", strings.Join(blockers, "; "))
		}
	}
	return &mergePreflightView{
		validation:     validation,
		trust:          trust,
		overrideReason: trimmedOverride,
	}, nil
}

func (d *mergeDomain) finalizeCRMergedState(cr *model.CR, validation *ValidationReport, trust *TrustReport, overrideReason, actor, mergedAt, sha, mergedHead string, resumed bool) error {
	if d == nil || d.svc == nil {
		return fmt.Errorf("merge domain is not initialized")
	}
	mergeStore := d.svc.activeMergeStoreProvider()
	mergeGit := d.svc.activeMergeGitProvider()
	mergedCommit := strings.TrimSpace(nonEmptyTrimmed(mergedHead, sha))
	if count, err := mergeGit.ChangedFileCount(mergedCommit); err == nil && count > 0 {
		cr.FilesTouchedCount = count
	} else if validation != nil && validation.Impact != nil && validation.Impact.FilesChanged > 0 {
		cr.FilesTouchedCount = validation.Impact.FilesChanged
	} else {
		files, diffErr := d.svc.diffNamesForCR(cr)
		if diffErr == nil {
			cr.FilesTouchedCount = len(files)
		} else {
			return diffErr
		}
	}
	cr.Status = model.StatusMerged
	cr.UpdatedAt = mergedAt
	cr.MergedAt = mergedAt
	cr.MergedBy = actor
	cr.MergedCommit = mergedCommit
	cr.BaseCommit = mergedCommit
	if resumed {
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    model.EventTypeCRMergeResumed,
			Summary: fmt.Sprintf("Resumed in-progress merge for CR %d", cr.ID),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
		})
	}
	if strings.TrimSpace(overrideReason) != "" {
		evidence := servicemerge.OverrideEvidence{}
		if validation != nil {
			evidence.ValidationErrorCount = len(validation.Errors)
			if validation.Impact != nil {
				evidence.RiskTier = validation.Impact.RiskTier
			}
		}
		if trust != nil {
			evidence.TrustVerdict = trust.Verdict
			evidence.TrustGateBlocked = trust.Gate.Blocked
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    model.EventTypeCRMergeOverridden,
			Summary: fmt.Sprintf("Merged with validation override: %s", overrideReason),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta:    servicemerge.BuildOverrideEventMeta(overrideReason, evidence),
		})
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      mergedAt,
		Actor:   actor,
		Type:    model.EventTypeCRMerged,
		Summary: fmt.Sprintf("Merged CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := mergeStore.SaveCR(cr); err != nil {
		return err
	}
	if err := d.svc.syncCRRef(cr); err != nil {
		return err
	}
	if err := d.svc.backfillChildrenAfterParentMerge(cr); err != nil {
		return err
	}
	if err := d.svc.syncDelegatedTasksAfterChildMerge(cr.ID); err != nil {
		return err
	}
	return nil
}

func (d *mergeDomain) recordMergeConflictEvent(cr *model.CR, actor, worktreePath, targetRef string, conflictFiles []string, cause error) error {
	if d == nil || d.svc == nil {
		return fmt.Errorf("merge domain is not initialized")
	}
	now := d.svc.timestamp()
	meta := servicemerge.BuildConflictEventMeta(worktreePath, targetRef, cr.Branch, conflictFiles, cause)
	return d.svc.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRMergeConflict,
		Summary: fmt.Sprintf("Merge conflict while merging CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta:    meta,
	})
}
