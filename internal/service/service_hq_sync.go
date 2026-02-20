package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"sophia/internal/model"
)

type HQPullResult struct {
	LocalCRID           int    `json:"local_cr_id"`
	CRUID               string `json:"cr_uid"`
	Created             bool   `json:"created"`
	Updated             bool   `json:"updated"`
	LocalAhead          bool   `json:"local_ahead"`
	UpToDate            bool   `json:"up_to_date"`
	Forced              bool   `json:"forced"`
	UpstreamFingerprint string `json:"upstream_fingerprint,omitempty"`
}

type HQPushResult struct {
	LocalCRID           int      `json:"local_cr_id"`
	CRUID               string   `json:"cr_uid"`
	CreatedRemote       bool     `json:"created_remote"`
	UpdatedRemote       bool     `json:"updated_remote"`
	Noop                bool     `json:"noop"`
	Forced              bool     `json:"forced"`
	UpstreamFingerprint string   `json:"upstream_fingerprint,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

type HQUpstreamMovedError struct {
	CRID                int
	CRUID               string
	UpstreamFingerprint string
	RemoteFingerprint   string
	SuggestedActions    []string
}

func (e *HQUpstreamMovedError) Error() string {
	if e == nil {
		return "hq upstream moved"
	}
	return fmt.Sprintf("%s: local upstream=%q remote=%q", ErrHQUpstreamMoved.Error(), strings.TrimSpace(e.UpstreamFingerprint), strings.TrimSpace(e.RemoteFingerprint))
}

func (e *HQUpstreamMovedError) Unwrap() error { return ErrHQUpstreamMoved }

func (e *HQUpstreamMovedError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":                e.CRID,
		"cr_uid":               e.CRUID,
		"upstream_fingerprint": e.UpstreamFingerprint,
		"remote_fingerprint":   e.RemoteFingerprint,
		"suggested_actions":    append([]string(nil), e.SuggestedActions...),
	}
}

type HQIntentDivergedError struct {
	CRID                int
	CRUID               string
	UpstreamFingerprint string
	LocalFingerprint    string
	RemoteFingerprint   string
	LocalChangedFields  []string
	RemoteChangedFields []string
	Conflicts           []HQIntentFieldConflict
	SuggestedActions    []string
}

func (e *HQIntentDivergedError) Error() string {
	if e == nil {
		return "hq intent diverged"
	}
	return fmt.Sprintf("%s: local=%q remote=%q upstream=%q", ErrHQIntentDiverged.Error(), strings.TrimSpace(e.LocalFingerprint), strings.TrimSpace(e.RemoteFingerprint), strings.TrimSpace(e.UpstreamFingerprint))
}

func (e *HQIntentDivergedError) Unwrap() error { return ErrHQIntentDiverged }

func (e *HQIntentDivergedError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	conflicts := make([]map[string]any, 0, len(e.Conflicts))
	for _, conflict := range e.Conflicts {
		conflicts = append(conflicts, map[string]any{
			"field":    conflict.Field,
			"upstream": conflict.Upstream,
			"local":    conflict.Local,
			"remote":   conflict.Remote,
		})
	}
	return map[string]any{
		"cr_id":                 e.CRID,
		"cr_uid":                e.CRUID,
		"upstream_fingerprint":  e.UpstreamFingerprint,
		"local_fingerprint":     e.LocalFingerprint,
		"remote_fingerprint":    e.RemoteFingerprint,
		"local_changed_fields":  append([]string(nil), e.LocalChangedFields...),
		"remote_changed_fields": append([]string(nil), e.RemoteChangedFields...),
		"conflicts":             conflicts,
		"suggested_actions":     append([]string(nil), e.SuggestedActions...),
	}
}

type HQPatchConflictError struct {
	CRID             int
	CRUID            string
	BaseFingerprint  string
	ApplyResult      *model.HQPatchApplyResponse
	SuggestedActions []string
}

func (e *HQPatchConflictError) Error() string {
	if e == nil {
		return "hq patch conflict"
	}
	conflictCount := 0
	if e.ApplyResult != nil {
		conflictCount = len(e.ApplyResult.Conflicts)
	}
	return fmt.Sprintf("%s: base=%q conflicts=%d", ErrHQPatchConflict.Error(), strings.TrimSpace(e.BaseFingerprint), conflictCount)
}

func (e *HQPatchConflictError) Unwrap() error { return ErrHQPatchConflict }

func (e *HQPatchConflictError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	details := map[string]any{
		"cr_id":            e.CRID,
		"cr_uid":           e.CRUID,
		"base_fingerprint": strings.TrimSpace(e.BaseFingerprint),
		"suggested_actions": func() []string {
			return append([]string(nil), e.SuggestedActions...)
		}(),
	}
	if e.ApplyResult == nil {
		return details
	}
	details["remote_fingerprint"] = strings.TrimSpace(e.ApplyResult.CRFingerprint)
	details["applied_ops"] = append([]int(nil), e.ApplyResult.AppliedOps...)
	details["skipped_ops"] = append([]int(nil), e.ApplyResult.SkippedOps...)
	details["warnings"] = append([]string(nil), e.ApplyResult.Warnings...)
	details["conflicts"] = append([]model.HQPatchConflict(nil), e.ApplyResult.Conflicts...)
	return details
}

type HQTaskSyncUnsupportedError struct {
	CRID             int
	CRUID            string
	RemoteMaxTaskID  int
	MissingLocalTask []int
	SuggestedActions []string
}

func (e *HQTaskSyncUnsupportedError) Error() string {
	if e == nil {
		return "hq task sync unsupported"
	}
	return fmt.Sprintf("%s: remote_max=%d missing_local=%v", ErrHQTaskSyncUnsupported.Error(), e.RemoteMaxTaskID, append([]int(nil), e.MissingLocalTask...))
}

func (e *HQTaskSyncUnsupportedError) Unwrap() error { return ErrHQTaskSyncUnsupported }

func (e *HQTaskSyncUnsupportedError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":              e.CRID,
		"cr_uid":             e.CRUID,
		"remote_max_task_id": e.RemoteMaxTaskID,
		"missing_local_task_ids": func() []int {
			return append([]int(nil), e.MissingLocalTask...)
		}(),
		"suggested_actions": append([]string(nil), e.SuggestedActions...),
	}
}

type hqRemoteCRDoc struct {
	UID         string
	Fingerprint string
	CR          *model.CR
}

func (s *Service) PullCRFromHQ(selector string, force bool) (*HQPullResult, error) {
	if err := s.ensureHQWritesAllowed(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}
	client := newHQClient(resolved.BaseURL, resolved.Token)

	localCR, remoteUID, err := s.resolveLocalCRForHQPull(selector)
	if err != nil {
		return nil, err
	}
	remoteCR, err := s.fetchHQRemoteCR(context.Background(), client, resolved.RepoID, remoteUID)
	if err != nil {
		return nil, err
	}

	now := s.timestamp()

	if localCR == nil {
		created, err := s.createLocalFromRemote(remoteCR, resolved, now)
		if err != nil {
			return nil, err
		}
		return &HQPullResult{
			LocalCRID:           created.ID,
			CRUID:               strings.TrimSpace(created.UID),
			Created:             true,
			Updated:             false,
			LocalAhead:          false,
			UpToDate:            false,
			Forced:              force,
			UpstreamFingerprint: strings.TrimSpace(created.HQ.UpstreamFingerprint),
		}, nil
	}

	localFingerprint, err := fingerprintHQIntentCR(localCR)
	if err != nil {
		return nil, err
	}
	upstream := strings.TrimSpace(localCR.HQ.UpstreamFingerprint)
	remoteFingerprint := strings.TrimSpace(remoteCR.Fingerprint)
	if remoteFingerprint == "" {
		return nil, fmt.Errorf("%w: missing cr_fingerprint", ErrHQRemoteMalformedResponse)
	}

	result := &HQPullResult{
		LocalCRID:           localCR.ID,
		CRUID:               strings.TrimSpace(localCR.UID),
		UpstreamFingerprint: remoteFingerprint,
		Forced:              force,
	}

	if upstream == "" {
		if localFingerprint != remoteFingerprint && !force {
			return nil, s.newHQIntentDivergedError(localCR, remoteCR.CR, upstream, localFingerprint, remoteFingerprint, "sophia cr pull --force", "sophia cr push --force")
		}
		if err := s.applyRemoteIntentAndPersist(localCR, remoteCR.CR, resolved, remoteFingerprint, now, force, "pull"); err != nil {
			return nil, err
		}
		result.Updated = true
		return result, nil
	}

	switch {
	case localFingerprint == upstream && remoteFingerprint == upstream:
		result.UpToDate = true
		return result, nil
	case localFingerprint == upstream && remoteFingerprint != upstream:
		if err := s.applyRemoteIntentAndPersist(localCR, remoteCR.CR, resolved, remoteFingerprint, now, false, "pull"); err != nil {
			return nil, err
		}
		result.Updated = true
		return result, nil
	case remoteFingerprint == upstream && localFingerprint != upstream:
		result.LocalAhead = true
		return result, nil
	default:
		if !force {
			return nil, s.newHQIntentDivergedError(localCR, remoteCR.CR, upstream, localFingerprint, remoteFingerprint, "sophia cr pull --force", "sophia cr push --force")
		}
		if err := s.applyRemoteIntentAndPersist(localCR, remoteCR.CR, resolved, remoteFingerprint, now, true, "pull"); err != nil {
			return nil, err
		}
		result.Updated = true
		return result, nil
	}
}

func (s *Service) PushCRToHQ(selector string, force bool) (*HQPushResult, error) {
	if err := s.ensureHQWritesAllowed(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}

	localCR, err := s.resolveLocalCRForHQPush(selector)
	if err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(localCR.UID)
	if uid == "" {
		return nil, fmt.Errorf("local CR %d has empty uid", localCR.ID)
	}

	client := newHQClient(resolved.BaseURL, resolved.Token)
	ctx := context.Background()
	now := s.timestamp()

	remote, err := s.fetchHQRemoteCR(ctx, client, resolved.RepoID, uid)
	if err != nil {
		var remoteErr *HQRemoteError
		if errors.As(err, &remoteErr) && remoteErr.StatusCode == 404 {
			created, createErr := s.upsertHQRemoteCR(ctx, client, resolved.RepoID, localCR)
			if createErr != nil {
				return nil, createErr
			}
			s.setCRHQState(localCR, resolved, created.Fingerprint, now, false, true)
			if err := s.store.SaveCR(localCR); err != nil {
				return nil, err
			}
			return &HQPushResult{
				LocalCRID:           localCR.ID,
				CRUID:               uid,
				CreatedRemote:       true,
				UpdatedRemote:       false,
				Noop:                false,
				Forced:              force,
				UpstreamFingerprint: strings.TrimSpace(created.Fingerprint),
			}, nil
		}
		return nil, err
	}

	localFingerprint, err := fingerprintHQIntentCR(localCR)
	if err != nil {
		return nil, err
	}
	upstream := strings.TrimSpace(localCR.HQ.UpstreamFingerprint)
	remoteFingerprint := strings.TrimSpace(remote.Fingerprint)
	if remoteFingerprint == "" {
		return nil, fmt.Errorf("%w: missing cr_fingerprint", ErrHQRemoteMalformedResponse)
	}

	if upstream == "" && !force {
		return nil, &HQUpstreamMovedError{
			CRID:                localCR.ID,
			CRUID:               uid,
			UpstreamFingerprint: "",
			RemoteFingerprint:   remoteFingerprint,
			SuggestedActions: []string{
				"sophia cr pull " + uid,
				"sophia cr push " + uid + " --force",
			},
		}
	}
	if upstream != "" && remoteFingerprint != upstream && !force {
		return nil, &HQUpstreamMovedError{
			CRID:                localCR.ID,
			CRUID:               uid,
			UpstreamFingerprint: upstream,
			RemoteFingerprint:   remoteFingerprint,
			SuggestedActions: []string{
				"sophia cr pull " + uid,
				"sophia cr push " + uid + " --force",
			},
		}
	}

	ops, warnings, err := buildHQIntentPatchOps(remote.CR, localCR)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 && localFingerprint == remoteFingerprint {
		s.setCRHQState(localCR, resolved, remoteFingerprint, now, false, true)
		if err := s.store.SaveCR(localCR); err != nil {
			return nil, err
		}
		return &HQPushResult{
			LocalCRID:           localCR.ID,
			CRUID:               uid,
			CreatedRemote:       false,
			UpdatedRemote:       false,
			Noop:                true,
			Forced:              force,
			UpstreamFingerprint: remoteFingerprint,
			Warnings:            warnings,
		}, nil
	}

	patch := model.CRPatch{
		SchemaVersion: patchSchemaV1,
		Target: model.CRPatchTarget{
			CRUID: uid,
		},
		Base: model.CRPatchBase{
			CRFingerprint: remoteFingerprint,
		},
		Ops: ops,
		Meta: model.CRPatchMeta{
			Tool:    "sophia-cli",
			Message: "cr push",
		},
	}
	applyResult, err := client.ApplyPatch(ctx, resolved.RepoID, uid, patch)
	if err != nil {
		return nil, err
	}
	if len(applyResult.Conflicts) > 0 {
		return nil, &HQPatchConflictError{
			CRID:            localCR.ID,
			CRUID:           uid,
			BaseFingerprint: remoteFingerprint,
			ApplyResult:     applyResult,
			SuggestedActions: []string{
				"sophia cr pull " + uid,
				"sophia cr push " + uid + " --force",
			},
		}
	}
	nextFingerprint := strings.TrimSpace(applyResult.CRFingerprint)
	if nextFingerprint == "" {
		updatedRemote, fetchErr := s.fetchHQRemoteCR(ctx, client, resolved.RepoID, uid)
		if fetchErr != nil {
			return nil, fetchErr
		}
		nextFingerprint = strings.TrimSpace(updatedRemote.Fingerprint)
		if nextFingerprint == "" {
			return nil, fmt.Errorf("%w: missing cr_fingerprint after push", ErrHQRemoteMalformedResponse)
		}
		if err := s.applyRemoteIntentAndPersist(localCR, updatedRemote.CR, resolved, nextFingerprint, now, false, "push"); err != nil {
			return nil, err
		}
	} else {
		s.setCRHQState(localCR, resolved, nextFingerprint, now, false, true)
		if err := s.store.SaveCR(localCR); err != nil {
			return nil, err
		}
	}

	return &HQPushResult{
		LocalCRID:           localCR.ID,
		CRUID:               uid,
		CreatedRemote:       false,
		UpdatedRemote:       true,
		Noop:                false,
		Forced:              force,
		UpstreamFingerprint: nextFingerprint,
		Warnings:            warnings,
	}, nil
}

func (s *Service) resolveLocalCRForHQPush(selector string) (*model.CR, error) {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		ctx, err := s.CurrentCR()
		if err != nil {
			return nil, err
		}
		return ctx.CR, nil
	}
	id, err := s.ResolveCRID(trimmed)
	if err != nil {
		return nil, err
	}
	return s.store.LoadCR(id)
}

func (s *Service) resolveLocalCRForHQPull(selector string) (*model.CR, string, error) {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		ctx, err := s.CurrentCR()
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(ctx.CR.UID) == "" {
			return nil, "", fmt.Errorf("current CR has empty uid")
		}
		return ctx.CR, strings.TrimSpace(ctx.CR.UID), nil
	}

	if id, parseErr := parsePositiveIntSelector(trimmed); parseErr == nil && id > 0 {
		localCR, err := s.store.LoadCR(id)
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(localCR.UID) == "" {
			return nil, "", fmt.Errorf("local CR %d has empty uid", localCR.ID)
		}
		return localCR, strings.TrimSpace(localCR.UID), nil
	}

	id, err := s.ResolveCRID(trimmed)
	if err == nil {
		localCR, loadErr := s.store.LoadCR(id)
		if loadErr != nil {
			return nil, "", loadErr
		}
		if strings.TrimSpace(localCR.UID) == "" {
			return nil, "", fmt.Errorf("local CR %d has empty uid", localCR.ID)
		}
		return localCR, strings.TrimSpace(localCR.UID), nil
	}

	return nil, trimmed, nil
}

func (s *Service) fetchHQRemoteCR(ctx context.Context, client *hqClient, repoID, uid string) (*hqRemoteCRDoc, error) {
	response, err := client.GetCR(ctx, repoID, uid)
	if err != nil {
		return nil, err
	}
	remote, err := decodeHQRemoteCR(uid, response)
	if err != nil {
		return nil, err
	}
	return remote, nil
}

func decodeHQRemoteCR(uid string, response *model.HQGetCRResponse) (*hqRemoteCRDoc, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: empty response body", ErrHQRemoteMalformedResponse)
	}
	docRaw := response.Doc
	if len(docRaw) == 0 {
		docRaw = response.CR
	}
	if len(docRaw) == 0 {
		return nil, fmt.Errorf("%w: missing cr doc", ErrHQRemoteMalformedResponse)
	}
	var doc CRDoc
	if err := json.Unmarshal(docRaw, &doc); err != nil {
		return nil, fmt.Errorf("decode remote CR doc: %w", err)
	}
	if strings.TrimSpace(doc.UID) == "" {
		doc.UID = strings.TrimSpace(uid)
	}
	remoteCR := crFromDoc(&doc)
	if remoteCR == nil {
		return nil, fmt.Errorf("%w: unable to decode remote cr", ErrHQRemoteMalformedResponse)
	}
	fingerprint := strings.TrimSpace(response.CRFingerprint)
	if fingerprint == "" {
		var err error
		fingerprint, err = fingerprintHQIntentCR(remoteCR)
		if err != nil {
			return nil, err
		}
	}
	return &hqRemoteCRDoc{
		UID:         strings.TrimSpace(remoteCR.UID),
		Fingerprint: fingerprint,
		CR:          remoteCR,
	}, nil
}

func (s *Service) createLocalFromRemote(remote *hqRemoteCRDoc, resolved hqRuntimeConfig, now string) (*model.CR, error) {
	if remote == nil || remote.CR == nil {
		return nil, fmt.Errorf("%w: empty remote CR", ErrHQRemoteMalformedResponse)
	}
	imported := cloneRemoteCR(remote.CR)
	nextID, err := s.store.NextCRID()
	if err != nil {
		return nil, err
	}
	imported.ID = nextID
	s.setCRHQState(imported, resolved, strings.TrimSpace(remote.Fingerprint), now, true, false)
	if err := s.store.SaveCR(imported); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(imported); err != nil {
		return nil, err
	}
	return imported, nil
}

func cloneRemoteCR(cr *model.CR) *model.CR {
	if cr == nil {
		return nil
	}
	out := *cr
	out.Notes = append([]string(nil), cr.Notes...)
	out.Evidence = append([]model.EvidenceEntry(nil), cr.Evidence...)
	out.Subtasks = append([]model.Subtask(nil), cr.Subtasks...)
	out.Events = append([]model.Event(nil), cr.Events...)
	return &out
}

func (s *Service) applyRemoteIntentAndPersist(localCR, remoteCR *model.CR, resolved hqRuntimeConfig, remoteFingerprint, now string, forced bool, source string) error {
	if localCR == nil || remoteCR == nil {
		return fmt.Errorf("%w: missing local or remote CR", ErrHQRemoteMalformedResponse)
	}
	mergeRemoteIntentIntoLocal(localCR, remoteCR)
	localCR.UpdatedAt = now
	eventType := "hq_synced"
	eventSummary := "Synced CR intent with remote"
	pulled := false
	pushed := false
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "pull":
		eventType = "hq_pulled"
		eventSummary = fmt.Sprintf("Pulled CR intent from remote (%s)", ternary(forced, "forced", "merge-safe"))
		pulled = true
	case "push":
		eventType = "hq_pushed"
		eventSummary = "Synced local CR intent to remote"
		pushed = true
	}
	localCR.Events = append(localCR.Events, model.Event{
		TS:      now,
		Actor:   s.git.Actor(),
		Type:    eventType,
		Summary: eventSummary,
		Ref:     fmt.Sprintf("cr:%d", localCR.ID),
		Meta: map[string]string{
			"remote_alias":         strings.TrimSpace(resolved.RemoteAlias),
			"repo_id":              strings.TrimSpace(resolved.RepoID),
			"upstream_fingerprint": strings.TrimSpace(remoteFingerprint),
		},
	})
	s.setCRHQState(localCR, resolved, remoteFingerprint, now, pulled, pushed)
	if err := s.store.SaveCR(localCR); err != nil {
		return err
	}
	return s.syncCRRef(localCR)
}

func mergeRemoteIntentIntoLocal(localCR, remoteCR *model.CR) {
	localCR.Title = strings.TrimSpace(remoteCR.Title)
	localCR.Description = strings.TrimSpace(remoteCR.Description)
	localCR.Status = strings.TrimSpace(remoteCR.Status)
	applyRemoteContractIntent(&localCR.Contract, remoteCR.Contract)
	localCR.Notes = append([]string(nil), normalizeStringList(remoteCR.Notes)...)

	existingByID := map[int]model.Subtask{}
	for _, task := range localCR.Subtasks {
		existingByID[task.ID] = task
	}
	merged := make([]model.Subtask, 0, len(remoteCR.Subtasks))
	for _, remoteTask := range remoteCR.Subtasks {
		if existing, ok := existingByID[remoteTask.ID]; ok {
			mergedTask := existing
			mergedTask.Title = strings.TrimSpace(remoteTask.Title)
			mergedTask.Status = strings.TrimSpace(remoteTask.Status)
			applyRemoteTaskContractIntent(&mergedTask.Contract, remoteTask.Contract)
			if strings.TrimSpace(remoteTask.UpdatedAt) != "" {
				mergedTask.UpdatedAt = strings.TrimSpace(remoteTask.UpdatedAt)
			}
			if strings.TrimSpace(remoteTask.CompletedAt) != "" {
				mergedTask.CompletedAt = strings.TrimSpace(remoteTask.CompletedAt)
				mergedTask.CompletedBy = strings.TrimSpace(remoteTask.CompletedBy)
			}
			merged = append(merged, mergedTask)
			continue
		}
		newTask := remoteTask
		newTask.Title = strings.TrimSpace(newTask.Title)
		newTask.Status = strings.TrimSpace(newTask.Status)
		newTask.CheckpointScope = append([]string(nil), newTask.CheckpointScope...)
		newTask.CheckpointChunks = append([]model.CheckpointChunk(nil), newTask.CheckpointChunks...)
		newTask.Delegations = append([]model.TaskDelegation(nil), newTask.Delegations...)
		newTask.Contract.AcceptanceCriteria = normalizeStringList(newTask.Contract.AcceptanceCriteria)
		newTask.Contract.Scope = normalizeStringList(newTask.Contract.Scope)
		newTask.Contract.AcceptanceChecks = normalizeStringList(newTask.Contract.AcceptanceChecks)
		merged = append(merged, newTask)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	localCR.Subtasks = merged
}

func applyRemoteContractIntent(dst *model.Contract, src model.Contract) {
	dst.Why = strings.TrimSpace(src.Why)
	dst.Scope = normalizeStringList(src.Scope)
	dst.NonGoals = normalizeStringList(src.NonGoals)
	dst.Invariants = normalizeStringList(src.Invariants)
	dst.BlastRadius = strings.TrimSpace(src.BlastRadius)
	dst.RiskCriticalScopes = normalizeStringList(src.RiskCriticalScopes)
	dst.RiskTierHint = strings.TrimSpace(src.RiskTierHint)
	dst.RiskRationale = strings.TrimSpace(src.RiskRationale)
	dst.TestPlan = strings.TrimSpace(src.TestPlan)
	dst.RollbackPlan = strings.TrimSpace(src.RollbackPlan)
}

func applyRemoteTaskContractIntent(dst *model.TaskContract, src model.TaskContract) {
	dst.Intent = strings.TrimSpace(src.Intent)
	dst.AcceptanceCriteria = normalizeStringList(src.AcceptanceCriteria)
	dst.Scope = normalizeStringList(src.Scope)
	dst.AcceptanceChecks = normalizeStringList(src.AcceptanceChecks)
}

func (s *Service) setCRHQState(cr *model.CR, resolved hqRuntimeConfig, upstreamFingerprint, now string, pulled bool, pushed bool) {
	if cr == nil {
		return
	}
	cr.HQ.RemoteAlias = strings.TrimSpace(resolved.RemoteAlias)
	cr.HQ.RepoID = strings.TrimSpace(resolved.RepoID)
	cr.HQ.UpstreamFingerprint = strings.TrimSpace(upstreamFingerprint)
	if strings.TrimSpace(upstreamFingerprint) == "" {
		cr.HQ.UpstreamIntent = nil
	} else {
		cr.HQ.UpstreamIntent = canonicalHQIntentSnapshot(cr)
	}
	if pulled {
		cr.HQ.LastPullAt = strings.TrimSpace(now)
	}
	if pushed {
		cr.HQ.LastPushAt = strings.TrimSpace(now)
	}
}

func (s *Service) newHQIntentDivergedError(localCR, remoteCR *model.CR, upstream, localFingerprint, remoteFingerprint string, actions ...string) error {
	localDoc := canonicalHQIntentSnapshot(localCR)
	remoteDoc := canonicalHQIntentSnapshot(remoteCR)

	upstreamSnapshot := localCR.HQ.UpstreamIntent
	localChanged := []string{}
	remoteChanged := []string{}
	conflicts := []HQIntentFieldConflict{}
	if upstreamSnapshot != nil && strings.TrimSpace(upstream) != "" {
		localChanged, remoteChanged, conflicts = diffHQIntentFields3(upstreamSnapshot, localDoc, remoteDoc)
	} else {
		changed, twoWayConflicts := diffHQIntentFields(localDoc, remoteDoc)
		localChanged, remoteChanged = inferHQChangedFields(strings.TrimSpace(upstream), strings.TrimSpace(localFingerprint), strings.TrimSpace(remoteFingerprint), changed)
		conflicts = twoWayConflicts
	}
	return &HQIntentDivergedError{
		CRID:                localCR.ID,
		CRUID:               strings.TrimSpace(localCR.UID),
		UpstreamFingerprint: strings.TrimSpace(upstream),
		LocalFingerprint:    strings.TrimSpace(localFingerprint),
		RemoteFingerprint:   strings.TrimSpace(remoteFingerprint),
		LocalChangedFields:  localChanged,
		RemoteChangedFields: remoteChanged,
		Conflicts:           conflicts,
		SuggestedActions:    append([]string(nil), actions...),
	}
}

func inferHQChangedFields(upstream, localFingerprint, remoteFingerprint string, changed []string) ([]string, []string) {
	fields := append([]string(nil), changed...)
	switch {
	case upstream == "":
		return fields, fields
	case localFingerprint == upstream && remoteFingerprint != upstream:
		return []string{}, fields
	case remoteFingerprint == upstream && localFingerprint != upstream:
		return fields, []string{}
	default:
		return fields, fields
	}
}

func buildHQIntentPatchOps(remoteCR, localCR *model.CR) ([]json.RawMessage, []string, error) {
	if remoteCR == nil || localCR == nil {
		return nil, nil, fmt.Errorf("both remote and local CRs are required")
	}
	ops := make([]json.RawMessage, 0)
	warnings := []string{}

	addSetField := func(field string, before any, after any) error {
		if reflect.DeepEqual(before, after) {
			return nil
		}
		payload, err := json.Marshal(map[string]any{
			"op":     "set_field",
			"field":  field,
			"before": before,
			"after":  after,
		})
		if err != nil {
			return err
		}
		ops = append(ops, payload)
		return nil
	}

	if err := addSetField("cr.title", strings.TrimSpace(remoteCR.Title), strings.TrimSpace(localCR.Title)); err != nil {
		return nil, nil, err
	}
	if err := addSetField("cr.description", strings.TrimSpace(remoteCR.Description), strings.TrimSpace(localCR.Description)); err != nil {
		return nil, nil, err
	}
	if err := addSetField("cr.status", strings.TrimSpace(remoteCR.Status), strings.TrimSpace(localCR.Status)); err != nil {
		return nil, nil, err
	}

	remoteContract := canonicalHQIntentContract(remoteCR.Contract)
	localContract := canonicalHQIntentContract(localCR.Contract)
	contractChanges := map[string]any{}
	addContractChange := func(field string, before any, after any) {
		if reflect.DeepEqual(before, after) {
			return
		}
		contractChanges[field] = map[string]any{
			"before": before,
			"after":  after,
		}
	}
	addContractChange("why", remoteContract.Why, localContract.Why)
	addContractChange("scope", remoteContract.Scope, localContract.Scope)
	addContractChange("non_goals", remoteContract.NonGoals, localContract.NonGoals)
	addContractChange("invariants", remoteContract.Invariants, localContract.Invariants)
	addContractChange("blast_radius", remoteContract.BlastRadius, localContract.BlastRadius)
	addContractChange("risk_critical_scopes", remoteContract.RiskCriticalScopes, localContract.RiskCriticalScopes)
	addContractChange("risk_tier_hint", remoteContract.RiskTierHint, localContract.RiskTierHint)
	addContractChange("risk_rationale", remoteContract.RiskRationale, localContract.RiskRationale)
	addContractChange("test_plan", remoteContract.TestPlan, localContract.TestPlan)
	addContractChange("rollback_plan", remoteContract.RollbackPlan, localContract.RollbackPlan)
	if len(contractChanges) > 0 {
		payload, err := json.Marshal(map[string]any{
			"op":      "set_contract",
			"changes": contractChanges,
		})
		if err != nil {
			return nil, nil, err
		}
		ops = append(ops, payload)
	}

	remoteNotes := map[string]struct{}{}
	for _, note := range normalizeStringList(remoteCR.Notes) {
		remoteNotes[noteHash(note)] = struct{}{}
	}
	for _, note := range normalizeStringList(localCR.Notes) {
		if _, ok := remoteNotes[noteHash(note)]; ok {
			continue
		}
		payload, err := json.Marshal(patchAddNoteOp{Op: "add_note", Text: note})
		if err != nil {
			return nil, nil, err
		}
		ops = append(ops, payload)
	}

	remoteTasks := map[int]model.Subtask{}
	remoteMaxTaskID := 0
	for _, task := range remoteCR.Subtasks {
		remoteTasks[task.ID] = task
		if task.ID > remoteMaxTaskID {
			remoteMaxTaskID = task.ID
		}
	}
	localTasks := map[int]model.Subtask{}
	for _, task := range localCR.Subtasks {
		localTasks[task.ID] = task
	}

	missingLocal := make([]int, 0)
	for id := range localTasks {
		if _, ok := remoteTasks[id]; ok {
			continue
		}
		missingLocal = append(missingLocal, id)
	}
	sort.Ints(missingLocal)
	if len(missingLocal) > 0 {
		expected := remoteMaxTaskID + 1
		for i, id := range missingLocal {
			if id != expected+i {
				uid := strings.TrimSpace(localCR.UID)
				return nil, nil, &HQTaskSyncUnsupportedError{
					CRID:             localCR.ID,
					CRUID:            uid,
					RemoteMaxTaskID:  remoteMaxTaskID,
					MissingLocalTask: missingLocal,
					SuggestedActions: []string{
						"sophia cr pull " + uid,
						"sophia cr push " + uid + " --force",
					},
				}
			}
		}
	}

	for id := range remoteTasks {
		if _, ok := localTasks[id]; ok {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("remote task %d exists but local task is missing; task deletion is not encoded in push patch", id))
	}

	ids := make([]int, 0, len(localTasks))
	for id := range localTasks {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		localTask := localTasks[id]
		remoteTask, ok := remoteTasks[id]
		if !ok {
			title := strings.TrimSpace(localTask.Title)
			payload, err := json.Marshal(map[string]any{
				"op":    "add_task",
				"title": title,
			})
			if err != nil {
				return nil, nil, err
			}
			ops = append(ops, payload)

			// After add_task, remote creates a new open task with the same title; publish remaining intent via update_task.
			localTaskContract := canonicalHQIntentTaskContract(localTask.Contract)
			taskChanges := map[string]any{}
			if status := strings.TrimSpace(localTask.Status); status != "" && status != model.TaskStatusOpen {
				taskChanges["status"] = map[string]any{
					"before": model.TaskStatusOpen,
					"after":  status,
				}
			}
			contract := map[string]any{}
			if localTaskContract.Intent != "" {
				contract["intent"] = map[string]any{"before": "", "after": localTaskContract.Intent}
			}
			if len(localTaskContract.AcceptanceCriteria) > 0 {
				contract["acceptance_criteria"] = map[string]any{"before": []string{}, "after": localTaskContract.AcceptanceCriteria}
			}
			if len(localTaskContract.Scope) > 0 {
				contract["scope"] = map[string]any{"before": []string{}, "after": localTaskContract.Scope}
			}
			if len(localTaskContract.AcceptanceChecks) > 0 {
				contract["acceptance_checks"] = map[string]any{"before": []string{}, "after": localTaskContract.AcceptanceChecks}
			}
			if len(contract) > 0 {
				taskChanges["contract"] = contract
			}
			if len(taskChanges) > 0 {
				updatePayload, err := json.Marshal(map[string]any{
					"op":      "update_task",
					"task_id": id,
					"changes": taskChanges,
				})
				if err != nil {
					return nil, nil, err
				}
				ops = append(ops, updatePayload)
			}
			continue
		}

		taskChanges := map[string]any{}
		if strings.TrimSpace(remoteTask.Title) != strings.TrimSpace(localTask.Title) {
			taskChanges["title"] = map[string]any{
				"before": strings.TrimSpace(remoteTask.Title),
				"after":  strings.TrimSpace(localTask.Title),
			}
		}
		if strings.TrimSpace(remoteTask.Status) != strings.TrimSpace(localTask.Status) {
			taskChanges["status"] = map[string]any{
				"before": strings.TrimSpace(remoteTask.Status),
				"after":  strings.TrimSpace(localTask.Status),
			}
		}

		remoteTaskContract := canonicalHQIntentTaskContract(remoteTask.Contract)
		localTaskContract := canonicalHQIntentTaskContract(localTask.Contract)
		contract := map[string]any{}
		if remoteTaskContract.Intent != localTaskContract.Intent {
			contract["intent"] = map[string]any{"before": remoteTaskContract.Intent, "after": localTaskContract.Intent}
		}
		if !reflect.DeepEqual(remoteTaskContract.AcceptanceCriteria, localTaskContract.AcceptanceCriteria) {
			contract["acceptance_criteria"] = map[string]any{"before": remoteTaskContract.AcceptanceCriteria, "after": localTaskContract.AcceptanceCriteria}
		}
		if !reflect.DeepEqual(remoteTaskContract.Scope, localTaskContract.Scope) {
			contract["scope"] = map[string]any{"before": remoteTaskContract.Scope, "after": localTaskContract.Scope}
		}
		if !reflect.DeepEqual(remoteTaskContract.AcceptanceChecks, localTaskContract.AcceptanceChecks) {
			contract["acceptance_checks"] = map[string]any{"before": remoteTaskContract.AcceptanceChecks, "after": localTaskContract.AcceptanceChecks}
		}
		if len(contract) > 0 {
			taskChanges["contract"] = contract
		}
		if len(taskChanges) == 0 {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"op":      "update_task",
			"task_id": id,
			"changes": taskChanges,
		})
		if err != nil {
			return nil, nil, err
		}
		ops = append(ops, payload)
	}

	return ops, warnings, nil
}

func (s *Service) upsertHQRemoteCR(ctx context.Context, client *hqClient, repoID string, localCR *model.CR) (*hqRemoteCRDoc, error) {
	doc := canonicalCRDoc(localCR)
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	response, err := client.UpsertCR(ctx, repoID, strings.TrimSpace(localCR.UID), model.HQUpsertCRRequest{
		SchemaVersion:    model.HQSchemaV1,
		DocSchemaVersion: crDocSchemaV1,
		Doc:              rawDoc,
	})
	if err != nil {
		return nil, err
	}
	remoteFingerprint := strings.TrimSpace(response.CRFingerprint)
	if remoteFingerprint == "" {
		remoteFingerprint, err = fingerprintHQIntentCR(localCR)
		if err != nil {
			return nil, err
		}
	}
	return &hqRemoteCRDoc{
		UID:         strings.TrimSpace(localCR.UID),
		Fingerprint: remoteFingerprint,
		CR:          cloneRemoteCR(localCR),
	}, nil
}

func ternary(condition bool, left, right string) string {
	if condition {
		return left
	}
	return right
}
