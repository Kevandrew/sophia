package service

import (
	"errors"
	"fmt"
	"sophia/internal/gitx"
	"sophia/internal/model"
	servicediff "sophia/internal/service/diff"
	"strconv"
	"strings"
)

var ErrCRBranchContextUnavailable = errors.New("cr branch context unavailable")

func (s *Service) summarizeCRDiff(cr *model.CR) (*diffSummary, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	return s.summarizeCRDiffWithPolicy(cr, policy)
}

func (s *Service) summarizeCRDiffWithPolicy(cr *model.CR, policy *model.RepoPolicy) (*diffSummary, error) {
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, errors.New("repo policy is required")
	}
	var (
		changes   []gitx.FileChange
		shortStat string
		diffErr   error
	)
	switch {
	case s.git.BranchExists(cr.Branch):
		changes, diffErr = s.diffNameStatusForCR(cr)
		if diffErr != nil {
			return nil, diffErr
		}
		shortStat, diffErr = s.diffShortStatForCR(cr)
		if diffErr != nil {
			return nil, diffErr
		}
	case cr.Status == model.StatusMerged:
		changes, shortStat, diffErr = s.summarizeMergedCRDiff(cr)
		if diffErr != nil {
			return nil, diffErr
		}
	default:
		return nil, fmt.Errorf("%w: unable to summarize CR %d diff: missing branch context (%q, %q)", ErrCRBranchContextUnavailable, cr.ID, cr.BaseBranch, cr.Branch)
	}

	return buildDiffSummaryFromChanges(changes, shortStat, policy), nil
}

func (s *Service) summarizeCRDiffFromTaskCheckpoints(cr *model.CR, policy *model.RepoPolicy) *diffSummary {
	derivedChanges := deriveChangesFromTaskCheckpointScopes(cr.Subtasks)
	if len(derivedChanges) == 0 {
		return &diffSummary{}
	}
	shortStat := fmt.Sprintf("%d file(s) changed (derived from task checkpoint scope)", len(derivedChanges))
	return buildDiffSummaryFromChanges(derivedChanges, shortStat, policy)
}

func buildDiffSummaryFromChanges(changes []gitx.FileChange, shortStat string, policy *model.RepoPolicy) *diffSummary {
	summary := servicediff.BuildSummaryFromChanges(
		changes,
		shortStat,
		func(path string) bool {
			return isTestFile(path, policy)
		},
		func(path string) bool {
			return isDependencyFile(path, policy)
		},
	)
	return &diffSummary{
		Files:           summary.Files,
		ShortStat:       summary.ShortStat,
		NewFiles:        summary.NewFiles,
		ModifiedFiles:   summary.ModifiedFiles,
		DeletedFiles:    summary.DeletedFiles,
		TestFiles:       summary.TestFiles,
		DependencyFiles: summary.DependencyFiles,
	}
}

func (s *Service) summarizeMergedCRDiff(cr *model.CR) ([]gitx.FileChange, string, error) {
	mergedCommit := strings.TrimSpace(cr.MergedCommit)
	var mergeDiffErr error
	if mergedCommit != "" {
		baseRef := mergedCommit + "^1"
		changes, err := s.git.DiffNameStatusBetween(baseRef, mergedCommit)
		if err != nil {
			mergeDiffErr = err
		} else {
			shortStat, statErr := s.git.DiffShortStatBetween(baseRef, mergedCommit)
			if statErr == nil {
				return changes, shortStat, nil
			}
			mergeDiffErr = statErr
		}
	}

	derivedChanges := deriveChangesFromTaskCheckpointScopes(cr.Subtasks)
	if len(derivedChanges) > 0 {
		shortStat := fmt.Sprintf("%d file(s) changed (derived from task checkpoint scope)", len(derivedChanges))
		return derivedChanges, shortStat, nil
	}

	if mergeDiffErr != nil {
		return nil, "", fmt.Errorf("unable to summarize merged CR %d diff: %w", cr.ID, mergeDiffErr)
	}
	return nil, "", fmt.Errorf("unable to summarize merged CR %d diff: merged commit and task checkpoint scope are unavailable", cr.ID)
}

func (s *Service) ensureCRBaseFields(cr *model.CR, persist bool) (bool, error) {
	return s.ensureCRBaseFieldsWithProviders(cr, persist, s.store, s.git)
}

type crBaseFieldsStore interface {
	LoadConfig() (model.Config, error)
	LoadCR(id int) (*model.CR, error)
	SaveCR(cr *model.CR) error
}

type crBaseFieldsGit interface {
	ResolveRef(ref string) (string, error)
}

type crBaseAnchorGit interface {
	BranchExists(branch string) bool
	ResolveRef(ref string) (string, error)
}

func (s *Service) ensureCRBaseFieldsWithProviders(cr *model.CR, persist bool, crStore crBaseFieldsStore, gitClient crBaseFieldsGit) (bool, error) {
	if cr == nil {
		return false, errors.New("cr cannot be nil")
	}
	if crStore == nil {
		return false, errors.New("cr store is required")
	}
	if gitClient == nil {
		return false, errors.New("git client is required")
	}
	changed := false
	if strings.TrimSpace(cr.BaseBranch) == "" {
		cfg, err := crStore.LoadConfig()
		if err != nil {
			return false, err
		}
		cr.BaseBranch = cfg.BaseBranch
		changed = true
	}
	if strings.TrimSpace(cr.BaseRef) == "" {
		cr.BaseRef = cr.BaseBranch
		changed = true
	}
	if strings.TrimSpace(cr.BaseCommit) == "" && strings.TrimSpace(cr.BaseRef) != "" {
		if resolved, err := gitClient.ResolveRef(cr.BaseRef); err == nil && strings.TrimSpace(resolved) != "" {
			cr.BaseCommit = strings.TrimSpace(resolved)
			changed = true
		}
	}
	if changed && persist {
		cr.UpdatedAt = s.timestamp()
		if err := crStore.SaveCR(cr); err != nil {
			return false, err
		}
	}
	return changed, nil
}

func (s *Service) ensureCRBaseFieldsPersisted(cr *model.CR) (*model.CR, error) {
	return s.ensureCRBaseFieldsPersistedWithProviders(cr, s.store, s.git)
}

func (s *Service) ensureCRBaseFieldsPersistedWithProviders(cr *model.CR, crStore crBaseFieldsStore, gitClient crBaseFieldsGit) (*model.CR, error) {
	if cr == nil {
		return nil, errors.New("cr cannot be nil")
	}
	if crStore == nil {
		return nil, errors.New("cr store is required")
	}
	if gitClient == nil {
		return nil, errors.New("git client is required")
	}
	changed, err := s.ensureCRBaseFieldsWithProviders(cr, false, crStore, gitClient)
	if err != nil {
		return nil, err
	}
	if !changed {
		return cr, nil
	}

	persisted := cr
	if err := s.withMutationLock(func() error {
		latest, err := crStore.LoadCR(cr.ID)
		if err != nil {
			return err
		}
		latestChanged, err := s.ensureCRBaseFieldsWithProviders(latest, false, crStore, gitClient)
		if err != nil {
			return err
		}
		if latestChanged {
			latest.UpdatedAt = s.timestamp()
			if err := crStore.SaveCR(latest); err != nil {
				return err
			}
		}
		persisted = latest
		return nil
	}); err != nil {
		return nil, err
	}
	return persisted, nil
}

func (s *Service) resolveCRBaseAnchor(cr *model.CR) (string, error) {
	return servicediff.ResolveCRBaseAnchor(cr, s.git.ResolveRef)
}

func (s *Service) diffNameStatusForCR(cr *model.CR) ([]gitx.FileChange, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffNameStatusBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffNameStatus(baseRef, cr.Branch)
}

func (s *Service) diffShortStatForCR(cr *model.CR) (string, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffShortStatBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffShortStat(baseRef, cr.Branch)
}

func (s *Service) diffNamesForCR(cr *model.CR) ([]string, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffNamesBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffNames(baseRef, cr.Branch)
}

func (s *Service) parentBaseAnchorWithProviders(parent *model.CR, crStore crBaseFieldsStore, gitClient crBaseAnchorGit) (string, string, error) {
	if parent == nil {
		return "", "", errors.New("parent cr is required")
	}
	if crStore == nil {
		return "", "", errors.New("cr store is required")
	}
	if gitClient == nil {
		return "", "", errors.New("git client is required")
	}
	if _, err := s.ensureCRBaseFieldsWithProviders(parent, false, crStore, gitClient); err != nil {
		return "", "", err
	}

	if parent.Status == model.StatusInProgress && gitClient.BranchExists(parent.Branch) {
		sha, err := gitClient.ResolveRef(parent.Branch)
		if err != nil {
			return "", "", err
		}
		return parent.Branch, strings.TrimSpace(sha), nil
	}
	if parent.Status == model.StatusMerged {
		if strings.TrimSpace(parent.MergedCommit) != "" {
			sha, err := gitClient.ResolveRef(parent.MergedCommit)
			if err == nil {
				return parent.BaseBranch, strings.TrimSpace(sha), nil
			}
			return parent.BaseBranch, strings.TrimSpace(parent.MergedCommit), nil
		}
		if strings.TrimSpace(parent.BaseCommit) != "" {
			return nonEmptyTrimmed(parent.BaseRef, parent.BaseBranch), strings.TrimSpace(parent.BaseCommit), nil
		}
	}
	anchorRef := nonEmptyTrimmed(parent.BaseRef, parent.BaseBranch)
	if strings.TrimSpace(anchorRef) == "" {
		return "", "", fmt.Errorf("parent CR %d has no base ref", parent.ID)
	}
	sha, err := gitClient.ResolveRef(anchorRef)
	if err != nil {
		return "", "", err
	}
	return anchorRef, strings.TrimSpace(sha), nil
}

func (s *Service) backfillChildrenAfterParentMerge(parent *model.CR) error {
	if parent == nil || parent.ID <= 0 {
		return nil
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return err
	}
	resolvedMergeCommit := strings.TrimSpace(parent.MergedCommit)
	if resolvedMergeCommit != "" {
		if resolved, resolveErr := s.git.ResolveRef(resolvedMergeCommit); resolveErr == nil {
			resolvedMergeCommit = strings.TrimSpace(resolved)
		}
	}
	for i := range crs {
		child := crs[i]
		if child.ParentCRID != parent.ID || child.Status != model.StatusInProgress {
			continue
		}
		changed := false
		if strings.TrimSpace(child.BaseRef) != strings.TrimSpace(child.BaseBranch) {
			child.BaseRef = child.BaseBranch
			changed = true
		}
		if strings.TrimSpace(resolvedMergeCommit) != "" && strings.TrimSpace(child.BaseCommit) != resolvedMergeCommit {
			child.BaseCommit = resolvedMergeCommit
			changed = true
		}
		if !changed {
			continue
		}
		now := s.timestamp()
		child.UpdatedAt = now
		child.Events = append(child.Events, model.Event{
			TS:      now,
			Actor:   s.git.Actor(),
			Type:    model.EventTypeCRParentMerged,
			Summary: fmt.Sprintf("Updated base anchor from merged parent CR %d", parent.ID),
			Ref:     fmt.Sprintf("cr:%d", child.ID),
			Meta: map[string]string{
				"parent_cr":   strconv.Itoa(parent.ID),
				"base_ref":    child.BaseRef,
				"base_commit": child.BaseCommit,
			},
		})
		if err := s.store.SaveCR(&child); err != nil {
			return err
		}
	}
	return nil
}
