package service

import (
	"errors"
	"fmt"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
)

func (s *Service) AddCR(title, description string) (*model.CR, error) {
	cr, _, err := s.AddCRWithOptionsWithWarnings(title, description, AddCROptions{})
	return cr, err
}

func (s *Service) AddCRWithWarnings(title, description string) (*model.CR, []string, error) {
	return s.AddCRWithOptionsWithWarnings(title, description, AddCROptions{})
}

func (s *Service) AddCRWithOptionsWithWarnings(title, description string, opts AddCROptions) (*model.CR, []string, error) {
	if strings.TrimSpace(title) == "" {
		return nil, nil, errors.New("title cannot be empty")
	}
	if strings.TrimSpace(opts.BaseRef) != "" && opts.ParentCRID > 0 {
		return nil, nil, errors.New("--base and --parent cannot be combined")
	}
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, nil, err
	}

	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, nil, err
	}

	currentBranch, _ := s.git.CurrentBranch()
	referenceDirs := map[string]struct{}{}
	if strings.TrimSpace(currentBranch) != "" && currentBranch != cfg.BaseBranch && s.git.BranchExists(currentBranch) && s.git.BranchExists(cfg.BaseBranch) {
		files, diffErr := s.git.DiffNames(cfg.BaseBranch, currentBranch)
		if diffErr == nil {
			referenceDirs = topLevelDirs(files)
		}
	}

	if err := s.git.EnsureBaseBranch(cfg.BaseBranch); err != nil {
		return nil, nil, fmt.Errorf("ensure base branch: %w", err)
	}
	if err := s.git.EnsureBootstrapCommit("chore: bootstrap base branch for Sophia"); err != nil {
		return nil, nil, fmt.Errorf("ensure bootstrap commit: %w", err)
	}
	if err := s.ensureNextCRIDFloor(cfg.BaseBranch); err != nil {
		return nil, nil, fmt.Errorf("align cr id sequence: %w", err)
	}

	baseRef := strings.TrimSpace(opts.BaseRef)
	baseCommit := ""
	parentID := 0
	if opts.ParentCRID > 0 {
		parent, err := s.store.LoadCR(opts.ParentCRID)
		if err != nil {
			return nil, nil, err
		}
		ref, commit, err := s.parentBaseAnchor(parent)
		if err != nil {
			return nil, nil, err
		}
		baseRef = ref
		baseCommit = commit
		parentID = parent.ID
	}
	if baseRef == "" {
		baseRef = cfg.BaseBranch
	}
	if strings.TrimSpace(baseCommit) == "" {
		resolved, err := s.git.ResolveRef(baseRef)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve base ref %q: %w", baseRef, err)
		}
		baseCommit = resolved
	}

	id, err := s.store.NextCRID()
	if err != nil {
		return nil, nil, err
	}
	uid, err := newCRUID()
	if err != nil {
		return nil, nil, err
	}

	branch := fmt.Sprintf("sophia/cr-%d", id)
	if s.git.BranchExists(branch) {
		return nil, nil, fmt.Errorf("branch %q already exists", branch)
	}
	if err := s.git.CreateBranchFrom(branch, baseCommit); err != nil {
		return nil, nil, err
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr := &model.CR{
		ID:          id,
		UID:         uid,
		Title:       title,
		Description: description,
		Status:      model.StatusInProgress,
		BaseBranch:  cfg.BaseBranch,
		BaseRef:     baseRef,
		BaseCommit:  baseCommit,
		ParentCRID:  parentID,
		Branch:      branch,
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events: []model.Event{
			{
				TS:      now,
				Actor:   actor,
				Type:    "cr_created",
				Summary: fmt.Sprintf("Created CR %d", id),
				Ref:     fmt.Sprintf("cr:%d", id),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.SaveCR(cr); err != nil {
		return nil, nil, err
	}

	warnings := s.computeOverlapWarnings(referenceDirs, cr.ID)
	return cr, warnings, nil
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

func (s *Service) AddNote(id int, note string) error {
	if strings.TrimSpace(note) == "" {
		return errors.New("note cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	cr.Notes = append(cr.Notes, note)
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "note_added",
		Summary: note,
		Ref:     fmt.Sprintf("cr:%d", id),
	})
	cr.UpdatedAt = now
	return s.store.SaveCR(cr)
}

func (s *Service) EditCR(id int, newTitle, newDescription *string) ([]string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	changedFields := make([]string, 0, 2)
	if newTitle != nil && cr.Title != *newTitle {
		cr.Title = *newTitle
		changedFields = append(changedFields, "title")
	}
	if newDescription != nil && cr.Description != *newDescription {
		cr.Description = *newDescription
		changedFields = append(changedFields, "description")
	}
	if len(changedFields) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_amended",
		Summary: fmt.Sprintf("Amended CR fields: %s", strings.Join(changedFields, ",")),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta: map[string]string{
			"fields": strings.Join(changedFields, ","),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return changedFields, nil
}

func (s *Service) SetCRContract(id int, patch ContractPatch) ([]string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	changed := []string{}
	if patch.Why != nil {
		if cr.Contract.Why != strings.TrimSpace(*patch.Why) {
			cr.Contract.Why = strings.TrimSpace(*patch.Why)
			changed = append(changed, "why")
		}
	}
	if patch.Scope != nil {
		scope, scopeErr := s.normalizeContractScopePrefixes(*patch.Scope)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if !equalStringSlices(cr.Contract.Scope, scope) {
			cr.Contract.Scope = scope
			changed = append(changed, "scope")
		}
	}
	if patch.NonGoals != nil {
		normalized := normalizeNonEmptyStringList(*patch.NonGoals)
		if !equalStringSlices(cr.Contract.NonGoals, normalized) {
			cr.Contract.NonGoals = normalized
			changed = append(changed, "non_goals")
		}
	}
	if patch.Invariants != nil {
		normalized := normalizeNonEmptyStringList(*patch.Invariants)
		if !equalStringSlices(cr.Contract.Invariants, normalized) {
			cr.Contract.Invariants = normalized
			changed = append(changed, "invariants")
		}
	}
	if patch.BlastRadius != nil {
		normalized := strings.TrimSpace(*patch.BlastRadius)
		if cr.Contract.BlastRadius != normalized {
			cr.Contract.BlastRadius = normalized
			changed = append(changed, "blast_radius")
		}
	}
	if patch.TestPlan != nil {
		normalized := strings.TrimSpace(*patch.TestPlan)
		if cr.Contract.TestPlan != normalized {
			cr.Contract.TestPlan = normalized
			changed = append(changed, "test_plan")
		}
	}
	if patch.RollbackPlan != nil {
		normalized := strings.TrimSpace(*patch.RollbackPlan)
		if cr.Contract.RollbackPlan != normalized {
			cr.Contract.RollbackPlan = normalized
			changed = append(changed, "rollback_plan")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.Contract.UpdatedAt = now
	cr.Contract.UpdatedBy = actor
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "contract_updated",
		Summary: fmt.Sprintf("Updated contract fields: %s", strings.Join(changed, ",")),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta: map[string]string{
			"fields": strings.Join(changed, ","),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return changed, nil
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
	return &contract, nil
}

func (s *Service) SetCRBase(id int, ref string, rebase bool) (*model.CR, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("base ref cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	baseCommit, err := s.git.ResolveRef(ref)
	if err != nil {
		return nil, fmt.Errorf("resolve base ref %q: %w", ref, err)
	}
	if rebase {
		if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
			return nil, err
		} else if dirty {
			return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
		}
		if !s.git.BranchExists(cr.Branch) {
			return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
		}
		if err := s.git.RebaseBranchOnto(cr.Branch, ref); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.BaseRef = ref
	cr.BaseCommit = strings.TrimSpace(baseCommit)
	cr.ParentCRID = 0
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_base_updated",
		Summary: fmt.Sprintf("Updated CR base to %s", ref),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
			"rebase":      strconv.FormatBool(rebase),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) RestackCR(id int) (*model.CR, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if cr.ParentCRID <= 0 {
		return nil, ErrParentCRRequired
	}
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	if !s.git.BranchExists(cr.Branch) {
		return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	parent, err := s.store.LoadCR(cr.ParentCRID)
	if err != nil {
		return nil, err
	}
	targetRef := ""
	switch {
	case parent.Status == model.StatusInProgress && s.git.BranchExists(parent.Branch):
		targetRef = parent.Branch
	case parent.Status == model.StatusMerged && strings.TrimSpace(parent.MergedCommit) != "":
		targetRef = strings.TrimSpace(parent.MergedCommit)
	default:
		return nil, fmt.Errorf("parent CR %d has no restack anchor", parent.ID)
	}

	if err := s.git.RebaseBranchOnto(cr.Branch, targetRef); err != nil {
		return nil, err
	}
	targetCommit, err := s.git.ResolveRef(targetRef)
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
		Actor:   s.git.Actor(),
		Type:    "cr_restacked",
		Summary: fmt.Sprintf("Restacked CR %d onto parent CR %d", cr.ID, parent.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"parent_cr":   strconv.Itoa(parent.ID),
			"target_ref":  targetRef,
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) WhyCR(id int) (*WhyView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cr.Description)
	contractWhy := strings.TrimSpace(cr.Contract.Why)
	effectiveWhy := ""
	source := "missing"
	switch {
	case contractWhy != "":
		effectiveWhy = contractWhy
		source = "contract_why"
	case description != "":
		effectiveWhy = description
		source = "description"
	}

	return &WhyView{
		CRID:              cr.ID,
		CRUID:             strings.TrimSpace(cr.UID),
		BaseRef:           strings.TrimSpace(cr.BaseRef),
		BaseCommit:        strings.TrimSpace(cr.BaseCommit),
		ParentCRID:        cr.ParentCRID,
		EffectiveWhy:      effectiveWhy,
		Source:            source,
		Description:       description,
		ContractWhy:       contractWhy,
		ContractUpdatedAt: strings.TrimSpace(cr.Contract.UpdatedAt),
		ContractUpdatedBy: strings.TrimSpace(cr.Contract.UpdatedBy),
	}, nil
}

func (s *Service) StatusCR(id int) (*CRStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}

	currentBranch, _ := s.git.CurrentBranch()
	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}

	modifiedStagedCount := 0
	untrackedCount := 0
	for _, entry := range statusEntries {
		if entry.Code == "??" {
			untrackedCount++
			continue
		}
		modifiedStagedCount++
	}

	tasksOpen := 0
	tasksDone := 0
	for _, task := range cr.Subtasks {
		if task.Status == model.TaskStatusDone {
			tasksDone++
			continue
		}
		tasksOpen++
	}

	missingFields := missingCRContractFields(cr.Contract)
	view := &CRStatusView{
		ID:                    cr.ID,
		UID:                   strings.TrimSpace(cr.UID),
		Title:                 cr.Title,
		Status:                cr.Status,
		BaseBranch:            cr.BaseBranch,
		BaseRef:               strings.TrimSpace(cr.BaseRef),
		BaseCommit:            strings.TrimSpace(cr.BaseCommit),
		ParentCRID:            cr.ParentCRID,
		Branch:                cr.Branch,
		CurrentBranch:         currentBranch,
		BranchMatch:           strings.TrimSpace(currentBranch) != "" && currentBranch == cr.Branch,
		ModifiedStagedCount:   modifiedStagedCount,
		UntrackedCount:        untrackedCount,
		Dirty:                 modifiedStagedCount > 0 || untrackedCount > 0,
		TasksTotal:            len(cr.Subtasks),
		TasksOpen:             tasksOpen,
		TasksDone:             tasksDone,
		ContractComplete:      len(missingFields) == 0,
		ContractMissingFields: missingFields,
		ValidationValid:       true,
		RiskTier:              "-",
	}
	if cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			view.ParentStatus = "missing"
		} else {
			view.ParentStatus = parent.Status
		}
	}

	if cr.Status == model.StatusInProgress {
		report, validateErr := s.ValidateCR(id)
		if validateErr != nil {
			return nil, validateErr
		}
		view.ValidationValid = report.Valid
		view.ValidationErrors = len(report.Errors)
		view.ValidationWarnings = len(report.Warnings)
		view.MergeBlocked = !report.Valid
		if report.Impact != nil {
			view.RiskTier = nonEmptyTrimmed(report.Impact.RiskTier, "-")
			view.RiskScore = report.Impact.RiskScore
		}
	}

	return view, nil
}

func (s *Service) ImpactCR(id int) (*ImpactReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	impact := buildImpactReport(cr, diff)
	return impact, nil
}

func (s *Service) ValidateCR(id int) (*ValidationReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	impact := buildImpactReport(cr, diff)

	errorsOut := make([]string, 0)
	for _, field := range missingCRContractFields(cr.Contract) {
		errorsOut = append(errorsOut, fmt.Sprintf("missing required contract field: %s", field))
	}
	for _, driftPath := range impact.ScopeDrift {
		errorsOut = append(errorsOut, fmt.Sprintf("scope drift: changed path %q is outside declared contract scope", driftPath))
	}

	warnings := append([]string(nil), impact.TaskScopeWarnings...)
	warnings = append(warnings, impact.TaskContractWarnings...)
	warnings = append(warnings, impact.TaskChunkWarnings...)
	return &ValidationReport{
		Valid:    len(errorsOut) == 0,
		Errors:   errorsOut,
		Warnings: warnings,
		Impact:   impact,
	}, nil
}

func (s *Service) RecordCRValidation(id int, report *ValidationReport) error {
	if report == nil {
		return errors.New("validation report is required")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	status := "passed"
	if !report.Valid {
		status = "failed"
	}
	meta := map[string]string{
		"risk_tier":           "-",
		"validation_errors":   strconv.Itoa(len(report.Errors)),
		"validation_warnings": strconv.Itoa(len(report.Warnings)),
	}
	if report.Impact != nil {
		meta["risk_tier"] = nonEmptyTrimmed(report.Impact.RiskTier, "-")
	}
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_validated",
		Summary: fmt.Sprintf("Validation %s with %d error(s) and %d warning(s)", status, len(report.Errors), len(report.Warnings)),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta:    meta,
	})
	return s.store.SaveCR(cr)
}

func (s *Service) RedactCRNote(id, noteIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}

	idx, err := oneBasedIndex(noteIndex, len(cr.Notes), "note")
	if err != nil {
		return err
	}
	if cr.Notes[idx] == redactedPlaceholder {
		return ErrAlreadyRedacted
	}
	cr.Notes[idx] = redactedPlaceholder

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            "cr_redacted",
		Summary:         fmt.Sprintf("Redacted note #%d", noteIndex),
		Ref:             fmt.Sprintf("note:%d", noteIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("note:%d", noteIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) RedactCREvent(id, eventIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}

	idx, err := oneBasedIndex(eventIndex, len(cr.Events), "event")
	if err != nil {
		return err
	}
	if cr.Events[idx].Redacted || cr.Events[idx].Summary == redactedPlaceholder {
		return ErrAlreadyRedacted
	}

	cr.Events[idx].Summary = redactedPlaceholder
	cr.Events[idx].Redacted = true
	cr.Events[idx].RedactionReason = reason
	if cr.Events[idx].Meta == nil {
		cr.Events[idx].Meta = map[string]string{}
	}
	cr.Events[idx].Meta["redacted_via"] = fmt.Sprintf("event:%d", eventIndex)

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            "cr_redacted",
		Summary:         fmt.Sprintf("Redacted event #%d", eventIndex),
		Ref:             fmt.Sprintf("event:%d", eventIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("event:%d", eventIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) HistoryCR(id int, showRedacted bool) (*CRHistory, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	history := &CRHistory{
		CRID:        cr.ID,
		Title:       cr.Title,
		Status:      cr.Status,
		Description: cr.Description,
		Notes:       make([]HistoryNote, 0, len(cr.Notes)),
		Events:      make([]HistoryEvent, 0, len(cr.Events)),
	}

	for i, note := range cr.Notes {
		redacted := note == redactedPlaceholder
		text := note
		if redacted {
			text = redactedPlaceholder
		}
		history.Notes = append(history.Notes, HistoryNote{
			Index:    i + 1,
			Text:     text,
			Redacted: redacted,
		})
	}

	for i, event := range cr.Events {
		summary := event.Summary
		redacted := event.Redacted || summary == redactedPlaceholder
		if redacted {
			summary = redactedPlaceholder
		}
		reason := ""
		if showRedacted {
			reason = event.RedactionReason
		}
		meta := map[string]string(nil)
		if showRedacted && len(event.Meta) > 0 {
			meta = cloneStringMap(event.Meta)
		}
		history.Events = append(history.Events, HistoryEvent{
			Index:           i + 1,
			TS:              event.TS,
			Actor:           event.Actor,
			Type:            event.Type,
			Summary:         summary,
			Ref:             event.Ref,
			Redacted:        redacted,
			RedactionReason: reason,
			Meta:            meta,
		})
	}

	return history, nil
}
