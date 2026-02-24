package service

import (
	"fmt"
	"sophia/internal/model"
	"strconv"
	"strings"
)

func (s *Service) ResolveCRIDByUID(uid string) (int, error) {
	cr, err := s.store.LoadCRByUID(uid)
	if err != nil {
		return 0, err
	}
	return cr.ID, nil
}

func (s *Service) ResolveCRByUID(uid string) (*model.CR, error) {
	return s.store.LoadCRByUID(uid)
}

func (s *Service) ResolveCRID(selector string) (int, error) {
	raw := strings.TrimSpace(selector)
	if raw == "" {
		return 0, fmt.Errorf("cr selector cannot be empty")
	}
	if cr, err := s.resolveCRByExactBranch(raw); err == nil && cr != nil {
		return cr.ID, nil
	}
	if looksLikeBranchSelector(raw) {
		return 0, fmt.Errorf("cr branch %q not found", raw)
	}
	id, err := parsePositiveIntSelector(raw)
	if err == nil && id > 0 {
		return id, nil
	}
	return s.ResolveCRIDByUID(raw)
}

func (s *Service) resolveCRByExactBranch(branch string) (*model.CR, error) {
	trimmed := strings.TrimSpace(branch)
	if trimmed == "" {
		return nil, fmt.Errorf("branch cannot be empty")
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	for i := range crs {
		if strings.TrimSpace(crs[i].Branch) == trimmed {
			cr := crs[i]
			return &cr, nil
		}
	}
	return nil, fmt.Errorf("branch %q not found", branch)
}

func looksLikeBranchSelector(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "sophia/cr-") {
		return true
	}
	if strings.HasPrefix(trimmed, "cr-") {
		return true
	}
	return strings.Contains(trimmed, "/cr-")
}

func parsePositiveIntSelector(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("empty selector")
	}
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("selector is not numeric")
	}
	return id, nil
}
