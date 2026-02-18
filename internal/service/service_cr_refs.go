package service

import (
	"fmt"
	"sophia/internal/model"
	"strconv"
	"strings"
)

const crRefPrefix = "refs/sophia/cr/"

type crAnchorResolution struct {
	baseRef    string
	baseCommit string
	headRef    string
	headCommit string
	mergeBase  string
	warnings   []string
}

func crRefName(id int) string {
	return fmt.Sprintf("%s%d", crRefPrefix, id)
}

func parseCRRefID(ref string) (int, bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, crRefPrefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(ref, crRefPrefix)
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *Service) syncCRRef(cr *model.CR) error {
	if cr == nil || cr.ID <= 0 {
		return fmt.Errorf("cr is required")
	}
	ref := crRefName(cr.ID)
	switch cr.Status {
	case model.StatusInProgress:
		branch := strings.TrimSpace(cr.Branch)
		if branch == "" || !s.git.BranchExists(branch) {
			return s.git.DeleteRef(ref)
		}
		return s.git.SetSymbolicRef(ref, "refs/heads/"+branch)
	case model.StatusMerged:
		commit := strings.TrimSpace(cr.MergedCommit)
		if commit == "" {
			return s.git.DeleteRef(ref)
		}
		if resolved, err := s.git.ResolveRef(commit); err == nil && strings.TrimSpace(resolved) != "" {
			commit = strings.TrimSpace(resolved)
		}
		return s.git.UpdateRef(ref, commit)
	default:
		return s.git.DeleteRef(ref)
	}
}

func (s *Service) syncAllCRRefs(crs []model.CR) error {
	known := map[int]struct{}{}
	for i := range crs {
		cr := crs[i]
		if cr.ID <= 0 {
			continue
		}
		if err := s.syncCRRef(&cr); err != nil {
			return err
		}
		known[cr.ID] = struct{}{}
	}
	refs, err := s.git.ListRefs(crRefPrefix)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		id, ok := parseCRRefID(ref)
		if !ok {
			continue
		}
		if _, exists := known[id]; exists {
			continue
		}
		if err := s.git.DeleteRef(ref); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCRAnchorKind(kind string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case CRAnchorKindBase:
		return CRAnchorKindBase, nil
	case CRAnchorKindHead:
		return CRAnchorKindHead, nil
	case CRAnchorKindMergeBase:
		return CRAnchorKindMergeBase, nil
	default:
		return "", fmt.Errorf("invalid --kind %q (expected base|head|merge-base)", strings.TrimSpace(kind))
	}
}

func (s *Service) resolveCRAnchors(cr *model.CR) (*crAnchorResolution, error) {
	if cr == nil {
		return nil, fmt.Errorf("cr is required")
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	res := &crAnchorResolution{
		baseRef: strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)),
	}

	if strings.TrimSpace(cr.BaseCommit) != "" {
		res.baseCommit = strings.TrimSpace(cr.BaseCommit)
	} else {
		if res.baseRef == "" {
			return nil, fmt.Errorf("cr %d has no base anchor", cr.ID)
		}
		baseCommit, err := s.git.ResolveRef(res.baseRef)
		if err != nil {
			return nil, fmt.Errorf("resolve base ref %q: %w", res.baseRef, err)
		}
		res.baseCommit = strings.TrimSpace(baseCommit)
	}

	crRef := crRefName(cr.ID)
	if s.git.RefExists(crRef) {
		if headCommit, err := s.git.ResolveRef(crRef); err == nil && strings.TrimSpace(headCommit) != "" {
			res.headRef = crRef
			res.headCommit = strings.TrimSpace(headCommit)
			if !s.git.BranchExists(cr.Branch) {
				res.warnings = append(res.warnings, "CR branch is unavailable; using canonical CR ref as head anchor")
			}
		} else if err != nil {
			res.warnings = append(res.warnings, fmt.Sprintf("unable to resolve %s: %v", crRef, err))
		}
	}

	if strings.TrimSpace(res.headCommit) == "" && s.git.BranchExists(cr.Branch) {
		headCommit, err := s.git.ResolveRef(cr.Branch)
		if err != nil {
			return nil, fmt.Errorf("resolve CR branch %q: %w", cr.Branch, err)
		}
		res.headRef = strings.TrimSpace(cr.Branch)
		res.headCommit = strings.TrimSpace(headCommit)
		res.warnings = append(res.warnings, "CR ref missing; using CR branch head as anchor")
	}

	if strings.TrimSpace(res.headCommit) == "" && cr.Status == model.StatusMerged && strings.TrimSpace(cr.MergedCommit) != "" {
		merged := strings.TrimSpace(cr.MergedCommit)
		if resolved, err := s.git.ResolveRef(merged); err == nil && strings.TrimSpace(resolved) != "" {
			merged = strings.TrimSpace(resolved)
		}
		res.headRef = merged
		res.headCommit = merged
		res.warnings = append(res.warnings, "CR branch is unavailable; using merged commit as head anchor")
	}

	if strings.TrimSpace(res.headCommit) == "" {
		return nil, fmt.Errorf("unable to resolve head anchor for CR %d", cr.ID)
	}

	mergeBase, err := s.git.MergeBase(res.baseCommit, res.headCommit)
	if err != nil {
		return nil, fmt.Errorf("compute merge-base(%s, %s): %w", shortHash(res.baseCommit), shortHash(res.headCommit), err)
	}
	res.mergeBase = strings.TrimSpace(mergeBase)
	return res, nil
}

func (s *Service) RangeCR(id int) (*CRRangeAnchorsView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveCRAnchors(cr)
	if err != nil {
		return nil, err
	}
	return &CRRangeAnchorsView{
		CRID:      cr.ID,
		Base:      resolved.baseCommit,
		Head:      resolved.headCommit,
		MergeBase: resolved.mergeBase,
		Warnings:  append([]string(nil), resolved.warnings...),
	}, nil
}

func (s *Service) RevParseCR(id int, kind string) (*CRRevParseView, error) {
	normalizedKind, err := normalizeCRAnchorKind(kind)
	if err != nil {
		return nil, err
	}
	view, err := s.RangeCR(id)
	if err != nil {
		return nil, err
	}
	commit := ""
	switch normalizedKind {
	case CRAnchorKindBase:
		commit = strings.TrimSpace(view.Base)
	case CRAnchorKindHead:
		commit = strings.TrimSpace(view.Head)
	case CRAnchorKindMergeBase:
		commit = strings.TrimSpace(view.MergeBase)
	}
	if commit == "" {
		return nil, fmt.Errorf("anchor %q resolved to empty commit for CR %d", normalizedKind, id)
	}
	return &CRRevParseView{
		CRID:     id,
		Kind:     normalizedKind,
		Commit:   commit,
		Warnings: append([]string(nil), view.Warnings...),
	}, nil
}
