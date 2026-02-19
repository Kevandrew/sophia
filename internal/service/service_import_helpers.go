package service

import (
	"fmt"
	"strings"
)

func normalizeImportMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = importModeCreate
	}
	if mode != importModeCreate && mode != importModeReplace {
		return "", fmt.Errorf("invalid import mode %q (expected create or replace)", mode)
	}
	return mode, nil
}
