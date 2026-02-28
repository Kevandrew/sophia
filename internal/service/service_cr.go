package service

import (
	"errors"
	"fmt"
	"sophia/internal/model"
	servicecr "sophia/internal/service/cr"
	"sort"
	"strconv"
	"strings"
)

func (s *Service) AddCR(title, description string) (*model.CR, error) {
	result, err := s.AddCRWithOptions(title, description, AddCROptions{Switch: true})
	if err != nil {
		return nil, err
	}
	return result.CR, nil
}

func (s *Service) AddCRWithWarnings(title, description string) (*model.CR, []string, error) {
	result, err := s.AddCRWithOptions(title, description, AddCROptions{Switch: true})
	if err != nil {
		return nil, nil, err
	}
	return result.CR, append([]string(nil), result.Warnings...), nil
}

type addCRBaseContext struct {
	baseRef    string
	baseCommit string
	parentID   int
}

type AddCRResult struct {
	CR       *model.CR
	Warnings []string
}

func (s *Service) AddCRWithOptionsWithWarnings(title, description string, opts AddCROptions) (*model.CR, []string, error) {
	result, err := s.AddCRWithOptions(title, description, opts)
	if err != nil {
		return nil, nil, err
	}
	return result.CR, append([]string(nil), result.Warnings...), nil
}

func (s *Service) AddCRWithOptions(title, description string, opts AddCROptions) (*AddCRResult, error) {
	var result *AddCRResult
	if err := s.withMutationLock(func() error {
		var err error
		result, err = s.addCRWithOptionsUnlocked(title, description, opts)
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) addCRWithOptionsUnlocked(title, description string, opts AddCROptions) (*AddCRResult, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	lifecycleGit := s.activeLifecycleGitProvider()
	if err := servicecr.ValidateAddRequest(title, opts.BaseRef, opts.ParentCRID, opts.BranchAlias, opts.OwnerPrefixSet); err != nil {
		return nil, err
	}
	if err := lifecycleStore.EnsureInitialized(); err != nil {
		return nil, err
	}
	if err := s.ensureNoMergeInProgressInCurrentWorktree(); err != nil {
		return nil, err
	}

	cfg, err := lifecycleStore.LoadConfig()
	if err != nil {
		return nil, err
	}

	currentBranch, _ := lifecycleGit.CurrentBranch()
	referenceDirs := map[string]struct{}{}
	if strings.TrimSpace(currentBranch) != "" && currentBranch != cfg.BaseBranch && lifecycleGit.BranchExists(currentBranch) && lifecycleGit.BranchExists(cfg.BaseBranch) {
		files, diffErr := lifecycleGit.DiffNames(cfg.BaseBranch, currentBranch)
		if diffErr == nil {
			referenceDirs = topLevelDirs(files)
		}
	}

	if err := lifecycleGit.EnsureBranchExists(cfg.BaseBranch); err != nil {
		return nil, fmt.Errorf("ensure base branch: %w", err)
	}
	if err := lifecycleGit.EnsureBootstrapCommit("chore: bootstrap base branch for Sophia"); err != nil {
		return nil, fmt.Errorf("ensure bootstrap commit: %w", err)
	}
	if err := s.ensureNextCRIDFloor(cfg.BaseBranch); err != nil {
		return nil, fmt.Errorf("align cr id sequence: %w", err)
	}

	baseContext, err := s.resolveAddCRBaseContext(cfg, opts)
	if err != nil {
		return nil, err
	}

	id, err := lifecycleStore.NextCRID()
	if err != nil {
		return nil, err
	}
	uid, err := resolveAddCRUID(opts)
	if err != nil {
		return nil, err
	}

	branch, err := s.resolveAddCRBranch(cfg, opts, id, uid, title)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.BranchAlias) != "" && lifecycleGit.BranchExists(branch) {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}
	switchBranch := servicecr.ShouldSwitch(opts.NoSwitch, opts.Switch)

	if switchBranch {
		if err := lifecycleGit.CreateBranchFrom(branch, baseContext.baseCommit); err != nil {
			return nil, err
		}
	} else {
		if err := lifecycleGit.CreateBranchAt(branch, baseContext.baseCommit); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := lifecycleGit.Actor()
	cr := servicecr.BuildCR(servicecr.BuildInput{
		ID:          id,
		UID:         uid,
		Title:       title,
		Description: description,
		BaseBranch:  cfg.BaseBranch,
		BaseRef:     baseContext.baseRef,
		BaseCommit:  baseContext.baseCommit,
		ParentCRID:  baseContext.parentID,
		Branch:      branch,
		Now:         now,
		Actor:       actor,
	})

	if err := lifecycleStore.SaveCR(cr); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(cr); err != nil {
		return nil, err
	}

	warnings := s.computeOverlapWarnings(referenceDirs, cr.ID)
	return &AddCRResult{
		CR:       cr,
		Warnings: append([]string{}, warnings...),
	}, nil
}

func (s *Service) resolveAddCRBaseContext(cfg model.Config, opts AddCROptions) (addCRBaseContext, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	lifecycleGit := s.activeLifecycleGitProvider()
	baseContext := addCRBaseContext{
		baseRef: strings.TrimSpace(opts.BaseRef),
	}
	if opts.ParentCRID > 0 {
		parent, err := lifecycleStore.LoadCR(opts.ParentCRID)
		if err != nil {
			return addCRBaseContext{}, err
		}
		if guardErr := s.ensureNoMergeInProgressForCR(parent); guardErr != nil {
			return addCRBaseContext{}, guardErr
		}
		ref, commit, err := s.parentBaseAnchorWithProviders(parent, lifecycleStore, lifecycleGit)
		if err != nil {
			return addCRBaseContext{}, err
		}
		baseContext.baseRef = ref
		baseContext.baseCommit = commit
		baseContext.parentID = parent.ID
	}
	if baseContext.baseRef == "" {
		baseContext.baseRef = cfg.BaseBranch
	}
	if strings.TrimSpace(baseContext.baseCommit) == "" {
		resolved, err := lifecycleGit.ResolveRef(baseContext.baseRef)
		if err != nil {
			return addCRBaseContext{}, fmt.Errorf("resolve base ref %q: %w", baseContext.baseRef, err)
		}
		baseContext.baseCommit = resolved
	}
	return baseContext, nil
}

func (s *Service) resolveAddCRBranch(cfg model.Config, opts AddCROptions, id int, uid, title string) (string, error) {
	lifecycleGit := s.activeLifecycleGitProvider()
	if strings.TrimSpace(opts.BranchAlias) != "" {
		return validateExplicitCRBranchAlias(opts.BranchAlias, id)
	}
	ownerPrefix := cfg.BranchOwnerPrefix
	if opts.OwnerPrefixSet {
		ownerPrefix = opts.OwnerPrefix
	}
	return formatCRBranchAliasWithFallback(title, ownerPrefix, uid, lifecycleGit.BranchExists)
}

func resolveAddCRUID(opts AddCROptions) (string, error) {
	if strings.TrimSpace(opts.UIDOverride) != "" {
		return normalizeCRUID(opts.UIDOverride)
	}
	return newCRUID()
}

func (s *Service) ListCRs() ([]model.CR, error) {
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	sort.Slice(crs, func(i, j int) bool {
		return crs[i].ID < crs[j].ID
	})
	return crs, nil
}

func (s *Service) loadCRForMutation(id int) (*model.CR, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	cr, err := lifecycleStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	return cr, nil
}

func (s *Service) appendCRMutationEventAndSave(cr *model.CR, event model.Event) error {
	lifecycleGit := s.activeLifecycleGitProvider()
	lifecycleStore := s.activeLifecycleStoreProvider()
	if strings.TrimSpace(event.TS) == "" {
		event.TS = s.timestamp()
	}
	if strings.TrimSpace(event.Actor) == "" {
		event.Actor = lifecycleGit.Actor()
	}
	cr.UpdatedAt = event.TS
	cr.Events = append(cr.Events, event)
	return lifecycleStore.SaveCR(cr)
}

func (s *Service) AddNote(id int, note string) error {
	if strings.TrimSpace(note) == "" {
		return errors.New("note cannot be empty")
	}
	return s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(id)
		if err != nil {
			return err
		}
		now := s.timestamp()
		actor := s.activeLifecycleGitProvider().Actor()
		cr.Notes = append(cr.Notes, note)
		return s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeNoteAdded,
			Summary: note,
			Ref:     fmt.Sprintf("cr:%d", id),
		})
	})
}

func (s *Service) EditCR(id int, newTitle, newDescription *string) ([]string, error) {
	changedFields := make([]string, 0, 2)
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(id)
		if err != nil {
			return err
		}

		changedFields = changedFields[:0]
		if newTitle != nil && cr.Title != *newTitle {
			cr.Title = *newTitle
			changedFields = append(changedFields, "title")
		}
		if newDescription != nil && cr.Description != *newDescription {
			cr.Description = *newDescription
			changedFields = append(changedFields, "description")
		}
		if len(changedFields) == 0 {
			return ErrNoCRChanges
		}

		now := s.timestamp()
		actor := s.activeLifecycleGitProvider().Actor()
		return s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeCRAmended,
			Summary: fmt.Sprintf("Amended CR fields: %s", strings.Join(changedFields, ",")),
			Ref:     fmt.Sprintf("cr:%d", id),
			Meta: map[string]string{
				"fields": strings.Join(changedFields, ","),
			},
		})
	}); err != nil {
		return nil, err
	}
	return changedFields, nil
}

func (s *Service) SetCRContract(id int, patch ContractPatch) ([]string, error) {
	result, err := s.SetCRContractWithOptions(id, patch, SetCRContractOptions{})
	if err != nil {
		return nil, err
	}
	if result.AlreadyApplied {
		return nil, ErrNoCRChanges
	}
	return append([]string(nil), result.ChangedFields...), nil
}

func (s *Service) SetCRContractWithOptions(id int, patch ContractPatch, opts SetCRContractOptions) (*SetCRContractResult, error) {
	result := &SetCRContractResult{
		ChangedFields:  []string{},
		AlreadyApplied: false,
		DryRun:         opts.DryRun,
	}
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(id)
		if err != nil {
			return err
		}
		policy, err := s.repoPolicy()
		if err != nil {
			return err
		}

		nextContract := cr.Contract
		beforeScope := append([]string(nil), nextContract.Scope...)
		changed, err := s.applyCRContractPatch(&nextContract, patch, policy)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			result.AlreadyApplied = true
			result.ChangedFields = []string{}
			return nil
		}
		result.ChangedFields = append([]string(nil), changed...)
		changeReason := ""
		if patch.ChangeReason != nil {
			changeReason = strings.TrimSpace(*patch.ChangeReason)
		}
		scopeChanged := !equalStringSlices(beforeScope, nextContract.Scope)
		if !crContractBaselineIsEmpty(cr.ContractBaseline) && scopeChanged && changeReason == "" {
			return fmt.Errorf("change reason is required when updating CR contract scope after first checkpoint freeze")
		}
		if opts.DryRun {
			return nil
		}

		now := s.timestamp()
		actor := s.activeLifecycleGitProvider().Actor()
		nextContract.UpdatedAt = now
		nextContract.UpdatedBy = actor
		cr.Contract = nextContract
		cr.UpdatedAt = now
		meta := map[string]string{
			"fields": strings.Join(changed, ","),
		}
		if changeReason != "" {
			meta["change_reason"] = changeReason
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeContractUpdated,
			Summary: fmt.Sprintf("Updated contract fields: %s", strings.Join(changed, ",")),
			Ref:     fmt.Sprintf("cr:%d", id),
			Meta:    meta,
		})
		if !crContractBaselineIsEmpty(cr.ContractBaseline) && scopeChanged {
			drift := model.CRContractDrift{
				ID:          nextCRContractDriftID(cr.ContractDrifts),
				TS:          now,
				Actor:       actor,
				Fields:      []string{"scope_changed"},
				BeforeScope: append([]string(nil), beforeScope...),
				AfterScope:  append([]string(nil), nextContract.Scope...),
				Reason:      changeReason,
			}
			cr.ContractDrifts = append(cr.ContractDrifts, drift)
			cr.Events = append(cr.Events, model.Event{
				TS:      now,
				Actor:   actor,
				Type:    model.EventTypeCRContractDriftRecorded,
				Summary: fmt.Sprintf("Recorded CR contract drift #%d (scope_changed)", drift.ID),
				Ref:     fmt.Sprintf("cr:%d", id),
				Meta: map[string]string{
					"drift_id": strconv.Itoa(drift.ID),
					"fields":   "scope_changed",
					"reason":   changeReason,
				},
			})
		}
		return s.activeLifecycleStoreProvider().SaveCR(cr)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) GetCRContract(id int) (*model.Contract, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	contract := cr.Contract
	contract.Scope = append([]string(nil), contract.Scope...)
	contract.NonGoals = append([]string(nil), contract.NonGoals...)
	contract.Invariants = append([]string(nil), contract.Invariants...)
	contract.RiskCriticalScopes = append([]string(nil), contract.RiskCriticalScopes...)
	return &contract, nil
}

func (s *Service) GetCRContractBaseline(crID int) (*model.CRContractBaseline, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	baseline := cr.ContractBaseline
	baseline.Scope = append([]string(nil), baseline.Scope...)
	return &baseline, nil
}

func (s *Service) ListCRContractDrifts(crID int) ([]model.CRContractDrift, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	drifts := append([]model.CRContractDrift(nil), cr.ContractDrifts...)
	sort.Slice(drifts, func(i, j int) bool {
		if drifts[i].ID == drifts[j].ID {
			return drifts[i].TS < drifts[j].TS
		}
		return drifts[i].ID < drifts[j].ID
	})
	return drifts, nil
}

func (s *Service) AckCRContractDrift(crID, driftID int, reason string) (*model.CRContractDrift, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("ack reason is required")
	}
	var ack model.CRContractDrift
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		driftIndex := indexOfCRDrift(cr.ContractDrifts, driftID)
		if driftIndex < 0 {
			return fmt.Errorf("cr %d drift %d not found", crID, driftID)
		}

		now := s.timestamp()
		actor := s.git.Actor()
		cr.ContractDrifts[driftIndex].Acknowledged = true
		cr.ContractDrifts[driftIndex].AcknowledgedAt = now
		cr.ContractDrifts[driftIndex].AcknowledgedBy = actor
		cr.ContractDrifts[driftIndex].AckReason = reason

		if err := s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeCRContractDriftAcknowledged,
			Summary: fmt.Sprintf("Acknowledged CR contract drift #%d", driftID),
			Ref:     fmt.Sprintf("cr:%d", crID),
			Meta: map[string]string{
				"drift_id": strconv.Itoa(driftID),
				"reason":   reason,
			},
		}); err != nil {
			return err
		}
		ack = cr.ContractDrifts[driftIndex]
		return nil
	}); err != nil {
		return nil, err
	}
	ack.Fields = append([]string(nil), ack.Fields...)
	ack.BeforeScope = append([]string(nil), ack.BeforeScope...)
	ack.AfterScope = append([]string(nil), ack.AfterScope...)
	return &ack, nil
}

func (s *Service) SetCRBase(id int, ref string, rebase bool) (*model.CR, error) {
	var cr *model.CR
	if err := s.withMutationLock(func() error {
		var err error
		cr, err = s.setCRBaseUnlocked(id, ref, rebase)
		return err
	}); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) setCRBaseUnlocked(id int, ref string, rebase bool) (*model.CR, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	lifecycleGit := s.activeLifecycleGitProvider()
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("base ref cannot be empty")
	}
	cr, err := lifecycleStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	baseCommit, err := lifecycleGit.ResolveRef(ref)
	if err != nil {
		return nil, fmt.Errorf("resolve base ref %q: %w", ref, err)
	}
	if rebase {
		if !lifecycleGit.BranchExists(cr.Branch) {
			return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
		}
		if err := s.rebaseBranchOnto(cr.Branch, ref); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := lifecycleGit.Actor()
	cr.BaseRef = ref
	cr.BaseCommit = strings.TrimSpace(baseCommit)
	cr.ParentCRID = 0
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRBaseUpdated,
		Summary: fmt.Sprintf("Updated CR base to %s", ref),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
			"rebase":      strconv.FormatBool(rebase),
		},
	})
	if err := lifecycleStore.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) RestackCR(id int) (*model.CR, error) {
	var cr *model.CR
	if err := s.withMutationLock(func() error {
		var err error
		cr, err = s.restackCRUnlocked(id)
		return err
	}); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) restackCRUnlocked(id int) (*model.CR, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	lifecycleGit := s.activeLifecycleGitProvider()
	cr, err := lifecycleStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if cr.ParentCRID <= 0 {
		return nil, ErrParentCRRequired
	}
	if !lifecycleGit.BranchExists(cr.Branch) {
		return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	parent, err := lifecycleStore.LoadCR(cr.ParentCRID)
	if err != nil {
		return nil, err
	}
	targetRef := ""
	switch {
	case parent.Status == model.StatusInProgress && lifecycleGit.BranchExists(parent.Branch):
		targetRef = parent.Branch
	case parent.Status == model.StatusMerged && strings.TrimSpace(parent.MergedCommit) != "":
		targetRef = strings.TrimSpace(parent.MergedCommit)
	default:
		return nil, fmt.Errorf("parent CR %d has no restack anchor", parent.ID)
	}

	if err := s.rebaseBranchOnto(cr.Branch, targetRef); err != nil {
		return nil, err
	}
	targetCommit, err := lifecycleGit.ResolveRef(targetRef)
	if err != nil {
		return nil, err
	}

	cr.BaseCommit = strings.TrimSpace(targetCommit)
	if parent.Status == model.StatusMerged {
		cr.BaseRef = cr.BaseBranch
	} else {
		cr.BaseRef = parent.Branch
	}
	now := s.timestamp()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   lifecycleGit.Actor(),
		Type:    model.EventTypeCRRestacked,
		Summary: fmt.Sprintf("Restacked CR %d onto parent CR %d", cr.ID, parent.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"parent_cr":   strconv.Itoa(parent.ID),
			"target_ref":  targetRef,
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
		},
	})
	if err := lifecycleStore.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}
