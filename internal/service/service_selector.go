package service

import (
	"fmt"
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

func (s *Service) ResolveCRID(selector string) (int, error) {
	raw := strings.TrimSpace(selector)
	if raw == "" {
		return 0, fmt.Errorf("cr selector cannot be empty")
	}
	if id, ok := parseCRIDFromBranchName(raw); ok {
		return id, nil
	}
	id, err := parsePositiveIntSelector(raw)
	if err == nil && id > 0 {
		return id, nil
	}
	return s.ResolveCRIDByUID(raw)
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
