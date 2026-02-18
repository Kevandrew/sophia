package service

import (
	"fmt"
	"sophia/internal/model"
	"strconv"
	"strings"
)

const crRefPrefix = "refs/sophia/cr/"
const crUIDRefPrefix = crRefPrefix + "uid/"

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

func crUIDRefName(uid string) string {
	return crUIDRefPrefix + strings.TrimSpace(uid)
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

func parseCRUIDRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, crUIDRefPrefix) {
		return "", false
	}
	uid := strings.TrimSpace(strings.TrimPrefix(ref, crUIDRefPrefix))
	if uid == "" {
		return "", false
	}
	return uid, true
}

func (s *Service) syncCRRef(cr *model.CR) error {
	if cr == nil || cr.ID <= 0 {
		return fmt.Errorf("cr is required")
	}
	ref := crRefName(cr.ID)
	uidRef := crUIDRefName(cr.UID)
	if strings.TrimSpace(cr.UID) == "" {
		uidRef = ""
	}
	switch cr.Status {
	case model.StatusInProgress:
		branch := strings.TrimSpace(cr.Branch)
		if branch == "" || !s.git.BranchExists(branch) {
			if err := s.git.DeleteRef(ref); err != nil {
				return err
			}
			if uidRef != "" {
				return s.git.DeleteRef(uidRef)
			}
			return nil
		}
		if err := s.git.SetSymbolicRef(ref, "refs/heads/"+branch); err != nil {
			return err
		}
		if uidRef != "" {
			return s.git.SetSymbolicRef(uidRef, "refs/heads/"+branch)
		}
		return nil
	case model.StatusMerged:
		commit := strings.TrimSpace(cr.MergedCommit)
		if commit == "" {
			if err := s.git.DeleteRef(ref); err != nil {
				return err
			}
			if uidRef != "" {
				return s.git.DeleteRef(uidRef)
			}
			return nil
		}
		if resolved, err := s.git.ResolveRef(commit); err == nil && strings.TrimSpace(resolved) != "" {
			commit = strings.TrimSpace(resolved)
		}
		if err := s.git.UpdateRef(ref, commit); err != nil {
			return err
		}
		if uidRef != "" {
			return s.git.UpdateRef(uidRef, commit)
		}
		return nil
	default:
		if err := s.git.DeleteRef(ref); err != nil {
			return err
		}
		if uidRef != "" {
			return s.git.DeleteRef(uidRef)
		}
		return nil
	}
}

func (s *Service) syncAllCRRefs(crs []model.CR) error {
	known := map[int]struct{}{}
	knownUID := map[string]struct{}{}
	for i := range crs {
		cr := crs[i]
		if cr.ID <= 0 {
			continue
		}
		if err := s.syncCRRef(&cr); err != nil {
			return err
		}
		known[cr.ID] = struct{}{}
		if strings.TrimSpace(cr.UID) != "" {
			knownUID[strings.TrimSpace(cr.UID)] = struct{}{}
		}
	}
	refs, err := s.git.ListRefs(crRefPrefix)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if id, ok := parseCRRefID(ref); ok {
			if _, exists := known[id]; exists {
				continue
			}
			if err := s.git.DeleteRef(ref); err != nil {
				return err
			}
			continue
		}
		if uid, ok := parseCRUIDRef(ref); ok {
			if _, exists := knownUID[uid]; exists {
				continue
			}
			if err := s.git.DeleteRef(ref); err != nil {
				return err
			}
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
